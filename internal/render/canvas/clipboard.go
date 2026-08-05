package canvas

import "sync"

// Clipboard is the system pasteboard seam for the text edit session. The
// engine only reads it (Cmd+V) and writes it (Cmd+C/X) while an input or
// textarea holds the session; hosts install a real implementation (the
// clipboardGet/clipboardSet native ops on macOS, cmd/qorm). Tests inject a
// fake via SetClipboard. A nil clipboard is the safe default: Get returns "",
// Set no-ops — an app without a host seam simply has a copy that does nothing.
type Clipboard interface {
	Get() string
	Set(text string)
}

// ClipboardFunc adapts plain functions to the Clipboard interface.
type ClipboardFunc struct {
	GetFn func() string
	SetFn func(string)
}

func (f ClipboardFunc) Get() string {
	if f.GetFn != nil {
		return f.GetFn()
	}
	return ""
}

func (f ClipboardFunc) Set(text string) {
	if f.SetFn != nil {
		f.SetFn(text)
	}
}

var (
	clipMu sync.RWMutex
	clip   Clipboard
)

// SetClipboard installs the system-clipboard implementation. Called by the
// host at startup; safe to call again (tests) from any goroutine.
func SetClipboard(c Clipboard) {
	clipMu.Lock()
	clip = c
	clipMu.Unlock()
}

// ClipboardGet returns the current system clipboard text ("" when no seam is
// installed). Safe from any goroutine.
func ClipboardGet() string {
	clipMu.RLock()
	defer clipMu.RUnlock()
	if clip != nil {
		return clip.Get()
	}
	return ""
}

// ClipboardSet puts text on the system clipboard (no-op when no seam is
// installed). Safe from any goroutine.
func ClipboardSet(text string) {
	clipMu.RLock()
	defer clipMu.RUnlock()
	if clip != nil {
		clip.Set(text)
	}
}
