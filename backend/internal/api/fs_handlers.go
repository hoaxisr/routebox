package api

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File browser for picking a local rule-set file in the UI. Lists one
// directory: subdirectories plus .srs/.json files, names and sizes only —
// file contents are never read. The whole filesystem is browsable on
// purpose (the files live wherever the user put them); the endpoint sits
// behind panel auth like the rest of the config API.

type fsEntry struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
	Size int64  `json:"size,omitempty"`
}

type fsListing struct {
	Path    string    `json:"path"`
	Parent  string    `json:"parent,omitempty"`
	Entries []fsEntry `json:"entries"`
}

// ListDir lists ?path= (empty → the config directory; relative paths resolve
// against it, as sing-box resolves local rule-set paths). PROTECTED.
func (h *Handler) ListDir(w http.ResponseWriter, r *http.Request) {
	configDir := filepath.Dir(h.config.GetPath())
	dir := r.URL.Query().Get("path")
	if dir == "" {
		dir = configDir
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(configDir, dir)
	}
	dir = filepath.Clean(dir)

	items, err := os.ReadDir(dir)
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, fs.ErrPermission):
			writeError(w, http.StatusForbidden, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	out := fsListing{Path: dir, Entries: []fsEntry{}}
	if p := filepath.Dir(dir); p != dir {
		out.Parent = p
	}
	for _, it := range items {
		// Stat follows symlinks, so a linked directory or file is shown as what it points to.
		info, err := os.Stat(filepath.Join(dir, it.Name()))
		if err != nil {
			continue
		}
		if info.IsDir() {
			out.Entries = append(out.Entries, fsEntry{Name: it.Name(), Dir: true})
			continue
		}
		switch strings.ToLower(filepath.Ext(it.Name())) {
		case ".srs", ".json":
			out.Entries = append(out.Entries, fsEntry{Name: it.Name(), Size: info.Size()})
		}
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		a, b := out.Entries[i], out.Entries[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	writeSuccess(w, out)
}
