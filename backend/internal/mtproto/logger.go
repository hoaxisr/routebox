package mtproto

import (
	"fmt"
	"log"

	"github.com/9seconds/mtg/v2/mtglib"
)

// newBufferedLogger adapts mtglib's structured logger onto RouteBox's stdlib
// logging and, when given a buffer, onto the panel's log view as well.
//
// It writes to both deliberately. Losing the stdout copy would break `docker
// logs routebox` and journalctl, which are what you reach for when the panel
// itself will not come up.
//
// The Bind* methods return the receiver: RouteBox does not render per-stream
// fields, and building strings nobody reads would cost an allocation on every
// connection.
func newBufferedLogger(buf *LogBuffer) mtglib.Logger { return proxyLogger{buf: buf} }

// newLogger is the stdout-only logger, for callers with no buffer to feed.
func newLogger() mtglib.Logger { return proxyLogger{} }

type proxyLogger struct {
	buf *LogBuffer
}

func (l proxyLogger) Named(string) mtglib.Logger            { return l }
func (l proxyLogger) BindInt(string, int) mtglib.Logger     { return l }
func (l proxyLogger) BindStr(string, string) mtglib.Logger  { return l }
func (l proxyLogger) BindJSON(string, string) mtglib.Logger { return l }

// record writes one line to both sinks.
func (l proxyLogger) record(level, msg string) {
	if level == "warning" {
		log.Printf("mtproto: WARNING %s", msg)
	} else {
		log.Printf("mtproto: %s", msg)
	}

	if l.buf != nil {
		l.buf.Add(level, msg)
	}
}

func (l proxyLogger) Printf(format string, args ...any) {
	l.record("info", fmt.Sprintf(format, args...))
}

func (l proxyLogger) Info(msg string)    { l.record("info", msg) }
func (l proxyLogger) Warning(msg string) { l.record("warning", msg) }

func (l proxyLogger) InfoError(msg string, err error) {
	l.record("info", msg+": "+errText(err))
}

func (l proxyLogger) WarningError(msg string, err error) {
	l.record("warning", msg+": "+errText(err))
}

// Debug is dropped on both sinks. mtglib emits several debug lines per
// connection, which would bury the panel's log view and push everything else
// out of the ring within seconds of real traffic.
func (l proxyLogger) Debug(string)             {}
func (l proxyLogger) DebugError(string, error) {}
