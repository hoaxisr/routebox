package awg

import "testing"

func TestValidateUintRange(t *testing.T) {
	ok := []string{"", "0", "64", "64-128", "4294967295"}
	for _, s := range ok {
		if _, err := ValidateUintRange(s); err != nil {
			t.Errorf("ValidateUintRange(%q) unexpected err %v", s, err)
		}
	}
	bad := []string{"-1", "1-2-3", "abc", "128-64", "4294967296"}
	for _, s := range bad {
		if _, err := ValidateUintRange(s); err == nil {
			t.Errorf("ValidateUintRange(%q) expected err", s)
		}
	}
}

func TestValidateHPKConstraint(t *testing.T) {
	if err := validateHPKConstraint(Obfuscation{S1: 12, S2: 12, S3: 12, S4: 12}, true); err != nil {
		t.Fatalf("S=12 with HPK should pass: %v", err)
	}
	if err := validateHPKConstraint(Obfuscation{S1: 12, S2: 11, S3: 12, S4: 12}, true); err == nil {
		t.Fatal("S2=11 with HPK must fail")
	}
	if err := validateHPKConstraint(Obfuscation{S1: 0}, false); err != nil {
		t.Fatal("HPK off → no S constraint")
	}
}

func TestValidateObfCPARAT(t *testing.T) {
	out, err := validateObf(Obfuscation{CPA: "3", RAT: "5-10"})
	if err != nil {
		t.Fatalf("valid CPA/RAT rejected: %v", err)
	}
	if out.CPA != "3" || out.RAT != "5-10" {
		t.Fatalf("CPA/RAT not carried through: got %q/%q", out.CPA, out.RAT)
	}
	if _, err := validateObf(Obfuscation{CPA: "abc"}); err == nil {
		t.Fatal("bad CPA must fail")
	}
	if _, err := validateObf(Obfuscation{RAT: "10-5"}); err == nil {
		t.Fatal("inverted RAT range must fail")
	}
	if out, err := validateObf(Obfuscation{}); err != nil || out.CPA != "" || out.RAT != "" {
		t.Fatalf("empty CPA/RAT must pass untouched: %v", err)
	}
}

// validateObf builds its result field by field, so every field added to
// Obfuscation later has to be added there too or it is silently dropped. The two
// AWG 3.1 flags were: they never reached the rendered config, and the running
// snapshot they were dropped from could never equal the saved settings, so the
// Apply banner stayed up forever (#74).
func TestValidateObfKeepsAwg31Flags(t *testing.T) {
	out, err := validateObf(Obfuscation{RandomTrailers: true, DisableCookies: true})
	if err != nil {
		t.Fatalf("3.1 flags rejected: %v", err)
	}
	if !out.RandomTrailers || !out.DisableCookies {
		t.Fatalf("3.1 flags dropped: %+v", out)
	}
	// Everything else must still survive a round trip untouched, flags or not.
	in := Obfuscation{
		Jc: 4, Jmin: 30, Jmax: 80, S1: 100, S2: 22, S3: 16, S4: 12,
		H1: "10", H2: "20", H3: "30", H4: "40",
		CPA: "0-64", RAT: "120-150", RekeyTimeout: "5", RejectAfterTime: "180",
		KeepaliveTimeout: "25-30", MaxHandshakeAttempts: "18",
		RandomTrailers: true,
	}
	if out, err := validateObf(in); err != nil || out != in {
		t.Fatalf("round trip changed the struct: %v\n got %+v\nwant %+v", err, out, in)
	}
}

func TestValidateObfDeviceTimers(t *testing.T) {
	out, err := validateObf(Obfuscation{
		RekeyTimeout: "5", RejectAfterTime: "180", KeepaliveTimeout: "25-30", MaxHandshakeAttempts: "18",
	})
	if err != nil {
		t.Fatalf("valid device timers rejected: %v", err)
	}
	if out.RekeyTimeout != "5" || out.RejectAfterTime != "180" || out.KeepaliveTimeout != "25-30" || out.MaxHandshakeAttempts != "18" {
		t.Fatalf("device timers not carried through: %+v", out)
	}
	if _, err := validateObf(Obfuscation{RekeyTimeout: "abc"}); err == nil {
		t.Fatal("bad rekey_timeout must fail")
	}
	if _, err := validateObf(Obfuscation{KeepaliveTimeout: "9-3"}); err == nil {
		t.Fatal("inverted keepalive_timeout range must fail")
	}
	if out, err := validateObf(Obfuscation{}); err != nil ||
		out.RekeyTimeout != "" || out.RejectAfterTime != "" || out.KeepaliveTimeout != "" || out.MaxHandshakeAttempts != "" {
		t.Fatalf("empty device timers must pass untouched: %v", err)
	}
}
