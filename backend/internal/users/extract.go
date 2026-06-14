package users

// ConfigUser is one server-inbound user extracted from a sing-box config inbound.
// It is the join surface between the active config and the registry.
type ConfigUser struct {
	InboundTag string
	Protocol   string // "vless" | "naive" | "hysteria2"
	Credential string // uuid (vless) | username (naive) | password (hysteria2)
	Name       string // display name (best-effort)
	Flow       string // vless only
}

// credentialKey maps a protocol to the user field that is its stable credential.
// This is the single source of the protocol→credential mapping; reconcile, the
// API draft helpers, and the share-link resolver all derive their field name
// from here (the validator keeps its own tiny inline copy to avoid importing
// users into config).
func credentialKey(protocol string) string {
	switch protocol {
	case "vless":
		return "uuid"
	case "naive":
		return "username"
	case "hysteria2":
		return "password"
	}
	return ""
}

// ServerInboundUsers extracts the users of a server inbound (vless/naive/hysteria2)
// as ConfigUsers. Non-server inbounds (tun/mixed/...) and users with a blank
// credential yield nothing. Pure: no I/O, no mutation. Reused by Reconcile and
// the API handlers.
func ServerInboundUsers(inbound map[string]interface{}) []ConfigUser {
	protocol, _ := inbound["type"].(string)
	key := credentialKey(protocol)
	if key == "" {
		return nil
	}
	tag, _ := inbound["tag"].(string)
	users, _ := inbound["users"].([]interface{})
	var out []ConfigUser
	for _, u := range users {
		um, ok := u.(map[string]interface{})
		if !ok {
			continue
		}
		cred, _ := um[key].(string)
		if cred == "" {
			continue
		}
		name, _ := um["name"].(string)
		if name == "" {
			name, _ = um["username"].(string) // naive has no name; use username
		}
		flow, _ := um["flow"].(string)
		out = append(out, ConfigUser{
			InboundTag: tag, Protocol: protocol, Credential: cred, Name: name, Flow: flow,
		})
	}
	return out
}

// ServerUsersOfConfig extracts every server user from a full config's inbounds.
func ServerUsersOfConfig(config map[string]interface{}) []ConfigUser {
	inbounds, _ := config["inbounds"].([]interface{})
	var out []ConfigUser
	for _, ib := range inbounds {
		if obj, ok := ib.(map[string]interface{}); ok {
			out = append(out, ServerInboundUsers(obj)...)
		}
	}
	return out
}
