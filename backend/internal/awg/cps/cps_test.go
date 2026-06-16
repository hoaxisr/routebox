package cps

import (
	"strings"
	"testing"
)

func TestMimicOffEmpty(t *testing.T) {
	s := Mimic("off")
	if s.I1 != "" || s.Itime != 0 {
		t.Fatalf("off must be empty, got %+v", s)
	}
	if Mimic("custom").I1 != "" {
		t.Fatal("custom must be empty")
	}
}

func TestMimicDNS(t *testing.T) {
	s := Mimic("dns")
	if !strings.HasPrefix(s.I1, "<b 0x") || !strings.HasSuffix(s.I1, ">") {
		t.Fatalf("I1 not a <b 0x..> tag: %q", s.I1)
	}
	if s.I2 == "" || s.Itime <= 0 {
		t.Fatalf("expected entropy fields + Itime, got %+v", s)
	}
	if Mimic("dns").I1 == s.I1 {
		t.Fatal("I1 should vary between calls")
	}
}
