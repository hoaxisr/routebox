package api

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"routebox/backend/internal/mtproto"
	"routebox/backend/internal/settings"
	"routebox/backend/internal/traffic"
)

// newMtprotoTrafficHandler wires a real traffic store, since the point of these
// tests is that the rows the flusher writes are the rows the API reads back.
func newMtprotoTrafficHandler(t *testing.T) (*Handler, http.Handler, *traffic.Store) {
	t.Helper()

	settingsMgr, err := settings.NewManager(filepath.Join(t.TempDir(), "routebox.toml"))
	if err != nil {
		t.Fatal(err)
	}

	store, err := traffic.OpenStore(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { store.Close() })

	h := &Handler{
		mtproto:  mtproto.NewManager(mtproto.NewStore("")),
		settings: settingsMgr,
		traffic:  store,
	}

	r := chi.NewRouter()
	// Registered exactly as main.go does: the static segment has to win over
	// /clients/{name}, or this route is never reached.
	r.Route("/api/mtproto", func(r chi.Router) {
		r.Get("/clients/traffic", h.GetMtprotoClientsTraffic)
		r.Get("/clients/{name}/link", h.GetMtprotoClientLink)
	})

	return h, r, store
}

type mtprotoTrafficRowJSON struct {
	Name     string `json:"name"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	History  []struct {
		Ts       int64 `json:"ts"`
		Upload   int64 `json:"upload"`
		Download int64 `json:"download"`
	} `json:"history"`
}

func TestClientsTrafficReadsBackWhatTheFlusherWrote(t *testing.T) {
	h, r, store := newMtprotoTrafficHandler(t)

	if err := h.mtproto.Store().Put(mtproto.Client{Name: "phone", Secret: testSecret, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	// Written under the same key mtproto.TrafficKey produces, which is the
	// contract this endpoint exists to honour.
	bucket := time.Now().Add(-2 * time.Minute).Truncate(time.Minute).Unix()
	if err := store.UpsertUser(bucket, mtproto.TrafficKey("phone"), 100, 250); err != nil {
		t.Fatal(err)
	}

	var rows []mtprotoTrafficRowJSON
	decodeMtprotoData(t, mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients/traffic", ""), &rows)

	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want 1", rows)
	}

	if rows[0].Name != "phone" || rows[0].Upload != 100 || rows[0].Download != 250 {
		t.Errorf("got %+v, want phone 100 up / 250 down", rows[0])
	}

	if len(rows[0].History) == 0 {
		t.Error("history is empty; the roster sparkline would have nothing to draw")
	}
}

func TestClientsTrafficReportsEveryClientEvenWithoutRows(t *testing.T) {
	h, r, _ := newMtprotoTrafficHandler(t)

	for _, n := range []string{"phone", "laptop"} {
		if err := h.mtproto.Store().Put(mtproto.Client{Name: n, Secret: testSecret, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}

	var rows []mtprotoTrafficRowJSON
	decodeMtprotoData(t, mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients/traffic", ""), &rows)

	// A client that has never connected must still appear, at zero — dropping
	// it would make the roster and the traffic view disagree about who exists.
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want both clients", rows)
	}

	for _, row := range rows {
		if row.Upload != 0 || row.Download != 0 {
			t.Errorf("%s = %+v, want zeroes", row.Name, row)
		}

		if row.History == nil {
			t.Errorf("%s history is null, want an empty array", row.Name)
		}
	}
}

func TestClientsTrafficIsSortedByName(t *testing.T) {
	h, r, _ := newMtprotoTrafficHandler(t)

	for _, n := range []string{"zoe", "adam"} {
		if err := h.mtproto.Store().Put(mtproto.Client{Name: n, Secret: testSecret, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}

	var rows []mtprotoTrafficRowJSON
	decodeMtprotoData(t, mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients/traffic", ""), &rows)

	if len(rows) != 2 || rows[0].Name != "adam" {
		t.Errorf("rows = %+v, want them sorted like the roster", rows)
	}
}

func TestClientsTrafficDoesNotLeakOtherClientsBytes(t *testing.T) {
	h, r, store := newMtprotoTrafficHandler(t)

	for _, n := range []string{"phone", "laptop"} {
		if err := h.mtproto.Store().Put(mtproto.Client{Name: n, Secret: testSecret, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}

	bucket := time.Now().Truncate(time.Minute).Unix()
	if err := store.UpsertUser(bucket, mtproto.TrafficKey("phone"), 10, 20); err != nil {
		t.Fatal(err)
	}
	// A panel user whose name collides with a client's must not be counted:
	// the prefix is what keeps the two namespaces apart.
	if err := store.UpsertUser(bucket, "phone", 9000, 9000); err != nil {
		t.Fatal(err)
	}

	var rows []mtprotoTrafficRowJSON
	decodeMtprotoData(t, mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients/traffic", ""), &rows)

	byName := map[string]mtprotoTrafficRowJSON{}
	for _, row := range rows {
		byName[row.Name] = row
	}

	if byName["phone"].Upload != 10 || byName["phone"].Download != 20 {
		t.Errorf("phone = %+v, want only its own 10/20 — the panel user's bytes leaked in", byName["phone"])
	}

	if byName["laptop"].Upload != 0 {
		t.Errorf("laptop = %+v, want zero", byName["laptop"])
	}
}

func TestClientsTrafficHonoursTheRange(t *testing.T) {
	h, r, store := newMtprotoTrafficHandler(t)

	if err := h.mtproto.Store().Put(mtproto.Client{Name: "phone", Secret: testSecret, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	recent := now.Add(-30 * time.Minute).Truncate(time.Minute).Unix()
	old := now.Add(-72 * time.Hour).Truncate(time.Minute).Unix()

	if err := store.UpsertUser(recent, mtproto.TrafficKey("phone"), 5, 5); err != nil {
		t.Fatal(err)
	}

	if err := store.UpsertUser(old, mtproto.TrafficKey("phone"), 700, 700); err != nil {
		t.Fatal(err)
	}

	var rows []mtprotoTrafficRowJSON
	decodeMtprotoData(t, mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients/traffic?range=1h", ""), &rows)

	if rows[0].Upload != 5 {
		t.Errorf("upload = %d, want only the last hour's 5", rows[0].Upload)
	}
}

func TestClientsTrafficWithoutAStoreReportsZeroes(t *testing.T) {
	h, r, _ := newMtprotoTrafficHandler(t)
	h.traffic = nil

	if err := h.mtproto.Store().Put(mtproto.Client{Name: "phone", Secret: testSecret, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	// History is off on this install; report the roster at zero rather than
	// failing the page, matching how the AWG and user endpoints behave.
	var rows []mtprotoTrafficRowJSON
	decodeMtprotoData(t, mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients/traffic", ""), &rows)

	if len(rows) != 1 || rows[0].Upload != 0 {
		t.Errorf("rows = %+v, want the client at zero", rows)
	}
}

func TestClientsTrafficIs503WithoutAManager(t *testing.T) {
	h := &Handler{}

	r := chi.NewRouter()
	r.Get("/api/mtproto/clients/traffic", h.GetMtprotoClientsTraffic)

	if rec := mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients/traffic", ""); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestTrafficRouteWinsOverTheClientNameRoute(t *testing.T) {
	h, r, _ := newMtprotoTrafficHandler(t)

	if err := h.mtproto.Store().Put(mtproto.Client{Name: "phone", Secret: testSecret, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	// "traffic" is a legal client name, so precedence is what stops
	// /clients/traffic from being read as a client called "traffic".
	rec := mtprotoReq(t, r, http.MethodGet, "/api/mtproto/clients/traffic", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var rows []mtprotoTrafficRowJSON
	decodeMtprotoData(t, rec, &rows)

	if len(rows) != 1 || rows[0].Name != "phone" {
		t.Errorf("rows = %+v, want the traffic listing, not a client lookup", rows)
	}
}
