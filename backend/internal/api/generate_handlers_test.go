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
