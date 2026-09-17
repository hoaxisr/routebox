package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// A config written by an older RouteBox carries tun "stack". Loading it must
// drop the key both in memory and on disk — amnezia-box has no gVisor build
// left, and 1.15's own stack is what runs when the option is absent.
func TestLoadStripsTunStack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	const raw = `{"inbounds":[
		{"type":"tun","tag":"tun-in","stack":"gvisor","auto_route":true},
		{"type":"mixed","tag":"mixed-in","stack":"system"}
	]}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	inbounds := m.Get()["inbounds"].([]interface{})
	tun := inbounds[0].(map[string]interface{})
	if _, has := tun["stack"]; has {
		t.Error("tun inbound kept its stack option in memory")
	}
	if tun["auto_route"] != true {
		t.Error("migration dropped more than the stack key")
	}
	// Not a tun inbound: sing-box ignores the key there, so leave it alone.
	if mixed := inbounds[1].(map[string]interface{}); mixed["stack"] != "system" {
		t.Error("migration touched a non-tun inbound")
	}

	onDisk := map[string]interface{}{}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatal(err)
	}
	diskTun := onDisk["inbounds"].([]interface{})[0].(map[string]interface{})
	if _, has := diskTun["stack"]; has {
		t.Error("the file on disk still has tun stack — the process would start on the old config")
	}
}

func TestStripTunStackReportsNoChange(t *testing.T) {
	cfg := map[string]interface{}{"inbounds": []interface{}{
		map[string]interface{}{"type": "tun", "tag": "tun-in"},
	}}
	if stripTunStack(cfg) {
		t.Error("reported a change on a config that has no stack option")
	}
}

// A hand-edited config posted to the API must be refused with a readable
// message instead of starting and dying on "gVisor is not included".
func TestValidateRejectsGVisorStack(t *testing.T) {
	m := NewEmptyManager(filepath.Join(t.TempDir(), "config.json"))
	for _, stack := range []string{"gvisor", "mixed"} {
		errs := m.Validate(map[string]interface{}{"inbounds": []interface{}{
			map[string]interface{}{
				"type": "tun", "tag": "tun-in", "stack": stack,
				"address": []interface{}{"172.19.0.1/30"},
			},
		}})
		found := false
		for _, e := range errs {
			if contains(e, "gVisor build") {
				found = true
			}
		}
		if !found {
			t.Errorf("stack %q was accepted: %v", stack, errs)
		}
	}
}
