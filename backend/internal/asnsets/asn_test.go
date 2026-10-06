package asnsets

import (
	"errors"
	"net/netip"
	"testing"
)

func TestParseASN(t *testing.T) {
	ok := map[string]uint32{"13335": 13335, "AS13335": 13335, " as32934 ": 32934, "4294967295": 4294967295}
	for in, want := range ok {
		got, err := ParseASN(in)
		if err != nil || got != want {
			t.Errorf("ParseASN(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "0", "AS", "AS-1", "13335x", "4294967296", "AS 13335"} {
		if _, err := ParseASN(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseASN(%q) err = %v, want ErrInvalid", in, err)
		}
	}
	if FormatASN(13335) != "AS13335" {
		t.Fatal("FormatASN")
	}
}

func pfx(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParsePrefix(s)
	}
	return out
}

func TestAggregateMergesNestedAndAdjacent(t *testing.T) {
	got := Aggregate(pfx(
		"2606:4700::/33", "2606:4700:8000::/33", // adjacent v6 → /32
		"1.1.1.0/24", "1.0.0.0/24", "1.1.1.0/25", // nested 1.1.1.0/25 dropped
		"104.16.0.0/13", "104.24.0.0/14", "104.16.5.0/24", // nested dropped
		"10.0.0.0/25", "10.0.0.128/25", "10.0.1.0/24", // → 10.0.0.0/23
		"1.0.0.0/24", // duplicate
	))
	want := []string{"1.0.0.0/24", "1.1.1.0/24", "10.0.0.0/23", "104.16.0.0/13", "104.24.0.0/14", "2606:4700::/32"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i].String() != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestAggregateMasksHostBits(t *testing.T) {
	got := Aggregate(pfx("192.0.2.77/24"))
	if len(got) != 1 || got[0].String() != "192.0.2.0/24" {
		t.Fatalf("got %v", got)
	}
}

func TestValidTag(t *testing.T) {
	for _, s := range []string{"cloudflare", "asn-telegram_1", "a.b"} {
		if !ValidTag(s) {
			t.Errorf("ValidTag(%q) = false", s)
		}
	}
	for _, s := range []string{"", ".", "..", "a/b", "../x", "with space", string(make([]byte, 65))} {
		if ValidTag(s) {
			t.Errorf("ValidTag(%q) = true", s)
		}
	}
}
