package bootstrap

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"routebox/backend/internal/util"
)

// Install is what CaddyParamsFromInstall needs from settings and the process:
// everything the Caddyfile depends on that the active sing-box config does not
// already say.
type Install struct {
	Domain, Dest, PanelPath, Caddyfile, StubRoot, ACMEEmail string
	FrontPort, PanelPort, WebFrontPort                      int
	// ACMEStaging is network.acme_staging. SyncCaddyfile follows it only in
	// the safe direction: off a staging file onto production, never back.
	ACMEStaging bool
}

// ErrReplanSkipped means the install no longer has what a Caddyfile is planned
// from (an inbound was removed or reshaped by hand). It is not a failure: the
// Caddyfile on disk keeps working, so the caller leaves it alone.
var ErrReplanSkipped = errors.New("caddyfile re-plan skipped")

// ErrReplanRepeated marks a re-plan that was not attempted again: nothing it
// is planned from has changed since an attempt that failed, so asking dest
// again would only earn the same refusal. The error it rides on reads as that
// failure, and callers that run on a timer use this to say it once rather
// than every tick.
var ErrReplanRepeated = errors.New("caddyfile re-plan unchanged since it last failed")

// RepeatedReplan wraps the error a re-plan last failed with, for a caller
// that is answering a repeat of it without asking dest again: it reads as err
// and matches both err and ErrReplanRepeated.
func RepeatedReplan(err error) error { return &repeatedReplan{err: err} }

type repeatedReplan struct{ err error }

func (e *repeatedReplan) Error() string   { return e.err.Error() }
func (e *repeatedReplan) Unwrap() []error { return []error{e.err, ErrReplanRepeated} }

// ReplanDisabled is the operator's switch for hand-maintained Caddyfiles.
func ReplanDisabled() bool {
	switch strings.ToLower(os.Getenv("BOOTSTRAP_CADDYFILE_REPLAN")) {
	case "0", "false":
		return true
	}
	return false
}

// CaddyParamsFromInstall rebuilds the Params the Caddyfile is planned from, out
// of what the install keeps: its settings and the active sing-box config. The
// transport ports and secret paths are read back from the inbounds, so nothing
// the bootstrap knew once has to be stored a second time.
func CaddyParamsFromInstall(in Install, active map[string]interface{}) (Params, error) {
	host, portStr, err := net.SplitHostPort(in.Dest)
	if err != nil {
		return Params{}, fmt.Errorf("dest %q: %w", in.Dest, err)
	}
	destPort, err := strconv.Atoi(portStr)
	if err != nil {
		return Params{}, fmt.Errorf("dest %q: %w", in.Dest, err)
	}

	grpc := inboundOf(active, TagVlessGRPC)
	ws := inboundOf(active, TagTrojanWS)
	grpcPort, grpcName := inboundPort(grpc), inboundTransport(grpc, "service_name")
	wsPort, wsPath := inboundPort(ws), inboundTransport(ws, "path")
	if grpcPort == 0 || grpcName == "" || wsPort == 0 || wsPath == "" {
		return Params{}, fmt.Errorf("%w: the %s or %s inbound is missing or incomplete", ErrReplanSkipped, TagVlessGRPC, TagTrojanWS)
	}

	return Params{
		Domain:     in.Domain,
		DestHost:   host,
		DestPort:   destPort,
		StubRoot:   in.StubRoot,
		NaiveUsers: NaiveUsersPath(in.Caddyfile),
		Ports: Ports{
			Front:     in.FrontPort,
			VlessGRPC: grpcPort,
			TrojanWS:  wsPort,
			Panel:     in.PanelPort,
			WebFront:  in.WebFrontPort,
		},
		Paths: Paths{VlessGRPC: grpcName, TrojanWS: wsPath, Panel: in.PanelPath},
		ACME:  ACME{Email: in.ACMEEmail, Staging: in.ACMEStaging},
	}, nil
}

// RealityServerName is the name the front steals, read from the active config.
// Empty when there is no reality inbound.
func RealityServerName(active map[string]interface{}) string {
	tls, _ := inboundOf(active, TagVlessReality)["tls"].(map[string]interface{})
	name, _ := tls["server_name"].(string)
	return name
}

// inboundOf finds the inbound with this tag; nil when there is none.
func inboundOf(cfg map[string]interface{}, tag string) map[string]interface{} {
	arr, _ := cfg["inbounds"].([]interface{})
	for _, it := range arr {
		if ib, ok := it.(map[string]interface{}); ok && ib["tag"] == tag {
			return ib
		}
	}
	return nil
}

func inboundPort(ib map[string]interface{}) int {
	port, _ := ib["listen_port"].(float64)
	return int(port)
}

func inboundTransport(ib map[string]interface{}, key string) string {
	tr, _ := ib["transport"].(map[string]interface{})
	v, _ := tr[key].(string)
	return v
}

// SyncCaddyfile makes the Caddyfile at path match what p plans, and tells dest.
//
// A bad write must never take the site down, so the planned text is checked by
// dest itself (Caddy's /adapt) before it replaces anything: a refusal returns
// with the file untouched. Only an unreachable dest skips the check — then there
// is nothing running to take down, and the file is written for dest's next start.
//
// changed reports whether the file now on disk differs from the one before. A
// refused reload restores the previous file and reports false; only a failed
// restore leaves changed true, since the refused file is then still there.
func SyncCaddyfile(p Params, path string) (changed bool, err error) {
	existing, err := os.ReadFile(path)
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	p.ACME.Staging = replanStaging(existing, existed, p.ACME.Staging)
	rendered, err := PlanCaddyfile(p)
	if err != nil {
		return false, err
	}
	if rendered == string(existing) {
		return false, nil
	}

	admin := caddyAdminAddr()
	if err := adaptCaddyfile(admin, rendered); err != nil {
		if errors.Is(err, ErrDestUnreachable) {
			log.Printf("bootstrap: dest did not answer on %s, writing the Caddyfile unchecked", admin)
		} else {
			return false, err
		}
	}

	if err := util.WriteFileAtomic(path, []byte(rendered), 0644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	if err := ReloadCaddy(path); err != nil {
		if errors.Is(err, ErrDestUnreachable) {
			log.Printf("bootstrap: %v; the new Caddyfile applies when dest starts", err)
			return true, nil
		}
		// /adapt only parses; /load can still refuse (provisioning, imports,
		// listeners). Caddy rolls back in memory, but dest's next start reads the
		// file, so a refused Caddyfile must not stay on disk.
		var restoreErr error
		if existed {
			restoreErr = util.WriteFileAtomic(path, existing, 0644)
		} else {
			restoreErr = os.Remove(path)
		}
		if restoreErr != nil {
			return true, fmt.Errorf("%w; restoring the previous Caddyfile also failed, the refused one is still on disk: %v", err, restoreErr)
		}
		return false, err
	}
	return true, nil
}

// stagingIgnored makes the "already on production" notice once per process:
// every re-plan would repeat it otherwise, on every apply and every expiry
// tick, for as long as the setting stays on.
var stagingIgnored sync.Once

// replanStaging decides whether the re-planned Caddyfile uses Let's Encrypt's
// staging CA, given the file on disk and network.acme_staging (want).
//
// The setting is followed only in the safe direction. A staging file with the
// setting off goes to production: the operator has finished the trial run. A
// production file stays on production whatever the setting says: swapping a
// working certificate for a staging one, which no browser trusts, would take
// the site down over a flag, so that is said once and ignored. With no file
// yet there is no certificate to lose, and the setting decides, as it does for
// the bootstrap's first plan.
func replanStaging(existing []byte, existed, want bool) bool {
	if !existed {
		return want
	}
	if !strings.Contains(string(existing), stagingCA) {
		if want {
			stagingIgnored.Do(func() {
				log.Printf("bootstrap: network.acme_staging is on, but dest's Caddyfile already uses the production CA; it stays on production, since a staging certificate would replace one browsers trust")
			})
		}
		return false
	}
	return want
}

// adaptCaddyfile asks dest to parse text without running it. An ErrDestUnreachable
// error means no answer; any other error is dest's refusal.
func adaptCaddyfile(admin, text string) error {
	req, err := http.NewRequest(http.MethodPost, "http://"+admin+"/adapt", strings.NewReader(text))
	if err != nil {
		return fmt.Errorf("build the check request for dest: %w", err)
	}
	req.Header.Set("Content-Type", "text/caddyfile")
	resp, err := (&http.Client{Timeout: caddyReloadTimeout}).Do(req)
	if err != nil {
		return fmt.Errorf("dest did not answer on %s: %w: %w", admin, ErrDestUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		return nil
	}
	reason, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("dest refused the planned Caddyfile (%s): %s", resp.Status, bytes.TrimSpace(reason))
}
