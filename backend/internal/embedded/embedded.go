package embedded

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// mimeTypes maps file extensions to MIME types
var mimeTypes = map[string]string{
	".js":    "application/javascript",
	".mjs":   "application/javascript",
	".css":   "text/css",
	".html":  "text/html",
	".json":  "application/json",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".eot":   "application/vnd.ms-fontobject",
}

// Handler returns an http.HandlerFunc that serves the embedded SPA
func Handler() http.HandlerFunc {
	// Get the dist subdirectory
	subFS, err := fs.Sub(distFS, "dist")
	if err != nil {
		// If dist doesn't exist, serve a placeholder
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>RouteBox</title></head>
<body>
<h1>RouteBox</h1>
<p>Frontend not built. Run <code>make build</code> first.</p>
</body>
</html>`))
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Try to open the file
		file, err := subFS.Open(path)
		if err != nil {
			// File not found - serve index.html for SPA routing
			path = "index.html"
			file, err = subFS.Open(path)
			if err != nil {
				http.Error(w, "Not found", http.StatusNotFound)
				return
			}
		}
		defer file.Close()

		// Check if it's a directory
		stat, err := file.Stat()
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if stat.IsDir() {
			// Try index.html in directory
			path = path + "/index.html"
			file.Close()
			file, err = subFS.Open(path)
			if err != nil {
				// Serve root index.html for SPA
				path = "index.html"
				file, err = subFS.Open(path)
				if err != nil {
					http.Error(w, "Not found", http.StatusNotFound)
					return
				}
			}
			stat, _ = file.Stat()
		}

		// Set correct Content-Type based on extension
		ext := strings.ToLower(filepath.Ext(path))
		if mimeType, ok := mimeTypes[ext]; ok {
			w.Header().Set("Content-Type", mimeType)
		}

		// Read and serve file content
		content, err := fs.ReadFile(subFS, path)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	}
}
