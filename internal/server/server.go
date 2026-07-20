// Package server serves a live QORM app over HTTP. Button presses POST to
// /event; the server updates state, dispatches the action, re-renders, and
// returns the new body HTML which a tiny inline script swaps in. No cgo, no
// external deps — so the binary cross-compiles to every platform cleanly.
package server

import (
	"crypto/ed25519"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/capability"
	"github.com/qorm/qorm/internal/mcp"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/ota"
	"github.com/qorm/qorm/internal/render"
	"github.com/qorm/qorm/internal/runtime"
)

// Server is an HTTP handler wrapping a live runtime. It serves the browser UI,
// hot OTA updates with rollback, and an MCP endpoint over which an agent shares
// the *same* live app session — so an AI's edits appear in a human's browser
// live, and vice-versa.
type Server struct {
	mu       sync.Mutex
	rt       *runtime.Runtime
	handlers []render.Handler
	rev      atomic.Int64 // bumped on every mutation; drives browser live-sync
	agent    *mcp.Server  // MCP handler sharing rt + mu

	// mcpReadOnly forces the shared MCP session into read-only mode: mutating
	// tools (dispatch/set_state/apply_patch/undo) are rejected. Set from
	// `qorm run --mcp-read-only`; re-applied whenever the runtime is swapped.
	mcpReadOnly bool

	subsMu sync.Mutex               // guards subs
	subs   map[chan string]struct{} // SSE subscribers, each gets pushed updates

	// Collaboration activity log: who (human/agent) did what, newest last.
	actMu    sync.Mutex
	activity []LogEntry
	actSeq   int
	lastSrc  string    // source of the most recent event (for live edit attribution)
	lastDet  string    // its short detail
	lastHash string    // tip of the entry hash chain (see LogEntry.Hash)
	auditW   io.Writer // optional append-only JSONL audit sink (--audit-log)

	// What the human is currently attending to (the focused / last-touched element),
	// surfaced to the agent via qorm_activity so it collaborates in context.
	humanFocus   string
	humanFocusAt time.Time
	// The human's last non-empty text entry, retained even after focus moves on (a
	// button tap must not erase what they just typed). Never a password value.
	humanTyping   string
	humanTypingAt time.Time
	// A hidden (password) field the human filled, retained by label only — the
	// value is never captured, but the agent may know the form is complete.
	humanFilled   string
	humanFilledAt time.Time

	measureMu sync.Mutex
	measure   []byte // latest self-reported layout (rects + key styles)

	// OTA state (populated when started from a bundle).
	trust   ed25519.PublicKey
	revoked bundle.RevocationList
	current *bundle.Bundle // active, verified bundle
	prev    *bundle.Bundle // last-good bundle for rollback

	// Window control (set by a native desktop host): the control engine (debug
	// window / agent) drives the app's native window.
	WindowMover func(id string, x, y, w, h int) // move + resize a window
	WindowOp    func(id, op string)             // focus/minimize/pin/unpin/close
	WindowOpen  func(id, url string, w, h int)  // open a secondary window
	WindowEval  func(id, js string)             // push JS to a window (window-to-window comms)

	// eventToken is a random secret generated at server start and embedded in the
	// rendered HTML page. /event and /presence POST require this token, enforcing
	// that only the real browser client (a human) can produce "human"-attributed
	// log entries. Agents use MCP, which produces "agent" entries. This prevents
	// either side from forging the other's identity — the foundational audit
	// principle for human-AI collaboration.
	eventToken string
}

// New builds a server for a runtime (no OTA).
func New(rt *runtime.Runtime) *Server {
	s := &Server{rt: rt, eventToken: genEventToken()}
	s.initAgent()
	return s
}

// NewBundle builds a server from a verified bundle, enabling OTA updates
// against the given trusted key (nil = integrity-only) and revocation list.
// It refuses (returns an error) when the bundle declares a required capability
// that the current platform does not support.
func NewBundle(b *bundle.Bundle, trust ed25519.PublicKey, revoked bundle.RevocationList) (*Server, error) {
	if err := CheckRequiredCapabilities(b); err != nil {
		return nil, err
	}
	s := &Server{rt: runtime.New(b.ToApp()), current: b, trust: trust, revoked: revoked, eventToken: genEventToken()}
	s.initAgent()
	return s, nil
}

// hostPlatform maps the running OS to a capability-registry platform key.
func hostPlatform() string {
	switch goruntime.GOOS {
	case "darwin":
		return capability.Mac
	case "windows":
		return capability.Windows
	default:
		return capability.Linux
	}
}

// CheckRequiredCapabilities verifies every capability the bundle declares in
// requiredCapabilities against the capability registry for the current
// platform. Capabilities are named by their canonical stem (== widget type for
// all but "badge", whose widget is "dockbadge"); both spellings are accepted.
// A missing capability is a hard startup error, not a warning.
func CheckRequiredCapabilities(b *bundle.Bundle) error {
	platform := hostPlatform()
	for _, name := range b.RequiredCapabilities() {
		widget := ""
		for i := range capability.All {
			if c := &capability.All[i]; c.Stem == name || c.Widget == name {
				widget = c.Widget
				break
			}
		}
		if widget == "" {
			return fmt.Errorf("bundle requires unknown capability %q (not in the capability registry)", name)
		}
		if !capability.Supported(widget, platform) {
			return fmt.Errorf("bundle requires capability %q, which is not supported on this platform (%s); refusing to start", name, platform)
		}
	}
	return nil
}

// genEventToken returns a cryptographically random 16-byte hex string used to
// bind /event and /presence to the real browser client.
func genEventToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// initAgent (re)binds the shared MCP handler to the current runtime. Called on
// construction and whenever the runtime is swapped (OTA). afterMutate runs
// while the agent holds s.mu, so bump() must not re-take s.mu.
func (s *Server) initAgent() {
	s.agent = mcp.NewShared(s.rt, &s.mu, func() { s.bump() })
	s.agent.SetReadOnly(s.mcpReadOnly)
	s.agent.SetMeasureProvider(func() []byte {
		s.measureMu.Lock()
		defer s.measureMu.Unlock()
		return s.measure
	})
	// Let the agent read the shared activity log, so it can see what the human
	// just did in the live app and respond — the reverse of the human's "AI
	// edited" toast.
	s.agent.SetActivityProvider(func() string {
		s.actMu.Lock()
		defer s.actMu.Unlock()
		out := map[string]any{"events": s.activity}
		if s.humanFocus != "" {
			out["humanFocus"] = map[string]any{
				"element":    s.humanFocus,
				"secondsAgo": int(time.Since(s.humanFocusAt).Seconds()),
			}
		}
		if s.humanTyping != "" {
			out["humanTyping"] = map[string]any{
				"entry":      s.humanTyping,
				"secondsAgo": int(time.Since(s.humanTypingAt).Seconds()),
			}
		}
		if s.humanFilled != "" {
			out["humanFilled"] = map[string]any{
				"field":      s.humanFilled, // a hidden field they filled; value NOT captured
				"secondsAgo": int(time.Since(s.humanFilledAt).Seconds()),
			}
		}
		b, _ := json.Marshal(out)
		return string(b)
	})
}

// SetMCPReadOnly switches the shared MCP session into (or out of) read-only
// mode: mutating agent tools are rejected with a JSON-RPC error. The setting
// survives OTA runtime swaps.
func (s *Server) SetMCPReadOnly(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mcpReadOnly = v
	s.agent.SetReadOnly(v)
}

// bump increments the revision, re-renders, refreshes the handler table and
// pushes the new UI to all SSE subscribers. Caller must hold s.mu.
func (s *Server) bump() (int64, string, string) {
	rev := s.rev.Add(1)
	res := render.RenderScene(s.rt, s.rt.CurrentScene())
	s.handlers = res.Handlers
	nav := s.rt.TakeNavDir()
	s.broadcast(rev, res.HTML, nav, s.rt.RoutePath())
	return rev, res.HTML, nav
}

// broadcast pushes a revision+HTML payload to every subscriber, dropping it for
// any client whose buffer is full rather than blocking. route is the current
// deep-link path (rt.RoutePath) so a client can keep the address bar in sync.
func (s *Server) broadcast(rev int64, html, nav, route string) {
	s.actMu.Lock()
	src, det := s.lastSrc, s.lastDet
	s.actMu.Unlock()
	m := map[string]any{"rev": rev, "html": html, "theme": s.rt.CurrentTheme(), "source": src, "detail": det, "route": route}
	if nav != "" {
		m["nav"] = nav
	}
	payload, _ := json.Marshal(m)
	msg := string(payload)
	s.subsMu.Lock()
	for ch := range s.subs {
		select {
		case ch <- msg:
		default:
		}
	}
	s.subsMu.Unlock()
}

// LogEntry is one line in the shared-session activity log. Entries are
// hash-chained (Hash covers the previous entry's hash + this entry's fields),
// so a persisted audit log is tamper-evident: editing, dropping or reordering
// any line breaks every hash after it. Verify with `qorm audit <file>`.
type LogEntry struct {
	Seq    int    `json:"seq"`
	Time   string `json:"time"`         // display time (HH:MM:SS)
	TS     string `json:"ts,omitempty"` // full RFC3339Nano timestamp (audit)
	Source string `json:"source"`       // "human" | "agent" | "devtool" | "app" | "system"
	Detail string `json:"detail"`
	Hash   string `json:"hash,omitempty"` // sha256(prevHash|seq|ts|source|detail)
}

// logEvent records a collaboration event (keeps the last 200 for display; the
// hash chain — and the optional audit file — cover every entry ever logged).
func (s *Server) logEvent(source, detail string) {
	s.actMu.Lock()
	s.actSeq++
	now := time.Now()
	e := LogEntry{Seq: s.actSeq, Time: now.Format("15:04:05"), TS: now.Format(time.RFC3339Nano), Source: source, Detail: detail}
	e.Hash = auditHash(s.lastHash, e)
	s.lastHash = e.Hash
	s.activity = append(s.activity, e)
	if len(s.activity) > 200 {
		s.activity = s.activity[len(s.activity)-200:]
	}
	s.lastSrc, s.lastDet = source, detail // for live edit attribution in the broadcast
	if s.auditW != nil {
		if b, err := json.Marshal(e); err == nil {
			s.auditW.Write(append(b, '\n'))
		}
	}
	s.actMu.Unlock()
}

// serveLog returns activity entries after ?since=<seq> as JSON.
func (s *Server) serveLog(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// App-level log messages. Token-gated (the app's own web.js runs in
		// the human's page and has it) and ALWAYS recorded as "app" — the
		// wire can never mint "human"/"agent" entries, so log attribution
		// stays trustworthy no matter who reaches this port.
		if r.Header.Get("X-Qorm-Token") != s.eventToken {
			http.Error(w, "invalid event token", http.StatusForbidden)
			return
		}
		var e struct{ Source, Detail string }
		if json.NewDecoder(r.Body).Decode(&e) == nil && e.Detail != "" {
			s.logEvent("app", e.Detail)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	since, _ := strconv.Atoi(r.URL.Query().Get("since"))
	s.actMu.Lock()
	out := make([]LogEntry, 0, len(s.activity))
	for _, e := range s.activity {
		if e.Seq > since {
			out = append(out, e)
		}
	}
	s.actMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// servePresence records what the human is currently attending to (the focused or
// just-touched element), so the agent sees it via qorm_activity — the human side
// of presence, mirroring the human's "AI edited" flash.
func (s *Server) servePresence(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// The human's own panel reads this to show what is shared with the agent.
		s.actMu.Lock()
		out := map[string]any{}
		if s.humanFocus != "" {
			out["focus"] = s.humanFocus
		}
		if s.humanTyping != "" {
			out["typing"] = s.humanTyping
		}
		if s.humanFilled != "" {
			out["filled"] = s.humanFilled
		}
		s.actMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
		return
	}
	// Enforce human-only: reject presence reports without the page-embedded event token.
	if r.Header.Get("X-Qorm-Token") != s.eventToken {
		http.Error(w, "invalid event token", http.StatusForbidden)
		return
	}
	var p struct{ Element string }
	if json.NewDecoder(r.Body).Decode(&p) == nil {
		el := strings.TrimSpace(p.Element)
		if len(el) > 120 {
			el = el[:120]
		}
		s.actMu.Lock()
		s.humanFocus = el
		s.humanFocusAt = time.Now()
		// A typed entry ("<field> = <value>") is retained separately so a later tap
		// doesn't erase it; "(hidden)" password markers are not.
		if strings.HasSuffix(el, "= (hidden)") {
			s.humanFilled = strings.TrimSuffix(el, " = (hidden)")
			s.humanFilledAt = time.Now()
		} else if strings.Contains(el, " = ") {
			s.humanTyping = el
			s.humanTypingAt = time.Now()
		}
		s.actMu.Unlock()
	}
	w.WriteHeader(http.StatusNoContent)
}

// serveViewport records the browser client's viewport size (POST {w,h}) and
// re-renders + broadcasts, so responsive `when` nodes track the real window.
// Like /event and /presence it is a human-side call: it requires the
// page-embedded event token. GET returns the current viewport as JSON.
func (s *Server) serveViewport(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.Lock()
		vp := s.rt.Viewport
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"w": vp.W, "h": vp.H})
		return
	}
	// Enforce human-only: reject reports without the page-embedded event token.
	if r.Header.Get("X-Qorm-Token") != s.eventToken {
		http.Error(w, "invalid event token", http.StatusForbidden)
		return
	}
	var p struct{ W, H int }
	if json.NewDecoder(r.Body).Decode(&p) != nil || p.W < 0 || p.H < 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if vp := (runtime.Viewport{W: p.W, H: p.H}); s.rt.Viewport != vp {
		s.rt.Viewport = vp
		s.bump() // re-render + push, so `when` branches swap live on resize
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// serveMeasure stores (POST) or returns (GET) the app's self-reported layout:
// each element's bounding rect + key computed styles, gathered by the running
// app in its own runtime. Lets the framework verify its own styles/positions
// without an external browser.
// userWebJS returns the app's native/web.js (custom callbacks + wiring for its
// own native ops), injected into the page so qormToNative(customOp) round-trips.
func userWebJS(rt *runtime.Runtime) string {
	if rt.App.BaseDir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(rt.App.BaseDir, "native", "web.js"))
	if err != nil {
		return ""
	}
	return string(b)
}

// SetAppBaseDir sets where the app was loaded from, so native/web.js (a sibling
// of a bundle) is injected on desktop even when loaded from a compiled bundle.
func (s *Server) SetAppBaseDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rt.App.BaseDir = dir
}

// AppWindow returns the app's desktop window config (size/style).
func (s *Server) AppWindow() model.Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rt.App.Window
}

// AppShortcutsJSON returns the app-icon quick actions as a JSON array ("[]" if
// none), for the native launcher/Dock menu.
func (s *Server) AppShortcutsJSON() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rt.App.Shortcuts) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(s.rt.App.Shortcuts)
	return string(b)
}

// AppMenuJSON is the desktop system-menu (menu bar) config as JSON.
func (s *Server) AppMenuJSON() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rt.App.DesktopMenu) == 0 {
		return ""
	}
	b, _ := json.Marshal(s.rt.App.DesktopMenu)
	return string(b)
}

// AppTrayJSON is the desktop tray config as JSON ("" when no tray configured).
func (s *Server) AppTrayJSON() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rt.App.Tray.Items) == 0 {
		return ""
	}
	b, _ := json.Marshal(s.rt.App.Tray)
	return string(b)
}

// SetWindowControl registers native window control (also exposes qorm_window MCP).
func (s *Server) SetWindowControl(mover func(id string, x, y, w, h int), op func(id, op string), open func(id, url string, w, h int), eval func(id, js string)) {
	s.WindowMover = mover
	s.WindowOp = op
	s.WindowOpen = open
	s.WindowEval = eval
	if s.agent != nil {
		s.agent.SetWindowControl(mover, op, open, eval)
	}
}

// serveWindow lets the control engine move/resize the native app window.
func (s *Server) serveWindow(w http.ResponseWriter, r *http.Request) {
	if s.WindowMover == nil {
		http.Error(w, "window control unavailable (not a native desktop app)", http.StatusNotImplemented)
		return
	}
	var m struct {
		ID, Op, URL, JS, Event string
		Data                   json.RawMessage
		X, Y, W, H             int
	}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if m.ID == "" {
		m.ID = "main"
	}
	switch m.Op {
	case "open":
		if s.WindowOpen != nil {
			s.WindowOpen(m.ID, m.URL, m.W, m.H)
		}
	case "eval":
		if s.WindowEval != nil {
			s.WindowEval(m.ID, m.JS)
		}
	case "emit":
		if s.WindowEval != nil {
			data := "null"
			if len(m.Data) > 0 {
				data = string(m.Data)
			}
			s.WindowEval(m.ID, "window.qormOnWindowEvent&&qormOnWindowEvent("+strconv.Quote(m.Event)+","+data+")")
		}
	case "", "move":
		s.WindowMover(m.ID, m.X, m.Y, m.W, m.H)
	default:
		if s.WindowOp != nil {
			s.WindowOp(m.ID, m.Op)
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) serveMeasure(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		s.measureMu.Lock()
		s.measure = body
		s.measureMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.measureMu.Lock()
	b := s.measure
	s.measureMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if b == nil {
		w.Write([]byte("[]"))
		return
	}
	w.Write(b)
}

// recordAgentCall inspects an incoming MCP JSON-RPC request and logs mutating
// tool calls so the human sees what the agent is doing in the shared session.
func (s *Server) recordAgentCall(body []byte) {
	var req struct {
		Method string `json:"method"`
		Params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if json.Unmarshal(body, &req) != nil || req.Method != "tools/call" {
		return
	}
	a := req.Params.Arguments
	switch req.Params.Name {
	case "qorm_set_state":
		s.logEvent("agent", fmt.Sprintf("set_state %v = %v", a["path"], a["value"]))
	case "qorm_dispatch":
		s.logEvent("agent", fmt.Sprintf("dispatch %v", a["action"]))
	case "qorm_apply_patch":
		s.logEvent("agent", "apply_patch (UI edit)")
	case "qorm_undo":
		s.logEvent("agent", "undo")
	}
}

// serveEvents streams live updates to the browser over Server-Sent Events.
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := make(chan string, 8)
	s.subsMu.Lock()
	if s.subs == nil {
		s.subs = map[chan string]struct{}{}
	}
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	s.logEvent("system", "client connected ("+clientHost(r)+")")
	defer func() {
		s.subsMu.Lock()
		delete(s.subs, ch)
		s.subsMu.Unlock()
		s.logEvent("system", "client disconnected ("+clientHost(r)+")")
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// Handler returns the HTTP mux.
// blockCrossOrigin rejects requests carrying a cross-origin (non-loopback)
// Origin header — the CSRF / DNS-rebind vector against a localhost server that
// exposes native power (/window eval, /update, /mcp). Requests with no Origin
// (local agents, curl, custom-scheme webviews) and loopback-origin requests
// (the app's own page) pass untouched, so MCP + dev-client workflows still work.
func blockCrossOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && o != "null" {
			bad := true
			if u, err := url.Parse(o); err == nil {
				h := u.Hostname()
				bad = !(h == "localhost" || h == "127.0.0.1" || h == "::1")
			}
			if bad {
				http.Error(w, "cross-origin request rejected", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.serveIndex)
	mux.HandleFunc("/event", blockCrossOrigin(s.serveEvent))
	mux.HandleFunc("/navigate", blockCrossOrigin(s.serveNavigate))
	mux.HandleFunc("/events", s.serveEvents)
	mux.HandleFunc("/poll", s.servePoll)
	mux.HandleFunc("/log", s.serveLog)
	mux.HandleFunc("/presence", blockCrossOrigin(s.servePresence))
	mux.HandleFunc("/viewport", blockCrossOrigin(s.serveViewport))
	mux.HandleFunc("/console", s.serveConsole)
	mux.HandleFunc("/logwindow", s.serveLogWindow)
	mux.HandleFunc("/window", blockCrossOrigin(s.serveWindow))
	mux.HandleFunc("/measure", s.serveMeasure)
	mux.HandleFunc("/mcp", blockCrossOrigin(s.serveMCP))
	mux.HandleFunc("/update", blockCrossOrigin(s.serveUpdate))
	mux.HandleFunc("/rollback", blockCrossOrigin(s.serveRollback))
	mux.HandleFunc("/dev/state", blockCrossOrigin(s.serveDevState))
	mux.HandleFunc("/dev/tree", blockCrossOrigin(s.serveDevTree))
	mux.HandleFunc("/dev/highlight", blockCrossOrigin(s.serveDevHighlight))
	return mux
}

// Reload swaps in a freshly-parsed runtime after its source files changed on
// disk (dev hot-reload), then re-renders and pushes the new UI to every client.
// The live session is carried across Flutter-style: in-progress state, the
// current scene + nav stack, and the viewport survive the reload, so editing a
// file doesn't reset where the user is or what they've typed. State keys the
// edit newly introduced get their fresh initials; if the current scene no longer
// exists, it falls back to the entry. A parse failure never reaches here — the
// caller keeps the current app on error (reload-by-inaction).
func (s *Server) Reload(next *runtime.Runtime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.rt; old != nil && next != nil {
		for k, v := range old.State { // keep in-progress values; new keys keep initials
			next.State[k] = v
		}
		next.Scene = old.Scene
		next.NavStack = old.NavStack
		if old.RouteParams != nil {
			next.RouteParams = old.RouteParams
		}
		next.Viewport = old.Viewport
		if next.Scene != "" {
			if _, ok := next.App.Scenes[next.Scene]; !ok { // scene deleted by the edit
				next.Scene = ""
				next.NavStack = nil
				next.RouteParams = map[string]any{}
			}
		}
	}
	s.rt = next
	s.handlers = nil
	s.initAgent()
	s.logEvent("system", "hot-reload: app source changed")
	s.bump()
}

// activate swaps in a new bundle, remembering the previous one for rollback.
// Caller must hold s.mu.
func (s *Server) activate(b *bundle.Bundle) {
	s.prev = s.current
	s.current = b
	s.rt = runtime.New(b.ToApp())
	s.handlers = nil
	s.initAgent()
	s.bump()
}

// Update fetches, verifies and activates a bundle from source. On any failure
// the current app is left untouched (rollback by inaction). Returns a status
// line describing the transition.
func (s *Server) Update(source string) (string, error) {
	next, err := ota.FetchVerified(source, s.trust, s.revoked)
	if err != nil {
		return "", err
	}
	if err := CheckRequiredCapabilities(next); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	from := "(none)"
	if s.current != nil {
		from = versionOr(s.current)
	}
	s.activate(next)
	return fmt.Sprintf("updated %s -> %s (%s)", from, versionOr(next), next.ContentHash), nil
}

// Rollback reactivates the previous bundle, if any.
func (s *Server) Rollback() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prev == nil {
		return "", fmt.Errorf("no previous bundle to roll back to")
	}
	restored := s.prev
	s.prev = nil
	from := versionOr(s.current)
	s.current = restored
	s.rt = runtime.New(restored.ToApp())
	s.handlers = nil
	s.initAgent()
	s.bump()
	return fmt.Sprintf("rolled back %s -> %s", from, versionOr(restored)), nil
}

func versionOr(b *bundle.Bundle) string {
	if v := b.Version(); v != "" {
		return v
	}
	return "unversioned"
}

func (s *Server) serveUpdate(w http.ResponseWriter, r *http.Request) {
	if s.current == nil {
		http.Error(w, "OTA not enabled (run from a bundle)", http.StatusBadRequest)
		return
	}
	if s.trust == nil {
		http.Error(w, "OTA disabled: authenticity is not verifiable without a trusted key — restart with --trust <key.pub>", http.StatusForbidden)
		return
	}
	var req struct {
		Source string `json:"source"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Source == "" {
		http.Error(w, "missing source", http.StatusBadRequest)
		return
	}
	status, err := s.Update(req.Source)
	if err != nil {
		// Update refused: the live app keeps running the previous bundle.
		http.Error(w, "update rejected (kept current): "+err.Error(), http.StatusConflict)
		return
	}
	fmt.Fprintln(w, status)
}

func (s *Server) serveRollback(w http.ResponseWriter, _ *http.Request) {
	status, err := s.Rollback()
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	fmt.Fprintln(w, status)
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	// Deep link: a `?scene=<id>&k=v` URL navigates the runtime (scene + route
	// params) before rendering, so the page loads straight into that scene with
	// its params bound to route.*. Unknown scenes are ignored by NavigateToPath
	// (falls back to the entry scene). Without a scene query we follow the live
	// navigation state as before.
	if r.URL.Query().Get("scene") != "" {
		s.rt.NavigateToPath(r.URL.RawQuery)
	}
	scene := s.rt.CurrentScene()
	res := render.RenderScene(s.rt, scene)
	s.handlers = res.Handlers
	rev := s.rev.Load()
	rt := s.rt
	// Build the page while still holding the lock: Page/userWebJS read rt.State
	// (locale/theme/rtl), which a concurrent POST /event mutates — reading it
	// unlocked is a concurrent-map read+write and crashes the process.
	html := Page(rt, res.HTML, rev, s.eventToken)
	if js := userWebJS(rt); js != "" {
		html = strings.Replace(html, "</body>", "<script>"+js+"</script></body>", 1)
	}
	transparent := rt.App.Window.Transparent
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if transparent {
		// transparent window  clear the page/stage background so the app's own
		// shaped content defines the visible window shape (rest is click-through).
		html = strings.Replace(html, "</head>", "<style>html,body,#qorm-stage{background:transparent!important;box-shadow:none!important;}</style></head>", 1)
	}
	fmt.Fprint(w, html)
}

// serveMCP is the HTTP transport for the shared MCP session: one JSON-RPC
// request in, one response out. The agent operates the same runtime the browser
// renders, guarded by the same mutex.
func (s *Server) serveMCP(w http.ResponseWriter, r *http.Request) {
	// Symmetric isolation. /event requires the human token, so an agent (which
	// never sees it) cannot pose as a human. The mirror: /mcp REFUSES the human
	// token, so the human browser — the only holder of the token — cannot route
	// operations through the agent channel and have them logged as "agent".
	// Each identity has exactly one door; neither can walk through the other's.
	if r.Header.Get("X-Qorm-Token") == s.eventToken {
		http.Error(w, "the human client must use the UI (/event), not the agent channel (/mcp)", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.recordAgentCall(body)
	resp := s.agent.HandleHTTP(body)
	w.Header().Set("Content-Type", "application/json")
	if resp == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_, _ = w.Write(resp)
}

// servePoll lets the browser observe out-of-band changes (e.g. an agent edit):
// it returns the current revision, plus fresh HTML when the revision advanced.
func (s *Server) servePoll(w http.ResponseWriter, r *http.Request) {
	clientRev, _ := strconv.ParseInt(r.URL.Query().Get("rev"), 10, 64)
	s.mu.Lock()
	cur := s.rev.Load()
	var html string
	if cur != clientRev {
		res := render.RenderScene(s.rt, s.rt.CurrentScene())
		s.handlers = res.Handlers
		html = res.HTML
	}
	route := s.rt.RoutePath()
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	out := map[string]any{"rev": cur, "route": route}
	if html != "" {
		out["html"] = html
	}
	_ = json.NewEncoder(w).Encode(out)
}

type eventReq struct {
	H      int            `json:"h"`
	Inputs map[string]any `json:"inputs"`
}

func (s *Server) serveEvent(w http.ResponseWriter, r *http.Request) {
	// Enforce human-only: reject requests without the page-embedded event token.
	// This prevents agents/scripts from forging "human"-attributed operations.
	if r.Header.Get("X-Qorm-Token") != s.eventToken {
		http.Error(w, "invalid event token — only the browser client can dispatch human events", http.StatusForbidden)
		return
	}
	var req eventReq
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Ensure the handler table exists: a client that POSTs /event before ever
	// GETting / (a reconnect, or an out-of-order request) would otherwise find
	// an empty table and silently drop the action.
	if s.handlers == nil {
		s.handlers = render.RenderScene(s.rt, s.rt.CurrentScene()).Handlers
	}
	// Fold current input values back into state before dispatching.
	for path, val := range req.Inputs {
		s.rt.State[path] = val
	}
	if req.H >= 0 && req.H < len(s.handlers) {
		h := s.handlers[req.H]
		if h.Name != "" {
			s.logEvent("human", "dispatch "+h.Name)
		}
		// Re-evaluate args in the handler's captured scope + fresh state.
		ctx := map[string]any{"state": s.rt.State}
		for k, v := range h.Scope {
			ctx[k] = v
		}
		args := map[string]any{}
		for name, exprStr := range h.Args {
			args[name] = runtime.EvalBinding(exprStr, ctx)
		}
		s.rt.Dispatch(h.Name, args)
	}
	rev, html, nav := s.bump()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Qorm-Rev", strconv.FormatInt(rev, 10))
	w.Header().Set("X-Qorm-Theme", s.rt.CurrentTheme())
	w.Header().Set("X-Qorm-Route", s.rt.RoutePath())
	if nav != "" {
		w.Header().Set("X-Qorm-Nav", nav)
	}
	fmt.Fprint(w, html)
}

// serveNavigate is the human-side URL-routing endpoint: the browser POSTs it on
// a popstate (browser Back/Forward) so the runtime tracks the address bar. Body
// is {scene, params} to go to a scene (params are strings) or {back:true} to
// pop. Token-gated like /event so only the real page can drive it, and recorded
// as a "human" navigation in the shared activity log.
func (s *Server) serveNavigate(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Qorm-Token") != s.eventToken {
		http.Error(w, "invalid event token — only the browser client can navigate", http.StatusForbidden)
		return
	}
	var req struct {
		Scene  string         `json:"scene"`
		Params map[string]any `json:"params"`
		Back   bool           `json:"back"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before := s.rt.RoutePath()
	if req.Back {
		s.rt.NavigateBack()
	} else {
		s.rt.NavigateTo(req.Scene, req.Params)
	}
	if s.rt.RoutePath() != before { // only log + re-render when it actually moved
		s.logEvent("human", "navigate "+s.rt.RoutePath())
		s.bump()
	}
	w.WriteHeader(http.StatusNoContent)
}

// Page wraps rendered body HTML in a full document with the live shim.
//
//go:embed app.js
var appJS string

// qormAppJS returns the client script with the current revision substituted.
func qormAppJS(rev int64, eventToken string) string {
	s := strings.ReplaceAll(appJS, "__QORM_REV__", strconv.FormatInt(rev, 10))
	s = strings.ReplaceAll(s, "__QORM_TOKEN__", eventToken)
	return s
}

func Page(rt *runtime.Runtime, body string, rev int64, eventToken ...string) string {
	tok := ""
	if len(eventToken) > 0 {
		tok = eventToken[0]
	}
	w := rt.App.Window
	width := w.Width
	if width == 0 {
		width = 420
	}
	height := w.Height
	if height == 0 {
		height = 720
	}
	title := w.Title
	if title == "" {
		title = rt.App.Name
	}
	lang := rt.CurrentLocale()
	if lang == "" {
		lang = "en"
	}
	dir := "ltr"
	if rt.IsRTL() {
		dir = "rtl"
	}
	theme := rt.CurrentTheme()
	return fmt.Sprintf(`<!doctype html>
<html lang="%s" dir="%s">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no, viewport-fit=cover">
<title>%s</title>
<style>
  /* ---- Design tokens (themes). Palettes live in internal/render/theme.go
     (single source of truth, shared with the miniapp export); "auto" — the
     implicit default — follows the OS light/dark setting. The app's manifest
     designTokens land after it as var(--qorm-token-*) on the stage. ---- */
  %s
  * { margin:0; padding:0; box-sizing:border-box; -webkit-font-smoothing:antialiased;
      touch-action:manipulation; -webkit-touch-callout:none;
      -webkit-user-select:none; user-select:none; -webkit-tap-highlight-color:transparent; }
  /* re-enable text selection only where it's real content input */
  input, textarea, [contenteditable="true"], .qorm-selectable { -webkit-user-select:text; user-select:text; }
  html, body { overscroll-behavior:none; -webkit-overflow-scrolling:touch; }
  body { background:var(--bg); color:var(--label); font-family:var(--font); letter-spacing:-0.01em;
         min-height:100vh; display:flex; align-items:flex-start; justify-content:center; padding:24px; }
  /* live collaborator presence: "AI edited" toast when an agent changes the shared app */
  #qorm-presence { position:fixed; left:50%%; bottom:20px; transform:translate(-50%%,20px);
    display:flex; align-items:center; gap:7px; padding:9px 15px; border-radius:20px;
    background:var(--accent); color:var(--on-accent,#fff); font-weight:600; font-size:13px;
    box-shadow:0 8px 24px rgba(0,0,0,.28); opacity:0; pointer-events:none; z-index:99999;
    transition:opacity .2s ease, transform .2s ease; }
  #qorm-presence.show { opacity:1; transform:translate(-50%%,0); }
  /* Responsive: a centered device frame on PC, full-bleed on phones. */
  #qorm-stage { width:%dpx; max-width:100%%; min-height:%dpx; background:var(--bg); color:var(--label);
                border-radius:var(--stage-radius); box-shadow:0 12px 48px rgba(0,0,0,.18);
                overflow:hidden; display:flex; }
  @media (max-width:640px) {
    body { padding:0; align-items:stretch; }
    #qorm-stage { width:100%%; max-width:100%%; min-height:100vh; border-radius:0; box-shadow:none; }
  }
  /* Desktop: expand into the available width instead of a lone phone frame,
     and mark the stage so widgets can switch to their desktop form. */
  @media (min-width:1024px) {
    body { padding:32px; align-items:flex-start; }
    #qorm-stage { width:min(1080px,92vw); border-radius:18px; }
    #qorm-stage.qorm-fluid { width:min(1400px,96vw); }
    /* Hybrid content: the main column is centered and capped for readability;
       naturally-wide widgets (grids, tables, charts, media) fill the width. */
    .qorm-body > * { max-width:960px; margin-left:auto; margin-right:auto; }
    .qorm-body .qorm-wide { max-width:none; }
    /* Bottom tab bar is a mobile idiom — on desktop lift it to a top bar. */
    .qorm-bottomnav { order:-1; border-top:0; border-bottom:1px solid var(--sep); justify-content:center; gap:6px; }
    .qorm-bottomnav .qorm-navitem { flex:0 0 auto; flex-direction:row; gap:8px; padding:12px 18px; }
    /* iOS action sheet rises from the bottom on phones; center it on desktop. */
    .qorm-sheet { align-items:center; }
  }
  /* Desktop hover feedback (pointer devices only). */
  @media (hover:hover) {
    button:hover { filter:brightness(0.96); }
    a:hover { opacity:0.85; }
    .qorm-tab:hover { color:var(--label); }
    th:hover .qorm-sort-ind { opacity:.7; }
    .qorm-tree-sum:hover, .qorm-tree-leaf:hover { background:var(--fill); }
    .qorm-acc:hover, .qorm-menu-panel button:hover, .qorm-navitem:hover { background:var(--fill); }
    .qorm-datatable tbody tr:hover { background:var(--fill); }
  }
  input,button,select,textarea { font-family:inherit; letter-spacing:inherit; }
  /* iOS switch: a 51x31 pill, green when checked. */
  .qorm-switch { position:relative; width:51px; height:31px; flex:none; }
  .qorm-switch input { position:absolute; opacity:0; width:0; height:0; }
  .qorm-switch span { position:absolute; inset:0; background:var(--fill); border-radius:16px; transition:background .25s; }
  .qorm-switch span::before { content:""; position:absolute; top:2px; left:2px; width:27px; height:27px; border-radius:50%%;
    background:#fff; box-shadow:0 2px 5px rgba(0,0,0,.25); transition:transform .25s; }
  .qorm-switch input:checked + span { background:var(--success); }
  .qorm-switch input:checked + span::before { transform:translateX(20px); }
  #qorm-root { flex:1; display:flex; }
  #qorm-root > * { flex:1; }
  @keyframes qorm-spin { to { transform: rotate(360deg); } }
  .qorm-spin { animation: qorm-spin .8s linear infinite; }
  /* Tabs: iOS underline tabs — active = accent underline + weight; inactive =
     secondary label; hover shifts text color, never a gray fill. */
  .qorm-tab { border:none; background:none; padding:12px 16px; min-height:44px; cursor:pointer; font-size:14px;
    color:var(--label2); font-weight:400; border-bottom:2px solid transparent; margin-bottom:-1px;
    transition:color .15s ease, border-color .15s ease; }
  .qorm-tab:active { opacity:.7; }
  .qorm-tab-active { border-bottom-color:var(--accent) !important; color:var(--accent); font-weight:600; }
  /* Tables: iOS hairline rows — no gray header fill, no full grid borders. */
  .qorm-table { border-collapse:collapse; width:100%%; font-size:14px; }
  .qorm-table th { text-align:left; font-weight:600; color:var(--label2); font-size:13px; padding:10px 12px; border-bottom:1px solid var(--sep); white-space:nowrap; }
  .qorm-table td { padding:10px 12px; border-bottom:1px solid var(--sep); color:var(--label); }
  .qorm-datatable { border-collapse:collapse; width:100%%; font-size:14px; }
  .qorm-datatable th { text-align:left; font-weight:600; color:var(--label2); padding:10px 12px; border-bottom:1px solid var(--sep); white-space:nowrap; }
  .qorm-datatable td { padding:10px 12px; border-bottom:1px solid var(--sep); color:var(--label); }
  .qorm-datatable tbody tr.qdt-sel { background:var(--fill); }
  .qorm-datatable .qdt-check { width:36px; text-align:center; cursor:pointer; }
  /* Sort headers: every sortable column shows a faint chevron at all times so
     the affordance is discoverable (no hover on touch); hover deepens it, and
     the sorted column gets a persistent accent chevron. */
  .qorm-table button.qdt-sort, .qorm-datatable button.qdt-sort { background:none; border:none; font:inherit; font-weight:600; color:var(--label2); cursor:pointer; padding:0; display:inline-flex; align-items:center; gap:3px; min-height:32px; }
  .qorm-sort-ind { opacity:.3; transition:opacity .15s ease, color .15s ease; font-size:11px; line-height:1; }
  .qorm-sort-ind.on { opacity:1; color:var(--accent); }
  /* Tree: Finder outline — custom chevron with a rotate transition, rounded
     rows, hover fill on pointer devices. */
  .qorm-tree summary.qorm-tree-sum { list-style:none; display:flex; align-items:center; gap:6px; padding:5px 8px; border-radius:6px; cursor:pointer; font-weight:500; color:var(--label); }
  .qorm-tree summary.qorm-tree-sum::-webkit-details-marker { display:none; }
  .qorm-tree summary.qorm-tree-sum::before { content:"›"; color:var(--label2); font-weight:700; display:inline-block; transition:transform .15s ease; }
  .qorm-tree details[open] > summary.qorm-tree-sum::before { transform:rotate(90deg); }
  .qorm-tree-kids { padding-left:18px; }
  .qorm-tree-leaf { padding:5px 8px 5px 26px; border-radius:6px; color:var(--label); }
  @keyframes qorm-shimmer { 0%% { background-position:200%% 0; } 100%% { background-position:-200%% 0; } }
  .qorm-skel { background:linear-gradient(90deg,#e9ecef 25%%,#f3f5f7 50%%,#e9ecef 75%%); background-size:200%% 100%%; animation:qorm-shimmer 1.3s ease-in-out infinite; }
  /* Range slider: two overlaid thumbs must stay interactive over a pass-through track. */
  .qorm-range-lo, .qorm-range-hi { -webkit-appearance:none; appearance:none; background:transparent; pointer-events:none; }
  .qorm-range-lo::-webkit-slider-thumb, .qorm-range-hi::-webkit-slider-thumb { -webkit-appearance:none; pointer-events:auto;
    width:22px; height:22px; border-radius:50%%; background:#fff; box-shadow:0 1px 4px rgba(0,0,0,.3); cursor:pointer; margin-top:-9px; }
  .qorm-range-lo::-moz-range-thumb, .qorm-range-hi::-moz-range-thumb { pointer-events:auto; width:22px; height:22px; border:none; border-radius:50%%; background:#fff; box-shadow:0 1px 4px rgba(0,0,0,.3); }
  /* iOS slider: thin track, round white thumb. */
  .qorm-slider { -webkit-appearance:none; appearance:none; height:28px; background:transparent; outline:none; cursor:pointer; }
  .qorm-slider::-webkit-slider-runnable-track { height:4px; border-radius:2px;
    background:linear-gradient(90deg,var(--accent) var(--pct,0%%),var(--fill) var(--pct,0%%)); }
  .qorm-slider::-webkit-slider-thumb { -webkit-appearance:none; width:28px; height:28px; border-radius:50%%;
    background:#fff; box-shadow:0 1px 5px rgba(0,0,0,.3); cursor:pointer; margin-top:-12px; }
  .qorm-slider::-moz-range-track { height:4px; border-radius:2px;
    background:linear-gradient(90deg,var(--accent) var(--pct,0%%),var(--fill) var(--pct,0%%)); }
  .qorm-slider::-moz-range-thumb { width:28px; height:28px; border:none; border-radius:50%%; background:#fff; box-shadow:0 1px 5px rgba(0,0,0,.3); }
  /* iOS activity indicator: 8 spokes ticking around. */
  .qorm-activity svg { animation:qorm-ios-spin 1s steps(8) infinite; }
  @keyframes qorm-ios-spin { to { transform:rotate(360deg); } }
  /* ---- Motion catalog: named keyframe animations. A widget's animation
     prop selects one; being bindable, an agent switches the effect live. ---- */
  /* Interactive: iOS press feedback (tap-to-scale) on buttons & tappables. */
  .qorm-tap { transition:transform .12s ease, opacity .12s ease; -webkit-tap-highlight-color:transparent; }
  .qorm-tap:active { transform:scale(.96); opacity:.7; }
  /* Draggable/DragTarget feedback: lift the item being dragged, highlight the drop zone. */
  .qorm-draggable { transition:opacity .15s ease; } .qorm-dragging { opacity:.5; }
  .qorm-dragover { outline:2px dashed var(--accent,#0a84ff); outline-offset:-2px; background:color-mix(in srgb,var(--accent,#0a84ff) 8%%,transparent); }
  /* Spatial attribution: a node the AI just changed pulses a blue outline. */
  .qorm-ai-touch { animation:qorm-ai-flash 1.3s ease-out; border-radius:inherit; }
  @keyframes qorm-ai-flash { 0%% { box-shadow:0 0 0 2px rgba(10,132,255,.9); } 60%% { box-shadow:0 0 0 2px rgba(10,132,255,.45); } 100%% { box-shadow:0 0 0 2px rgba(10,132,255,0); } }
  @keyframes qa-fade { from { opacity:0; } to { opacity:1; } }
  @keyframes qa-fadeup { from { opacity:0; transform:translateY(16px); } to { opacity:1; transform:none; } }
  @keyframes qa-fadedown { from { opacity:0; transform:translateY(-16px); } to { opacity:1; transform:none; } }
  @keyframes qa-slideup { from { transform:translateY(100%%); } to { transform:none; } }
  @keyframes qa-slidedown { from { transform:translateY(-100%%); } to { transform:none; } }
  @keyframes qa-slideleft { from { transform:translateX(100%%); } to { transform:none; } }
  @keyframes qa-slideright { from { transform:translateX(-100%%); } to { transform:none; } }
  @keyframes qa-scale { from { opacity:0; transform:scale(.8); } to { opacity:1; transform:none; } }
  @keyframes qa-zoomout { from { opacity:0; transform:scale(1.15); } to { opacity:1; transform:none; } }
  @keyframes qa-rotate { from { opacity:0; transform:rotate(-180deg) scale(.6); } to { opacity:1; transform:none; } }
  @keyframes qa-flip { from { opacity:0; transform:perspective(600px) rotateY(90deg); } to { opacity:1; transform:none; } }
  @keyframes qa-pop { 0%% { opacity:0; transform:scale(.5); } 70%% { transform:scale(1.06); } 100%% { opacity:1; transform:none; } }
  @keyframes qa-bounce { 0%% { transform:translateY(-120%%); } 60%% { transform:translateY(8%%); } 80%% { transform:translateY(-4%%); } 100%% { transform:none; } }
  @keyframes qa-shake { 0%%,100%% { transform:none; } 20%%,60%% { transform:translateX(-8px); } 40%%,80%% { transform:translateX(8px); } }
  @keyframes qa-pulse { 0%%,100%% { transform:none; } 50%% { transform:scale(1.08); } }
  @keyframes qa-spin { to { transform:rotate(360deg); } }
  @keyframes qa-size { from { transform:scaleY(0); transform-origin:top; } to { transform:scaleY(1); transform-origin:top; } }
  [data-tooltip] { position:relative; }
  [data-tooltip]:hover::after { content:attr(data-tooltip); position:absolute; bottom:100%%; left:50%%; transform:translateX(-50%%);
    background:#111827; color:#fff; padding:4px 8px; border-radius:6px; font-size:12px; white-space:nowrap; margin-bottom:6px; z-index:100; pointer-events:none; }
</style>
</head>
<body>
<div id="qorm-stage" class="qorm-theme-%s"><div id="qorm-root">%s</div></div>
<script>%s</script>
</body>
</html>`, lang, dir, htmlEscape(title), themeCSS(rt), width, height, theme, body, qormAppJS(rev, tok))
}

// themeCSS is the shell's theme block: the shared built-in palettes plus the
// app's own manifest designTokens rendered as var(--qorm-token-*) on the
// stage, so scenes can style against them.
func themeCSS(rt *runtime.Runtime) string {
	if css := render.TokenCSS("#qorm-stage", rt.App.DesignTokens); css != "" {
		return render.ThemeCSS + "\n  " + css
	}
	return render.ThemeCSS
}

func htmlEscape(s string) string {
	repl := map[rune]string{'&': "&amp;", '<': "&lt;", '>': "&gt;"}
	out := make([]rune, 0, len(s))
	for _, c := range s {
		if r, ok := repl[c]; ok {
			out = append(out, []rune(r)...)
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}

// clientHost returns a readable client identifier for the activity log — the
// remote IP, so a physical device joining the session is visible.
func clientHost(r *http.Request) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "::1" || host == "127.0.0.1" {
		return "local"
	}
	return host
}
