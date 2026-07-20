package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// WebSocket upgrader with same-host origin checking.
// Non-browser clients (curl, native apps) send no Origin header and are allowed
// through; cross-origin browser requests are rejected.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // non-browser client (curl, native app) — no Origin header
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return u.Host == r.Host // same-origin only
	},
}

// getClashAddr returns the Clash API address from config or fallback
func (h *Handler) getClashAddr() string {
	// If explicit address was provided at startup, use it
	if h.clashAddr != "" {
		return h.clashAddr
	}

	// Try to get from current config (dynamic)
	cfg := h.config.Get()
	if cfg != nil {
		// Look for experimental.clash_api.external_controller
		if exp, ok := cfg["experimental"].(map[string]interface{}); ok {
			if clashApi, ok := exp["clash_api"].(map[string]interface{}); ok {
				if controller, ok := clashApi["external_controller"].(string); ok && controller != "" {
					return controller
				}
			}
		}
	}

	// Default fallback: sing-box default Clash API address
	// Only return if process is running (otherwise Clash API won't be available)
	if h.process.GetStatus().Running {
		return "127.0.0.1:9090"
	}

	return ""
}

// getClashSecret returns the Clash API secret from config, or "" when not set.
// The panel is the legitimate Clash API client: proxied requests must
// authenticate upstream with this secret or sing-box answers 401.
func (h *Handler) getClashSecret() string {
	cfg := h.config.Get()
	if cfg != nil {
		if exp, ok := cfg["experimental"].(map[string]interface{}); ok {
			if clashApi, ok := exp["clash_api"].(map[string]interface{}); ok {
				if secret, ok := clashApi["secret"].(string); ok {
					return secret
				}
			}
		}
	}
	return ""
}

// isEnrichmentEnabled checks if GeoIP enrichment is enabled in settings
func (h *Handler) isEnrichmentEnabled() bool {
	if h.settings == nil {
		return true // Default to enabled if no settings
	}
	cfg := h.settings.Get()
	return cfg.GeoIP.Enabled && cfg.Monitoring.EnrichmentEnabled
}

// ProxyClashAPI proxies HTTP requests to Clash API
func (h *Handler) ProxyClashAPI(w http.ResponseWriter, r *http.Request) {
	clashAddr := h.getClashAddr()
	if clashAddr == "" {
		writeError(w, http.StatusServiceUnavailable, "Clash API not configured")
		return
	}

	// Get the path after /api/clash/
	path := chi.URLParam(r, "*")
	if path == "" {
		path = "/"
	}

	// Build target URL with query parameters
	targetURL := fmt.Sprintf("http://%s/%s", clashAddr, path)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// Create proxy request
	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}
	// Don't leak panel credentials to the clash process.
	proxyReq.Header.Del("Cookie")
	proxyReq.Header.Del("Authorization")
	// Authenticate upstream with the configured Clash API secret, if any.
	if secret := h.getClashSecret(); secret != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+secret)
	}

	// Send request
	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Clash API error: %v", err))
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status and body
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// ProxyClashWebSocket proxies WebSocket connections to Clash API
// Also handles regular HTTP requests by falling back to HTTP proxy
func (h *Handler) ProxyClashWebSocket(w http.ResponseWriter, r *http.Request) {
	clashAddr := h.getClashAddr()
	if clashAddr == "" {
		writeError(w, http.StatusServiceUnavailable, "Clash API not configured")
		return
	}

	// Check if this is a WebSocket upgrade request
	if r.Header.Get("Upgrade") != "websocket" {
		// Fall back to HTTP proxy
		path := strings.TrimPrefix(r.URL.Path, "/api/clash")
		targetURL := fmt.Sprintf("http://%s%s", clashAddr, path)
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}

		proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create request: %v", err))
			return
		}

		for key, values := range r.Header {
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}
		// Don't leak panel credentials to the clash process.
		proxyReq.Header.Del("Cookie")
		proxyReq.Header.Del("Authorization")
		// Authenticate upstream with the configured Clash API secret, if any.
		if secret := h.getClashSecret(); secret != "" {
			proxyReq.Header.Set("Authorization", "Bearer "+secret)
		}

		client := &http.Client{}
		resp, err := client.Do(proxyReq)
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("Clash API error: %v", err))
			return
		}
		defer resp.Body.Close()

		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}

	// Upgrade client connection to WebSocket
	clientConn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer clientConn.Close()

	// Connect to Clash API WebSocket
	path := strings.TrimPrefix(r.URL.Path, "/api/clash")
	wsURL := url.URL{
		Scheme:   "ws",
		Host:     clashAddr,
		Path:     path,
		RawQuery: r.URL.RawQuery,
	}

	// Authenticate the upstream WS dial with the configured Clash API secret,
	// if any (server-side dial, so a header is fine — nil header is valid).
	var wsHeader http.Header
	if secret := h.getClashSecret(); secret != "" {
		wsHeader = http.Header{"Authorization": []string{"Bearer " + secret}}
	}
	clashConn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), wsHeader)
	if err != nil {
		errMsg := fmt.Sprintf(`{"error": "Failed to connect to Clash API at %s: %v"}`, clashAddr, err)
		clientConn.WriteMessage(websocket.TextMessage, []byte(errMsg))
		return
	}
	defer clashConn.Close()

	// Resolve keepalive intervals from settings (defaults: 30s ping, 10s pong)
	pingInterval := 30 * time.Second
	pongTimeout := 10 * time.Second
	if h.settings != nil {
		adv := h.settings.Get().Advanced
		if adv.WsPingIntervalSec > 0 {
			pingInterval = time.Duration(adv.WsPingIntervalSec) * time.Second
		}
		if adv.WsPongTimeoutSec > 0 {
			pongTimeout = time.Duration(adv.WsPongTimeoutSec) * time.Second
		}
	}
	const writeWait = 10 * time.Second
	readWait := pingInterval + pongTimeout

	// Half-dead client detection: require a pong within readWait. Only pongs
	// reset the deadline (via the pong handler below) — regular data frames
	// from the client do not extend it.
	clientConn.SetReadDeadline(time.Now().Add(readWait))
	clientConn.SetPongHandler(func(string) error {
		clientConn.SetReadDeadline(time.Now().Add(readWait))
		return nil
	})

	done := make(chan struct{})
	stopPing := make(chan struct{})

	// Check if this is the connections endpoint (needs GeoIP enrichment)
	isConnections := strings.HasSuffix(path, "/connections")

	// writeMu serializes writes to clientConn (data pump + ping ticker)
	var writeMu sync.Mutex
	writeToClient := func(msgType int, msg []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		clientConn.SetWriteDeadline(time.Now().Add(writeWait))
		return clientConn.WriteMessage(msgType, msg)
	}

	// Ping ticker: a client that stops reading/ponging fails the write or the
	// read deadline, tearing down both connections instead of leaking them.
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := writeToClient(websocket.PingMessage, nil); err != nil {
					clientConn.Close()
					clashConn.Close()
					return
				}
			case <-stopPing:
				return
			}
		}
	}()

	// Proxy messages from Clash to client
	go func() {
		defer close(done)
		for {
			msgType, msg, err := clashConn.ReadMessage()
			if err != nil {
				return
			}

			// Enrich connections data with GeoIP if available and enabled
			if isConnections && h.geoip != nil && h.geoip.IsLoaded() && msgType == websocket.TextMessage && h.isEnrichmentEnabled() {
				msg = h.enrichConnectionsMessage(msg)
			}

			if err := writeToClient(msgType, msg); err != nil {
				return
			}
		}
	}()

	// Proxy messages from client to Clash. This ReadMessage loop also services
	// incoming control frames (pongs), driving the pong handler above. On read
	// deadline expiry or client error it closes clashConn, which unblocks the
	// Clash->client pump and ends the handler.
	go func() {
		for {
			msgType, msg, err := clientConn.ReadMessage()
			if err != nil {
				clashConn.Close()
				return
			}
			clashConn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := clashConn.WriteMessage(msgType, msg); err != nil {
				clashConn.Close()
				return
			}
		}
	}()

	<-done
	close(stopPing)
}

// enrichConnectionsMessage adds GeoIP data to connections response
func (h *Handler) enrichConnectionsMessage(msg []byte) []byte {
	var data map[string]interface{}
	if err := json.Unmarshal(msg, &data); err != nil {
		return msg // Return original on parse error
	}

	connections, ok := data["connections"].([]interface{})
	if !ok {
		return msg
	}

	for _, conn := range connections {
		connMap, ok := conn.(map[string]interface{})
		if !ok {
			continue
		}

		metadata, ok := connMap["metadata"].(map[string]interface{})
		if !ok {
			continue
		}

		// Lookup destination IP
		if destIP, ok := metadata["destinationIP"].(string); ok && destIP != "" {
			if info := h.geoip.Lookup(destIP); info != nil {
				connMap["geoip"] = info
			}
		}
	}

	enriched, err := json.Marshal(data)
	if err != nil {
		return msg
	}
	return enriched
}
