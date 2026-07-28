package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"routebox/backend/internal/subscriptions"
	"routebox/backend/internal/util"
)

// On the live box a full disk came back from POST /api/subscriptions as
// 400 Bad Request with the text "no space left on device". The text was right
// and the code was not: the request was fine, the machine could not carry it
// out. The store's own refusals split in two, and only one half is a 400.
func TestSubscriptionErrorsGetTheirOwnStatus(t *testing.T) {
	cases := map[string]struct {
		err  error
		want int
	}{
		"caller sent a duplicate": {
			err:  fmt.Errorf("%w", subscriptionInvalidFixture()),
			want: http.StatusBadRequest,
		},
		"the file cannot be written": {
			err:  util.ReadOnlyError("/etc/routebox/subscriptions.toml"),
			want: http.StatusConflict,
		},
		"the disk is full": {
			err:  errors.New("write /etc/routebox/subscriptions.toml.tmp: no space left on device"),
			want: http.StatusInternalServerError,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeSubError(rr, c.err)
			if rr.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rr.Code, c.want, rr.Body.String())
			}
		})
	}
}

// A real refusal from the store, so the mapping above is pinned to what the
// store actually produces rather than to a hand-built error.
func subscriptionInvalidFixture() error {
	m := subscriptions.NewManager("")
	_, err := m.Add("", "", 0)
	return err
}
