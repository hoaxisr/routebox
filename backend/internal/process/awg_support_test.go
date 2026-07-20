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

func TestParseSupportsAWGServer_Awg3(t *testing.T) {
	cases := map[string]bool{
		"sing-box version 1.14.0-alpha.48-awg3-xhttp-mieru\n": true, // bare awg3, no dot
		"sing-box version 1.13.13-awg2.1\n":                   true,
		"sing-box version 1.13.13-awg2.0\n":                   false,
		"sing-box version 1.14.0-alpha.48\n":                  true, // 1.14 base, no awg token
		"sing-box version 1.13.13\n":                          false,
	}
	for in, want := range cases {
		if got := parseSupportsAWGServer(in); got != want {
			t.Errorf("Server(%q)=%v want %v", in, got, want)
		}
	}
}

func TestParseSupportsAWG3(t *testing.T) {
	cases := map[string]bool{
		"sing-box version 1.14.0-alpha.48-awg3-xhttp-mieru\n": true,
		"sing-box version 1.14.0-alpha.48\n":                  true,  // 1.14 base
		"sing-box version 1.13.13-awg2.1\n":                   false, // awg2.1 has no header_protection_key
		"sing-box version 1.13.13\n":                          false,
	}
	for in, want := range cases {
		if got := parseSupportsAWG3(in); got != want {
			t.Errorf("AWG3(%q)=%v want %v", in, got, want)
		}
	}
}
