package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/serverlinks"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/users"
)

func destNaive(t *testing.T, h *Handler) map[string]interface{} {
	t.Helper()
	rr := httptest.NewRecorder()
	h.GetDestNaive(rr, httptest.NewRequest("GET", "/api/dest/naive", nil))
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("answer is not JSON: %v (%s)", err, rr.Body.String())
	}
	return resp.Data
}

// naive is the one protocol of the five that sing-box does not serve, so it has
// no inbound and the panel's inbounds page cannot show it. Without this the
// operator sees four protocols on a server that runs five, and the only way to
// hand anyone naive access is to read a file over SSH.
func TestDestNaiveListsTheNodeAndItsLink(t *testing.T) {
	h, _, _, _ := destInstall(t, oneUser)
	if err := h.settings.Update(map[string]interface{}{"server.public_host": "vpn.example.com"}); err != nil {
		t.Fatalf("set the host: %v", err)
	}
	data := destNaive(t, h)
	if data["enabled"] != true {
		t.Fatalf("naive reported as absent on a bootstrapped install: %v", data)
	}
	if data["host"] != "vpn.example.com" || data["port"] != float64(443) {
		t.Fatalf("public endpoint is wrong: %v", data)
	}
	list, _ := data["users"].([]interface{})
	if len(list) != 1 {
		t.Fatalf("want one naive user, got %v", data["users"])
	}
	one, _ := list[0].(map[string]interface{})
	if one["name"] != "alice" {
		t.Fatalf("wrong user: %v", one)
	}
	link, _ := one["link"].(string)
	if !strings.HasPrefix(link, "naive+https://alice:pw-alice@vpn.example.com:443") {
		t.Fatalf("link does not match what dest accepts: %q", link)
	}
}

// An install that did not come up from the bootstrap plan has no dest at all:
// answering with a node would invent a protocol the server does not serve.
func TestDestNaiveIsAbsentWithoutDest(t *testing.T) {
	h, _, _, _ := destInstall(t, oneUser)
	// server.bootstrapped is not a runtime setting — only SetBootstrap writes it —
	// so an install without dest is one whose settings never saw it.
	sm, err := settings.NewManager(filepath.Join(t.TempDir(), "routebox.toml"))
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	h.settings = sm
	if data := destNaive(t, h); data["enabled"] != false {
		t.Fatalf("naive reported on an install without dest: %v", data)
	}
}

// A disabled or expired user is dropped from the list dest authenticates
// against, so their link must not be offered either — it would be a link the
// server refuses.
func TestDestNaiveDropsBlockedUsers(t *testing.T) {
	h, _, um, _ := destInstall(t, oneUser)
	if err := h.settings.Update(map[string]interface{}{"server.public_host": "vpn.example.com"}); err != nil {
		t.Fatalf("set the host: %v", err)
	}
	list := um.List()
	if len(list) != 1 {
		t.Fatalf("want one panel user, got %d", len(list))
	}
	u := list[0]
	u.Enabled = false
	if err := um.Put(&u); err != nil {
		t.Fatalf("disable the user: %v", err)
	}
	data := destNaive(t, h)
	if got, _ := data["users"].([]interface{}); len(got) != 0 {
		t.Fatalf("a blocked user was still offered a link: %v", got)
	}
}

// The subscription is where clients actually take their nodes from, so naive
// has to be in it — otherwise the protocol exists, is documented, and reaches
// nobody.
func TestSubscriptionCarriesTheNaiveNode(t *testing.T) {
	h, _, um, _ := destInstall(t, oneUser)
	if err := h.settings.Update(map[string]interface{}{"server.public_host": "vpn.example.com"}); err != nil {
		t.Fatalf("set the host: %v", err)
	}
	u := um.List()[0]
	link := h.naiveUserLink(u.Name, "vpn.example.com")
	if link == "" {
		t.Fatal("no naive link for a user dest authenticates")
	}
	body, err := users.BuildSubscription(&u, h.config.GetActive(),
		serverlinks.PublicAddr{Host: "vpn.example.com", Port: 443}, link)
	if err != nil {
		t.Fatalf("BuildSubscription: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("subscription is not base64: %v", err)
	}
	decoded := string(raw)
	if !strings.Contains(decoded, "naive+https://") {
		t.Fatalf("the subscription has no naive node:\n%s", decoded)
	}
	if !strings.Contains(decoded, "trojan://") {
		t.Fatalf("the other nodes went missing:\n%s", decoded)
	}
}
