package process

import (
	"strings"
	"testing"
	"time"
)

func TestReloadOutcome(t *testing.T) {
	type snap struct {
		running bool
		pid     int
	}
	cases := []struct {
		name    string
		seq     []snap
		wantErr string
	}{
		{"alive same pid", []snap{{true, 10}, {true, 10}, {true, 10}}, ""},
		{"exited", []snap{{true, 10}, {false, 0}}, "exited"},
		{"respawned by supervisor", []snap{{true, 10}, {true, 11}}, "restarted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := 0
			status := func() (bool, int) {
				s := tc.seq[i]
				if i < len(tc.seq)-1 {
					i++
				}
				return s.running, s.pid
			}
			err := reloadOutcome(10, status, 5*time.Millisecond, time.Millisecond)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
