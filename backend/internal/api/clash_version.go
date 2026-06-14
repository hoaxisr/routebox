package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// clashVersionTimeout bounds the loopback query to the Clash API /version
// endpoint so a wedged sing-box process can never stall the status handler.
const clashVersionTimeout = 2 * time.Second

// parseClashVersion extracts the "version" field from a Clash API /version
// response body. sing-box returns e.g. {"meta":true,"version":"1.13.13-awg2.1"}.
// Returns (version, true) when a non-empty version is present; ("", false) on
// malformed JSON or a missing/empty version.
func parseClashVersion(body []byte) (string, bool) {
	var v struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", false
	}
	if v.Version == "" {
		return "", false
	}
	// sing-box's clash /version reports e.g. "sing-box 1.13.13-awg2.1"; strip the
	// product prefix so the result matches the on-disk runVersion format
	// ("1.13.13-awg2.1") — keep the last space-separated token.
	ver := v.Version
	if i := strings.LastIndex(ver, " "); i >= 0 {
		ver = ver[i+1:]
	}
	if ver == "" {
		return "", false
	}
	return ver, true
}

// runningSingboxVersion queries the running sing-box process for its self-reported
// version via the Clash API /version endpoint (loopback, short timeout). Returns
// (version, true) on success; ("", false) on any error, non-200, or empty version
// so callers can fall back to the on-disk binary version.
func runningSingboxVersion(clashAddr string) (string, bool) {
	if clashAddr == "" {
		return "", false
	}

	ctx, cancel := context.WithTimeout(context.Background(), clashVersionTimeout)
	defer cancel()

	url := fmt.Sprintf("http://%s/version", clashAddr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", false
	}

	return parseClashVersion(body)
}
