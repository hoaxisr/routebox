package api

import (
	"path/filepath"
	"testing"

	"routebox/backend/internal/config"
	"routebox/backend/internal/process"
	"routebox/backend/internal/users"
)

// newLifecycleHandler builds a handler over a vless config + reconciled registry,
// with a not-running statusSource so syncRejectRule runs the writer but skips the
// process reload (no real process needed).
func newLifecycleHandler(t *testing.T) (*Handler, *config.Manager, *users.Manager) {
	t.Helper()
	cfg, dir := newConfigWithVless(t)
	um := users.NewManager(filepath.Join(dir, "users.toml"))
	if _, err := um.Reconcile(cfg.GetActive()); err != nil {
		t.Fatal(err)
	}
	h := &Handler{config: cfg, statusSource: func() process.Status { return process.Status{Running: false} }}
	h.SetUsers(um)
	return h, cfg, um
}

// rejectRuleNames returns the auth_user list of the first reject rule in active,
// or nil if none.
func rejectRuleNames(cfg map[string]interface{}) []interface{} {
	route, _ := cfg["route"].(map[string]interface{})
	rules, _ := route["rules"].([]interface{})
	for _, r := range rules {
		if rm, ok := r.(map[string]interface{}); ok {
			if a, _ := rm["action"].(string); a == "reject" {
				if au, ok := rm["auth_user"].([]interface{}); ok {
					return au
				}
			}
		}
	}
	return nil
}

func TestSyncRejectRule_NilUsersNoPanic(t *testing.T) {
	h := &Handler{}    // panelUsers nil, no config needed
	h.syncRejectRule() // must not panic / not touch anything
}

func TestSyncRejectRule_DisabledUserWritesRule(t *testing.T) {
	h, cfg, um := newLifecycleHandler(t)
	// Disable alice in the registry.
	u := um.List()[0]
	u.Enabled = false
	if err := um.Put(&u); err != nil {
		t.Fatal(err)
	}
	h.syncRejectRule()
	au := rejectRuleNames(cfg.GetActive())
	if len(au) != 1 || au[0] != "alice" {
		t.Fatalf("expected reject auth_user=[alice], got %#v", au)
	}

	// Idempotent: a second call makes no change (no reload churn).
	h.syncRejectRule()
	au = rejectRuleNames(cfg.GetActive())
	if len(au) != 1 || au[0] != "alice" {
		t.Fatalf("second syncRejectRule changed the rule: %#v", au)
	}
}

func TestSyncRejectRule_AllActiveNoRule(t *testing.T) {
	h, cfg, _ := newLifecycleHandler(t)
	h.syncRejectRule() // alice is active by default
	if au := rejectRuleNames(cfg.GetActive()); au != nil {
		t.Fatalf("all-active must leave no reject rule, got %#v", au)
	}
}

func TestSyncRejectRule_DefersWhenDraft(t *testing.T) {
	h, cfg, um := newLifecycleHandler(t)
	u := um.List()[0]
	u.Enabled = false
	if err := um.Put(&u); err != nil {
		t.Fatal(err)
	}
	if err := cfg.EnsureDraft(); err != nil { // pending draft
		t.Fatal(err)
	}
	h.syncRejectRule()
	if au := rejectRuleNames(cfg.GetActive()); au != nil {
		t.Fatalf("draft pending must defer; active must have no reject rule, got %#v", au)
	}
}
