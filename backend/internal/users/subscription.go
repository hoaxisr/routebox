package users

import (
	"encoding/base64"
	"log"
	"strings"

	"routebox/backend/internal/serverlinks"
)

// BuildSubscription aggregates a panel user's bound nodes into a base64-encoded
// subscription body consumed by client apps (v2rayN/NekoBox/Streisand).
//
// PURE and HOST-AGNOSTIC: it performs no I/O and does NOT decide policy. The
// empty-host / "public host not configured" 503 is the HANDLER's responsibility
// (sub_handlers.go); here an empty host simply makes serverlinks.BuildShareLink
// error per binding, so every binding is skipped and the result is base64 of "".
//
// SOURCE = ACTIVE config (clients must only see running nodes; never the draft).
// For each binding it locates the inbound by tag in active, finds the user inside
// it by (CredentialKey(protocol), credential), and renders a client share-link.
// Bindings whose inbound or credential is absent from active are SKIPPED (a stale
// binding must never make the PUBLIC endpoint error). Links are "\n"-joined; the
// result is base64.StdEncoding (the de-facto subscription wire format). A user
// with no resolvable nodes yields base64 of "" (i.e. ""), not an error.
func BuildSubscription(user *PanelUser, active map[string]interface{}, host string) (string, error) {
	var links []string
	if user != nil {
		// Index active inbounds by tag for O(1) lookup.
		byTag := map[string]map[string]interface{}{}
		if inbounds, ok := active["inbounds"].([]interface{}); ok {
			for _, ib := range inbounds {
				if obj, ok := ib.(map[string]interface{}); ok {
					if tag, _ := obj["tag"].(string); tag != "" {
						byTag[tag] = obj
					}
				}
			}
		}

		for _, b := range user.Bindings {
			inbound, ok := byTag[b.InboundTag]
			if !ok {
				continue // inbound gone from active: skip, don't fail
			}
			key := CredentialKey(b.Protocol)
			if key == "" {
				continue // non-server / unknown protocol
			}
			userMap, found := findUserByCredential(inbound, key, b.Credential)
			if !found {
				continue // credential not present in active (e.g. unapplied): skip
			}
			link, err := serverlinks.BuildShareLink(inbound, userMap, host)
			if err != nil {
				// A resolvable inbound+credential that still can't render a link is
				// the diagnosable silent-skip: an unsupported inbound type
				// (BuildShareLink's default branch) or a malformed inbound. Log it
				// (tag+type, never the credential) so a dropped binding is
				// discoverable. host=="" is NOT this case — it is the handler's
				// "public host not configured" policy where every binding skips by
				// design, so stay silent there to avoid per-request log noise.
				if host != "" {
					typ, _ := inbound["type"].(string)
					log.Printf("users: subscription skipped binding tag=%q type=%q: %v", b.InboundTag, typ, err)
				}
				continue // empty host / malformed inbound: skip, don't fail
			}
			links = append(links, link)
		}
	}

	joined := strings.Join(links, "\n")
	return base64.StdEncoding.EncodeToString([]byte(joined)), nil
}

// findUserByCredential returns the user map inside inbound whose field `key`
// equals cred. Self-contained so subscription.go imports neither api nor config.
func findUserByCredential(inbound map[string]interface{}, key, cred string) (map[string]interface{}, bool) {
	if cred == "" {
		// Mirror extract.go's blank-credential skip: an empty binding credential
		// must never match a user merely missing the key field ("" == "").
		return nil, false
	}
	arr, _ := inbound["users"].([]interface{})
	for _, u := range arr {
		um, ok := u.(map[string]interface{})
		if !ok {
			continue
		}
		if c, _ := um[key].(string); c == cred {
			return um, true
		}
	}
	return nil, false
}
