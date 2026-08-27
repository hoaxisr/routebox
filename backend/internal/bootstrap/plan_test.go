package bootstrap

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/config"
)

// fixture is the planner's whole input, fixed: the planner is pure, so a fixed
// input must give a fixed output. Secrets here are throwaway literals.
func fixture() Params {
	return Params{
		Domain:   "media.example.com",
		DestHost: "127.0.0.1",
		DestPort: 8443,
		StubRoot: "/var/lib/routebox/stub",
		User: User{
			Name:     "owner",
			UUID:     "11111111-2222-3333-4444-555555555555",
			Password: "s3cret-pw",
		},
		Reality: Reality{
			PrivateKey: "aFqZ8sPPy7yHY0Vzs8fyIm0e1uVWvZKPy5Q1G0y3d0A",
			ShortID:    "0123abcd",
		},
		Ports: Ports{Front: 443, Mieru: 443, VlessGRPC: 8444, TrojanWS: 8445, Panel: 8080},
		Paths: Paths{VlessGRPC: "grpc-7a1c9e", TrojanWS: "/ws-4f2b8d", Panel: "/panel-9c2f1a"},
	}
}

// inboundsByTag indexes the planned inbounds so each assertion below names the
// inbound it is about instead of an array position.
func inboundsByTag(t *testing.T, cfg map[string]interface{}) map[string]map[string]interface{} {
	t.Helper()
	arr, ok := cfg["inbounds"].([]interface{})
	if !ok {
		t.Fatalf("inbounds missing or not an array: %T", cfg["inbounds"])
	}
	out := map[string]map[string]interface{}{}
	for _, it := range arr {
		ib, ok := it.(map[string]interface{})
		if !ok {
			t.Fatalf("inbound is not an object: %T", it)
		}
		tag, _ := ib["tag"].(string)
		out[tag] = ib
	}
	return out
}

func planOK(t *testing.T, p Params) map[string]interface{} {
	t.Helper()
	cfg, err := PlanSingbox(p)
	if err != nil {
		t.Fatalf("PlanSingbox: %v", err)
	}
	return cfg
}

func TestPlanSingboxHasAllFourInbounds(t *testing.T) {
	ibs := inboundsByTag(t, planOK(t, fixture()))
	want := map[string]string{
		TagVlessReality: "vless",
		TagVlessGRPC:    "vless",
		TagTrojanWS:     "trojan",
		TagMieru:        "mieru",
	}
	if len(ibs) != len(want) {
		t.Fatalf("got %d inbounds, want %d: %v", len(ibs), len(want), ibs)
	}
	for tag, typ := range want {
		ib, ok := ibs[tag]
		if !ok {
			t.Fatalf("inbound %q missing", tag)
		}
		if ib["type"] != typ {
			t.Errorf("%s: type = %v, want %s", tag, ib["type"], typ)
		}
	}
}

func TestPlanSingboxRealityIsSelfSteal(t *testing.T) {
	p := fixture()
	ib := inboundsByTag(t, planOK(t, p))[TagVlessReality]

	tls, ok := ib["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("reality inbound has no tls block")
	}
	// Self-steal: the borrowed name IS our own domain, so SNI, certificate,
	// A-record and IP owner all agree (ADR 0001).
	if tls["server_name"] != p.Domain {
		t.Errorf("server_name = %v, want the own domain %q", tls["server_name"], p.Domain)
	}
	reality, ok := tls["reality"].(map[string]interface{})
	if !ok {
		t.Fatalf("no reality block")
	}
	if enabled, _ := reality["enabled"].(bool); !enabled {
		t.Errorf("reality not enabled")
	}
	hs, ok := reality["handshake"].(map[string]interface{})
	if !ok {
		t.Fatalf("no reality handshake block")
	}
	if hs["server"] != p.DestHost || hs["server_port"] != float64(p.DestPort) {
		t.Errorf("handshake = %v:%v, want dest %s:%d", hs["server"], hs["server_port"], p.DestHost, p.DestPort)
	}
	if ib["listen_port"] != float64(p.Ports.Front) {
		t.Errorf("listen_port = %v, want the front port %d", ib["listen_port"], p.Ports.Front)
	}
}

func TestPlanSingboxRealityNeedsNoCertificate(t *testing.T) {
	tls := inboundsByTag(t, planOK(t, fixture()))[TagVlessReality]["tls"].(map[string]interface{})
	// The certificate is dest's; the front must not carry, request or reuse one.
	for _, k := range []string{"certificate", "certificate_path", "key", "key_path", "acme"} {
		if _, present := tls[k]; present {
			t.Errorf("reality tls carries %q — dest owns the certificate", k)
		}
	}
}

func TestPlanSingboxMieruIsUDP(t *testing.T) {
	p := fixture()
	ib := inboundsByTag(t, planOK(t, p))[TagMieru]
	// mieru has no TLS ClientHello, so it cannot share the TCP front — it takes
	// 443/UDP, a different socket with a different owner.
	if ib["transport"] != "UDP" {
		t.Errorf("mieru transport = %v, want UDP", ib["transport"])
	}
	if ib["listen_port"] != float64(p.Ports.Mieru) {
		t.Errorf("mieru listen_port = %v, want %d", ib["listen_port"], p.Ports.Mieru)
	}
}

func TestPlanSingboxTransportInboundsCarrySecretPathsOnLoopback(t *testing.T) {
	p := fixture()
	ibs := inboundsByTag(t, planOK(t, p))

	grpc := ibs[TagVlessGRPC]["transport"].(map[string]interface{})
	if grpc["type"] != "grpc" || grpc["service_name"] != p.Paths.VlessGRPC {
		t.Errorf("grpc transport = %v, want type grpc with service_name %q", grpc, p.Paths.VlessGRPC)
	}
	ws := ibs[TagTrojanWS]["transport"].(map[string]interface{})
	if ws["type"] != "ws" || ws["path"] != p.Paths.TrojanWS {
		t.Errorf("ws transport = %v, want type ws with path %q", ws, p.Paths.TrojanWS)
	}
	// Behind dest means loopback-only: reachable through the path fork, never
	// on its own port from outside. serverlinks reads exactly this to decide
	// that the public port is the front's, not the inbound's.
	for _, tag := range []string{TagVlessGRPC, TagTrojanWS} {
		if listen, _ := ibs[tag]["listen"].(string); listen != "127.0.0.1" {
			t.Errorf("%s: listen = %q, want 127.0.0.1", tag, listen)
		}
	}
}

func TestPlanSingboxOneUserNameEverywhere(t *testing.T) {
	p := fixture()
	for tag, ib := range inboundsByTag(t, planOK(t, p)) {
		users, ok := ib["users"].([]interface{})
		if !ok || len(users) != 1 {
			t.Fatalf("%s: want exactly one user, got %v", tag, ib["users"])
		}
		u := users[0].(map[string]interface{})
		// PanelUser is derived from inbound.users[] BY NAME — one shared name
		// gives the panel one user, not four, and keeps the subscription whole.
		if u["name"] != p.User.Name {
			t.Errorf("%s: user name = %v, want %q", tag, u["name"], p.User.Name)
		}
	}
}

func TestPlanSingboxIsDeterministic(t *testing.T) {
	a, err := json.Marshal(planOK(t, fixture()))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(planOK(t, fixture()))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("same input gave different bytes:\n%s\n%s", a, b)
	}
}

func TestPlanSingboxRejectsIncompleteInput(t *testing.T) {
	cases := map[string]func(*Params){
		"no domain":     func(p *Params) { p.Domain = "" },
		"no dest host":  func(p *Params) { p.DestHost = "" },
		"no dest port":  func(p *Params) { p.DestPort = 0 },
		"no user name":  func(p *Params) { p.User.Name = "" },
		"no uuid":       func(p *Params) { p.User.UUID = "" },
		"no password":   func(p *Params) { p.User.Password = "" },
		"no reality":    func(p *Params) { p.Reality.PrivateKey = "" },
		"no short id":   func(p *Params) { p.Reality.ShortID = "" },
		"no front port": func(p *Params) { p.Ports.Front = 0 },
		"no grpc path":  func(p *Params) { p.Paths.VlessGRPC = "" },
		"no ws path":    func(p *Params) { p.Paths.TrojanWS = "" },
	}
	for name, break_ := range cases {
		t.Run(name, func(t *testing.T) {
			p := fixture()
			break_(&p)
			if _, err := PlanSingbox(p); err == nil {
				t.Fatalf("incomplete input accepted")
			}
		})
	}
}

// The planned config must survive the panel's own validator: it is what runs on
// every apply, and a config it rejects is a config the operator cannot save.
func TestPlanSingboxPassesPanelValidator(t *testing.T) {
	cfg := planOK(t, fixture())
	// Round-trip through JSON: the validator reads the shapes json.Unmarshal
	// produces (numbers as float64), which is also what lands on disk.
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if errs := config.NewEmptyManager("").Validate(decoded); len(errs) != 0 {
		t.Fatalf("panel validator rejected the planned config: %s", strings.Join(errs, "; "))
	}
}

// The real oracle, when there is one on this machine: sing-box itself.
func TestPlanSingboxPassesSingboxCheck(t *testing.T) {
	binary := ""
	for _, name := range []string{"amnezia-box", "sing-box"} {
		if path, err := exec.LookPath(name); err == nil {
			binary = path
			break
		}
	}
	if binary == "" {
		t.Skip("no sing-box/amnezia-box binary on PATH")
	}
	raw, err := json.Marshal(planOK(t, fixture()))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	if ok, errs := config.CheckConfigWith(binary, path); !ok {
		t.Fatalf("%s check rejected the planned config: %s", binary, strings.Join(errs, "; "))
	}
}
