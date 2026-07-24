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
