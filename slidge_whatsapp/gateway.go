package whatsapp

import (
	// Standard library.
	"fmt"
	"os"
	"runtime"

	// Third-party libraries.
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	walog "go.mau.fi/whatsmeow/util/log"
)

// A LinkedDevice represents a unique pairing session between the gateway and WhatsApp. It is not
// unique to the underlying "main" device (or phone number), as multiple linked devices may be paired
// with any main device.
type LinkedDevice struct {
	// ID is an opaque string identifying this LinkedDevice to the Session. Noted that this string
	// is currently equivalent to a password, and needs to be protected accordingly.
	ID string
}

// JID returns the WhatsApp JID corresponding to the LinkedDevice ID. Empty or invalid device IDs
// may return invalid JIDs, and this function does not handle errors.
func (d LinkedDevice) JID() types.JID {
	jid, _ := types.ParseJID(d.ID)
	return jid
}

// A ErrorLevel is a value representing the severity of a log message being handled.
type ErrorLevel int

// The log levels handled by the overarching Session logger.
const (
	LevelError ErrorLevel = 1 + iota
	LevelWarning
	LevelInfo
	LevelDebug
)

// HandleLogFunc is the signature for the overarching Gateway log handling function.
type HandleLogFunc func(ErrorLevel, string)

// Errorf handles the given message as representing a (typically) fatal error.
func (h HandleLogFunc) Errorf(msg string, args ...interface{}) {
	h(LevelError, fmt.Sprintf(msg, args...))
}

// Warn handles the given message as representing a non-fatal error or warning thereof.
func (h HandleLogFunc) Warnf(msg string, args ...interface{}) {
	h(LevelWarning, fmt.Sprintf(msg, args...))
}

// Infof handles the given message as representing an informational notice.
func (h HandleLogFunc) Infof(msg string, args ...interface{}) {
	h(LevelInfo, fmt.Sprintf(msg, args...))
}

// Debugf handles the given message as representing an internal-only debug message.
func (h HandleLogFunc) Debugf(msg string, args ...interface{}) {
	h(LevelDebug, fmt.Sprintf(msg, args...))
}

// Sub is a no-op and will return the receiver itself.
func (h HandleLogFunc) Sub(string) walog.Logger {
	return h
}

// A Gateway represents a persistent process for establishing individual sessions between linked
// devices and WhatsApp.
type Gateway struct {
	DBPath  string // The filesystem path for the client database.
	Name    string // The name to display when linking devices on WhatsApp.
	TempDir string // The directory to create temporary files under.

	// Internal variables.
	container *sqlstore.Container
	logger    walog.Logger
}

// NewGateway returns a new, un-initialized Gateway. This function should always be followed by calls
// to [Gateway.Init], assuming a valid [Gateway.DBPath] is set.
func NewGateway() *Gateway {
	return &Gateway{}
}

// SetLogHandler specifies the log handling function to use for all [Gateway] and [Session] operations.
func (w *Gateway) SetLogHandler(h HandleLogFunc) {
	w.logger = HandleLogFunc(func(level ErrorLevel, message string) {
		// Don't allow other Goroutines from using this thread, as this might lead to concurrent
		// use of the GIL, which can lead to crashes.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		h(level, message)
	})
}

// Init performs initialization procedures for the Gateway, and is expected to be run before any
// calls to [Gateway.Session].
func (w *Gateway) Init() error {
	container, err := sqlstore.New("sqlite3", w.DBPath, w.logger)
	if err != nil {
		return err
	}

	if w.Name != "" {
		store.SetOSInfo(w.Name, [...]uint32{1, 0, 0})
	}

	if w.TempDir != "" {
		tempDir = w.TempDir
	}

	w.container = container
	return nil
}

// NewSession returns a new [Session] for the LinkedDevice given. If the linked device does not have
// a valid ID, a pair operation will be required, as described in [Session.Login].
func (w *Gateway) NewSession(device LinkedDevice) *Session {
	return &Session{device: device, gateway: w}
}

// CleanupSession will remove all invalid and obsolete references to the given device, and should be
// used when pairing a new device or unregistering from the Gateway.
func (w *Gateway) CleanupSession(device LinkedDevice) error {
	devices, err := w.container.GetAllDevices()
	if err != nil {
		return err
	}

	for _, d := range devices {
		if d.ID == nil {
			w.logger.Infof("Removing invalid device %s from database", d.ID.String())
			_ = d.Delete()
		} else if device.ID != "" {
			if jid := device.JID(); d.ID.ToNonAD() == jid.ToNonAD() && *d.ID != jid {
				w.logger.Infof("Removing obsolete device %s from database", d.ID.String())
				_ = d.Delete()
			}
		}
	}

	return nil
}

var (
	// The default path for storing temporary files.
	tempDir = os.TempDir()
)

// CreateTempFile creates a temporary file in the Gateway-wide temporary directory (or the default,
// system-wide temporary directory, if no Gateway-specific value was set) and returns the absolute
// path for the file, or an error if none could be created.
func createTempFile(data []byte) (string, error) {
	f, err := os.CreateTemp(tempDir, "slidge-whatsapp-*")
	if err != nil {
		return "", fmt.Errorf("failed creating temporary file: %w", err)
	}

	defer f.Close()
	if len(data) > 0 {
		if n, err := f.Write(data); err != nil {
			return "", fmt.Errorf("failed writing to temporary file: %w", err)
		} else if n < len(data) {
			return "", fmt.Errorf("failed writing to temporary file: incomplete write, want %d, write %d bytes", len(data), n)
		}
	}

	return f.Name(), nil
}
