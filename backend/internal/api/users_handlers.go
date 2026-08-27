package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"routebox/backend/internal/config"
	"routebox/backend/internal/serverlinks"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/users"
)

// userView is the GET /api/users wire shape: a registry user plus a Pending flag
// (true for draft-only users not yet in the registry). KEEP A's choice: ONE
// unified list; pending entries carry pending:true and an empty ID.
type userView struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Enabled       bool            `json:"enabled"`
	ExpiresAt     int64           `json:"expires_at"`
	Pending       bool            `json:"pending"`
	Token         string          `json:"token"`
	TokenDisabled bool            `json:"token_disabled"`
	Bindings      []users.Binding `json:"bindings"`
	Upload        int64           `json:"upload"`
	Download      int64           `json:"download"`
	// Warning is set when the change was made but did not reach everywhere it
	// had to — today, dest, which serves naive on its own. Omitted when empty,
	// so every other answer keeps its shape.
	Warning string `json:"warning,omitempty"`
}

// ListUsers returns registry (applied) users plus pending users that exist only
// in the draft (working\active diff over server-inbound credentials), as ONE
// unified list.
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	if h.panelUsers == nil {
		writeError(w, http.StatusServiceUnavailable, "users not initialized")
		return
	}
	views := make([]userView, 0)
	registered := map[string]bool{} // (tag\x00cred) covered by the registry

	for _, u := range h.panelUsers.List() {
		for _, b := range u.Bindings {
			registered[b.InboundTag+"\x00"+b.Credential] = true
		}
		var up, down int64
		if h.traffic != nil {
			for _, name := range userTrafficNames(u) {
				nu, nd, err := h.traffic.QueryUserTotals(0, 1<<62, name)
				if err == nil {
					up += nu
					down += nd
				}
			}
		}
		views = append(views, userView{
			ID: u.ID, Name: u.Name, Enabled: u.Enabled, ExpiresAt: u.ExpiresAt,
			Pending: false, Token: u.Token, TokenDisabled: u.TokenDisabled, Bindings: u.Bindings,
			Upload: up, Download: down,
		})
	}

	// Pending = server users present in the working (draft) config but whose
	// (tag, credential) is not yet registered (created but not applied).
	for _, ib := range h.config.ListInbounds() {
		for _, cu := range users.ServerInboundUsers(ib) {
			if registered[cu.InboundTag+"\x00"+cu.Credential] {
				continue
			}
			views = append(views, userView{
				Name: cu.Name, Enabled: true, Pending: true,
				Bindings: []users.Binding{{
					InboundTag: cu.InboundTag, Credential: cu.Credential,
					Protocol: cu.Protocol, Name: cu.Name, Flow: cu.Flow,
				}},
			})
		}
	}

	writeSuccess(w, views)
}

// createUserBody is the POST /api/users payload.
type createUserBody struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	InboundTag string `json:"inbound_tag"`
}

// CreateUser generates a credential, adds the user to the draft inbound, and
// returns its pending view. The registry is NOT touched (materializes on Apply).
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if h.panelUsers == nil {
		writeError(w, http.StatusServiceUnavailable, "users not initialized")
		return
	}
	var body createUserBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.nameTaken(body.Name) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("a user named %q already exists", body.Name))
		return
	}
	cred, err := h.stageUserInDraft(body.InboundTag, body.Protocol, body.Name)
	if err != nil {
		writeConfigError(w, http.StatusBadRequest, err)
		return
	}
	writeSuccess(w, userView{
		Name: body.Name, Enabled: true, Pending: true,
		Bindings: []users.Binding{{
			InboundTag: body.InboundTag, Credential: cred, Protocol: body.Protocol, Name: body.Name,
		}},
	})
}

// AddBinding adds the existing panel user into another inbound's draft user list,
// generating a fresh credential. Persists nothing to the registry (reconcile on Apply).
func (h *Handler) AddBinding(w http.ResponseWriter, r *http.Request) {
	if h.panelUsers == nil {
		writeError(w, http.StatusServiceUnavailable, "users not initialized")
		return
	}
	id := chi.URLParam(r, "id")
	u, ok := h.panelUsers.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	var body struct {
		Protocol   string `json:"protocol"`
		InboundTag string `json:"inbound_tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Dedup: reject if the target inbound already contains this user. Check the
	// WORKING (draft-or-active) config so this catches BOTH an already-applied
	// binding and a pending one staged earlier this session — preventing the
	// infinite same-inbound binding bug. Match on extracted Name (server users
	// carry the panel user's name; vless/trojan/hy2 use "name", naive "username").
	if ib, ok := h.config.GetInbound(body.InboundTag); ok {
		for _, cu := range users.ServerInboundUsers(ib) {
			if cu.Name == u.Name {
				writeError(w, http.StatusConflict,
					fmt.Sprintf("user %q is already bound to inbound %q", u.Name, body.InboundTag))
				return
			}
		}
	}
	if _, err := h.stageUserInDraft(body.InboundTag, body.Protocol, u.Name); err != nil {
		writeConfigError(w, http.StatusBadRequest, err)
		return
	}
	writeSuccess(w, map[string]string{"message": "binding staged in draft"})
}

// DeleteUser removes every binding's credential from its draft inbound. The
// registry entry is cleaned up by reconcile on the next Apply (A1).
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if h.panelUsers == nil {
		writeError(w, http.StatusServiceUnavailable, "users not initialized")
		return
	}
	id := chi.URLParam(r, "id")
	u, ok := h.panelUsers.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	// Pre-check ALL bindings before staging any removal: if any binding would
	// trip the mieru last-user guard, return 409 WITHOUT mutating the draft.
	// Otherwise an earlier (non-mieru) binding would already be removed from the
	// draft when a later binding trips the guard — a partial mutation hidden
	// behind a "nothing happened" 409. Uses the SAME predicate the guard enforces,
	// read against the current working config (bindings live on distinct inbounds,
	// so this dry-run matches what the sequential loop would see).
	for _, b := range u.Bindings {
		field := users.CredentialKey(b.Protocol)
		if field == "" {
			continue // unknown protocol: removeUserFromDraft is a no-op anyway
		}
		ib, ok := h.config.GetInbound(b.InboundTag)
		if !ok {
			continue // inbound already gone: removal is a harmless no-op
		}
		if mieruLastUserRemoval(ib, field, b.Credential) {
			writeError(w, http.StatusConflict,
				fmt.Sprintf("%s: inbound %q", ErrLastMieruUser, b.InboundTag))
			return
		}
	}
	for _, b := range u.Bindings {
		if err := h.removeUserFromDraft(b.InboundTag, b.Protocol, b.Credential); err != nil {
			// The last-user-of-a-mieru-inbound guard is a client-actionable
			// rejection (delete the inbound instead), not a server fault.
			if errors.Is(err, ErrLastMieruUser) {
				writeConfigError(w, http.StatusConflict, err)
				return
			}
			writeConfigError(w, http.StatusInternalServerError, err)
			return
		}
	}
	// #19: drop the deleted client's per-user Breakdown history, keyed by the same
	// names GetUserTraffic sums over (Name + binding names). Best-effort — an orphaned
	// series must not fail the delete. ponytail: purged on delete-intent, not on Apply;
	// a delete-then-discard loses the stats too, which is acceptable for a deleted client.
	if h.traffic != nil {
		if err := h.traffic.DeleteUsers(userTrafficNames(u)); err != nil {
			log.Printf("api: purge user traffic for %q: %v", u.Name, err)
		}
	}
	writeSuccess(w, map[string]string{"message": "user removed from draft (apply to finalize)"})
}

// GetUserLinkByID builds a share link for one registry user's binding, resolved
// against the ACTIVE config (the user must be applied/running for the link to work).
// Query: tag (which binding) + optional host (defaults to settings server.public_host).
func (h *Handler) GetUserLinkByID(w http.ResponseWriter, r *http.Request) {
	if h.panelUsers == nil {
		writeError(w, http.StatusServiceUnavailable, "users not initialized")
		return
	}
	id := chi.URLParam(r, "id")
	tag := r.URL.Query().Get("tag")
	host := r.URL.Query().Get("host")
	if host == "" && h.settings != nil {
		host = h.settings.Get().Server.PublicHost
	}
	// Sanitize the host before it is interpolated into the share-link URL: a
	// free-form query host must pass the same validation as server.public_host.
	sanitized, err := settings.SanitizePublicHost(host)
	if err != nil || sanitized == "" {
		writeError(w, http.StatusBadRequest, "valid host required (set server.public_host or pass ?host=)")
		return
	}
	host = sanitized
	u, ok := h.panelUsers.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	var cred string
	for _, b := range u.Bindings {
		if b.InboundTag == tag {
			cred = b.Credential
			break
		}
	}
	if cred == "" {
		writeError(w, http.StatusNotFound, fmt.Sprintf("user has no binding for inbound %q", tag))
		return
	}
	// Resolve against the ACTIVE config.
	active := h.config.GetActive()
	inbound, found := findActiveInbound(active, tag)
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("inbound %q not found in active config", tag))
		return
	}
	user, found := findActiveUserByCredential(inbound, cred)
	if !found {
		writeError(w, http.StatusNotFound, "credential not present in active config (apply pending changes first)")
		return
	}
	link, err := serverlinks.BuildShareLink(inbound, user, serverlinks.PublicAddr{Host: host, Port: h.frontPort()})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeSuccess(w, map[string]string{"link": link})
}

// --- draft mutation helpers (operate on the working/draft config) ---

// stageUserInDraft generates a credential for the protocol, appends the user to
// the inbound's draft users array, and returns the generated credential. The
// validation and mutation run inside config.MutateInbound's single write lock,
// so two concurrent stages on the same inbound serialize (no lost update); the
// fn mutates a draft-private clone, so the active config is never touched.
func (h *Handler) stageUserInDraft(tag, protocol, name string) (string, error) {
	cred, err := genCredential(protocol)
	if err != nil {
		return "", err
	}
	user := map[string]interface{}{}
	switch protocol {
	case "vless":
		user["name"] = name
		user["uuid"] = cred
		user["flow"] = "xtls-rprx-vision"
	case "naive":
		user["username"] = name
		user["password"] = cred
	case "trojan":
		user["name"] = name
		user["password"] = cred
	case "hysteria2":
		user["name"] = name
		user["password"] = cred
	case "mieru":
		user["name"] = name
		user["password"] = cred
	default:
		return "", fmt.Errorf("unsupported protocol %q", protocol)
	}

	err = h.config.MutateInbound(tag, func(inbound map[string]interface{}) error {
		if got, _ := inbound["type"].(string); got != protocol {
			return fmt.Errorf("inbound %q is type %q, not %q", tag, got, protocol)
		}
		// inbound is a draft-private clone held under the lock; mutate directly.
		existing, _ := inbound["users"].([]interface{})
		inbound["users"] = append(existing, user)
		return nil
	})
	if err != nil {
		return "", err
	}
	return cred, nil
}

// ErrLastMieruUser is returned by removeUserFromDraft when a removal would empty
// a mieru inbound's users list. The mieru validator (Task 1) rejects a 0-user
// mieru inbound, so letting the draft reach that state would make EVERY later
// ApplyConfig (including lifecycle syncs that defer on a pending draft) fail with
// a validation error that looks unrelated. Callers surface the tag, never the
// credential. Delete the inbound itself to remove its last user.
var ErrLastMieruUser = errors.New("cannot remove the last user of a mieru inbound — delete the inbound instead")

// removeUserFromDraft removes the user with the given credential from the
// inbound's draft users array. The match+filter runs inside MutateInbound's
// single write lock on a draft-private clone (active config untouched). A removal
// that would empty a mieru inbound is rejected with ErrLastMieruUser (see above).
func (h *Handler) removeUserFromDraft(tag, protocol, cred string) error {
	field := users.CredentialKey(protocol)
	if field == "" {
		return nil // unknown protocol: nothing to match/remove
	}
	err := h.config.MutateInbound(tag, func(inbound map[string]interface{}) error {
		// Guard: never stage a mieru inbound into a 0-user state (unappliable).
		// Only fires when this removal is what empties it — an already-empty
		// inbound is a harmless no-op. Named error, tags the inbound but never
		// the credential. Shared predicate so DeleteUser's pre-check cannot drift.
		if mieruLastUserRemoval(inbound, field, cred) {
			return fmt.Errorf("%w: inbound %q", ErrLastMieruUser, tag)
		}
		// inbound is a draft-private clone held under the lock; mutate directly.
		arr, _ := inbound["users"].([]interface{})
		kept := make([]interface{}, 0, len(arr))
		for _, u := range arr {
			um, ok := u.(map[string]interface{})
			if !ok {
				continue
			}
			if c, _ := um[field].(string); c == cred {
				continue
			}
			kept = append(kept, u)
		}
		inbound["users"] = kept
		return nil
	})
	// The inbound being gone is not an error for removal (already absent).
	if errors.Is(err, config.ErrInboundNotFound) {
		return nil
	}
	return err
}

// mieruLastUserRemoval reports whether removing the user matched by (field, cred)
// from this inbound would empty a MIERU inbound — the exact condition
// ErrLastMieruUser guards (mieru + had users + no survivor remains). It is the
// SINGLE source of truth for that guard: removeUserFromDraft (the enforcing
// mutation) and DeleteUser (the dry-run pre-check) both call it, so the two can
// never diverge. Non-mieru inbounds and already-empty inbounds always return
// false (no guard, no-op). Mirrors removeUserFromDraft's filter: malformed
// (non-map) entries are treated as removed, exactly as the mutation drops them.
func mieruLastUserRemoval(inbound map[string]interface{}, field, cred string) bool {
	if t, _ := inbound["type"].(string); t != "mieru" {
		return false
	}
	arr, _ := inbound["users"].([]interface{})
	if len(arr) == 0 {
		return false // already empty: harmless no-op, not a guard trip
	}
	for _, u := range arr {
		um, ok := u.(map[string]interface{})
		if !ok {
			continue // malformed entry is dropped by the mutation too
		}
		if c, _ := um[field].(string); c != cred {
			return false // a survivor remains → removal does not empty it
		}
	}
	return true
}

// genCredential returns a fresh credential for the protocol: a UUIDv4 for vless,
// otherwise a random password.
func genCredential(protocol string) (string, error) {
	if protocol == "vless" {
		return uuid.NewString(), nil
	}
	return randomPassword()
}

// RotateUserToken mints a fresh subscription token for the registry user,
// invalidating the previous one. Registry-only: it does NOT touch the config
// draft (no pending-changes bar). PROTECTED route.
func (h *Handler) RotateUserToken(w http.ResponseWriter, r *http.Request) {
	if h.panelUsers == nil {
		writeError(w, http.StatusServiceUnavailable, "users not initialized")
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := h.panelUsers.Get(id); !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	tok, err := h.panelUsers.RotateToken(id)
	if err != nil {
		// User exists (checked above): a non-nil error here is a save failure,
		// a genuine server fault — not a missing user.
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	writeSuccess(w, map[string]string{"token": tok})
}

// RevokeUserToken clears the registry user's subscription token, disabling
// /sub/{token} for it (subsequent requests 404). Registry-only. PROTECTED route.
func (h *Handler) RevokeUserToken(w http.ResponseWriter, r *http.Request) {
	if h.panelUsers == nil {
		writeError(w, http.StatusServiceUnavailable, "users not initialized")
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := h.panelUsers.Get(id); !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := h.panelUsers.RevokeToken(id); err != nil {
		// User exists (checked above): a non-nil error here is a save failure,
		// a genuine server fault — not a missing user.
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	writeSuccess(w, map[string]string{"message": "token revoked"})
}

// updateUserBody is the PATCH /api/users/{id} payload. Pointer fields are nil
// when the JSON key is absent (= leave unchanged); a present field with its zero
// value (enabled:false, expires_at:0) IS an explicit change.
type updateUserBody struct {
	Enabled   *bool  `json:"enabled"`
	ExpiresAt *int64 `json:"expires_at"`
}

// UpdateUser applies lifecycle changes (enabled / expires_at) to a registry user
// and enforces them IMMEDIATELY via the managed reject rule + reload — derived
// enforcement, no draft->apply (consistent with Rotate/Revoke). PROTECTED.
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	if h.panelUsers == nil {
		writeError(w, http.StatusServiceUnavailable, "users not initialized")
		return
	}
	id := chi.URLParam(r, "id")
	u, ok := h.panelUsers.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	var body updateUserBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Enabled != nil {
		u.Enabled = *body.Enabled
	}
	if body.ExpiresAt != nil {
		u.ExpiresAt = *body.ExpiresAt
	}
	if err := h.panelUsers.Put(&u); err != nil {
		// User existed (checked above): a non-nil error here is a save failure.
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	view := userView{
		ID: u.ID, Name: u.Name, Enabled: u.Enabled, ExpiresAt: u.ExpiresAt,
		Pending: false, Token: u.Token, TokenDisabled: u.TokenDisabled, Bindings: u.Bindings,
	}
	// Immediate enforcement + reload-on-change. sing-box takes the change here;
	// naive is dest's, and a lifecycle decision that did not reach dest leaves
	// the user still connecting over naive — said out loud rather than logged,
	// because the panel would otherwise report a clean success.
	if err := h.syncRejectRule(); err != nil {
		view.Warning = fmt.Sprintf("the change did not reach dest, so naive still uses the previous user list: %v", err)
	}
	writeSuccess(w, view)
}

// findActiveInbound returns the inbound with tag from an active config map.
func findActiveInbound(active map[string]interface{}, tag string) (map[string]interface{}, bool) {
	inbounds, ok := active["inbounds"].([]interface{})
	if !ok {
		return nil, false
	}
	for _, ib := range inbounds {
		obj, ok := ib.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := obj["tag"].(string); t == tag {
			return obj, true
		}
	}
	return nil, false
}

// findActiveUserByCredential returns the user map whose protocol credential
// equals cred within the given inbound.
func findActiveUserByCredential(inbound map[string]interface{}, cred string) (map[string]interface{}, bool) {
	protocol, _ := inbound["type"].(string)
	field := users.CredentialKey(protocol)
	if field == "" {
		return nil, false
	}
	arr, _ := inbound["users"].([]interface{})
	for _, u := range arr {
		um, ok := u.(map[string]interface{})
		if !ok {
			continue
		}
		if c, _ := um[field].(string); c == cred {
			return um, true
		}
	}
	return nil, false
}

// frontPort is the external port fronting loopback-bound inbounds, or 0 when
// nothing fronts them. Reading it here (rather than at each link site) keeps the
// nil-settings fallback in one place.
func (h *Handler) frontPort() int {
	if h.settings == nil {
		return 0
	}
	return h.settings.Get().Server.FrontPort
}
