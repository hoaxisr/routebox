package mtproto

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/9seconds/mtg/v2/mtglib"
	"github.com/9seconds/mtg/v2/mtglib/web"
)

// SecretKeyBytes is the length of an MTProto secret's key half.
const SecretKeyBytes = 16

// GenerateSecret returns a fresh key as 32 hex characters — the key half of a
// tg:// secret. The masking domain is folded in at link time by FakeTLSSecret.
func GenerateSecret() (string, error) {
	buf := make([]byte, SecretKeyBytes)

	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return hex.EncodeToString(buf), nil
}

// FakeTLSSecret assembles the secret a client actually pastes: the "ee" marker
// that selects FakeTLS mode, the key, then the masking domain hex-encoded.
//
// The domain being part of the secret is why it is panel-wide and why changing
// it invalidates every link already handed out.
func FakeTLSSecret(secretHex, maskingDomain string) string {
	return "ee" + secretHex + hex.EncodeToString([]byte(maskingDomain))
}

// CanIssueLink reports whether a shareable link can be built at all.
//
// Without a masking domain or a public host the result is a well-formed link
// that fails silently inside Telegram, which is worse than not offering one —
// so callers ask first and explain what is missing.
func CanIssueLink(maskingDomain, publicHost string) bool {
	return strings.TrimSpace(maskingDomain) != "" && strings.TrimSpace(publicHost) != ""
}

// proxyQuery builds the query both link forms share.
func proxyQuery(host string, port int, secretHex, maskingDomain string) string {
	v := url.Values{}
	v.Set("server", host)
	v.Set("port", strconv.Itoa(port))
	v.Set("secret", FakeTLSSecret(secretHex, maskingDomain))

	return v.Encode()
}

// ProxyLink is the tg://proxy form, which Telegram clients open directly.
func ProxyLink(host string, port int, secretHex, maskingDomain string) string {
	return "tg://proxy?" + proxyQuery(host, port, secretHex, maskingDomain)
}

// WebLink is the https://t.me/proxy form. It carries exactly the same query and
// exists because tg:// links do not survive every chat client and browser.
func WebLink(host string, port int, secretHex, maskingDomain string) string {
	return "https://t.me/proxy?" + proxyQuery(host, port, secretHex, maskingDomain)
}

// WebProxyLinks builds the tg://webproxy and https://t.me/webproxy links for a
// client. They carry the "dd" form of the secret and no port or masking
// domain: the WEB proxy is reached over HTTPS on the hostname itself.
func WebProxyLinks(hostname, secretHex string) (tg, webLink string, err error) {
	raw, err := hex.DecodeString(secretHex)
	if err != nil || len(raw) != SecretKeyBytes {
		return "", "", errors.New("malformed secret key")
	}

	var secret mtglib.Secret

	copy(secret.Key[:], raw)

	tg, webLink = web.Links(hostname, secret)

	return tg, webLink, nil
}

// CanIssueWebLink reports whether a WEB link is worth offering: it needs a
// hostname the WEB handler accepts, a WEB handler actually serving (the
// running proxy's Status().Web.Enabled, not the setting: a proxy that is
// stopped, or started with WEB skipped, serves none) and a front listening to
// hand it requests. Anything less yields a link that fails silently inside
// Telegram — a hostname the handler refuses means the proxy runs with WEB
// skipped (see Handler.MtprotoConfig), and a front whose bind failed at boot
// leaves the handler with no way in.
func CanIssueWebLink(hostname string, handlerServing, frontListening bool) bool {
	return web.ValidateHostname(hostname) == nil && handlerServing && frontListening
}
