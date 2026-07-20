package settings

import "testing"

func TestAwgSettings_AWG3(t *testing.T) {
	m := &Manager{settings: Default()}
	if err := m.Update(map[string]interface{}{"awg.header_protection": true}); err != nil {
		t.Fatal(err)
	}
	if !m.settings.Awg.HeaderProtection {
		t.Fatal("header_protection not set")
	}
	if err := m.Update(map[string]interface{}{"awg.obf": map[string]interface{}{
		"content_padding_addition": "64-128", "rekey_after_time": "120",
	}}); err != nil {
		t.Fatal(err)
	}
	if m.settings.Awg.Obf.ContentPaddingAddition != "64-128" || m.settings.Awg.Obf.RekeyAfterTime != "120" {
		t.Fatalf("cpa/rat not decoded: %+v", m.settings.Awg.Obf)
	}
	if err := m.Update(map[string]interface{}{"awg.header_protection": "nope"}); err == nil {
		t.Fatal("expected type error")
	}
}
