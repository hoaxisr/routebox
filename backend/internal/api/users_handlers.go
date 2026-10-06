package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

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
	// QuotaBytes is the one-shot limit on UsedRx+UsedTx (0 = no limit) and
	// Used*/UsedResetAt the cumulative counters behind it (#95). Upload/Download
	// above are the SQLite history totals for the picked range — a different
	// number with a different lifetime, so both travel.
	QuotaBytes  int64 `json:"quota_bytes"`
	UsedRx      int64 `json:"used_rx"`
	UsedTx      int64 `json:"used_tx"`
	UsedResetAt int64 `json:"used_reset_at"`
	// SuspendReason is manual/quota/expired, or "" while the user is in service.
	// DERIVED at response time from the stored numbers (never persisted), so the
	// row cannot disagree with the reject rule the same numbers produce.
	SuspendReason string `json:"suspend_reason"`
	// Warning is set when the change was made but did not reach everywhere it
	// had to — dest, which serves naive on its own, or a reject rule the sweep
	// could not put in force (a config draft is pending, the config is read-only,
	// the reload failed). Omitted when empty, so every other answer keeps its
	// shape.
	Warning string `json:"warning,omitempty"`
}

// newUserView projects a registry user onto the wire shape, deriving
// SuspendReason at now. The ONE place that projection lives: list, PATCH and the
// counter reset all answer with the same fields.
func newUserView(u users.PanelUser, now int64) userView {
	return userView{
		ID: u.ID, Name: u.Name, Enabled: u.Enabled, ExpiresAt: u.ExpiresAt,
		Pending: false, Token: u.Token, TokenDisabled: u.TokenDisabled, Bindings: u.Bindings,
		QuotaBytes: u.QuotaBytes, UsedRx: u.UsedRx, UsedTx: u.UsedTx, UsedResetAt: u.UsedResetAt,
		SuspendReason: string(users.SuspendReason(u, now)),
	}
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
	now := time.Now().Unix()

	for _, u := range h.panelUsers.List() {
		for _, b := range u.Bindings {
			registered[b.InboundTag+"\x00"+b.Credential] = true
		}
		var up, down int64
		if h.traffic != nil {
			for _, name := range u.TrafficNames() {
				nu, nd, err := h.traffic.QueryUserTotals(0, 1<<62, name)
				if err == nil {
					up += nu
					down += nd
				}
			}
		}
		view := newUserView(u, now)
		view.Upload, view.Download = up, down
		views = append(views, view)
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
		if err := h.traffic.DeleteUsers(u.TrafficNames()); err != nil {
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
	// QuotaBytes is the one-shot traffic limit; 0 clears it. Negative is
	// rejected (400) rather than read as "no limit", so a UI bug cannot silently
	// turn a limit off. Never resets the counters — that is the reset route.
	QuotaBytes *int64 `json:"quota_bytes"`
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
	if body.QuotaBytes != nil {
		if *body.QuotaBytes < 0 {
			writeError(w, http.StatusBadRequest, "quota_bytes must be >= 0 (0 = no limit)")
			return
		}
		u.QuotaBytes = *body.QuotaBytes
	}
	if err := h.panelUsers.Put(&u); err != nil {
		// User existed (checked above): a non-nil error here is a save failure.
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	// Answer from the registry, not from the copy this handler mutated: Put keeps
	// the stored Used* counters (they belong to the sampler alone), so the copy
	// is only as fresh as the Get above. Today the two agree — the Get already
	// carried the counters — so this is a cheap guarantee about where the numbers
	// come from, not a fix for a bug a test could reproduce.
	if stored, ok := h.panelUsers.Get(id); ok {
		u = stored
	}
	view := newUserView(u, time.Now().Unix())
	// Immediate enforcement + reload-on-change. Whatever kept it from being
	// immediate — a pending draft, a read-only config, a dest that refused —
	// travels in the answer: the panel would otherwise report a clean success
	// over a user who is still connecting.
	view.Warning = enforcementWarning(h.syncRejectRule())
	writeSuccess(w, view)
}

// ResetUserTraffic zeroes the user's cumulative quota counters (used_rx/used_tx),
// stamps used_reset_at and re-admits the user immediately when the quota was what
// suspended them (spec Q9/Q19) — same derived enforcement as the enabled toggle,
// no draft->apply. The limit itself is kept; raising it is the other way back in
// service. Does NOT touch the SQLite traffic history (GET /users/{id}/traffic):
// this counter is the quota's own, stored beside the user. PROTECTED.
func (h *Handler) ResetUserTraffic(w http.ResponseWriter, r *http.Request) {
	if h.panelUsers == nil {
		writeError(w, http.StatusServiceUnavailable, "users not initialized")
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := h.panelUsers.Get(id); !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	now := time.Now().Unix()
	if err := h.panelUsers.ResetUsage(id, now); err != nil {
		// User exists (checked above): a non-nil error here is a save failure.
		writeConfigError(w, http.StatusInternalServerError, err)
		return
	}
	u, ok := h.panelUsers.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	view := newUserView(u, now)
	// Same as UpdateUser: whatever stopped the re-admission short of the running
	// process is said out loud instead of only logged.
	view.Warning = enforcementWarning(h.syncRejectRule())
	writeSuccess(w, view)
}

// enforcementWarning turns syncRejectRule's two answers into the one sentence a
// user response carries, or "" when the change is live everywhere. Both halves
// travel when both went wrong — they are different repairs (apply the draft /
// fix dest), and picking one would hide the other.
func enforcementWarning(deferred string, err error) string {
	var parts []string
	if deferred != "" {
		parts = append(parts, "the change is saved but not in force yet: "+deferred)
	}
	if err != nil {
		parts = append(parts, fmt.Sprintf("the change did not reach dest, so naive still uses the previous user list: %v", err))
	}
	return strings.Join(parts, "; ")
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
