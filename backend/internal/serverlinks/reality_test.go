package serverlinks

import "testing"

// Real keypair from `sing-box generate reality-keypair` (amnezia-box).
const (
	fixturePriv = "SN5HcFLrdjYEYbYYowow0k8zRF5m2uvX6_vcun25p2s"
	fixturePub  = "onu9CnSwBGKrgJGKK_WkggznnOwUuvNjTHw4nBlSdwU"
)

func TestRealityPublicFromPrivate(t *testing.T) {
	got, err := RealityPublicFromPrivate(fixturePriv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fixturePub {
		t.Fatalf("got %q, want %q", got, fixturePub)
	}
}

func TestRealityPublicFromPrivateRejectsGarbage(t *testing.T) {
	if _, err := RealityPublicFromPrivate("not-base64!!"); err == nil {
		t.Fatal("expected error for invalid base64")
	}
	if _, err := RealityPublicFromPrivate("c2hvcnQ"); err == nil {
		t.Fatal("expected error for wrong key length")
	}
}
