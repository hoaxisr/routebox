package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"routebox/backend/internal/process"
)

// Менеджер здесь строится ТОЛЬКО через шов: process.NewManager() детектит юнит
// этой машины, и на роутере, где RouteBox держит настоящий drop-in, обычный
// `go test ./backend/...` под root удалил бы боевой файл и перечитал systemd.
// Соседний fix_unit_test.go защищён структурно (нет расхождения — нет записи),
// у снятия такой защиты нет: оно идёт прямо в os.Remove.
func newDropInHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	root := t.TempDir()
	return &Handler{process: process.NewManagerForTest("amnezia-box", root)}, root
}

// testDropInPath — путь нашего drop-in внутри подменённого каталога юнитов.
// Имя продублировано осознанно: файл называется и в интерфейсе, и в
// changelog'е, так что молчаливая смена имени обязана ронять тест.
func testDropInPath(root string) string {
	return filepath.Join(root, "amnezia-box.service.d", "10-routebox-config.conf")
}

// Снимать нечего — это состояние, а не поломка: 409 и внятный текст. 500 здесь
// выглядел бы как «кнопка сломалась», хотя drop-in просто не установлен.
func TestRemoveUnitConfigDropInWithoutDropIn(t *testing.T) {
	h, _ := newDropInHandler(t)

	rec := httptest.NewRecorder()
	h.RemoveUnitConfigDropIn(rec, httptest.NewRequest(http.MethodDelete, "/api/config/unit-dropin", nil))

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
	if !strings.Contains(strings.ToLower(resp.Error), "drop-in") {
		t.Errorf("error %q should say there is no drop-in to remove", resp.Error)
	}
}

// Установленный drop-in снимается, и файла после этого нет — на подменённом
// каталоге, настоящий /etc в тестах не участвует.
func TestRemoveUnitConfigDropInDeletesTheFile(t *testing.T) {
	h, root := newDropInHandler(t)
	path := testDropInPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "[Service]\nExecStart=\nExecStart=/usr/bin/box run -c /etc/amnezia-box/config.json\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seed drop-in: %v", err)
	}

	rec := httptest.NewRecorder()
	h.RemoveUnitConfigDropIn(rec, httptest.NewRequest(http.MethodDelete, "/api/config/unit-dropin", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the drop-in file must be gone, stat err = %v", err)
	}
}
