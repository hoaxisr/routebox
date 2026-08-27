package users

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/serverlinks"
)

// TestSubscriptionIncludesMieru is the mandatory silent-skip guard for the mieru
// inbound feature. It proves the whole chain end to end: a mieru inbound with one
// user must (1) reconcile into a panel user named "bob" whose binding credential
// is the password (mieru's CredentialKey=="password"), and (2) enumerate all the
// way to a mierus:// node in the (base64-decoded) subscription output. If either
// half regresses, a Task-1/2/3 enumeration site was lost and this fails loudly.
//
// Two reality traps baked in on purpose: BuildSubscription's output is
// base64.StdEncoding, so we decode before asserting (a raw Contains on the
// encoded string would pass/fail meaninglessly); and subscriptions.ParseLinks
// has NO mierus support, so we assert on the raw decoded links, not a round trip.
func TestSubscriptionIncludesMieru(t *testing.T) {
	active := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"type": "mieru", "tag": "mieru-in", "listen": "::",
				"listen_port": float64(2020),
				"transport":   "TCP",
				"users": []interface{}{
					map[string]interface{}{"name": "bob", "password": "pw"},
				},
			},
		},
	}

	// (1) Reconcile is the real config→registry entrypoint (Manager method).
	m := NewManager(filepath.Join(t.TempDir(), "users.toml"))
	if _, err := m.Reconcile(active); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var bob *PanelUser
	list := m.List()
	for i := range list {
		if list[i].Name == "bob" {
			bob = &list[i]
		}
	}
	if bob == nil {
		t.Fatal("reconcile did not derive panel user 'bob' from the mieru inbound")
	}
	if len(bob.Bindings) != 1 {
		t.Fatalf("bob should have exactly one binding, got %d: %#v", len(bob.Bindings), bob.Bindings)
	}
	// mieru's credential is the password (CredentialKey("mieru")=="password").
	if got := bob.Bindings[0].Credential; got != "pw" {
		t.Errorf("mieru panel-user credential = %q, want \"pw\"", got)
	}
	if got := bob.Bindings[0].Protocol; got != "mieru" {
		t.Errorf("binding protocol = %q, want \"mieru\"", got)
	}

	// (2) Subscription output is base64.StdEncoding — decode before asserting.
	sub, err := BuildSubscription(bob, active, serverlinks.PublicAddr{Host: "vpn.example.com"})
	if err != nil {
		t.Fatalf("BuildSubscription: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(sub)
	if err != nil {
		t.Fatalf("subscription output is not std base64: %v", err)
	}
	decoded := string(raw)

	if !strings.Contains(decoded, "mierus://") {
		t.Fatalf("subscription missing mieru node:\n%s", decoded)
	}
	// The fork's mierus:// grammar carries the port and transport in the query.
	if !strings.Contains(decoded, "port=2020") || !strings.Contains(decoded, "protocol=TCP") {
		t.Errorf("mieru node malformed (want port=2020 & protocol=TCP):\n%s", decoded)
	}
}
