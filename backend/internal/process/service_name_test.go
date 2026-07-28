package process

import (
	"errors"
	"testing"
)

// A host that once ran upstream sing-box keeps sing-box.service around — the
// installer never disables it. If RouteBox adopts that unit, every later
// decision is made about a service it does not manage: the config-path check
// compares our config against a foreign ExecStart (false mismatch, Start /
// Restart / Reload blocked), "point the unit at our config" writes a drop-in
// into somebody else's unit, and "adopt the detected path" moves RouteBox onto
// somebody else's config. amnezia-box is the unit both installers create, so it
// must win whenever it is present.
func TestPickServiceNamePrefersAmneziaBox(t *testing.T) {
	present := map[string]bool{"sing-box": true, "amnezia-box": true}
	if got := pickServiceName(func(n string) bool { return present[n] }); got != "amnezia-box" {
		t.Fatalf("service name = %q, want amnezia-box", got)
	}
}

// With no amnezia-box unit, a lone sing-box unit is still the best guess —
// that is the pre-rename installation RouteBox is meant to take over.
func TestPickServiceNameFallsBackToSingBox(t *testing.T) {
	present := map[string]bool{"sing-box": true}
	if got := pickServiceName(func(n string) bool { return present[n] }); got != "sing-box" {
		t.Fatalf("service name = %q, want sing-box", got)
	}
}

func TestPickServiceNameNoUnits(t *testing.T) {
	if got := pickServiceName(func(string) bool { return false }); got != "" {
		t.Fatalf("service name = %q, want empty", got)
	}
}

// fakeSystemctl installs a stub for the systemctl seam for the duration of the
// test. unitFiles are the units `list-unit-files` knows about, running are the
// units is-enabled / is-active exit 0 for.
// fakeSystemctl installs a stub for the systemctl seam for the duration of the
// test. unitFiles maps a unit to the state `list-unit-files` reports for it
// ("" — no unit file at all), running are the units is-enabled / is-active exit
// 0 for.
func fakeSystemctl(t *testing.T, unitFiles map[string]string, running map[string]bool) {
	t.Helper()
	restore := systemctl
	t.Cleanup(func() { systemctl = restore })
	systemctl = func(args ...string) ([]byte, error) {
		if len(args) == 0 {
			t.Fatal("systemctl called with no arguments")
		}
		unit := args[len(args)-1]
		switch args[0] {
		case "list-unit-files":
			if state := unitFiles[unit]; state != "" {
				return []byte(unit + " " + state + " enabled\n"), nil
			}
			// systemd prints nothing at all when the pattern matches no unit file.
			return nil, nil
		case "is-enabled", "is-active":
			if running[unit] {
				return []byte("active\n"), nil
			}
			return nil, errors.New("exit status 1")
		}
		t.Fatalf("unexpected systemctl %v", args)
		return nil, nil
	}
}

// The scenario the whole unit-preference order exists for: amnezia-box.service
// is installed but disabled and stopped, while a leftover sing-box.service is
// running. Judging presence by is-enabled/is-active made the foreign unit win —
// and then the drop-in fix would have been written into somebody else's unit.
// A unit file on disk is a unit that is present, whatever its state.
func TestSystemdUnitPresentSeesADisabledStoppedUnit(t *testing.T) {
	fakeSystemctl(t,
		map[string]string{"amnezia-box.service": "disabled", "sing-box.service": "enabled"},
		map[string]bool{"sing-box.service": true},
	)
	if got := pickServiceName(systemdUnitPresent); got != "amnezia-box" {
		t.Fatalf("service name = %q, want amnezia-box", got)
	}
}

// Замаскированный юнит есть на диске, но запустить его нельзя: systemctl start
// на маске падает всегда. Выбрать его — значит отдать пользователю юнит,
// который мы же и выбрали, а он не стартует, — при живом sing-box рядом.
// is-enabled/is-active на маске тоже отвечают отказом (1 и 3), так что
// fallback-ветки её не вернут.
func TestSystemdUnitPresentSkipsAMaskedUnit(t *testing.T) {
	fakeSystemctl(t,
		map[string]string{"amnezia-box.service": "masked", "sing-box.service": "enabled"},
		map[string]bool{"sing-box.service": true},
	)
	if got := pickServiceName(systemdUnitPresent); got != "sing-box" {
		t.Fatalf("service name = %q, want sing-box", got)
	}
}

// Template instances (sing-box@config.service) have no unit file of their own —
// only the template sing-box@.service does — so an enabled or running instance
// must still count as present.
func TestSystemdUnitPresentFallsBackToUnitState(t *testing.T) {
	fakeSystemctl(t, nil, map[string]bool{"sing-box@config.service": true})
	if got := pickServiceName(systemdUnitPresent); got != "sing-box@config" {
		t.Fatalf("service name = %q, want sing-box@config", got)
	}
}

func TestSystemdUnitPresentNoUnitsAtAll(t *testing.T) {
	fakeSystemctl(t, nil, nil)
	if got := pickServiceName(systemdUnitPresent); got != "" {
		t.Fatalf("service name = %q, want empty", got)
	}
}

// `systemctl list-unit-files sing-box@config.service` answers with the template
// file sing-box@.service on some versions. A row for a different unit is not
// this unit, and must not be mistaken for one.
func TestUnitFileListedMatchesTheExactUnit(t *testing.T) {
	if unitFileListed("sing-box@.service enabled enabled\n", "sing-box@config.service") {
		t.Error("a row for another unit must not count as present")
	}
	if !unitFileListed("amnezia-box.service disabled disabled\n", "amnezia-box.service") {
		t.Error("a row for the unit itself must count as present")
	}
	if unitFileListed("", "amnezia-box.service") {
		t.Error("empty output means no unit file")
	}
}

// Состояния, в которых юнит-файл есть, но юнит не запускается, — не «юнит,
// которым RouteBox управляет». Остальные состояния из полного списка systemd
// (`systemctl --state=help`) описывают вполне работающий юнит.
func TestUnitFileListedSkipsUnstartableStates(t *testing.T) {
	unstartable := []string{"masked", "masked-runtime", "bad"}
	for _, state := range unstartable {
		if unitFileListed("amnezia-box.service "+state+" enabled\n", "amnezia-box.service") {
			t.Errorf("state %q cannot be started, so it must not count as present", state)
		}
	}
	startable := []string{
		"enabled", "enabled-runtime", "linked", "linked-runtime", "alias",
		"static", "disabled", "indirect", "generated", "transient",
	}
	for _, state := range startable {
		if !unitFileListed("amnezia-box.service "+state+" enabled\n", "amnezia-box.service") {
			t.Errorf("state %q is a perfectly startable unit, it must count as present", state)
		}
	}
	// Вывод без колонки состояния судить не о чем — юнит-файл в листинге есть.
	if !unitFileListed("amnezia-box.service\n", "amnezia-box.service") {
		t.Error("a row without a state column must still count as present")
	}
}

// The probe must not be asked about a candidate after one has already matched:
// each probe shells out to systemctl twice.
func TestPickServiceNameStopsAtTheFirstMatch(t *testing.T) {
	var asked []string
	got := pickServiceName(func(n string) bool { asked = append(asked, n); return true })
	if got != "amnezia-box" {
		t.Fatalf("service name = %q, want amnezia-box", got)
	}
	if len(asked) != 1 {
		t.Fatalf("probed %v, want only the first candidate", asked)
	}
}
