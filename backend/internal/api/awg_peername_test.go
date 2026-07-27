package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCreateAWGPeerKeepsUnicodeName: POST /api/awg/peers {"name":"Ноутбук"} must
// come back as "Ноутбук" — the bug stored it as "name", so a user who named three
// devices in Russian ended up with three peers called "name".
func TestCreateAWGPeerKeepsUnicodeName(t *testing.T) {
	_, r := newAWGTestHandler(t)

	pubs := map[string]string{}
	for _, name := range []string{"Ноутбук", "Телефон Ани"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/awg/peers",
			strings.NewReader(`{"name":`+jsonString(name)+`}`)))
		if rec.Code != http.StatusOK {
			t.Fatalf("POST %q = %d; body=%q", name, rec.Code, rec.Body.String())
		}
		var resp struct {
			Data struct {
				Name      string `json:"name"`
				PublicKey string `json:"public_key"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v (%s)", err, rec.Body.String())
		}
		if resp.Data.Name != name {
			t.Errorf("created peer name = %q; want %q", resp.Data.Name, name)
		}
		pubs[name] = resp.Data.PublicKey
	}

	// And the list keeps them distinct.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/awg/peers", nil))
	body := rec.Body.String()
	for name := range pubs {
		if !strings.Contains(body, name) {
			t.Errorf("GET /peers missing %q:\n%s", name, body)
		}
	}
}

// TestCreateAWGPeerRejectsControlChars: a name that could forge a .conf directive
// or split a header is a 400, not a silent rewrite.
func TestCreateAWGPeerRejectsControlChars(t *testing.T) {
	_, r := newAWGTestHandler(t)
	for _, body := range []string{
		`{"name":"a\nPublicKey = ATTACKER"}`,
		`{"name":"   "}`,
		`{"name":""}`,
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/awg/peers", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s = %d; want 400 (body=%q)", body, rec.Code, rec.Body.String())
		}
	}
}

// TestAWGPeerConfigUnicodeFilename: the download keeps an ASCII filename for old
// clients AND an RFC 6266 filename* so a browser saves "Ноутбук.conf".
func TestAWGPeerConfigUnicodeFilename(t *testing.T) {
	_, r := newAWGTestHandler(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/awg/peers",
		strings.NewReader(`{"name":`+jsonString("Ноутбук")+`}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d; body=%q", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			PublicKey string `json:"public_key"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/awg/peers/"+urlEscapePub(resp.Data.PublicKey)+"/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("config = %d; body=%q", rec.Code, rec.Body.String())
	}
	cd := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, `attachment; filename="`) {
		t.Fatalf("Content-Disposition = %q; want an ASCII filename first", cd)
	}
	// The ASCII part must stay ASCII (header values are bytes on the wire).
	for i := 0; i < len(cd); i++ {
		if cd[i] > 0x7e || cd[i] < 0x20 {
			t.Fatalf("Content-Disposition has a non-ASCII byte: %q", cd)
		}
	}
	if !strings.Contains(cd, `filename*=UTF-8''`) {
		t.Fatalf("Content-Disposition lacks filename*: %q", cd)
	}
	if !strings.Contains(cd, "%D0%9D%D0%BE%D1%83%D1%82%D0%B1%D1%83%D0%BA.conf") {
		t.Fatalf("filename* is not the percent-encoded name: %q", cd)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// urlEscapePub makes a base64 public key safe as a path segment ("/" and "+").
func urlEscapePub(pub string) string {
	return strings.NewReplacer("/", "%2F", "+", "%2B", "=", "%3D").Replace(pub)
}
