package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/bootstrap"
	"routebox/backend/internal/settings"
)

// installed brings up a fresh out-of-the-box install in its own directory and
// returns what the operator would see and what landed on disk.
func installed(t *testing.T) (*settings.Manager, allInOne, string) {
	t.Helper()
	dir := t.TempDir()
	sm := newVPSSettings(t, dir)
	if err := sm.Update(map[string]interface{}{"server.public_host": "media.example.com"}); err != nil {
		t.Fatalf("set the domain: %v", err)
	}
	a, err := planAllInOne(sm.Get(), sm.GetPath(), filepath.Join(dir, "config.json"), "0.0.0.0:8443")
	if err != nil {
		t.Fatalf("planAllInOne: %v", err)
	}
	var out strings.Builder
	if err := runAllInOne(sm, a, &out); err != nil {
		t.Fatalf("runAllInOne: %v", err)
	}
	return sm, a, out.String()
}

func readJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("%s is not JSON: %v", path, err)
	}
	return cfg
}

// The whole point of the ticket: one domain in, a server that carries all five
// protocols out. Four of them are sing-box inbounds; naive is dest's.
func TestAllInOneConfiguresEveryProtocol(t *testing.T) {
	_, a, _ := installed(t)

	inbounds, ok := readJSON(t, a.ConfigPath)["inbounds"].([]interface{})
	if !ok || len(inbounds) != 4 {
		t.Fatalf("want four planned inbounds, got %#v", readJSON(t, a.ConfigPath)["inbounds"])
	}
	caddyfile, err := os.ReadFile(a.CaddyPath)
	if err != nil {
		t.Fatalf("read the Caddyfile: %v", err)
	}
	for _, want := range []string{"forward_proxy {", "import ", "file_server"} {
		if !strings.Contains(string(caddyfile), want) {
			t.Errorf("the Caddyfile has no %q:\n%s", want, caddyfile)
		}
	}
	// naive's credentials live in the file the Caddyfile imports — the one the
	// panel rewrites whenever a user changes. Without it dest does not parse.
	naive, err := os.ReadFile(bootstrap.NaiveUsersPath(a.CaddyPath))
	if err != nil {
		t.Fatalf("read the naive credential list: %v", err)
	}
	if !strings.Contains(string(naive), "basic_auth "+bootstrapUser+" ") {
		t.Errorf("the credential list has no %q:\n%s", bootstrapUser, naive)
	}
}

// The settings are how the rest of RouteBox learns this install's shape: the
// mark the connections monitor reads, the dest, the external port client links
// must carry, and the gate the operator gets back from `routebox panel-url`.
func TestAllInOneRecordsTheInstallInSettings(t *testing.T) {
	sm, _, _ := installed(t)

	got := sm.Get()
	if !got.Server.Bootstrapped {
		t.Error("the install is not marked bootstrapped, so nothing downstream can tell")
	}
	if got.Server.Dest != "127.0.0.1:9443" {
		t.Errorf("dest = %q, want the loopback address the Caddyfile was planned for", got.Server.Dest)
	}
	if got.Server.FrontPort != frontPort {
		t.Errorf("front_port = %d, want %d — without it every client link is skipped", got.Server.FrontPort, frontPort)
	}
	if got.Network.ACMEEnabled {
		t.Error("the panel's own ACME is still on; it would race dest for :80 and issue a second certificate")
	}
	if want := "https://media.example.com" + got.Server.PanelPath; panelURL(got) != want {
		t.Errorf("panelURL = %q, want %q", panelURL(got), want)
	}

	// Reloading proves it survives a restart, which is what "can be learned
	// later" means in practice.
	reloaded := newVPSSettings(t, filepath.Dir(sm.GetPath()))
	if panelURL(reloaded.Get()) != panelURL(got) {
		t.Errorf("after a reload the gate is %q, was %q", panelURL(reloaded.Get()), panelURL(got))
	}
}

// A stub site plus a fixed set of secrets would make every install of RouteBox
// answer to the same keys — the fingerprint the whole architecture exists to
// avoid.
func TestAllInOneSecretsDifferBetweenInstalls(t *testing.T) {
	seen := map[string]string{}
	for i := 0; i < 2; i++ {
		sm, a, _ := installed(t)
		raw, err := os.ReadFile(a.ConfigPath)
		if err != nil {
			t.Fatal(err)
		}
		for name, value := range map[string]string{
			"config": string(raw),
			"gate":   sm.Get().Server.PanelPath,
		} {
			if prev, ok := seen[name]; ok && prev == value {
				t.Errorf("two installs got the same %s", name)
			}
			seen[name] = value
		}
	}
}

// The users, keys and secret paths in that config are what every client is
// configured with. A second bootstrap would replace them and lock everyone out —
// including when the config file is the thing that went missing.
func TestAllInOneNeverRunsTwice(t *testing.T) {
	sm, a, _ := installed(t)
	gate := sm.Get().Server.PanelPath

	if err := os.Remove(a.ConfigPath); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := runAllInOne(sm, a, &out); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if _, err := os.Stat(a.ConfigPath); err == nil {
		t.Error("the second run rewrote the config file")
	}
	if sm.Get().Server.PanelPath != gate {
		t.Error("the second run minted a new gate")
	}
	if out.Len() != 0 {
		t.Errorf("the second run announced the install again:\n%s", out.String())
	}
}

// Announced once, and only what is safe to leave in a container log: the gate is
// useless without the panel password behind it, while a client link carries the
// user's UUID and password in the clear.
func TestAllInOneAnnouncesTheGateAndNoClientLinks(t *testing.T) {
	sm, _, banner := installed(t)

	if !strings.Contains(banner, panelURL(sm.Get())) {
		t.Errorf("the banner does not print the gate:\n%s", banner)
	}
	for _, leak := range []string{"vless://", "trojan://", "mierus://", "naive+https://"} {
		if strings.Contains(banner, leak) {
			t.Errorf("the banner leaks a client link (%s):\n%s", leak, banner)
		}
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(sm.GetPath()), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Inbounds []struct {
			Users []struct {
				UUID     string `json:"uuid"`
				Password string `json:"password"`
			} `json:"users"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, in := range cfg.Inbounds {
		for _, u := range in.Users {
			for _, secret := range []string{u.UUID, u.Password} {
				if secret != "" && strings.Contains(banner, secret) {
					t.Errorf("the banner prints a client credential:\n%s", banner)
				}
			}
		}
	}
}

func TestPlanAllInOneRefusesWhatItCannotGuess(t *testing.T) {
	withDomain := settings.Settings{}
	withDomain.Server.PublicHost = "media.example.com"

	cases := map[string]struct {
		cfg    settings.Settings
		listen string
	}{
		// Reality steals its own name and dest issues the certificate for it.
		"no domain":     {settings.Settings{}, "0.0.0.0:8443"},
		"no panel port": {withDomain, "8443"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if _, err := planAllInOne(c.cfg, filepath.Join(dir, "routebox.toml"), filepath.Join(dir, "config.json"), c.listen); err == nil {
				t.Fatal("planned an install it cannot bring up")
			}
		})
	}
}

// The address the operator gave the installer has to reach the account that
// actually issues certificates here — dest's, not the panel's, whose own ACME
// this same bootstrap turns off.
func TestAllInOnePassesTheACMEContactToDest(t *testing.T) {
	dir := t.TempDir()
	sm := newVPSSettings(t, dir)
	if err := sm.Update(map[string]interface{}{
		"server.public_host": "media.example.com",
		"network.acme_email": "you@example.com",
	}); err != nil {
		t.Fatalf("set the domain and the contact: %v", err)
	}
	a, err := planAllInOne(sm.Get(), sm.GetPath(), filepath.Join(dir, "config.json"), "0.0.0.0:8443")
	if err != nil {
		t.Fatalf("planAllInOne: %v", err)
	}
	var out strings.Builder
	if err := runAllInOne(sm, a, &out); err != nil {
		t.Fatalf("runAllInOne: %v", err)
	}
	caddyfile, err := os.ReadFile(a.CaddyPath)
	if err != nil {
		t.Fatalf("read the Caddyfile: %v", err)
	}
	if !strings.Contains(string(caddyfile), "email you@example.com") {
		t.Fatalf("the contact never reached dest:\n%s", caddyfile)
	}
}
