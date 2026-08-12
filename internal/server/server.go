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
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
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
	// handlerHist retains the handler tables of the last handlerHistory
	// revisions so POST /event can resolve its handler index against the frame
	// the browser was actually showing. Cleared whenever the runtime is swapped
	// (hot reload / OTA): a table from a previous app must never resolve.
	handlerHist []handlerFrame
	rev         atomic.Int64 // bumped on every mutation; drives browser live-sync
	agent       *mcp.Server  // MCP handler sharing rt + mu

	// canvasHost marks the server as embedded in the native canvas window:
	// the HTTP listener is never served in that mode, so frame() skips the
	// HTML render/broadcast (zero subscribers) and only drives OnStateChange.
	canvasHost bool

	// marshal, when non-nil (canvas host only), parks a state-touching closure
	// onto the render thread (engine.EnqueueMutation) instead of running it on
	// the caller's goroutine. Covers the two paths the HTTP middleware cannot
	// reach: the internal async-http completion (spawn) and the SSE catch-up
	// render (serveEvents).
	marshal func(func())

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

	// OnStateChange is called whenever the runtime state updates, passing the live runtime.
	// Used by the Canvas kernel to re-layout and re-render.
	OnStateChange func(rt *runtime.Runtime)

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
	// Seed the runtime's viewport from the manifest's window hints. Without
	// this, the very first render — before any browser has reported its size
	// via POST /viewport — sees a zero-value Viewport, and any `when` node
	// gated on `viewport.width` falls into the else branch. A side-scroller
	// game or fixed-aspect dashboard declares its target size in the manifest
	// precisely to avoid that race.
	if rt != nil && rt.App != nil && rt.App.Window.Width > 0 {
		rt.Viewport = runtime.Viewport{W: rt.App.Window.Width, H: rt.App.Window.Height}
	}
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

// initAgent (re)binds this host to the current runtime: the shared MCP handler
// and the intermediate-frame sink. Called on construction and whenever the
// runtime is swapped (hot reload, OTA activate/rollback). afterMutate runs
// while the agent holds s.mu, so bump() must not re-take s.mu.
func (s *Server) initAgent() {
	s.agent = mcp.NewShared(s.rt, &s.mu, func() { s.bump() })
	// Install this host's frame sink on the live runtime, so a `render` step
	// inside an action publishes an intermediate frame (render + SSE broadcast)
	// mid-dispatch. This is the ONE install point: every path that swaps s.rt
	// (New, NewBundle, Reload, activate, Rollback) funnels through initAgent, so
	// a fresh runtime can never be left without the sink — and runtime.New still
	// installs nothing itself, keeping bare runtimes and Clone()s synchronous.
	// frame() takes no lock; the dispatch that calls it already holds s.mu.
	s.rt.Commit = func() { s.frame() }
	// Install this host's background work sink on the same schedule, so an
	// http.* step marked {"async": true} runs its round trip off the dispatch
	// goroutine instead of holding s.mu for the length of the request.
	s.rt.Async = s.spawn
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

// Runtime returns the server's current live runtime.
func (s *Server) Runtime() *runtime.Runtime {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rt
}

// bump increments the revision, re-renders, refreshes the handler table and
// pushes the new UI to all SSE subscribers. Caller must hold s.mu.
// It first drains a pending scene-entry hook: every mutation path (human
// /event, /navigate, agent MCP, viewport, OTA activate) funnels through bump,
// so a navigation that landed on a scene with onEnter dispatches it exactly
// once, before the frame that ships is rendered.
func (s *Server) bump() (int64, string, string) {
	s.rt.RunPendingEnter()
	return s.frame()
}

// SetCanvasHost marks the server as embedded in the native canvas window.
// In that mode the HTTP listener IS served (the agent's MCP channel stays
// available), but the canvas host marshals every request onto the main thread
// and frame() skips RenderScene/broadcast (no browser clients) — it only
// advances the revision and drives OnStateChange.
func (s *Server) SetCanvasHost(v bool) {
	s.mu.Lock()
	s.canvasHost = v
	s.mu.Unlock()
}

// SetMarshal installs the canvas host's render-thread serializer (typically
// engine.EnqueueMutation). nil (the default) runs work inline on the caller's
// goroutine — correct for the browser host, where s.mu alone serializes state.
func (s *Server) SetMarshal(fn func(func())) {
	s.mu.Lock()
	s.marshal = fn
	s.mu.Unlock()
}

// marshalWork runs fn inline, or parked on the canvas render thread when a
// serializer is installed. The caller must NOT hold s.mu: the parked closure
// takes s.mu on the render thread, and blocking on the render thread while
// holding s.mu would deadlock against it.
func (s *Server) marshalWork(fn func()) {
	s.mu.Lock()
	m := s.marshal
	s.mu.Unlock()
	if m != nil {
		m(fn)
		return
	}
	fn()
}

// frame renders + publishes exactly one revision WITHOUT draining the pending
// scene-entry hook. It is the sink the runtime's `render` step calls mid-action
// (installed as rt.Commit in initAgent), and the second half of bump.
//
// Deliberately no RunPendingEnter here: an action that navigates and then
// renders publishes the target scene's first frame immediately, but the
// target's onEnter still fires exactly once at the dispatch boundary, when
// bump runs. Draining it here would re-enter Dispatch from inside Dispatch —
// stacking callDepth and possibly tripping maxEnterChain — for no gain.
// Caller must hold s.mu.
func (s *Server) frame() (int64, string, string) {
	if s.canvasHost {
		rev := s.rev.Add(1)
		if s.OnStateChange != nil {
			s.OnStateChange(s.rt)
		}
		return rev, "", ""
	}
	rev := s.rev.Add(1)
	res := render.RenderScene(s.rt, s.rt.CurrentScene())
	s.setHandlers(rev, res.Handlers)
	nav := s.rt.TakeNavDir()
	s.broadcast(rev, res.HTML, nav, s.rt.RoutePath())
	if s.OnStateChange != nil {
		s.OnStateChange(s.rt)
	}
	return rev, res.HTML, nav
}

// spawn is this host's background work sink (installed as rt.Async): it runs an
// async http step's round trip on its own goroutine and delivers the outcome
// back under s.mu, then publishes the resulting frame over live-sync. The
// dispatch that called it has already returned, so the completion frame is a
// second, later revision — which is exactly what makes the loading state
// visible without freezing every other request behind a 20-second timeout.
//
// Three properties this function exists to guarantee:
//
//   - It takes NO lock itself, only starting a goroutine. An MCP tool call
//     reaches Dispatch while already holding s.mu, so a sink that locked here
//     would deadlock the agent path.
//   - The completion runs the continuation and the render inside one critical
//     section, so it interleaves with human events, timer ticks and agent tools
//     exactly like any other mutation — never halfway through one.
//   - It pins the runtime generation it was started on. A hot reload, an OTA
//     activate or a rollback swaps s.rt while the request is open; the reply to
//     a request issued by an app that is no longer running is dropped rather
//     than written into its successor's state, which would resurrect values
//     from a retired app (and could publish a frame against a scene that no
//     longer exists). The abandoned runtime keeps its raised Inflight count,
//     which is correct: nothing reads it any more.
//
// A NAVIGATION during the request is deliberately NOT treated the same way. The
// state store is global and cross-scene, so a reply that arrives after the user
// moved on is still the answer to a question this same session asked: it is
// written, and the frame renders whatever scene is current by then. Dropping it
// would discard data the app asked for and make "did my fetch land?" depend on
// how fast the user tapped.
func (s *Server) spawn(work func() any, resume func(any)) {
	owner := s.rt // caller holds s.mu — this is the generation snapshot
	go func() {
		v := work()
		s.settleAsync(owner, v, resume)
	}()
}

// settleAsync applies a finished async round trip. The completion writes
// rt.State, so under a canvas host it must be marshalled onto the render
// thread like every other mutation — the HTTP middleware cannot reach this
// internal goroutine. The goroutine deliberately does not hold s.mu across
// the park (see marshalWork).
func (s *Server) settleAsync(owner *runtime.Runtime, v any, resume func(any)) {
	s.marshalWork(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.rt != owner {
			// The reply belongs to a runtime we no longer serve. Abandoning it
			// settles the retired runtime's counters and cancels its keyed
			// transports, so a caller polling Inflight() on it cannot wait for
			// ever (Reload copies state forward, so "retired" is not "unread").
			owner.AbandonInflight()
			s.logEvent("system", "dropped an async response from a replaced runtime")
			return
		}
		resume(v)
		s.bump()
	})
}

// handlerFrame is one revision's handler table, kept so a /event that names the
// revision the browser was showing resolves its handler index against THAT
// frame rather than whatever has been rendered since.
type handlerFrame struct {
	rev int64
	h   []render.Handler
}

// handlerHistory is how many recent frames' handler tables are retained. One
// top-level interaction can publish at most runtime.MaxFrames intermediate
// frames plus its final boundary frame (runtime.frameBudget), so the ring sizes
// to exactly that span: a click from ANY frame of even the most frame-flooding
// interaction still resolves against the table it was minted on, instead of
// silently degrading to the newest table.
const handlerHistory = runtime.MaxFrames + 1

// setHandlers records the handler table produced for a revision: it becomes the
// current table and enters the history ring.
//
// INVARIANT: one revision publishes exactly one frame. Every path that changes
// what renders (a dispatch, a navigation, a viewport write, a deep link) bumps
// the revision FIRST, so a same-revision call here is a deterministic RE-render
// of the frame already filed for it (a serveIndex refresh, /poll, an SSE
// catch-up) and must carry the identical table — re-filing it is a no-op, never
// a duplicate ring entry. A same-revision call with a DIFFERENT table means a
// mutation reached a render without bumping: the already-published table wins
// (clients holding that rev keep resolving against the frame they actually
// saw), the new one is dropped, and the violation is logged. Caller must hold
// s.mu.
func (s *Server) setHandlers(rev int64, h []render.Handler) {
	for i := range s.handlerHist {
		if s.handlerHist[i].rev == rev {
			if sameHandlers(s.handlerHist[i].h, h) {
				s.handlers = h
				return
			}
			s.logEvent("system", fmt.Sprintf("revision %d re-rendered with a different handler table — a mutation reached render without bumping the revision; keeping the published frame's table", rev))
			s.handlers = s.handlerHist[i].h
			return
		}
	}
	s.handlers = h
	s.handlerHist = append(s.handlerHist, handlerFrame{rev: rev, h: h})
	if len(s.handlerHist) > handlerHistory {
		s.handlerHist = s.handlerHist[len(s.handlerHist)-handlerHistory:]
	}
}

// sameHandlers reports whether two handler tables are interchangeable for
// positional dispatch — same length, same action at every index, same captured
// args and scope. Render is deterministic, so a same-revision re-render must
// produce an identical table; anything else is a mutation that skipped its
// revision bump.
func sameHandlers(a, b []render.Handler) bool {
	return reflect.DeepEqual(a, b)
}

// lookupHandlers returns the handler table the given revision was rendered
// with, falling back to the newest table when no revision was reported (nil —
// a non-browser driver) or when that frame has already been evicted from the
// ring. The fallback is exactly the pre-M1 behavior, so nothing regresses when
// the revision is missing — it only gets more precise when it is present.
//
// A NAMED revision that misses the ring is the moment the rev-scoped guarantee
// silently degraded before (the frame was evicted, or predates this app
// generation): it is logged, so an agent flood or a pathological frame burst
// shows up in the shared activity log instead of failing quiet. At most one
// line per such click — a routine click always hits, so this cannot spam.
// Caller must hold s.mu.
func (s *Server) lookupHandlers(rev *int64) []render.Handler {
	if rev != nil {
		for i := range s.handlerHist {
			if s.handlerHist[i].rev == *rev {
				return s.handlerHist[i].h
			}
		}
		oldest := "none"
		if len(s.handlerHist) > 0 {
			oldest = strconv.FormatInt(s.handlerHist[0].rev, 10)
		}
		s.logEvent("system", fmt.Sprintf("event named revision %d, which is no longer in the handler ring (oldest kept: %s) — resolving against the newest table", *rev, oldest))
	}
	return s.handlers
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
	Hash   string `json:"hash,omitempty"` // sha256(prevHash|seq|time|ts|source|detail)
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

// focusMaxRunes caps how much of a presence element label is stored. Counted
// in runes so over-long labels are truncated on a rune boundary — a byte cut
// could split a multi-byte UTF-8 sequence and store invalid UTF-8.
const focusMaxRunes = 120

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
	if json.NewDecoder(r.Body).Decode(&p) != nil {
		// Malformed JSON is a client error — consistent with /event and
		// /viewport, never a silent 204.
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	el := strings.TrimSpace(p.Element)
	// Cap the stored label at focusMaxRunes RUNES (not bytes), so truncation
	// lands on a rune boundary and never splits a multi-byte UTF-8 sequence
	// into invalid bytes for a non-ASCII label.
	if runes := []rune(el); len(runes) > focusMaxRunes {
		el = string(runes[:focusMaxRunes])
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
		// The stored bytes are served back verbatim as application/json (and
		// feed the agent's qorm_measure / qorm_check_layout), so refuse an
		// empty or non-JSON report: GET /measure must never return garbage.
		// Client-error policy consistent with /event, /presence, /viewport.
		if len(body) == 0 || !json.Valid(body) {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
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

	// Catch-up: resync a viewer whose revision is behind the live app instead of
	// leaving it stale until the next mutation — SSE exists "to keep every viewer
	// in sync" (docs/collaboration.md). The client tells us the last revision it
	// applied through two channels:
	//   - ?rev=<n>: the revision of the page render that produced its HTML. The
	//     FIRST connection ships this, closing the window between GET / (the page
	//     render) and the EventSource opening — a mutation landing in that gap
	//     broadcast before this viewer subscribed, so without the handshake it
	//     would be silently lost (the first connection sends no Last-Event-Id).
	//   - Last-Event-Id: on reconnect the EventSource replays the id of the last
	//     frame it received (each frame ships its rev as the id: line below) while
	//     re-requesting the same URL, so its ?rev= is stale by then.
	// Take the larger of the two as the last revision actually applied. A client
	// already at the tip gets no snapshot (cur > last is false), and any duplicate
	// a racing broadcast buffered on ch is dropped by the client's rev guard
	// (qormApply skips rev <= __rev).
	last, haveLast := int64(0), false
	if v, err := strconv.ParseInt(r.Header.Get("Last-Event-Id"), 10, 64); err == nil {
		last, haveLast = v, true
	}
	if v, err := strconv.ParseInt(r.URL.Query().Get("rev"), 10, 64); err == nil && (!haveLast || v > last) {
		last, haveLast = v, true
	}
	if haveLast {
		// The catch-up render reads rt.State, so under a canvas host it is
		// marshalled onto the render thread (this handler is exempted from
		// marshalToMain because the SSE stream is long-lived; the catch-up
		// itself is a bounded snapshot and safe to park). The write/flush
		// stays on this goroutine — only the state read is serialized.
		var snap []byte
		var cur int64
		s.marshalWork(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if c := s.rev.Load(); c > last {
				res := render.RenderScene(s.rt, s.rt.CurrentScene())
				s.setHandlers(c, res.Handlers)
				snap, _ = json.Marshal(map[string]any{"rev": c, "html": res.HTML, "theme": s.rt.CurrentTheme(), "route": s.rt.RoutePath()})
				cur = c
			}
		})
		if snap != nil {
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", cur, snap)
			flusher.Flush()
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			// Ship the payload's rev as the event id, so a reconnecting browser
			// replays it as Last-Event-Id and the catch-up above can resync it.
			var f struct {
				Rev int64 `json:"rev"`
			}
			_ = json.Unmarshal([]byte(msg), &f)
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", f.Rev, msg)
			flusher.Flush()
		}
	}
}

// Handler returns the HTTP mux.
// blockCrossOrigin rejects requests carrying a cross-origin (non-loopback)
// Origin header — the CSRF / DNS-rebind vector against a localhost server that
// exposes native power (/window eval, /update, /mcp). It also guards the data
// surfaces: /events is readable cross-origin (EventSource, unlike fetch, does
// NOT enforce CORS — without the guard any web page the user visits could
// snoop the app's live UI), and /measure is an unauthenticated write whose
// stored layout the agent's qorm_check_layout trusts. Requests with no Origin
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
	mux.HandleFunc("/events", blockCrossOrigin(s.serveEvents))
	mux.HandleFunc("/poll", s.servePoll)
	mux.HandleFunc("/log", s.serveLog)
	mux.HandleFunc("/presence", blockCrossOrigin(s.servePresence))
	mux.HandleFunc("/viewport", blockCrossOrigin(s.serveViewport))
	mux.HandleFunc("/console", s.serveConsole)
	mux.HandleFunc("/logwindow", s.serveLogWindow)
	mux.HandleFunc("/window", blockCrossOrigin(s.serveWindow))
	mux.HandleFunc("/measure", blockCrossOrigin(s.serveMeasure))
	mux.HandleFunc("/mcp", blockCrossOrigin(s.serveMCP))
	mux.HandleFunc("/update", blockCrossOrigin(s.serveUpdate))
	mux.HandleFunc("/rollback", blockCrossOrigin(s.serveRollback))
	mux.HandleFunc("/dev/state", blockCrossOrigin(s.serveDevState))
	mux.HandleFunc("/dev/tree", blockCrossOrigin(s.serveDevTree))
	mux.HandleFunc("/dev/highlight", blockCrossOrigin(s.serveDevHighlight))
	// Static asset serve: /assets/* and /themes/* and /locales/* are read from
	// the app's directory tree (qorm.json "id" is the bundle root). Without
	// this an image widget's `src: "assets/mario.png"` 404s in the browser
	// and the canvas draws a grey placeholder, even though the bundle has
	// the file. The filesystem layout is the source of truth — the loader
	// already validated that every asset path in the manifest resolves.
	if s.rt != nil && s.rt.App != nil && s.rt.App.BaseDir != "" {
		mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(s.rt.App.BaseDir, "assets")))))
		mux.Handle("/themes/", http.StripPrefix("/themes/", http.FileServer(http.Dir(filepath.Join(s.rt.App.BaseDir, "themes")))))
		mux.Handle("/locales/", http.StripPrefix("/locales/", http.FileServer(http.Dir(filepath.Join(s.rt.App.BaseDir, "locales")))))
	}
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
		// The reload CONTINUES this session on the same scene — a fresh
		// runtime's entry mark must not replay the scene's onEnter hook.
		next.ClearPendingEnter()
		// State carries forward but open requests do not: the old runtime is
		// never resumed, so settle its counters and cancel its transports.
		old.AbandonInflight()
	}
	s.rt = next
	s.handlers = nil
	s.handlerHist = nil // tables from the previous app must never resolve
	s.initAgent()
	s.logEvent("system", "hot-reload: app source changed")
	s.bump()
}

// activate swaps in a new bundle, remembering the previous one for rollback.
// Caller must hold s.mu.
func (s *Server) activate(b *bundle.Bundle) {
	s.prev = s.current
	s.current = b
	if s.rt != nil {
		s.rt.AbandonInflight() // the outgoing runtime will never be resumed
	}
	s.rt = runtime.New(b.ToApp())
	s.handlers = nil
	s.handlerHist = nil
	s.initAgent()
	s.bump()
}

// Update fetches, verifies and activates a bundle from source. On any failure
// the current app is left untouched (rollback by inaction). Returns a status
// line describing the transition.
//
// The fetch runs with ota.BlockPrivate: source reaches us from POST /update
// (any same-origin caller), so an unrestricted fetch would let that caller use
// this process as a proxy into networks only IT can reach — private LAN
// ranges and the 169.254.169.254 cloud metadata service — with redirects
// re-vetted per hop. Loopback sources (local dev bundle servers) and local
// file paths stay allowed; see ota.BlockPrivate for the full threat model.
func (s *Server) Update(source string) (string, error) {
	next, err := ota.FetchVerified(source, s.trust, s.revoked, ota.BlockPrivate())
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
	if s.rt != nil {
		s.rt.AbandonInflight() // the outgoing runtime will never be resumed
	}
	s.rt = runtime.New(restored.ToApp())
	s.handlers = nil
	s.handlerHist = nil
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
	// Snapshot the OTA gate state under the lock. activate() swaps s.current
	// under s.mu on a concurrently handled /update or /rollback, so reading
	// it bare here is a data race. The lock is dropped before Update takes it
	// again, so there is no re-entrancy (or deadlock) hazard.
	s.mu.Lock()
	hasBundle, hasTrust := s.current != nil, s.trust != nil
	s.mu.Unlock()
	if !hasBundle {
		http.Error(w, "OTA not enabled (run from a bundle)", http.StatusBadRequest)
		return
	}
	if !hasTrust {
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
	mutated := s.rt.PendingEnter()
	if r.URL.Query().Get("scene") != "" {
		before := s.rt.RoutePath()
		s.rt.NavigateToPath(r.URL.RawQuery)
		// A page load renders the deep-linked scene DIRECTLY — there is no page
		// transition to play — so drop the pending nav direction here. Left set,
		// it would leak into the next unrelated broadcast (say an agent edit)
		// and a later scene swap would replay it as a stale "push".
		s.rt.TakeNavDir()
		mutated = mutated || s.rt.RoutePath() != before || s.rt.PendingEnter()
	}
	var rev int64
	var body string
	if mutated && s.handlers != nil {
		// This load MUTATES the session (a deep link that actually moved, or a
		// pending scene-entry hook about to run) and at least one frame is
		// already published. INVARIANT: one revision publishes exactly one
		// frame — the frame this page renders differs from what the current
		// revision shipped, so it ships as a NEW revision: bump renders, files
		// the handler table under the new rev and broadcasts, so the other
		// viewers follow the navigation (it is a session event, not a local
		// one). Filing it under the old rev would overwrite that rev's table in
		// place: another tab honestly reporting the rev ITS page was minted on
		// would resolve its click against THIS scene's actions (P0-4).
		//
		// The very first load (s.handlers == nil) files the first frame at
		// revision 0 instead: nothing was published yet, so no client can hold
		// a conflicting frame. A deep-link REFRESH that moved nothing and has
		// no pending hook takes the plain path below — a deterministic
		// re-render of the current revision, no rev churn.
		rev, body, _ = s.bump()
	} else {
		// Scene lifecycle: drain a pending onEnter before this page renders —
		// the initial load of the entry scene and a first-ever deep link both
		// dispatch here. A plain refresh of an already-entered scene has no
		// pending mark (the server session persists), so it never replays;
		// neither does an SSE reconnect (its catch-up render doesn't touch the
		// mark).
		revBefore := s.rev.Load()
		s.rt.RunPendingEnter()
		if s.rev.Load() != revBefore {
			// The drained onEnter carried a `render` step: it already published
			// intermediate frame(s), the LAST of them under the current
			// revision. Rendering the final state under that same revision
			// would be a second, different frame at one rev — the page would be
			// stamped with a rev whose ring entry holds the intermediate frame's
			// (shorter) handler table, deadening any button the final frame
			// added; and when both frames happen to carry the same table the
			// subscribers would silently never see the final state. Publish the
			// final frame as a NEW revision instead (bump's drain is now a
			// no-op). This is the same invariant as the branch above, reached
			// through the first load, where s.handlers was still nil.
			rev, body, _ = s.bump()
		} else {
			scene := s.rt.CurrentScene()
			res := render.RenderScene(s.rt, scene)
			// The page is stamped with the same revision its handler table is
			// filed under, which is what lets a later /event from this page
			// resolve against exactly this frame.
			rev = s.rev.Load()
			s.setHandlers(rev, res.Handlers)
			body = res.HTML
		}
	}
	rt := s.rt
	// Build the page while still holding the lock: Page/userWebJS read rt.State
	// (locale/theme/rtl), which a concurrent POST /event mutates — reading it
	// unlocked is a concurrent-map read+write and crashes the process.
	html := Page(rt, body, rev, s.eventToken)
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
	resp, err := s.agent.HandleHTTP(body)
	w.Header().Set("Content-Type", "application/json")
	if resp == nil {
		// A notification: by definition it receives no response.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		// An unparseable JSON-RPC body is a client error, answered with the
		// -32700 parse-error payload — never a silent 204.
		w.WriteHeader(http.StatusBadRequest)
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
		s.setHandlers(cur, res.Handlers)
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
	H int `json:"h"`
	// Action is the alternative to H: the name of the action to dispatch.
	// Used by the HTML client's scene-level key bindings (scene JSON
	// `keys` / `keyReleases`) — those names are NOT in the handler table
	// because no rendered element invokes them; only the canvas engine
	// (which talks straight to rt.Dispatch) and the page-side key
	// listener (which talks to /event) need to reach them. Resolved
	// against the current handler table by name; falls through to H if
	// empty.
	Action string `json:"action"`
	// Rev is the revision of the frame the browser was showing when the human
	// acted. Handler indices are positional and are renumbered by every render,
	// so a frame that arrived between the paint and the click — an agent edit,
	// or an intermediate frame published by a `render` step, which widens that
	// window from milliseconds to the length of a whole action — would otherwise
	// make index h name a DIFFERENT action than the one that was pressed. With
	// the revision, the server resolves h against the exact frame it was minted
	// on. A POINTER because the very first page is rendered at revision 0: a
	// plain int64 could not tell "the browser was showing frame 0" from "this
	// driver does not report frames at all" (curl, ci-smoke, the offline WASM
	// driver), and those two must behave differently.
	Rev    *int64         `json:"rev"`
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
		s.setHandlers(s.rev.Load(), render.RenderScene(s.rt, s.rt.CurrentScene()).Handlers)
	}
	// Fold current input values back into state before dispatching.
	for path, val := range req.Inputs {
		s.rt.State[path] = val
	}
	// Resolve the handler index against the frame the click came from (see
	// eventReq.Rev); an unknown/absent revision falls back to the newest table.
	table := s.lookupHandlers(req.Rev)
	dispatched := false
	if req.Action != "" {
		// Scene-level key bindings: dispatch by name. The action may not
		// be in the handler table (no rendered element invokes it), so
		// we go straight to rt.Dispatch with no captured scope. The
		// args are empty because the binding came from a key name, not
		// from a handler entry with an Args map.
		s.logEvent("human", "dispatch "+req.Action)
		s.rt.Dispatch(req.Action, nil)
		dispatched = true
	} else if req.H >= 0 && req.H < len(table) {
		h := table[req.H]
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
		dispatched = true
	}
	_ = dispatched
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

// qormKeyBindings returns a JSON snippet declaring __qormKeys,
// __qormKeyReleases, and __qormSwipes for the current scene, plus
// __qormKeyToIdx (handler index) for action names. The HTML path has no
// equivalent of the canvas engine's HandleKey / swipe recognizer — without
// this, a scene's `keys` / `keyReleases` / `swipes` JSON is invisible in the
// browser and a "hold to run" or swipe-to-slide game simply does not respond.
// The names are normalised to lowercase to match the runtime's KeyAction /
// SwipeAction lookup. Handler index resolution: render.RenderScene gives us
// the full handler table for this scene, so we walk it once and emit a
// name → index map; the client key/swipe handlers look up an action by
// name then dispatch via /event {action}.
func qormKeyBindings(rt *runtime.Runtime) string {
	if rt == nil || rt.App == nil {
		return ""
	}
	scene := rt.CurrentScene()
	if scene == "" {
		scene = rt.App.Entry
	}
	keys := rt.App.SceneKeys[scene]
	rels := rt.App.SceneKeyReleases[scene]
	swipes := rt.App.SceneSwipes[scene]
	if len(keys) == 0 && len(rels) == 0 && len(swipes) == 0 {
		return ""
	}
	// Get current handler table — the same call the server uses after a
	// render. RenderScene is cheap; on the page-serve path it doubles as
	// a "this is the table the body HTML was rendered against" guarantee.
	// Built even when only swipes are present so keys can still resolve
	// if a later morph updates __qormKeys without re-emitting this block.
	res := render.RenderScene(rt, scene)
	nameToIdx := map[string]int{}
	for i, h := range res.Handlers {
		if _, exists := nameToIdx[h.Name]; !exists {
			nameToIdx[h.Name] = i
		}
	}
	keysJSON, _ := json.Marshal(keys)
	relsJSON, _ := json.Marshal(rels)
	idxJSON, _ := json.Marshal(nameToIdx)
	swipesJSON, _ := json.Marshal(swipes)
	return fmt.Sprintf("window.__qormKeys=%s;window.__qormKeyReleases=%s;window.__qormKeyToIdx=%s;window.__qormSwipes=%s;",
		keysJSON, relsJSON, idxJSON, swipesJSON)
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
	// fixedSize is true when the manifest declared an explicit width/height.
	// When set, the stage element gets a `qorm-fixed` class so the responsive
	// media queries below (max-width:640 / min-width:1024) are skipped, and
	// the inline JS calls window.resizeTo so the browser opens at the right
	// size. Without this a 1024x480 side-scroller game would render into a
	// 420x720 portrait-shaped stage and look like a stretched phone app.
	fixedSize := w.Width > 0 && w.Height > 0
	fixedClass := ""
	fixedCSS := ""
	resizeJS := ""
	if fixedSize {
		fixedClass = " qorm-fixed"
		fixedCSS = fmt.Sprintf("#qorm-stage.qorm-fixed { width:%dpx !important; min-width:%dpx; max-width:%dpx; height:%dpx !important; min-height:%dpx; max-height:%dpx; } body { align-items:center; padding:0; }",
			w.Width, w.Width, w.Width, w.Height, w.Height, w.Height)
		// window.resizeTo is allowed on window.open'd windows; the
		// initial load of a tab gets the call silently no-op'd on
		// some browsers, but the explicit viewport hint still seeds
		// the responsive layout correctly.
		resizeJS = fmt.Sprintf("try{window.resizeTo(%d,%d);}catch(e){}", w.Width, w.Height)
	}
	lang := langTag(rt.CurrentLocale())
	dir := "ltr"
	if rt.IsRTL() {
		dir = "rtl"
	}
	theme := themeClass(rt.CurrentTheme())
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
  %s
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
  /* ---- Pseudo-state styles (hover / pressed / focus / disabled). Authors
     declare them as style keys; the renderer emits each one as a CSS custom
     property on the node's own inline style (pseudoStateCSS in
     internal/render/render_style.go) and these FIXED rules consume it, matching
     on the variable's presence in the style attribute. No JS and no per-node
     stylesheet, so the behaviour survives every DOM morph, and the author value
     never leaves the styleAttr-escaped style attribute. The declarations are
     !important because an inline style otherwise outranks any stylesheet rule.
     Variable names are chosen not to prefix one another (--qorm-dis vs
     --qorm-dop), since these are substring matches. ---- */
  [style*="--qorm-hov-"], [style*="--qorm-prs-"] { transition:background .15s ease, color .15s ease, opacity .15s ease, transform .12s ease; }
  @media (hover:hover) {
    [style*="--qorm-hov-bg"]:hover { background:var(--qorm-hov-bg) !important; }
    [style*="--qorm-hov-fg"]:hover { color:var(--qorm-hov-fg) !important; }
    [style*="--qorm-hov-op"]:hover { opacity:var(--qorm-hov-op) !important; }
  }
  [style*="--qorm-prs-sc"]:active { transform:scale(var(--qorm-prs-sc)) !important; }
  [style*="--qorm-prs-op"]:active { opacity:var(--qorm-prs-op) !important; }
  [style*="--qorm-foc-bc"]:focus-within { border-color:var(--qorm-foc-bc) !important; outline:2px solid var(--qorm-foc-bc); outline-offset:2px; }
  /* Keyboard focus indicator (:focus-visible semantics — no ring on pointer focus). */
  [style*="--qorm-foc-bc"]:focus-visible { outline:2px solid var(--qorm-foc-bc) !important; outline-offset:2px; }
  [style*="--qorm-dis"] { opacity:var(--qorm-dop,.4) !important; pointer-events:none !important; cursor:not-allowed !important; }
  /* ---- Frosted glass (the backdropBlur / backdropTint style keys, emitted
     as custom properties by backdropCSS in internal/render/render_style.go).
     backdrop-filter is not universally available, and a translucent panel over
     an un-blurred background is unreadable — so the SOLID fill is the base rule
     and the blur + tint only apply inside the @supports guard. Neither rule is
     !important, so a node's own inline background declaration still wins. ---- */
  [style*="--qorm-bdb"] { background:var(--surface); }
  @supports ((-webkit-backdrop-filter:blur(1px)) or (backdrop-filter:blur(1px))) {
    [style*="--qorm-bdb"] { background:var(--qorm-bdt,color-mix(in srgb,var(--surface) 62%%,transparent));
      -webkit-backdrop-filter:blur(var(--qorm-bdb)) saturate(180%%);
      backdrop-filter:blur(var(--qorm-bdb)) saturate(180%%); }
  }
  /* ---- Native constraint validation: a field the browser rejects gets the same
     red border the declarative error prop draws. :user-invalid waits for the
     user to have interacted, so an untouched empty required field does not look
     wrong before anyone typed; the @supports fallback approximates that with
     :not(:placeholder-shown). textformfield draws its border on the wrapper. ---- */
  #qorm-stage input:user-invalid, #qorm-stage textarea:user-invalid, #qorm-stage select:user-invalid { border-color:var(--danger) !important; }
  @supports not selector(:user-invalid) {
    #qorm-stage input:invalid:not(:placeholder-shown), #qorm-stage textarea:invalid:not(:placeholder-shown) { border-color:var(--danger) !important; }
  }
  #qorm-stage div:has(> input:user-invalid) { border-color:var(--danger) !important; }
  /* ---- Collapsing large title (CupertinoSliverNavigationBar / SliverAppBar).
     The compact bar is position:sticky, so the big title simply scrolls up and
     vanishes BEHIND it — the collapse itself needs no JS and no modern CSS at
     all. The cross-fade between the two titles is a scroll-driven animation:
     the big title names a view() timeline, its exit range is exactly the
     scroll distance over which it leaves the top of the scrollport, and both
     titles animate along it — so the transition tracks the finger frame for
     frame, off the main thread, and survives every DOM morph (it is a
     stylesheet rule, not state). timeline-scope hoists the name to the wrapper
     so the mini title, which is inside the PRECEDING sibling, can see it.
     Where the browser has no scroll-driven animations, qormLargeTitleSync in
     app.js toggles .qorm-lt-stuck from a scroll listener instead. ---- */
  .qorm-lt-mini { opacity:0; transition:opacity .2s ease; }
  .qorm-lt-big { transform-origin:left center; transition:opacity .2s ease, transform .2s ease; }
  .qorm-lt-bar { transition:border-color .2s ease; }
  .qorm-lt-stuck .qorm-lt-mini { opacity:1; }
  .qorm-lt-stuck .qorm-lt-big { opacity:0; transform:translateY(-6px) scale(.94); }
  .qorm-lt-stuck .qorm-lt-bar { border-bottom-color:var(--sep); }
  @keyframes qorm-lt-collapse { to { opacity:0; transform:translateY(-6px) scale(.94); } }
  @keyframes qorm-lt-reveal { from { opacity:0; } to { opacity:1; } }
  @keyframes qorm-lt-hairline { to { border-bottom-color:var(--sep); } }
  @supports (animation-timeline:view()) and (timeline-scope:--qorm-lt) {
    .qorm-lt { timeline-scope:--qorm-lt; }
    .qorm-lt-big { view-timeline-name:--qorm-lt; view-timeline-axis:block; transition:none;
      animation:qorm-lt-collapse linear both; animation-timeline:--qorm-lt; animation-range:exit 0%% exit 100%%; }
    .qorm-lt-mini { transition:none;
      animation:qorm-lt-reveal linear both; animation-timeline:--qorm-lt; animation-range:exit 0%% exit 100%%; }
    .qorm-lt-bar { transition:none;
      animation:qorm-lt-hairline linear both; animation-timeline:--qorm-lt; animation-range:exit 0%% exit 100%%; }
  }
  /* Draggable sheet (DraggableScrollableSheet): the grab row owns the gesture,
     so the body below it keeps its own scrolling. touch-action:none stops the
     browser claiming the drag for a scroll before pointermove ever fires. */
  .qorm-dsheet-grab { touch-action:none; cursor:grab; }
  .qorm-dsheet-grab:active { cursor:grabbing; }
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
  /* ---- Legacy "tooltip" PROP (a11y() in internal/render/render_style.go emits
     data-tooltip). Kept byte-for-byte as it was — every app using the attribute
     must render and look exactly the same — with ONE purely additive selector:
     :focus-visible, so the hint a mouse can see is also reachable from the
     keyboard. Authors wanting placement, wrapping long text or a {{ binding }}
     use the "tooltip" WIDGET below. ---- */
  [data-tooltip] { position:relative; }
  [data-tooltip]:hover::after, [data-tooltip]:focus-visible::after { content:attr(data-tooltip); position:absolute; bottom:100%%; left:50%%; transform:translateX(-50%%);
    background:#111827; color:#fff; padding:4px 8px; border-radius:6px; font-size:12px; white-space:nowrap; margin-bottom:6px; z-index:100; pointer-events:none; }
  /* ---- Tooltip WIDGET ({"type":"tooltip"} — r.tooltip in
     internal/render/render_feedback.go). The bubble is a REAL element holding an
     escaped text node, not content:attr(): a long hint therefore WRAPS inside
     max-width instead of running off-screen, and assistive tech reaches it
     through role="tooltip" + aria-describedby. Hover sits inside
     @media (hover:hover) so a tap on a touch device cannot leave a bubble stuck
     open; :focus-visible covers the wrapper when it is itself the tab stop and
     :focus-within covers the case where it wraps a control. Placement is one of
     four fixed direction classes, so no author value ever reaches a rule.
     Caveat: an absolutely-positioned bubble is still clipped by an ancestor with
     overflow:hidden — escaping to the top layer needs the Popover API, which
     cannot be opened on hover without script. ---- */
  .qorm-tip { position:relative; }
  .qorm-tip-bubble { position:absolute; z-index:120; opacity:0; visibility:hidden; transition:opacity .12s ease;
    box-sizing:border-box; width:max-content; max-width:240px; padding:6px 9px; border-radius:6px;
    background:#111827; color:#fff; font-size:12px; font-weight:400; line-height:1.45; text-align:left;
    white-space:normal; overflow-wrap:anywhere; pointer-events:none; box-shadow:0 6px 20px rgba(0,0,0,.28); }
  @media (hover:hover) { .qorm-tip:hover > .qorm-tip-bubble { opacity:1; visibility:visible; } }
  .qorm-tip:focus-visible > .qorm-tip-bubble, .qorm-tip:focus-within > .qorm-tip-bubble { opacity:1; visibility:visible; }
  .qorm-tip-top > .qorm-tip-bubble { bottom:100%%; left:50%%; transform:translateX(-50%%); margin-bottom:7px; }
  .qorm-tip-bottom > .qorm-tip-bubble { top:100%%; left:50%%; transform:translateX(-50%%); margin-top:7px; }
  .qorm-tip-left > .qorm-tip-bubble { right:100%%; top:50%%; transform:translateY(-50%%); margin-right:7px; }
  .qorm-tip-right > .qorm-tip-bubble { left:100%%; top:50%%; transform:translateY(-50%%); margin-left:7px; }
</style>
</head>
<body>
<div id="qorm-stage" class="qorm-theme-%s%s"><div id="qorm-root">%s</div></div>
<script>%s%s%s</script>
</body>
</html>`, html.EscapeString(lang), dir, htmlEscape(title), themeCSS(rt), width, height, fixedCSS, html.EscapeString(theme), fixedClass, body, qormAppJS(rev, tok), resizeJS, qormKeyBindings(rt))
}

// themeClass turns the active theme name (state.theme — writable by an action,
// by an http response, by MCP qorm_set_state, or by a theme picker bound to a
// text input) into the CSS class token the shell stamps on the stage:
// class="qorm-theme-<theme>".
//
// This is a NORMALIZING WHITELIST, not an escape, and deliberately so. Entity
// encoding alone would stop the attribute breakout but not the injection: a
// space is a perfectly legal HTML attribute character, so `dark qorm-fluid`
// would still silently add a second class, and the value's whole job is to be a
// CSS identifier — a character set of exactly [A-Za-z0-9_-]. Normalizing to
// that set makes the value structurally incapable of carrying markup, a quote,
// or a second class, independent of the surrounding quoting; the call site
// still html.EscapeString's it so the attribute stays sound even if this
// function is ever loosened.
//
// A value outside the set is REJECTED WHOLESALE (fall back to the documented
// default "auto") rather than stripped: a stripped value would render under a
// theme the author never named, whereas "auto" is exactly what an unknown theme
// already resolved to in render.ThemeVarsFor. Every legitimate theme name —
// the built-in apple/material/dark/auto and any custom name an app styles via
// designTokens — passes through byte-identically.
func themeClass(theme string) string {
	if theme == "" || len(theme) > 32 {
		return "auto"
	}
	for i := 0; i < len(theme); i++ {
		c := theme[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return "auto"
		}
	}
	return theme
}

// langTag turns the active locale (state.locale — same write surfaces as
// state.theme) into the value of <html lang="…">.
//
// Same reasoning as themeClass: a BCP47 language tag is a restricted grammar
// (alphanumeric subtags joined by single separators), so normalizing to that
// grammar is strictly stronger than escaping — it rules out not just the quote
// breakout but any content that is not a language tag at all. `_` is accepted
// alongside `-` because runtime.IsRTL already treats both as subtag separators.
// A value that is not tag-shaped falls back to "en", which is what an empty
// locale already produced.
func langTag(locale string) string {
	if locale == "" || len(locale) > 35 {
		return "en"
	}
	prevSep := true // a tag may not start with a separator
	for i := 0; i < len(locale); i++ {
		c := locale[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			prevSep = false
		case c == '-', c == '_':
			if prevSep {
				return "en" // leading or doubled separator: not a language tag
			}
			prevSep = true
		default:
			return "en"
		}
	}
	if prevSep {
		return "en" // trailing separator
	}
	return locale
}

// themeCSS is the shell's theme block: the shared built-in palettes plus the
// app's own manifest designTokens rendered as var(--qorm-token-*) on the
// stage, so scenes can style against them. When the active theme is a custom
// skin (themes/<name>.json, not one of the built-in palettes) we also emit
// `.qorm-theme-<name> { --<color>: ...; }` so scenes that reference
// var(--sky) / var(--ground) / ... see real values, not the empty string.
func themeCSS(rt *runtime.Runtime) string {
	base := render.ThemeCSS
	if css := render.TokenCSS("#qorm-stage", rt.App.DesignTokens); css != "" {
		base += "\n  " + css
	}
	if rt.Theme != nil {
		if extra := rt.Theme.CSSClassRules(); extra != "" {
			base += "\n  " + extra
		}
	}
	return base
}

// htmlEscape entity-encodes an app-supplied string (the manifest name / window
// title) for the HTML shells. It escapes QUOTES as well as & < >, because the
// same helper feeds attribute contexts, not only text: the console frames the
// app as <iframe title="{{title}}"> (console.go) and the offline shell writes
// <meta ... content="…"> (offline.go). Without the quote entities a manifest
// name carrying a double quote closes the attribute and injects markup — and a
// manifest is exactly the artifact an OTA bundle replaces. In text contexts
// (<title>…</title>) the extra entities are decoded back by the parser, so the
// rendered text is unchanged.
func htmlEscape(s string) string {
	repl := map[rune]string{'&': "&amp;", '<': "&lt;", '>': "&gt;", '"': "&#34;", '\'': "&#39;"}
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
