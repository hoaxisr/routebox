package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"syscall"
	"time"

	"routebox/backend/internal/updates"
)

// scheduleExit is swapped in tests; production re-execs the running process
// onto the freshly-swapped binary. syscall.Exec replaces the image in place
// (same PID), so the restart works under any systemd Restart= policy — and
// even outside systemd — instead of relying on a respawn after os.Exit.
//
// path must be the KNOWN install path (target.BinaryPath() captured BEFORE
// the swap): after Apply renames the running binary to <path>.old,
// os.Executable() (/proc/self/exe) follows the renamed inode and would
// re-exec the OLD binary. Empty path falls back to os.Executable()
// best-effort.
var scheduleExit = func(path string) {
	time.AfterFunc(500*time.Millisecond, func() {
		exe := path
		if exe == "" {
			var err error
			if exe, err = os.Executable(); err != nil {
				exe = os.Args[0]
			}
		}
		if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
			// Re-exec failed: fall back to a plain exit so a systemd
			// Restart=always unit still respawns the new binary.
			log.Printf("updates: re-exec %s failed: %v; exiting for respawn", exe, err)
			os.Exit(0)
		}
	})
}

// dockerUpdateCommand is what the Updates page shows in place of the RouteBox
// Apply button when running in Docker: the image is the source of truth for
// the routebox binary, so "updating" means recreating the container from a
// newer tag rather than replacing the on-disk binary in place.
const dockerUpdateCommand = "docker compose pull routebox && docker compose up -d routebox"

type targetStatus struct {
	Name            string     `json:"name"`
	Supported       bool       `json:"supported"`
	Current         string     `json:"current"`
	Latest          string     `json:"latest,omitempty"`
	Notes           string     `json:"notes,omitempty"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	LastChecked     *time.Time `json:"last_checked,omitempty"`
	UpdateAvailable bool       `json:"update_available"`
	Error           string     `json:"error,omitempty"`
	// DockerManaged is true when this target's Apply is refused because
	// RouteBox is running in the official Docker image; UpdateCommand is the
	// command the UI should show instead.
	DockerManaged bool   `json:"docker_managed,omitempty"`
	UpdateCommand string `json:"update_command,omitempty"`
}

func (h *Handler) buildTargetStatus(t updates.Target) targetStatus {
	ts := targetStatus{Name: t.Name}
	_, ts.Supported = t.AssetSuffix(runtime.GOARCH)
	if cur, err := t.CurrentVersion(); err == nil {
		ts.Current = cur
	}
	cached, ok := h.updates.Checker.Cached(t.Name)
	if !ok {
		return ts
	}
	lc := cached.LastChecked
	ts.LastChecked = &lc
	ts.Error = cached.Error
	if cached.Release != nil {
		ts.Latest = cached.Release.Version
		ts.Notes = cached.Release.Notes
		if !cached.Release.PublishedAt.IsZero() {
			pa := cached.Release.PublishedAt
			ts.PublishedAt = &pa
		}
		// GitHub /releases/latest is authoritative for "newest"; the fork's
		// tags (alpha.48-awg3-xhttp-mieru-4 vs beta.1-awgm.1) aren't
		// numerically orderable, so compare identity, not magnitude.
		ts.UpdateAvailable = ts.Current != "" && ts.Latest != "" &&
			updates.NormalizeVersion(ts.Current) != updates.NormalizeVersion(ts.Latest)
	}
	if t.SelfUpdate && h.dockerMode {
		ts.DockerManaged = true
		ts.UpdateCommand = dockerUpdateCommand
	}
	return ts
}

func (h *Handler) updatesStatusPayload() map[string]interface{} {
	statuses := make([]targetStatus, 0, len(h.updates.Targets))
	for _, t := range h.updates.Targets {
		statuses = append(statuses, h.buildTargetStatus(t))
	}
	return map[string]interface{}{"targets": statuses}
}

// GetUpdatesStatus returns cached release info for all targets.
func (h *Handler) GetUpdatesStatus(w http.ResponseWriter, r *http.Request) {
	if h.updates == nil {
		writeError(w, http.StatusServiceUnavailable, "Updates not configured")
		return
	}
	writeSuccess(w, h.updatesStatusPayload())
}

// CheckUpdates synchronously re-checks GitHub for all supported targets.
func (h *Handler) CheckUpdates(w http.ResponseWriter, r *http.Request) {
	if h.updates == nil {
		writeError(w, http.StatusServiceUnavailable, "Updates not configured")
		return
	}
	for _, t := range h.updates.Targets {
		if _, ok := t.AssetSuffix(runtime.GOARCH); !ok {
			continue
		}
		h.updates.Checker.Check(t) // errors land in the cache, surfaced per-target
	}
	writeSuccess(w, h.updatesStatusPayload())
}

// ApplyUpdate downloads, verifies and installs the latest release of one
// target. Blocking: the UI polls /api/updates/progress in parallel.
func (h *Handler) ApplyUpdate(w http.ResponseWriter, r *http.Request) {
	if h.updates == nil {
		writeError(w, http.StatusServiceUnavailable, "Updates not configured")
		return
	}
	var req struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}
	target, ok := h.updates.Target(req.Target)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Unknown target: %s", req.Target))
		return
	}

	if target.SelfUpdate && h.dockerMode {
		writeError(w, http.StatusConflict, fmt.Sprintf(
			"RouteBox is running in Docker; the image is the source of truth for its own binary. Update by running: %s",
			dockerUpdateCommand,
		))
		return
	}

	cached, ok := h.updates.Checker.Cached(target.Name)
	if !ok || cached.Release == nil {
		if _, err := h.updates.Checker.Check(target); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		cached, _ = h.updates.Checker.Cached(target.Name)
	}
	rel := *cached.Release

	// Capture the install path BEFORE Apply swaps binaries: post-swap both
	// os.Executable() and BinaryPath() (for the routebox target) resolve to
	// the <path>.old backup, and re-execing that would restart the OLD binary.
	execPath := target.BinaryPath()

	current, err := target.CurrentVersion()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Cannot determine current version: %v", err))
		return
	}
	if updates.NormalizeVersion(rel.Version) == updates.NormalizeVersion(current) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%s is already up to date (%s)", target.Name, current))
		return
	}

	result, err := h.updates.Updater.Apply(target, rel)
	if err != nil {
		if errors.Is(err, updates.ErrBusy) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if target.SelfUpdate {
		// Seam from updates.Updater: Apply never exits the process; the
		// handler responds first, then schedules exit so systemd respawns
		// the new binary.
		writeSuccess(w, map[string]interface{}{
			"restarting": result.Restarting,
			"version":    rel.Version,
		})
		if result.Restarting {
			scheduleExit(execPath)
		}
		return
	}

	writeSuccess(w, map[string]interface{}{
		"message":    fmt.Sprintf("%s updated to %s and restarted", target.Name, rel.Version),
		"restarting": false,
		"version":    rel.Version,
	})
}

// GetUpdatesProgress returns the current Apply progress snapshot.
func (h *Handler) GetUpdatesProgress(w http.ResponseWriter, r *http.Request) {
	if h.updates == nil {
		writeError(w, http.StatusServiceUnavailable, "Updates not configured")
		return
	}
	writeSuccess(w, h.updates.Updater.Progress())
}
