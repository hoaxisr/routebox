package bootstrap

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// CaddyAdminEnv overrides where dest's admin API listens, for an install that
// moved it off Caddy's default.
const CaddyAdminEnv = "BOOTSTRAP_CADDY_ADMIN"

// defaultCaddyAdmin is Caddy's own default admin endpoint. It is loopback-only,
// which is why posting a whole config to it is not a hole.
const defaultCaddyAdmin = "127.0.0.1:2019"

// caddyReloadTimeout bounds the reload: dest re-issues nothing here, it only
// re-adapts a file it already has, so a request still running after this is a
// dest that is not answering.
const caddyReloadTimeout = 15 * time.Second

// ErrDestUnreachable marks a failure where dest's admin API gave no HTTP answer
// at all (refused, timed out). It is not a verdict on the config, which is why a
// caller that only wants to know whether dest rejected something tells the two
// apart with errors.Is.
var ErrDestUnreachable = errors.New("dest admin API unreachable")

// caddyAdminAddr is where dest's admin API listens.
func caddyAdminAddr() string {
	if admin := os.Getenv(CaddyAdminEnv); admin != "" {
		return admin
	}
	return defaultCaddyAdmin
}

// ReloadCaddy makes dest pick up the current Caddyfile — the generated naive
// credential list is imported by it, so this is what makes a user change take
// effect for naive.
//
// It posts the file to Caddy's admin API rather than shelling out to `caddy
// reload`, so it needs neither the binary's path nor the working directory it
// was started from. The reload is graceful: existing connections finish on the
// old config instead of being cut.
func ReloadCaddy(caddyfile string) error {
	body, err := os.ReadFile(caddyfile)
	if err != nil {
		return fmt.Errorf("read %s: %w", caddyfile, err)
	}
	admin := caddyAdminAddr()
	req, err := http.NewRequest(http.MethodPost, "http://"+admin+"/load", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build the reload request for dest: %w", err)
	}
	// Without this dest reads the body as JSON and rejects every Caddyfile.
	req.Header.Set("Content-Type", "text/caddyfile")

	resp, err := (&http.Client{Timeout: caddyReloadTimeout}).Do(req)
	if err != nil {
		return fmt.Errorf("dest did not answer on %s: %w: %w", admin, ErrDestUnreachable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		return nil
	}
	// Caddy's own reason, truncated: it is the only thing that says which line of
	// the config it choked on.
	reason, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("dest refused the config (%s): %s", resp.Status, strings.TrimSpace(string(reason)))
}
