package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"

	"routebox/backend/internal/bootstrap"
	"routebox/backend/internal/settings"
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
)

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
}

// planAllInOne fills in whatever the operator did not have to say. Returns an
// error only for the one thing nobody can guess: the domain.
func planAllInOne(cfg settings.Settings, settingsPath, configPath, listenAddr string) (allInOne, error) {
	dir := filepath.Dir(settingsPath)
	if settingsPath == "" {
		dir = filepath.Dir(configPath)
	}
	// Absolute, because the stub root and the Caddyfile are read by Caddy — a
	// different process with a different working directory, where a relative
	// root silently serves nothing.
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	a := allInOne{
		Domain:     cfg.Server.PublicHost,
		ConfigPath: configPath,
		CaddyPath:  orDefault(os.Getenv(caddyfileEnv), filepath.Join(dir, "Caddyfile")),
		StubRoot:   orDefault(os.Getenv(stubRootEnv), filepath.Join(dir, "stub")),
		PanelPort:  panelPortOf(listenAddr),
		ACMEStagng: cfg.Network.ACMEStaging,
	}
	if a.Domain == "" {
		return a, fmt.Errorf("%s needs a domain: set PUBLIC_HOST (server.public_host). Reality steals its own name and dest issues the certificate for it, so there is nothing to plan without one", allInOneEnv)
	}
	if a.PanelPort == 0 {
		return a, fmt.Errorf("cannot tell which port the panel listens on from %q, so dest has nothing to forward to", listenAddr)
	}
	return a, nil
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
			VlessGRPC: grpcPort, TrojanWS: trojanWSPort, Panel: a.PanelPort,
		},
		Paths: bootstrap.Paths{
			VlessGRPC: "grpc-" + tokens[1],
			TrojanWS:  "/ws-" + tokens[2],
			Panel:     "/" + tokens[3],
		},
		ACME: bootstrap.ACME{Staging: a.ACMEStagng},
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
