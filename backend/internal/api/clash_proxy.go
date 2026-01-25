package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// WebSocket upgrader with permissive origin checking (internal API)
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for internal API
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

	clashConn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		errMsg := fmt.Sprintf(`{"error": "Failed to connect to Clash API at %s: %v"}`, clashAddr, err)
		clientConn.WriteMessage(websocket.TextMessage, []byte(errMsg))
		return
	}
	defer clashConn.Close()

	done := make(chan struct{})

	// Check if this is the connections endpoint (needs GeoIP enrichment)
	isConnections := strings.HasSuffix(path, "/connections")

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

			if err := clientConn.WriteMessage(msgType, msg); err != nil {
				return
			}
		}
	}()

	// Proxy messages from client to Clash
	go func() {
		for {
			msgType, msg, err := clientConn.ReadMessage()
			if err != nil {
				clashConn.Close()
				return
			}
			if err := clashConn.WriteMessage(msgType, msg); err != nil {
				return
			}
		}
	}()

	<-done
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
