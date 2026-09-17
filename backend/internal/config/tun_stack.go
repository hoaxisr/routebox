package config

// stripTunStack removes the "stack" option from every TUN inbound, reporting
// whether anything changed.
//
// sing-box 1.15 gave sing-tun its own TCP/IP stack: it is what runs when
// "stack" is absent, it beats every previous implementation on throughput,
// CPU and memory, and the option itself is deprecated (removed in 1.17).
// Our amnezia-box fork is built without with_gvisor on top of that, so a
// leftover "gvisor" or "mixed" aborts the start outright. Dropping the key is
// the migration and the fix in one.
func stripTunStack(config map[string]interface{}) bool {
	inbounds, _ := config["inbounds"].([]interface{})
	changed := false
	for _, ib := range inbounds {
		inbound, ok := ib.(map[string]interface{})
		if !ok || inbound["type"] != "tun" {
			continue
		}
		if _, has := inbound["stack"]; has {
			delete(inbound, "stack")
			changed = true
		}
	}
	return changed
}
