package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"routebox/backend/internal/process"
)

// Чинить нечего — значит 409, а не тихая запись drop-in "на всякий случай":
// этот хендлер правит файл вне RouteBox, и делать это без повода нельзя.
//
// Менеджер — отрезанный от машины: с process.NewManager() тест держался на
// счастливой случайности (пустой configPath → нет расхождения → 409 до любой
// записи). Стоит кому-нибудь задать здесь путь конфига — и на роутере с
// настоящим юнитом этот тест начнёт писать drop-in в боевой /etc/systemd/system.
func TestFixUnitConfigPathWithoutMismatch(t *testing.T) {
	h := &Handler{process: process.NewManagerForTest("amnezia-box", t.TempDir())}

	rec := httptest.NewRecorder()
	h.FixUnitConfigPath(rec, httptest.NewRequest(http.MethodPost, "/api/config/fix-unit", nil))

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
	if !strings.Contains(resp.Error, "mismatch") {
		t.Errorf("error %q should say there is no mismatch to fix", resp.Error)
	}
}
