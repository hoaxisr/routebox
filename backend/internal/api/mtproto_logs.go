package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"routebox/backend/internal/mtproto"
)

// mtprotoLogFrame is one line on the wire. The shape matches what the Clash log
// stream sends, so the panel's log view renders both sources with one code path.
type mtprotoLogFrame struct {
	Type    string `json:"type"` // level, as the view colours by
	Payload string `json:"payload"`
	Time    string `json:"time"`
}

// StreamMtprotoLogs streams the Telegram proxy's log over a WebSocket: the
// buffered backlog first, then lines as they happen.
//
// The proxy writes to the container's stdout, which the panel cannot read, and
// it is not part of amnezia-box so the Clash log stream never carries it. This
// is the only way to see it from the UI. PROTECTED.
func (h *Handler) StreamMtprotoLogs(w http.ResponseWriter, r *http.Request) {
	// Answered before the upgrade: with no manager there is nothing to stream,
	// and a WebSocket that immediately closes tells the page nothing.
	if h.mtproto == nil {
		writeError(w, http.StatusServiceUnavailable, "mtproto not available")

		return
	}

	buf := h.mtproto.Logs()

	// Subscribed before the backlog is read, so a line logged between the two
	// arrives on the channel rather than falling into the gap.
	live, cancel := buf.Subscribe()
	defer cancel()

	backlog := buf.Recent()

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error
	}

	defer conn.Close()

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

	// Half-dead viewer detection, same contract as the Clash proxy: only pongs
	// extend the deadline.
	conn.SetReadDeadline(time.Now().Add(readWait)) //nolint: errcheck
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(readWait)) //nolint: errcheck

		return nil
	})

	var writeMu sync.Mutex

	send := func(msgType int, msg []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()

		conn.SetWriteDeadline(time.Now().Add(writeWait)) //nolint: errcheck

		return conn.WriteMessage(msgType, msg)
	}

	sendEntry := func(e mtproto.LogEntry) error {
		frame, err := json.Marshal(mtprotoLogFrame{
			Type:    e.Level,
			Payload: e.Message,
			Time:    e.Time.Format(time.RFC3339Nano),
		})
		if err != nil {
			return err
		}

		return send(websocket.TextMessage, frame)
	}

	// The reader exists only to notice the viewer going away: this stream is
	// one-directional, but without a read the close frame is never seen.
	closed := make(chan struct{})

	go func() {
		defer close(closed)

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for _, e := range backlog {
		if err := sendEntry(e); err != nil {
			return
		}
	}

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-closed:
			return

		case <-r.Context().Done():
			return

		case e, ok := <-live:
			if !ok {
				return
			}

			if err := sendEntry(e); err != nil {
				return
			}

		case <-ticker.C:
			if err := send(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
