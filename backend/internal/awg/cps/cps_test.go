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

func TestMimicTLS(t *testing.T) {
	pkt := mimicTLS()
	// TLS record: content type 22 (handshake), version 0x0301..0x0303.
	if pkt[0] != 0x16 || pkt[1] != 0x03 {
		t.Fatalf("not a TLS handshake record: % x", pkt[:3])
	}
	// Handshake type 1 = ClientHello at record-payload start (offset 5).
	if pkt[5] != 0x01 {
		t.Fatalf("not a ClientHello: %#x", pkt[5])
	}
	if mimicTLSHex() == mimicTLSHex() {
		t.Fatal("ClientHello should vary (random SNI)")
	}
}

func mimicTLSHex() string { return string(mimicTLS()) }

func TestMimicSIP(t *testing.T) {
	pkt := string(mimicSIP())
	if !strings.HasPrefix(pkt, "REGISTER sip:") {
		t.Fatalf("not a SIP REGISTER: %q", pkt[:20])
	}
	if !strings.Contains(pkt, "Call-ID:") || !strings.Contains(pkt, "CSeq:") {
		t.Fatal("missing SIP headers")
	}
	if string(mimicSIP()) == pkt {
		t.Fatal("SIP packet should vary (random Call-ID/branch)")
	}
}
