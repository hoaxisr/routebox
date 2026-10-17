package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"routebox/backend/internal/bootstrap"
	"routebox/backend/internal/config"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/webfront"
)

// allInOneEnv turns the out-of-the-box bootstrap on. It is a deliberate opt-in
// and not something inferred from an empty config directory: every other way of
// installing RouteBox also starts with no config, and coming up with five
// inbounds and a stub site when the operator asked for none would be a surprise
// they cannot undo.
const allInOneEnv = "BOOTSTRAP_ALLINONE"

// Where the two planned files go, and what dest serves at the root, unless the
// installer says otherwise. All three live next to routebox.toml, which is the
// directory an install already treats as its own.
const (
	caddyfileEnv = "BOOTSTRAP_CADDYFILE"
	stubRootEnv  = "BOOTSTRAP_STUB_ROOT"
)

// The internal ports of the out-of-the-box layout. Only frontPort and mieruPort
// are published; the rest are loopback, which is also how serverlinks knows an
// inbound is behind the front. ponytail: constants, not settings — an operator
// who needs different numbers has a layout this bootstrap does not plan for, and
// can edit both files afterwards.
const (
	frontPort    = 443  // external TCP: vless-reality, the front
	mieruPort    = 443  // external UDP: a different socket, no conflict
	destPort     = 9443 // loopback: Caddy — stub site, panel, naive
	grpcPort     = 9444 // loopback: vless + grpc, behind dest
	trojanWSPort = 9445 // loopback: trojan + ws, behind dest
	webFrontPort = 9446 // loopback: RouteBox's web front — stub site and Telegram WEB proxy, behind dest
)

// bootstrapMtprotoListen is where the Telegram proxy's Fake TLS listens in the
// out-of-the-box layout: off its default 9443, which is dest's loopback port
// here, so the two do not fight over it.
const bootstrapMtprotoListen = "0.0.0.0:9447"

// bootstrapUser is the name every inbound's single user carries. PanelUser is
// derived from inbound.users[] by name, so one shared name means the panel shows
// one user and the subscription does not fragment into five.
const bootstrapUser = "owner"

// allInOne is everything the bootstrap needs that is not a secret. Secrets are
// generated inside, once, and only ever leave through the two planned files.
type allInOne struct {
	Domain     string // dest's certificate, the front's stolen name, the panel's URL
	ConfigPath string // sing-box config to write
	CaddyPath  string // Caddyfile to write
	StubRoot   string // directory dest serves at the domain root
	PanelPort  int    // where this process is about to listen
	ACMEStagng bool
	ACMEEmail  string // contact for the account dest issues the certificate under
}

// planAllInOne fills in whatever the operator did not have to say. Returns an
// error only for the one thing nobody can guess: the domain.
func planAllInOne(cfg settings.Settings, settingsPath, configPath, listenAddr string) (allInOne, error) {
	a := allInOne{
		Domain:     cfg.Server.PublicHost,
		ConfigPath: configPath,
		CaddyPath:  orDefault(os.Getenv(caddyfileEnv), filepath.Join(installDir(settingsPath, configPath), "Caddyfile")),
		StubRoot:   stubRootOf(settingsPath, configPath),
		PanelPort:  panelPortOf(listenAddr),
		ACMEStagng: cfg.Network.ACMEStaging,
		// The panel's own ACME is off in this layout, so this address would
		// otherwise sit in the settings unused — while the one account that
		// does issue certificates here, dest's, had no contact at all.
		ACMEEmail: cfg.Network.ACMEEmail,
	}
	if a.Domain == "" {
		return a, fmt.Errorf("%s needs a domain: set PUBLIC_HOST (server.public_host). Reality steals its own name and dest issues the certificate for it, so there is nothing to plan without one", allInOneEnv)
	}
	if a.PanelPort == 0 {
		return a, fmt.Errorf("cannot tell which port the panel listens on from %q, so dest has nothing to forward to", listenAddr)
	}
	return a, nil
}

// installDir is the directory an install treats as its own: the one
// routebox.toml lives in, or the config's when there is no settings file.
// Absolute, because the stub root and the Caddyfile are read by Caddy — a
// different process with a different working directory, where a relative root
// silently serves nothing.
func installDir(settingsPath, configPath string) string {
	dir := filepath.Dir(settingsPath)
	if settingsPath == "" {
		dir = filepath.Dir(configPath)
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return dir
}

// stubRootOf is the directory dest's stub site is served from. One rule for
// the bootstrap that plans it, the re-plan that keeps the Caddyfile pointing
// at it, and the web front that serves it — three readers that must agree, or
// strangers would see a different site depending on who answered.
func stubRootOf(settingsPath, configPath string) string {
	return orDefault(os.Getenv(stubRootEnv), filepath.Join(installDir(settingsPath, configPath), "stub"))
}

// panelPortOf takes the port out of a listen address. 0 means "unusable", which
// the caller reports rather than guessing: dest forwarding to the wrong port
// would look exactly like a panel that is down.
func panelPortOf(listenAddr string) int {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n > 65535 {
		return 0
	}
	return n
}

// runAllInOne brings a fresh server up whole: it plans the sing-box config and
// the Caddyfile from one set of freshly generated secrets, writes both, records
// in the settings what the rest of RouteBox needs to know about this shape, and
// says once where the panel is.
//
// It runs at most once per install. An install already marked bootstrapped is
// left alone even if its config file has gone missing: that file holds the
// users, keys and secret paths clients are configured with, and regenerating
// them would lock every existing client out of a server that was working.
//
// Client links are deliberately not printed. They carry the user's UUID and
// password, and stdout here is a container log and a shell scrollback — both
// places a credential outlives the person reading it. The panel hands them out.
func runAllInOne(sm *settings.Manager, a allInOne, out io.Writer) error {
	if sm.Get().Server.Bootstrapped {
		return nil
	}

	p, err := plannedParams(a)
	if err != nil {
		return err
	}
	singbox, err := bootstrap.PlanSingbox(p)
	if err != nil {
		return err
	}
	caddyfile, err := bootstrap.PlanCaddyfile(p)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(singbox, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the planned sing-box config: %w", err)
	}

	// Both files or neither: a sing-box config whose Caddyfile never landed is a
	// front relaying to a dest that does not exist, which answers nothing at all
	// on 443 — worse than the untouched machine we started from.
	if err := os.MkdirAll(filepath.Dir(a.CaddyPath), 0755); err != nil {
		return fmt.Errorf("create the directory for %s: %w", a.CaddyPath, err)
	}
	// The credential list first: the Caddyfile imports it, so a Caddyfile
	// without it is one dest refuses to parse at all. From here on the panel
	// rewrites this file on every user change (see Handler.syncDest).
	if _, err := bootstrap.SyncNaiveUsers(p.NaiveUsers, []bootstrap.NaiveUser{
		{Name: p.User.Name, Password: p.User.Password},
	}); err != nil {
		return err
	}
	if err := os.WriteFile(a.CaddyPath, []byte(caddyfile), 0644); err != nil {
		return fmt.Errorf("write %s: %w", a.CaddyPath, err)
	}
	if err := os.WriteFile(a.ConfigPath, raw, 0644); err != nil {
		// Leaving the Caddyfile behind is harmless: dest serving the stub site on
		// loopback with no front in front of it is a web server nobody reaches.
		return fmt.Errorf("write %s: %w", a.ConfigPath, err)
	}

	// RouteBox's own ACME would race Caddy for :80 and issue a second
	// certificate for a domain whose 443 it does not hold (ADR 0001). dest is
	// the only issuer in this layout.
	if err := sm.Update(map[string]interface{}{"network.acme_enabled": false}); err != nil {
		return fmt.Errorf("turn the panel's own ACME off: %w", err)
	}
	// The Telegram WEB proxy needs a domain with a certificate and a front
	// behind dest, which this layout already is — so it comes up on, masking
	// as the install's own site. Fake TLS moves off its default 9443: that is
	// dest's loopback port here, and the two would fight over it. Ahead of
	// SetBootstrap, like the ACME switch above: Update only stages, and
	// SetBootstrap's save is what makes all of it survive a restart.
	if err := sm.Update(map[string]interface{}{
		"mtproto.web_enabled":    true,
		"mtproto.masking_domain": a.Domain,
		"mtproto.listen":         bootstrapMtprotoListen,
	}); err != nil {
		return fmt.Errorf("turn the Telegram WEB proxy on: %w", err)
	}
	dest := net.JoinHostPort(p.DestHost, strconv.Itoa(p.DestPort))
	if err := sm.SetBootstrap(p.Paths.Panel, dest, a.CaddyPath, p.Ports.Front); err != nil {
		return err
	}

	fmt.Fprintln(out, "==================================================================")
	fmt.Fprintln(out, " OUT-OF-THE-BOX INSTALL: the server is configured and the panel is")
	fmt.Fprintln(out, " behind a secret address. Open it once, from the browser you will")
	fmt.Fprintln(out, " administer this server with:")
	fmt.Fprintf(out, "   %s\n", panelURL(sm.Get()))
	fmt.Fprintln(out, " It sets a cookie and sends you to the panel. Without that cookie")
	fmt.Fprintf(out, " https://%s is the stub site, which is the point.\n", a.Domain)
	fmt.Fprintln(out, " `routebox panel-url` prints this address again.")
	fmt.Fprintln(out, "==================================================================")
	return nil
}

// migrateBootstrapMtproto brings an all-in-one install that predates the
// Telegram WEB proxy up to the keys runAllInOne writes on a fresh one. Those
// installs were bootstrapped once and never are again, so without this they
// keep Fake TLS on dest's own port and no masking domain, and the WEB proxy
// fails the moment the operator turns it on — while the panel, which treats
// both keys as managed on this layout, offers nothing to fix.
//
// Three keys, each moved only when it is in the state that cannot work: a Fake
// TLS port equal to dest's, an empty masking domain, and a web_listen host the
// Caddyfile cannot reach (it dials the front at 127.0.0.1:<port>, so a front
// on any other host than 127.0.0.1 or a wildcard is unreachable; the host
// moves to 127.0.0.1 and the port stays). Anything else is the operator's
// choice and stays. web_enabled is deliberately not touched: an upgrade must
// not switch a feature on that the operator never asked for, so they opt in
// with the panel toggle. One save for all, and a log line each, so the change
// is not silent.
//
// The web_listen move is also what makes the panel's loopback check on that
// key safe to roll back to: after boot, the value a failed save restores is
// one the check accepts.
//
// Runs on every boot and is a no-op once the keys are right, which makes it
// one-time in effect without a marker of its own.
func migrateBootstrapMtproto(sm *settings.Manager) error {
	s := sm.Get()
	if !s.Server.Bootstrapped {
		return nil
	}

	updates := map[string]interface{}{}
	if _, mtPort, err := net.SplitHostPort(s.Mtproto.Listen); err == nil {
		if _, destPort, err := net.SplitHostPort(s.Server.Dest); err == nil && mtPort == destPort {
			updates["mtproto.listen"] = bootstrapMtprotoListen
			log.Printf("mtproto.listen %s shares port %s with dest (%s); moving it to %s", s.Mtproto.Listen, mtPort, s.Server.Dest, bootstrapMtprotoListen)
		}
	}
	if host, port, err := net.SplitHostPort(s.Mtproto.WebListen); err == nil && !settings.WebListenReachesLoopback(host) {
		moved := net.JoinHostPort("127.0.0.1", port)
		updates["mtproto.web_listen"] = moved
		log.Printf("mtproto.web_listen %s is on a host dest's Caddyfile cannot reach (it dials 127.0.0.1:%s); moving it to %s", s.Mtproto.WebListen, port, moved)
	}
	if s.Mtproto.MaskingDomain == "" && s.Server.PublicHost != "" {
		updates["mtproto.masking_domain"] = s.Server.PublicHost
		log.Printf("mtproto.masking_domain is empty; setting it to server.public_host %s", s.Server.PublicHost)
	}
	if len(updates) == 0 {
		return nil
	}

	if err := sm.Update(updates); err != nil {
		return fmt.Errorf("migrate the Telegram proxy settings: %w", err)
	}
	if err := sm.Save(); err != nil {
		return fmt.Errorf("save the migrated Telegram proxy settings: %w", err)
	}
	return nil
}

// startBootstrapFront brings the web front up on a bootstrapped install, then
// re-plans dest's Caddyfile to point at it.
//
// A bind failure is logged, not fatal: this process also runs amnezia-box,
// the VPN and the panel, and crashing all of them in a restart loop over one
// busy loopback port would take down far more than the stub site. The re-plan
// is skipped in that case, so the Caddyfile is not moved to a front that is
// not listening: the one already on disk keeps pointing at the old address,
// and a planned Caddyfile answers with its handle_errors stub when the front
// is down. The operator fixes mtproto.web_listen in the panel, whose PUT
// retries the bind.
func startBootstrapFront(front *webfront.Front, listen string, replan func() error) {
	if err := front.Start(listen); err != nil {
		log.Printf("WARNING: the web front cannot listen on %s (mtproto.web_listen), and dest's catch-all points at it; strangers get dest's fallback page until mtproto.web_listen is fixed in the panel. The Caddyfile is not re-planned: %v", listen, err)
		return
	}
	// Only now: a Caddyfile re-planned to route to the front before the front
	// listens would answer strangers with an error until it did. Not fatal —
	// the file on disk keeps serving.
	if err := replan(); err != nil {
		log.Printf("WARNING: cannot re-plan dest's Caddyfile: %v", err)
	}
}

// replanRetryAfter is how long caddyReplanner trusts a remembered failure
// before asking dest again; replanClock is its clock, a test seam.
var (
	replanRetryAfter = 5 * time.Minute
	replanClock      = time.Now
)

// caddyReplanner returns the one function that keeps a bootstrapped install's
// Caddyfile in step with its settings and active sing-box config: run once at
// startup, before every naive sync on apply (and on every expiry tick), and
// after the web front moves.
//
// Everything the Caddyfile depends on is re-derived on each call rather than
// captured here, so a changed domain, panel gate or web_listen is followed
// without a restart. The calls are serialized: SyncCaddyfile writes through a
// shared temporary file, and startup, an apply and a settings save can all
// arrive at once.
//
// A skipped re-plan (an inbound the plan routes to was removed or reshaped by
// hand) is not an error — the file on disk keeps working, and overwriting the
// operator's choice is not ours to do. Neither is touching a Caddyfile the
// operator has taken over with BOOTSTRAP_CADDYFILE_REPLAN=0, which is said
// once, here, instead of on every apply.
//
// The expiry ticker calls this every 30 seconds, so a standing problem must
// not be re-reported, or re-sent to dest, on every tick. A skip is logged once
// per distinct cause. A failure is remembered with what it was planned from
// (the parameters and the file on disk): while neither changes, dest is not
// asked again and the same error comes back marked ErrReplanRepeated, which
// the ticker's callers do not log a second time.
//
// The memory lasts replanRetryAfter. Not everything a refusal depends on is in
// those inputs — a port dest could not bind that has since been freed, a full
// disk, an upgraded dest — so a remembered refusal is tried again once it is
// that old, by whichever call comes first. A retry that fails the same way is
// still answered as a repeat, so the cause stays logged once.
func caddyReplanner(sm *settings.Manager, cfg *config.Manager, panelPort int, stubRoot string) func() error {
	if bootstrap.ReplanDisabled() {
		if s := sm.Get().Server; s.Bootstrapped && s.Caddyfile != "" {
			log.Printf("BOOTSTRAP_CADDYFILE_REPLAN is off: %s is left as it is, and changes to the domain, the panel gate or the web front will not reach dest until it is edited by hand.", s.Caddyfile)
		}
		return func() error { return nil }
	}

	var (
		mu sync.Mutex
		// lastSkip and lastRealityWarning are the causes last logged, so a
		// standing one is said once.
		lastSkip, lastRealityWarning string
		// failedInputs is what the last failed attempt was planned from,
		// failedErr its error (nil once an attempt goes through), and failedAt
		// when it was last tried.
		failedInputs string
		failedErr    error
		failedAt     time.Time
	)
	// remembered answers a call whose inputs match the last failure, unless
	// that failure is old enough to try again.
	remembered := func(inputs string) error {
		if failedErr != nil && inputs == failedInputs && replanClock().Sub(failedAt) < replanRetryAfter {
			return bootstrap.RepeatedReplan(failedErr)
		}
		return nil
	}
	// remember records a failure. One that repeats the last, on the same
	// inputs and with the same text (a retry after the memory ran out), is
	// answered as a repeat, so its cause is not logged a second time.
	remember := func(inputs string, err error) error {
		repeat := failedErr != nil && inputs == failedInputs && err.Error() == failedErr.Error()
		failedInputs, failedErr, failedAt = inputs, err, replanClock()
		if repeat {
			return bootstrap.RepeatedReplan(err)
		}
		return err
	}
	return func() error {
		mu.Lock()
		defer mu.Unlock()

		all := sm.Get()
		if !all.Server.Bootstrapped || all.Server.Caddyfile == "" {
			return nil
		}
		in := bootstrap.Install{
			Domain:       all.Server.PublicHost,
			Dest:         all.Server.Dest,
			PanelPath:    all.Server.PanelPath,
			Caddyfile:    all.Server.Caddyfile,
			StubRoot:     stubRoot,
			ACMEEmail:    all.Network.ACMEEmail,
			ACMEStaging:  all.Network.ACMEStaging,
			FrontPort:    all.Server.FrontPort,
			PanelPort:    panelPort,
			WebFrontPort: panelPortOf(all.Mtproto.WebListen),
		}
		active := cfg.GetActive()
		p, err := bootstrap.CaddyParamsFromInstall(in, active)
		if errors.Is(err, bootstrap.ErrReplanSkipped) {
			if err.Error() != lastSkip {
				log.Printf("Caddyfile re-plan skipped, %s left as it is: %v", in.Caddyfile, err)
				lastSkip = err.Error()
			}
			return nil
		}
		lastSkip = ""
		if err != nil {
			// A dest address the plan cannot read stands until the settings
			// change, like a refusal does.
			inputs := "params\x00" + err.Error()
			if repeat := remembered(inputs); repeat != nil {
				return repeat
			}
			return remember(inputs, err)
		}
		// Reality's stolen name is the operator's to change in the sing-box
		// config; a domain that moved without it makes the front relay a name
		// dest no longer has a certificate for.
		warning := ""
		if name := bootstrap.RealityServerName(active); name != "" && name != in.Domain {
			warning = fmt.Sprintf("WARNING: the vless-reality inbound steals %q but dest is planned for %q: update the inbound's server_name, or the front relays a name dest has no certificate for.", name, in.Domain)
		}
		if warning != lastRealityWarning {
			if warning != "" {
				log.Print(warning)
			}
			lastRealityWarning = warning
		}

		// The file on disk is part of what the attempt is planned from: the
		// staging CA is read from it, and an operator who fixed it by hand has
		// changed what dest would be asked.
		existing, readErr := os.ReadFile(in.Caddyfile)
		inputs := fmt.Sprintf("%+v\x00%v\x00%s", p, readErr, existing)
		if repeat := remembered(inputs); repeat != nil {
			return repeat
		}

		changed, err := bootstrap.SyncCaddyfile(p, in.Caddyfile)
		if err != nil {
			return remember(inputs, err)
		}
		failedInputs, failedErr = "", nil
		if changed {
			log.Printf("Caddyfile %s re-planned from the current settings", in.Caddyfile)
		}
		return nil
	}
}

// panelURL is the one place the secret address is spelled, so the banner and the
// panel-url subcommand cannot disagree. Empty when this install has no gate.
func panelURL(cfg settings.Settings) string {
	if cfg.Server.PanelPath == "" || cfg.Server.PublicHost == "" {
		return ""
	}
	return "https://" + cfg.Server.PublicHost + cfg.Server.PanelPath
}

// plannedParams is the bootstrap's whole input: the operator's domain, the fixed
// layout, and one set of secrets minted here. The planner takes secrets rather
// than making its own precisely so this is the only place randomness enters.
func plannedParams(a allInOne) (bootstrap.Params, error) {
	realityKey, err := realityPrivateKey()
	if err != nil {
		return bootstrap.Params{}, err
	}
	password, err := generatePassword()
	if err != nil {
		return bootstrap.Params{}, fmt.Errorf("generate the user password: %w", err)
	}
	tokens := make([]string, 4)
	for i := range tokens {
		if tokens[i], err = secretToken(); err != nil {
			return bootstrap.Params{}, err
		}
	}
	return bootstrap.Params{
		Domain:     a.Domain,
		DestHost:   "127.0.0.1",
		DestPort:   destPort,
		StubRoot:   a.StubRoot,
		NaiveUsers: bootstrap.NaiveUsersPath(a.CaddyPath),
		User: bootstrap.User{
			Name:     bootstrapUser,
			UUID:     uuid.NewString(),
			Password: password,
		},
		Reality: bootstrap.Reality{PrivateKey: realityKey, ShortID: tokens[0][:8]},
		Ports: bootstrap.Ports{
			Front: frontPort, Mieru: mieruPort,
			VlessGRPC: grpcPort, TrojanWS: trojanWSPort, Panel: a.PanelPort, WebFront: webFrontPort,
		},
		Paths: bootstrap.Paths{
			VlessGRPC: "grpc-" + tokens[1],
			TrojanWS:  "/ws-" + tokens[2],
			Panel:     "/" + tokens[3],
		},
		ACME: bootstrap.ACME{Staging: a.ACMEStagng, Email: a.ACMEEmail},
	}, nil
}

// secretToken returns 16 random bytes as URL-safe text. Same alphabet the panel
// path is restricted to, so one generator serves every secret here.
func secretToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate a secret: %w", err)
	}
	// hex, not base64url: Reality's short_id is hex and is taken as a prefix of
	// one of these, and one alphabet for all of them beats a second generator.
	return hex.EncodeToString(b), nil
}

// realityPrivateKey mints the X25519 key the front's Reality handshake uses, in
// the base64url-no-padding encoding sing-box stores and serverlinks derives the
// client's public key from. Generated in-process: the alternative is shelling
// out to `amnezia-box generate reality-keypair`, which makes first boot depend
// on a binary that may not be installed yet.
func realityPrivateKey() (string, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate the reality key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(priv.Bytes()), nil
}
