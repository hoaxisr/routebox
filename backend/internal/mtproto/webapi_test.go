package mtproto

import (
	"testing"

	"github.com/9seconds/mtg/v2/essentials"
	"github.com/9seconds/mtg/v2/mtglib"
	"github.com/9seconds/mtg/v2/mtglib/web"
)

func TestForkExposesTheWebProxyAPI(t *testing.T) {
	var serve func(conn essentials.Conn, secretIdx int) error = (*mtglib.Proxy)(nil).ServeWebConn
	_ = serve
	var s mtglib.Secret
	copy(s.Key[:], []byte("0123456789abcdef"))
	if got := web.WebSecret(s); got != "dd30313233343536373839616263646566" {
		t.Fatalf("WebSecret = %q", got)
	}
	tg, tme := web.Links("proxy.example.com", s)
	if tg != "tg://webproxy?server=proxy.example.com&secret="+web.WebSecret(s) || tme == "" {
		t.Fatalf("Links = %q, %q", tg, tme)
	}
}
