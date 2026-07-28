package api

import (
	"errors"
	"net/http"

	"routebox/backend/internal/config"
)

// writeConfigError answers an error that came out of the config manager.
//
// A read-only config is not a malformed request and not an internal failure:
// the request was fine, the state forbids it. That is 409, and the message
// carries the reason and the path so the user knows what to make writable.
// Anything else keeps the status the caller would have used anyway.
func writeConfigError(w http.ResponseWriter, fallback int, err error) {
	if errors.Is(err, config.ErrReadOnly) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeError(w, fallback, err.Error())
}

// writeOpError is writeConfigError for handlers that answer with a deliberately
// generic message instead of the raw error (the AWG endpoints, which keep
// internals out of the response). A read-only config still gets its own 409 and
// its own explicit text — the user has to learn what to make writable.
func writeOpError(w http.ResponseWriter, fallback int, message string, err error) {
	if errors.Is(err, config.ErrReadOnly) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeError(w, fallback, message)
}
