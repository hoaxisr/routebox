package api

import (
	"net/http"
	"time"

	"routebox/backend/internal/bootstrap"
	"routebox/backend/internal/serverlinks"
	"routebox/backend/internal/users"
)

// naiveNode is one person's access to naive, the only one of the five protocols
// an out-of-the-box install does not serve from sing-box (ADR 0002). It has no
// inbound, so the panel cannot show it the way it shows the other four, and
// without this the operator sees four protocols on a server that runs five.
type naiveNode struct {
	Name string `json:"name"`
	Link string `json:"link"`
}

// naiveUsers returns the credentials dest currently authenticates against, or
// nil on an install with no dest. Derived from the config by the same function
// that renders the file dest imports, so what the panel shows and what the
// server accepts cannot drift apart.
func (h *Handler) naiveUsers() []bootstrap.NaiveUser {
	if h.settings == nil || h.config == nil {
		return nil
	}
	if !h.settings.Get().Server.Bootstrapped {
		return nil
	}
	// Disabled and expired people are dropped from dest's list; offering them a
	// link would be offering one the server refuses.
	var blocked map[string]bool
	if h.panelUsers != nil {
		blocked = map[string]bool{}
		for _, name := range users.EffectiveRejectNames(h.panelUsers.List(), time.Now().Unix()) {
			blocked[name] = true
		}
	}
	return bootstrap.NaiveUsersOfConfig(h.config.GetActive(), blocked)
}

// naiveUserLink returns the client link for one person's naive access, or ""
// when this install has no dest, the person is not in dest's list, or nothing
// fronts the loopback dest listens on.
func (h *Handler) naiveUserLink(name, host string) string {
	if name == "" || host == "" {
		return ""
	}
	for _, u := range h.naiveUsers() {
		if u.Name != name {
			continue
		}
		link, err := serverlinks.BuildNaiveLink(u.Name, u.Password,
			serverlinks.PublicAddr{Host: host, Port: h.frontPort()})
		if err != nil {
			return ""
		}
		return link
	}
	return ""
}

// GetDestNaive reports the naive node dest serves, so the inbounds page can show
// the fifth protocol next to the four sing-box carries.
func (h *Handler) GetDestNaive(w http.ResponseWriter, r *http.Request) {
	out := map[string]interface{}{"enabled": false, "users": []naiveNode{}}
	list := h.naiveUsers()
	if list == nil {
		writeSuccess(w, out)
		return
	}
	host := ""
	if h.settings != nil {
		host = h.settings.Get().Server.PublicHost
	}
	nodes := make([]naiveNode, 0, len(list))
	for _, u := range list {
		// A person with no usable link is still worth naming: the operator can
		// see naive exists for them and fix the missing public host or front port.
		nodes = append(nodes, naiveNode{Name: u.Name, Link: h.naiveUserLink(u.Name, host)})
	}
	out["enabled"] = true
	out["host"] = host
	out["port"] = h.frontPort()
	out["users"] = nodes
	writeSuccess(w, out)
}
