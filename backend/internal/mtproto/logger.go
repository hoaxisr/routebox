package mtproto

import (
	"log"

	"github.com/9seconds/mtg/v2/mtglib"
)

// newLogger adapts mtglib's structured logger onto RouteBox's stdlib logging.
//
// The Bind* methods return the receiver unchanged: RouteBox does not carry
// per-stream fields into its log lines, and building strings nobody reads would
// cost an allocation on every connection.
func newLogger() mtglib.Logger { return proxyLogger{} }

type proxyLogger struct{}

func (l proxyLogger) Named(string) mtglib.Logger            { return l }
func (l proxyLogger) BindInt(string, int) mtglib.Logger     { return l }
func (l proxyLogger) BindStr(string, string) mtglib.Logger  { return l }
func (l proxyLogger) BindJSON(string, string) mtglib.Logger { return l }

func (l proxyLogger) Printf(format string, args ...any) { log.Printf("mtproto: "+format, args...) }
func (l proxyLogger) Info(msg string)                   { log.Printf("mtproto: %s", msg) }
func (l proxyLogger) Warning(msg string)                { log.Printf("mtproto: WARNING %s", msg) }

func (l proxyLogger) InfoError(msg string, err error) { log.Printf("mtproto: %s: %v", msg, err) }

func (l proxyLogger) WarningError(msg string, err error) {
	log.Printf("mtproto: WARNING %s: %v", msg, err)
}

// Debug is dropped. mtglib logs several lines per connection at debug level,
// which would bury everything else in the panel's log view.
func (l proxyLogger) Debug(string)             {}
func (l proxyLogger) DebugError(string, error) {}
