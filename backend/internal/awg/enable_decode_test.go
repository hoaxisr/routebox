package awg

import (
	"encoding/json"
	"testing"
)

// TestEnableInputJSONDecode guards the bug where EnableInput had no json tags:
// the HTTP handler (EnableAWG) decodes the request body straight into
// EnableInput, and the panel sends snake_case keys. Without a `json:"listen_port"`
// tag, Go's case-insensitive matcher does NOT map "listen_port" -> ListenPort
// (underscore != camelCase), so ListenPort stayed 0 -> "listen_port out of range".
func TestEnableInputJSONDecode(t *testing.T) {
	body := `{"subnet":"10.10.0.0/24","listen_port":51820,"mtu":1420,"dns":["1.1.1.1"],"wan_iface":"ens3"}`
	var in EnableInput
	if err := json.Unmarshal([]byte(body), &in); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if in.ListenPort != 51820 {
		t.Fatalf("ListenPort = %d, want 51820 (EnableInput missing json tag)", in.ListenPort)
	}
	if in.Subnet != "10.10.0.0/24" || in.MTU != 1420 || in.WANIface != "ens3" {
		t.Fatalf("decoded = %+v", in)
	}
	if len(in.DNS) != 1 || in.DNS[0] != "1.1.1.1" {
		t.Fatalf("DNS = %v", in.DNS)
	}
}
