package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"routebox/backend/internal/process"
)

// Переходить некуда — значит отказ, а не молчаливая смена пути "на всякий
// случай": хендлер двигает RouteBox на чужой файл, и делать это без
// прочитанного ExecStart нельзя. Ничего при этом не трогается — h.config здесь
// намеренно nil, и любая попытка что-то записать уронила бы тест.
func TestAdoptUnitConfigPathWithoutUnit(t *testing.T) {
	procMgr := process.NewManager()
	if procMgr.GetStatus().ServiceName != "" {
		t.Skip("a systemd unit is present on this host; this case needs the standalone one")
	}
	h := &Handler{process: procMgr}

	rec := httptest.NewRecorder()
	h.AdoptUnitConfigPath(rec, httptest.NewRequest(http.MethodPost, "/api/config/adopt-unit-path", nil))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Success {
		t.Error("response must not report success")
	}
	if !strings.Contains(resp.Error, "systemd") {
		t.Errorf("error %q should say there is no systemd unit to adopt a path from", resp.Error)
	}
}
