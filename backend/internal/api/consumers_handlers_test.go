package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"routebox/backend/internal/consumers"
)

type stubSource struct{ rows []consumers.Row }

func (s stubSource) Kind() string                                   { return "awg" }
func (s stubSource) List(start, end int64) ([]consumers.Row, error) { return s.rows, nil }
func (s stubSource) Counters(context.Context) (map[string]consumers.Counter, error) {
	return nil, nil
}

func TestListConsumers(t *testing.T) {
	h := &Handler{}
	h.SetConsumers([]consumers.Source{stubSource{rows: []consumers.Row{{Kind: "awg", ID: "PK", Name: "alex"}}}})

	rec := httptest.NewRecorder()
	h.ListConsumers(rec, httptest.NewRequest("GET", "/api/consumers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Data struct {
			Range   string `json:"range"`
			Step    int64  `json:"step"`
			StartTs int64  `json:"start_ts"`
			EndTs   int64  `json:"end_ts"`
			Rows    []struct {
				Kind    string            `json:"kind"`
				ID      string            `json:"id"`
				History []json.RawMessage `json:"history"`
			} `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	d := body.Data
	if d.Range != "24h" || d.Step != 60 || d.EndTs-d.StartTs != 86400 || len(d.Rows) != 1 || d.Rows[0].History == nil {
		t.Fatalf("got %+v", d)
	}

	rec = httptest.NewRecorder()
	h.ListConsumers(rec, httptest.NewRequest("GET", "/api/consumers?range=year", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad range: status %d, want 400", rec.Code)
	}
}

func TestLiveConsumersShape(t *testing.T) {
	h := &Handler{}
	h.SetConsumers(nil)
	rec := httptest.NewRecorder()
	h.LiveConsumers(rec, httptest.NewRequest("GET", "/api/consumers/live", nil))
	var body struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusOK || string(body.Data["rows"]) != "[]" || string(body.Data["unavailable"]) != "{}" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
}

// A handler whose sources were never wired answers with the empty shape and
// stays unwired: a request goroutine must not initialise handler state.
func TestLiveConsumersUnwiredIsEmptyAndDoesNotInit(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	h.LiveConsumers(rec, httptest.NewRequest("GET", "/api/consumers/live", nil))
	var body struct {
		Data struct {
			Ts          int64             `json:"ts"`
			Rows        []json.RawMessage `json:"rows"`
			Unavailable map[string]string `json:"unavailable"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	d := body.Data
	if rec.Code != http.StatusOK || d.Ts != 0 || d.Rows == nil || len(d.Rows) != 0 || d.Unavailable == nil || len(d.Unavailable) != 0 {
		t.Fatalf("status %d body %s", rec.Code, rec.Body)
	}
	if h.live != nil {
		t.Fatal("LiveConsumers must not lazily initialise h.live")
	}
}
