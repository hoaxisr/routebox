package users

// Reconcile makes the registry mirror the active config. It is the ONLY function
// that writes the registry from the config side. Call it at startup and after a
// successful Apply (never on reads, never holding config.Manager's lock).
//
// Algorithm:
//  1. Collect every server-inbound user in active.
//  2. Index existing registry bindings by (tag, credential). For each active
//     user: if a binding with that key exists, refresh its cached Name/Flow/
//     Protocol; otherwise import it as a new single-binding PanelUser with a
//     fresh ID — AND register the new binding into the index so a duplicate
//     (tag, credential) later in the same active collapses to that one entry.
//  3. A1: drop any binding whose key is absent from active. Panel users left
//     with zero bindings are deleted.
//  4. Persist + return changed=true iff anything changed. Idempotent.
func (m *Manager) Reconcile(active map[string]interface{}) (bool, error) {
	want := ServerUsersOfConfig(active)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Index registry bindings by (tag, credential) → owning user + slot.
	type ref struct {
		user *PanelUser
		idx  int
	}
	bindingIndex := map[[2]string]ref{}
	for _, u := range m.byID {
		for i := range u.Bindings {
			b := u.Bindings[i]
			bindingIndex[[2]string{b.InboundTag, b.Credential}] = ref{user: u, idx: i}
		}
	}

	changed := false
	seen := map[[2]string]bool{} // active keys matched/created this pass

	// Pass 1: refresh existing bindings; import new ones.
	for _, cu := range want {
		k := [2]string{cu.InboundTag, cu.Credential}
		seen[k] = true
		if r, ok := bindingIndex[k]; ok {
			b := &r.user.Bindings[r.idx]
			if b.Name != cu.Name || b.Flow != cu.Flow || b.Protocol != cu.Protocol {
				b.Name, b.Flow, b.Protocol = cu.Name, cu.Flow, cu.Protocol
				changed = true
			}
			continue
		}
		// Import: new single-binding user with a fresh, collision-checked ID.
		id, err := newID()
		if err != nil {
			return changed, err
		}
		for _, exists := m.byID[id]; exists; _, exists = m.byID[id] {
			if id, err = newID(); err != nil {
				return changed, err
			}
		}
		nu := &PanelUser{
			ID:      id,
			Name:    cu.Name,
			Enabled: true,
			Bindings: []Binding{{
				InboundTag: cu.InboundTag, Credential: cu.Credential,
				Protocol: cu.Protocol, Name: cu.Name, Flow: cu.Flow,
			}},
		}
		m.byID[id] = nu
		// MUST-FIX 5: register within-pass import so a duplicate (tag,credential)
		// later in the same active collapses to this entry instead of re-importing.
		bindingIndex[k] = ref{user: nu, idx: 0}
		changed = true
	}

	// Pass 2 (A1): drop vanished bindings; delete now-empty users.
	for id, u := range m.byID {
		kept := u.Bindings[:0]
		for _, b := range u.Bindings {
			if seen[[2]string{b.InboundTag, b.Credential}] {
				kept = append(kept, b)
			} else {
				changed = true // A1: binding's credential vanished from active
			}
		}
		u.Bindings = kept
		if len(u.Bindings) == 0 {
			delete(m.byID, id)
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
