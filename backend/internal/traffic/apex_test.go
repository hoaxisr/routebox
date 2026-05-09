package traffic

import "testing"

func TestApexDomain(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"googlevideo.com", "googlevideo.com"},
		{"r3---sn-uxa.googlevideo.com", "googlevideo.com"},
		{"a.b.c.googlevideo.com", "googlevideo.com"},
		{"bbc.co.uk", "bbc.co.uk"},
		{"news.bbc.co.uk", "bbc.co.uk"},
		{"1.2.3.4", "1.2.3.4"},
		{"::1", "::1"},
		{"2001:db8::1", "2001:db8::1"},
		{"localhost", "localhost"},
		{"", ""},
		{"-", "-"},
	}
	for _, c := range cases {
		got := ApexDomain(c.in)
		if got != c.want {
			t.Errorf("ApexDomain(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
