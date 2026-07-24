package awg

import (
	"testing"

	"routebox/backend/internal/settings"
)

// TestEnableInputFromSettings pins the single settings<->awg mapper: every field
// (scalars, DNS slice, base obfuscation, AWG3 CPA/RAT + the four device-timers)
// must transfer. This covers the path previously only reachable via main.awgDesired.
func TestEnableInputFromSettings(t *testing.T) {
	in := EnableInputFromSettings(settings.AwgSettings{
		Subnet: "10.30.0.0/24", ListenPort: 5100, MTU: 1400,
		DNS: []string{"1.1.1.1", "8.8.8.8"}, WANIface: "eth0",
		ObfPreset: "stealth", HeaderProtection: true,
		Obf: settings.AwgObf{
			Jc: 3, Jmin: 50, Jmax: 900, S1: 15, S2: 25, S3: 35, S4: 45,
			H1: "111", H2: "222", H3: "333", H4: "444",
			ContentPaddingAddition: "64-128", RekeyAfterTime: "120",
			RekeyTimeout: "5", RejectAfterTime: "180",
			KeepaliveTimeout: "25", MaxHandshakeAttempts: "18",
		},
	})

	if in.Subnet != "10.30.0.0/24" || in.ListenPort != 5100 || in.MTU != 1400 ||
		in.WANIface != "eth0" || in.ObfPreset != "stealth" || !in.HeaderProtection {
		t.Fatalf("scalar fields not mapped: %+v", in)
	}
	if len(in.DNS) != 2 || in.DNS[0] != "1.1.1.1" || in.DNS[1] != "8.8.8.8" {
		t.Fatalf("dns not mapped: %v", in.DNS)
	}
	if in.Obf.Jc != 3 || in.Obf.Jmin != 50 || in.Obf.Jmax != 900 ||
		in.Obf.S1 != 15 || in.Obf.S2 != 25 || in.Obf.S3 != 35 || in.Obf.S4 != 45 ||
		in.Obf.H1 != "111" || in.Obf.H2 != "222" || in.Obf.H3 != "333" || in.Obf.H4 != "444" {
		t.Fatalf("base obfuscation not mapped: %+v", in.Obf)
	}
	if in.Obf.CPA != "64-128" || in.Obf.RAT != "120" {
		t.Fatalf("awg3 cpa/rat not mapped: %+v", in.Obf)
	}
	if in.Obf.RekeyTimeout != "5" || in.Obf.RejectAfterTime != "180" ||
		in.Obf.KeepaliveTimeout != "25" || in.Obf.MaxHandshakeAttempts != "18" {
		t.Fatalf("device timers not mapped: %+v", in.Obf)
	}
}
