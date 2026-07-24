package awg

import "routebox/backend/internal/settings"

// EnableInputFromSettings maps the persisted RouteBox settings to the Enable
// orchestrator input. This is the single source of truth for the settings<->awg
// field mapping: it was previously hand-copied in api.awgEnableInput and
// main.awgDesired, which drifted apart once (the AWG3 device-timers were added to
// one but not the other — a bug). Kept in the awg package so both call sites
// depend on it (awg may import settings; settings stays awg-agnostic).
func EnableInputFromSettings(s settings.AwgSettings) EnableInput {
	return EnableInput{
		Subnet: s.Subnet, ListenPort: s.ListenPort, MTU: s.MTU,
		DNS: s.DNS, WANIface: s.WANIface, ObfPreset: s.ObfPreset,
		HeaderProtection: s.HeaderProtection,
		Obf: Obfuscation{
			Jc: s.Obf.Jc, Jmin: s.Obf.Jmin, Jmax: s.Obf.Jmax,
			S1: s.Obf.S1, S2: s.Obf.S2, S3: s.Obf.S3, S4: s.Obf.S4,
			H1: s.Obf.H1, H2: s.Obf.H2, H3: s.Obf.H3, H4: s.Obf.H4,
			CPA: s.Obf.ContentPaddingAddition, RAT: s.Obf.RekeyAfterTime,
			RekeyTimeout: s.Obf.RekeyTimeout, RejectAfterTime: s.Obf.RejectAfterTime,
			KeepaliveTimeout: s.Obf.KeepaliveTimeout, MaxHandshakeAttempts: s.Obf.MaxHandshakeAttempts,
		},
	}
}
