// Package mobilebridge is the gomobile-facing facade over the canvas engine.
// gomobile bind translates only simple types (strings, numbers, byte slices,
// interfaces of those), so every engine capability a mobile host needs is
// flattened into this surface: load an app from JSON docs, render frames into
// caller-owned RGBA planes, forward pointer/key events, and read state back.
// A mobile host (Swift/Kotlin) implements exactly the same contract the
// darwin host does (internal/app): a bitmap Surface, an event loop, and a
// present step — this package is where its calls land.
//
// The facade is platform-neutral and fully headless-testable; platform code
// (Swift/Kotlin) lives in mobile/ios and mobile/android.
package mobilebridge

import (
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"sync"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
)

// Session is one running app instance (engine + surface state). It is NOT
// goroutine-safe: a host must drive it from one thread (the same
// single-threaded-render rule as the desktop engine) or hold Mu.
type Session struct {
	Eng   *canvas.Engine
	rt    *runtime.Runtime
	buf   *image.RGBA
	size  image.Point
	scale int

	mu sync.Mutex
}

// NewSession loads an app from its JSON docs (the concatenated qorm.json +
// scenes + actions the host reads from the bundle) and creates the engine.
// docsJSON is a JSON array of doc objects ({"type":"app"|"scene"|"action",…}).
func NewSession(docsJSON string) (*Session, error) {
	var docs []map[string]any
	if err := json.Unmarshal([]byte(docsJSON), &docs); err != nil {
		return nil, fmt.Errorf("mobilebridge: docs JSON: %w", err)
	}
	app := loader.FromDocs(docs)
	if app == nil || len(app.Scenes) == 0 {
		return nil, fmt.Errorf("mobilebridge: no scenes in docs")
	}
	rt := runtime.New(app)
	return &Session{Eng: canvas.NewEngine(rt, canvas.SoftwareRenderer{}), rt: rt}, nil
}

// NewSessionFromDir loads an unpacked app directory (dev/simulator path).
func NewSessionFromDir(dir string) (*Session, error) {
	app, err := loader.LoadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("mobilebridge: load %s: %w", dir, err)
	}
	rt := runtime.New(app)
	return &Session{Eng: canvas.NewEngine(rt, canvas.SoftwareRenderer{}), rt: rt}, nil
}

// SetViewport sizes the frame the next RenderFrame produces (physical px,
// scale included — the host passes device pixels and the backing scale).
func (s *Session) SetViewport(width, height, scale int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scale < 1 {
		scale = 1
	}
	s.size = image.Pt(width, height)
	s.scale = scale
	s.buf = image.NewRGBA(image.Rect(0, 0, width, height))
	s.Eng.MarkDirty()
}

// RenderFrame renders into the session plane when needed and returns its
// pixels (RGBA, straight alpha, stride = width*4) plus whether a NEW frame
// was produced — false means nothing changed and the host may skip present.
// The returned slice is owned by the session and reused next call.
func (s *Session) RenderFrame() ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buf == nil {
		return nil, false
	}
	if rendered, _ := s.Eng.RenderInto(s.size, s.scale, s.buf); !rendered {
		return nil, false
	}
	return s.buf.Pix, true
}

// Pointer forwards a pointer event ("move"/"press"/"release") in PHYSICAL px
// (the host multiplies logical points by its scale — same rule as desktop).
func (s *Session) Pointer(kind string, x, y float64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := canvas.PointerMove
	switch strings.ToLower(kind) {
	case "press":
		t = canvas.PointerPress
	case "release":
		t = canvas.PointerRelease
	}
	return s.Eng.HandlePointer(canvas.PointerInput{Type: t, X: x, Y: y})
}

// Scroll forwards a wheel/scroll delta in physical px.
func (s *Session) Scroll(dx, dy float64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Eng.HandleScroll(canvas.ScrollInput{DX: dx, DY: dy})
}

// Key forwards a key event: name is the normalized key ("left","return","a",…)
// with runeVal carrying the printable character for text input (0 = none).
func (s *Session) Key(name string, runeVal int32, down bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Eng.HandleKey(canvas.KeyInput{Key: name, Rune: rune(runeVal), Down: down})
}

// State returns the JSON-encoded value at a dotted state path ("" if unset).
func (s *Session) State(path string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.rt.State[path]
	if !ok {
		return ""
	}
	return fmt.Sprint(v)
}

// SetState writes a string value at a dotted state path and requests a frame.
func (s *Session) SetState(path, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rt.State[path] = value
	s.Eng.RequestDraw()
}

// Dispatch invokes a named action (button onPress handlers etc.).
func (s *Session) Dispatch(action string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rt.Dispatch(action, nil)
	s.Eng.RequestDraw()
}

// Animating reports whether the engine needs continuous frames (spinner,
// tweens, games) — the host's loop should free-run; otherwise it sleeps
// until MarkDirty/RequestDraw.
func (s *Session) Animating() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Eng.Animating()
}

// CursorHint exposes the hovered cursor shape: 0 arrow, 1 text, 2 pointer,
// 3 not-allowed (mirrors canvas.CursorHint's ordinals).
func (s *Session) CursorHint() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int(s.Eng.CursorHint())
}

// Background returns the frame's background color (the white the engine
// clears to — exposed so the host window can match it and avoid edge flashes).
func Background() []int { return []int{255, 255, 255, 255} }
