package process

import "testing"

func TestParseSupportsAWGServer(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"awg2.1", "sing-box version 1.13.13-awg2.1\nEnvironment: go1.25\n", true},
		{"awg2.0 too old", "sing-box version 1.13.13-awg2.0\n", false},
		{"awg3.0 newer", "sing-box version 1.13.13-awg3.0\n", true},
		{"plain upstream", "sing-box version 1.13.13\n", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseSupportsAWGServer(c.in); got != c.want {
				t.Fatalf("parseSupportsAWGServer(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
