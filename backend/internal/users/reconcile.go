package users

import "sort"

// Reconcile makes the registry mirror the active config. It is the ONLY function
// that writes the registry from the config side. Call it at startup and after a
// successful Apply (never on reads, never holding config.Manager's lock).
//
// IDENTITY = NAME. Server-users in active are grouped so the registry converges
// to AT MOST ONE PanelUser per non-empty name, holding ALL that name's bindings
// (one per inbound credential). This is the fix for panel-user fragmentation:
// "Add binding" used to mint a separate single-binding user per (inbound,
// credential), splitting one person across several subscriptions.
//
// Algorithm:
//  1. Group active server-users (ServerUsersOfConfig) by identity:
//     name != "" → identity = name; name == "" → identity = (tag, credential)
//     (nameless credentials stay per-credential — distinct nameless peers must
//     NEVER merge). Within a group, dedup bindings by (tag, credential).
//  2. For each desired group, pick a SURVIVOR among existing registry users
//     whose identity matches: the lexicographically smallest ID wins; the rest
//     (pre-existing fragments) are DELETED, their tokens dropped (the intended
//     consolidation). If none match, create a fresh collision-checked user.
//  3. The survivor KEEPS its ID, Token, TokenDisabled, Enabled, ExpiresAt; only
//     its Name and Bindings are (re)set from the desired group. Survivor lifecycle
//     wins; dropped fragments' state (incl. any revoke) is discarded with them.
//  4. Auto-mint a token for any resulting tokenless user with !TokenDisabled
//     (sticky revoke: a TokenDisabled user is NEVER auto-re-minted).
//  5. A1: an identity absent from active means its user is deleted; a binding
//     whose (tag, cred) vanished is dropped (it simply isn't in the desired set).
//  6. Idempotent: changed=true ONLY when the registry actually changes; no save
//     on a steady-state pass. Deterministic: group keys, survivor selection, and
//     bindings are sorted so repeated runs are stable.
func (m *Manager) Reconcile(active map[string]interface{}) (bool, error) {
	want := ServerUsersOfConfig(active)

	m.mu.Lock()
	defer m.mu.Unlock()

	// --- Step 1: build desired groups keyed by identity. ---
	// identity: name if non-empty, else a per-credential sentinel that cannot
	// collide with any real name ("\x00" + tag + "\x00" + credential).
	type group struct {
		identity string
		name     string
		bindings []Binding
		seen     map[[2]string]bool // dedup bindings by (tag, credential)
	}
	groups := map[string]*group{}
	var order []string // deterministic identity iteration order
	for _, cu := range want {
		id := cu.Name
		if id == "" {
			id = "\x00" + cu.InboundTag + "\x00" + cu.Credential
		}
		g, ok := groups[id]
		if !ok {
			g = &group{identity: id, name: cu.Name, seen: map[[2]string]bool{}}
			groups[id] = g
			order = append(order, id)
		}
		key := [2]string{cu.InboundTag, cu.Credential}
		if g.seen[key] {
			continue // duplicate (tag, credential) within the group: collapse
		}
		g.seen[key] = true
		g.bindings = append(g.bindings, Binding{
			InboundTag: cu.InboundTag, Credential: cu.Credential,
			Protocol: cu.Protocol, Name: cu.Name, Flow: cu.Flow,
		})
	}
	sort.Strings(order)
	// Global (tag, credential) dedup: a single credential physically belongs to
	// exactly ONE panel user. If a malformed active lists the same (tag, cred)
	// under two different names, the lexicographically-first group (order is
	// sorted) keeps the binding; the loser drops it. A group left with zero
	// bindings is discarded (no panel user materializes for it).
	claimed := map[[2]string]bool{}
	keptOrder := order[:0]
	for _, identity := range order {
		g := groups[identity]
		kept := g.bindings[:0]
		for _, b := range g.bindings {
			k := [2]string{b.InboundTag, b.Credential}
			if claimed[k] {
				continue
			}
			claimed[k] = true
			kept = append(kept, b)
		}
		g.bindings = kept
		if len(g.bindings) == 0 {
			delete(groups, identity)
			continue
		}
		keptOrder = append(keptOrder, identity)
	}
	order = keptOrder
	for _, g := range groups {
		sort.Slice(g.bindings, func(i, j int) bool {
			if g.bindings[i].InboundTag != g.bindings[j].InboundTag {
				return g.bindings[i].InboundTag < g.bindings[j].InboundTag
			}
			return g.bindings[i].Credential < g.bindings[j].Credential
		})
	}

	// --- Step 2: index existing registry users for survivor matching. ---
	// A registry user is a candidate for a desired group if it shares the group's
	// name (non-empty) OR shares any of the group's (tag, credential) bindings.
	// The binding-key fallback keeps a user's ID stable across a pure RENAME (the
	// credential is unchanged) and lets a nameless user be claimed by the group
	// that now owns its credential.
	byName := map[string][]string{}          // name → user IDs
	byBindingKey := map[[2]string][]string{} // (tag, cred) → user IDs
	for uid, u := range m.byID {
		if u.Name != "" {
			byName[u.Name] = append(byName[u.Name], uid)
		}
		for _, b := range u.Bindings {
			k := [2]string{b.InboundTag, b.Credential}
			byBindingKey[k] = append(byBindingKey[k], uid)
		}
	}

	changed := false
	survivors := map[string]bool{} // user IDs that must be kept

	// --- Step 3: converge each desired group to a single survivor. ---
	for _, identity := range order {
		g := groups[identity]

		// Collect distinct candidate IDs (by name and by any binding key), minus
		// any already claimed by an earlier group, sorted ascending so the
		// smallest ID is the deterministic survivor.
		candSet := map[string]bool{}
		if g.name != "" {
			for _, uid := range byName[g.name] {
				candSet[uid] = true
			}
		}
		for _, b := range g.bindings {
			for _, uid := range byBindingKey[[2]string{b.InboundTag, b.Credential}] {
				candSet[uid] = true
			}
		}
		cands := make([]string, 0, len(candSet))
		for uid := range candSet {
			if survivors[uid] {
				continue // already claimed as another group's survivor
			}
			if _, alive := m.byID[uid]; !alive {
				continue // already deleted as another group's dropped fragment
			}
			cands = append(cands, uid)
		}
		sort.Strings(cands)

		var survivor *PanelUser
		if len(cands) > 0 {
			survivor = m.byID[cands[0]]
			// Delete the other fragments (their tokens go with them).
			for _, uid := range cands[1:] {
				delete(m.byID, uid)
				changed = true
			}
		} else {
			id, err := m.freshIDLocked()
			if err != nil {
				return changed, err
			}
			survivor = &PanelUser{ID: id, Enabled: true}
			m.byID[id] = survivor
			changed = true
		}
		survivors[survivor.ID] = true

		// Set Name + Bindings from the desired group; detect change.
		if survivor.Name != g.name {
			survivor.Name = g.name
			changed = true
		}
		if !bindingsEqual(survivor.Bindings, g.bindings) {
			survivor.Bindings = g.bindings
			changed = true
		}
	}

	// --- Step 5 (A1): delete every user not chosen as a survivor this pass. ---
	for uid := range m.byID {
		if !survivors[uid] {
			delete(m.byID, uid)
			changed = true
		}
	}

	// --- Step 4: auto-mint tokens for resulting tokenless, non-revoked users. ---
	for _, u := range m.byID {
		if u.Token == "" && !u.TokenDisabled {
			tok, err := GenerateToken()
			if err != nil {
				return changed, err
			}
			u.Token = tok
			changed = true
		}
	}

	if changed {
		if err := m.saveLocked(); err != nil {
			return true, err
		}
	}
	return changed, nil
}

// freshIDLocked returns a collision-checked new ID. Caller holds m.mu.
func (m *Manager) freshIDLocked() (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	for _, exists := m.byID[id]; exists; _, exists = m.byID[id] {
		if id, err = newID(); err != nil {
			return "", err
		}
	}
	return id, nil
}

// bindingsEqual reports whether two binding slices hold the same SET of bindings,
// ignoring order. Order-insensitive so a steady-state survivor whose stored
// bindings already match the desired set (in any order) is left untouched —
// keeping reconcile idempotent and preserving the user's on-disk binding order.
func bindingsEqual(a, b []Binding) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[Binding]int, len(a))
	for _, x := range a {
		counts[x]++
	}
	for _, y := range b {
		counts[y]--
		if counts[y] < 0 {
			return false
		}
	}
	return true
}
