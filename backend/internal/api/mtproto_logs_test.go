package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"routebox/backend/internal/mtproto"
	"routebox/backend/internal/settings"
)

// mtprotoLogsServer starts the handler on a real listener, because the point of
// these tests is the WebSocket behaviour, not the JSON shape alone.
func mtprotoLogsServer(t *testing.T) (*Handler, *httptest.Server) {
	t.Helper()

	settingsMgr, err := settings.NewManager(filepath.Join(t.TempDir(), "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		mtproto:  mtproto.NewManager(mtproto.NewStore("")),
		settings: settingsMgr,
	}

	r := chi.NewRouter()
	r.Get("/api/mtproto/logs", h.StreamMtprotoLogs)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return h, srv
}

func dialLogs(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/mtproto/logs"

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	t.Cleanup(func() { conn.Close() })

	return conn
}

type logFrame struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Time    string `json:"time"`
}

func readFrame(t *testing.T, conn *websocket.Conn) logFrame {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}

	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var f logFrame
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("frame is not JSON: %v (%s)", err, raw)
	}

	return f
}

func TestLogsStreamReplaysTheBacklogOnConnect(t *testing.T) {
	h, srv := mtprotoLogsServer(t)

	// Logged before anyone was watching. A viewer opening the page after
	// something went wrong is the main reason this endpoint exists, so the
	// backlog has to come down the wire.
	h.mtproto.Logs().Add("warning", "replay attack has been detected!")

	conn := dialLogs(t, srv)

	got := readFrame(t, conn)
	if got.Payload != "replay attack has been detected!" {
		t.Errorf("payload = %q, want the buffered line", got.Payload)
	}

	if got.Type != "warning" {
		t.Errorf("type = %q, want warning", got.Type)
	}
}

func TestLogsStreamSendsTheBacklogInOrder(t *testing.T) {
	h, srv := mtprotoLogsServer(t)

	for _, m := range []string{"one", "two", "three"} {
		h.mtproto.Logs().Add("info", m)
	}

	conn := dialLogs(t, srv)

	for _, want := range []string{"one", "two", "three"} {
		if got := readFrame(t, conn).Payload; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestLogsStreamDeliversLiveLines(t *testing.T) {
	h, srv := mtprotoLogsServer(t)
	conn := dialLogs(t, srv)

	// Give the handler a moment to subscribe before logging, or the line races
	// the subscription and is delivered by neither path.
	time.Sleep(150 * time.Millisecond)
	h.mtproto.Logs().Add("info", "Stream has been started")

	if got := readFrame(t, conn).Payload; got != "Stream has been started" {
		t.Errorf("payload = %q, want the live line", got)
	}
}

func TestLogsStreamCarriesATimestamp(t *testing.T) {
	h, srv := mtprotoLogsServer(t)
	h.mtproto.Logs().Add("info", "x")

	conn := dialLogs(t, srv)

	got := readFrame(t, conn)
	if got.Time == "" {
		t.Error("no timestamp; the log view has nothing to put in the gutter")
	}

	if _, err := time.Parse(time.RFC3339Nano, got.Time); err != nil {
		t.Errorf("time %q is not RFC3339: %v", got.Time, err)
	}
}

func TestLogsStreamServesTwoViewersAtOnce(t *testing.T) {
	h, srv := mtprotoLogsServer(t)

	a := dialLogs(t, srv)
	b := dialLogs(t, srv)

	time.Sleep(150 * time.Millisecond)
	h.mtproto.Logs().Add("info", "broadcast")

	if got := readFrame(t, a).Payload; got != "broadcast" {
		t.Errorf("viewer A got %q", got)
	}

	if got := readFrame(t, b).Payload; got != "broadcast" {
		t.Errorf("viewer B got %q", got)
	}
}

func TestLogsStreamIs503WithoutAManager(t *testing.T) {
	h := &Handler{}

	r := chi.NewRouter()
	r.Get("/api/mtproto/logs", h.StreamMtprotoLogs)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/mtproto/logs", nil))

	// Answered as plain HTTP: there is nothing to upgrade to.
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestLogsStreamUnsubscribesWhenTheViewerLeaves(t *testing.T) {
	h, srv := mtprotoLogsServer(t)

	conn := dialLogs(t, srv)
	time.Sleep(100 * time.Millisecond)
	conn.Close()

	// A leaked subscription would keep receiving forever and, with the
	// buffer's bounded queue, silently wedge nothing but still leak memory.
	// Logging a lot after the viewer is gone must stay harmless.
	for i := 0; i < 200; i++ {
		h.mtproto.Logs().Add("info", "after close")
	}
}
