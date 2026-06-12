package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func decodeDataBytes(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var env struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, body)
	}
	if !env.Success {
		t.Fatalf("expected success, got %s", body)
	}
	return env.Data
}

func TestGenerateUUID(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	h.GenerateUUID(rec, httptest.NewRequest(http.MethodPost, "/api/generate/uuid", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	data := decodeDataBytes(t, rec.Body.Bytes())
	uuid, _ := data["uuid"].(string)
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !re.MatchString(uuid) {
		t.Fatalf("not a v4 uuid: %q", uuid)
	}
}

func TestGeneratePassword(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	h.GeneratePassword(rec, httptest.NewRequest(http.MethodPost, "/api/generate/password", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	data := decodeDataBytes(t, rec.Body.Bytes())
	pw, _ := data["password"].(string)
	if len(pw) < 20 { // 16 bytes base64url ≈ 22 chars
		t.Fatalf("password too short: %q", pw)
	}
}

func TestParseRealityKeypair(t *testing.T) {
	out := "PrivateKey: SN5HcFLrdjYEYbYYowow0k8zRF5m2uvX6_vcun25p2s\nPublicKey: onu9CnSwBGKrgJGKK_WkggznnOwUuvNjTHw4nBlSdwU\n"
	priv, pub, err := parseRealityKeypair(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if priv != "SN5HcFLrdjYEYbYYowow0k8zRF5m2uvX6_vcun25p2s" {
		t.Fatalf("priv mismatch: %q", priv)
	}
	if pub != "onu9CnSwBGKrgJGKK_WkggznnOwUuvNjTHw4nBlSdwU" {
		t.Fatalf("pub mismatch: %q", pub)
	}
}

func TestParseRealityKeypairRejectsEmpty(t *testing.T) {
	if _, _, err := parseRealityKeypair("some unrelated output\n"); err == nil {
		t.Fatal("expected error when keys absent")
	}
}

func TestParseRealityKeypairPartial(t *testing.T) {
	cases := []struct {
		name string
		out  string
	}{
		{"only private", "PrivateKey: SN5HcFLrdjYEYbYYowow0k8zRF5m2uvX6_vcun25p2s\n"},
		{"only public", "PublicKey: onu9CnSwBGKrgJGKK_WkggznnOwUuvNjTHw4nBlSdwU\n"},
		{"empty value", "PrivateKey:\nPublicKey:\n"},
	}
	for _, c := range cases {
		if _, _, err := parseRealityKeypair(c.out); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

func TestParseRealityKeypairCRLF(t *testing.T) {
	out := "PrivateKey: SN5HcFLrdjYEYbYYowow0k8zRF5m2uvX6_vcun25p2s\r\nPublicKey: onu9CnSwBGKrgJGKK_WkggznnOwUuvNjTHw4nBlSdwU\r\n"
	priv, pub, err := parseRealityKeypair(out)
	if err != nil || priv == "" || pub == "" {
		t.Fatalf("CRLF parse failed: priv=%q pub=%q err=%v", priv, pub, err)
	}
}
