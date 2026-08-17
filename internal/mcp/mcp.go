// Package mcp exposes a QORM app to agents (Claude, Cursor, ...) over the Model
// Context Protocol (JSON-RPC 2.0). It offers the full agent surface — inspect,
// render, query, side-effect-free simulate, plus operate (dispatch/set_state),
// test (assert) and design (preview_patch → apply_patch) — so an agent can
// understand, run, test and safely modify a UI.
//
// The same handler works over two transports: newline-delimited stdio (for a
// spawned agent) and HTTP POST (so an agent and a human browser can share one
// live app session — see package server).
package mcp

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"sync"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/policy"
	"github.com/qorm/platform/internal/runtime"
)

const (
	protocolVersion = "2024-11-05"
	serverName      = "qorm"
	serverVersion   = "0.1.0"

	// codeParseError is the JSON-RPC 2.0 reserved code for "invalid JSON was
	// received by the server".
	codeParseError = -32700
)

// ErrParse signals that a request body was not parseable as JSON. It
// accompanies the JSON-RPC -32700 parse-error response so a transport can
// tell a malformed request (an HTTP client error) apart from a notification
// (which legitimately receives no response).
var ErrParse = errors.New("mcp: parse error (invalid JSON)")

// Server processes MCP messages against a runtime. It may own its runtime
// (stdio mode) or share one with an HTTP server (live-session mode).
type Server struct {
	rt      *runtime.Runtime
	in      *bufio.Scanner
	out     io.Writer
	preview *previewState
	history []*model.App // pre-images before each apply_patch, for undo

	mu            *sync.Mutex   // guards rt during shared sessions
	readOnly      bool          // reject mutating tools (qorm run --mcp-read-only)
	afterMutate   func()        // called after a mutating tool (for live-sync)
	bumped        bool          // current tool call already published (currentHTML drained a hook) — handleToolCall must not bump twice
	measureProv   func() []byte // latest self-reported layout, if a live client is measuring
	activityProv  func() string // shared-session activity log JSON (who did what), if wired
	canvasCapture CanvasCaptureProvider
	windowMover   func(id string, x, y, w, h int)
	windowOp      func(id, op string)
	windowOpen    func(id, url string, w, h int)
	windowEval    func(id, js string)
}

const (
	MaxCanvasCaptureDimension = 4096
	MaxCanvasCapturePixels    = 16 * 1024 * 1024
	MaxCanvasCapturePNGBytes  = 16 * 1024 * 1024
)

// CanvasCapture is the last presented native-canvas pixel plane. When a node
// id was requested, Clip identifies that node inside the full-surface PNG; the
// provider never implies that it isolated or re-rendered the subtree.
type CanvasCapture struct {
	PNG    []byte
	Width  int
	Height int
	Scale  int
	Clip   *CanvasCaptureRect
}

type CanvasCaptureRect struct {
	X                int  `json:"x"`
	Y                int  `json:"y"`
	W                int  `json:"w"`
	H                int  `json:"h"`
	ClippedToSurface bool `json:"clippedToSurface,omitempty"`
}

type CanvasCaptureProvider func(id string) (CanvasCapture, error)

const maxHistory = 50

// previewState remembers the last side-effect-free preview so apply_patch can
// require an operator to have previewed the exact change first.
type previewState struct {
	token string
	ops   []PatchOp
}

// New builds a stdio MCP server that owns its runtime.
func New(rt *runtime.Runtime, in io.Reader, out io.Writer) *Server {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return &Server{rt: rt, in: sc, out: out, mu: &sync.Mutex{}}
}

// NewShared builds an MCP handler that shares a runtime (and its mutex) with
// another server, invoking afterMutate whenever a tool changes state or design.
func NewShared(rt *runtime.Runtime, mu *sync.Mutex, afterMutate func()) *Server {
	return &Server{rt: rt, mu: mu, afterMutate: afterMutate}
}

// SetReadOnly switches the server into (or out of) read-only mode. In
// read-only mode every mutating tool (qorm_dispatch, qorm_set_state,
// qorm_apply_patch, qorm_undo) is rejected with a JSON-RPC error before it
// reaches the tool handler; inspection/preview tools work unchanged.
func (s *Server) SetReadOnly(v bool) { s.readOnly = v }

// SetMeasureProvider supplies the latest layout self-measurement (from a live
// browser/WebView client), enabling the qorm_measure/qorm_check_layout tools.
func (s *Server) SetMeasureProvider(f func() []byte) { s.measureProv = f }

// SetCanvasCaptureProvider exposes the last frame actually presented by a
// live native canvas host. Stdio and browser/WebView sessions leave it nil.
func (s *Server) SetCanvasCaptureProvider(f CanvasCaptureProvider) { s.canvasCapture = f }

func validateCanvasCapture(c CanvasCapture, needsClip bool) error {
	if c.Width < 1 || c.Height < 1 || c.Width > MaxCanvasCaptureDimension || c.Height > MaxCanvasCaptureDimension || int64(c.Width)*int64(c.Height) > MaxCanvasCapturePixels {
		return fmt.Errorf("canvas capture dimensions %dx%d exceed safety limits", c.Width, c.Height)
	}
	if c.Scale < 1 || c.Scale > 8 {
		return fmt.Errorf("canvas capture scale %d is invalid", c.Scale)
	}
	if len(c.PNG) == 0 || len(c.PNG) > MaxCanvasCapturePNGBytes {
		return fmt.Errorf("canvas capture PNG is empty or exceeds %d bytes", MaxCanvasCapturePNGBytes)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(c.PNG))
	if err != nil || cfg.Width != c.Width || cfg.Height != c.Height {
		return fmt.Errorf("canvas capture provider returned invalid PNG metadata")
	}
	if needsClip && c.Clip == nil {
		return fmt.Errorf("canvas capture provider omitted requested node clip metadata")
	}
	if c.Clip != nil && (c.Clip.W < 1 || c.Clip.H < 1 || c.Clip.X < 0 || c.Clip.Y < 0 || c.Clip.X+c.Clip.W > c.Width || c.Clip.Y+c.Clip.H > c.Height) {
		return fmt.Errorf("canvas capture provider returned an out-of-bounds node clip")
	}
	return nil
}

func canvasCaptureJSON(c CanvasCapture, id string) (string, error) {
	if err := validateCanvasCapture(c, id != ""); err != nil {
		return "", err
	}
	out := map[string]any{
		"mimeType": "image/png", "encoding": "base64",
		"pngBase64": base64.StdEncoding.EncodeToString(c.PNG),
		"scope":     "full-surface", "source": "last-presented-canvas-frame",
		"width": c.Width, "height": c.Height, "scale": c.Scale,
		"logicalWidth": c.Width / c.Scale, "logicalHeight": c.Height / c.Scale,
	}
	if id != "" {
		out["nodeId"] = id
		out["clip"] = c.Clip
		out["clipUnits"] = "physical-px"
	}
	b, err := json.MarshalIndent(out, "", "  ")
	return string(b), err
}

// SetActivityProvider supplies the shared-session activity log (who did what),
// enabling the qorm_activity tool so an agent can see the human's live actions.
func (s *Server) SetActivityProvider(f func() string) { s.activityProv = f }

// SetWindowMover supplies the native window mover (desktop app only), enabling
// the qorm_window tool for the agent to position the user's window.
func (s *Server) SetWindowControl(mover func(id string, x, y, w, h int), op func(id, op string), open func(id, url string, w, h int), eval func(id, js string)) {
	s.windowMover = mover
	s.windowOp = op
	s.windowOpen = open
	s.windowEval = eval
}

// ---- JSON-RPC framing ----

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve runs the stdio read-dispatch-write loop until stdin closes.
func (s *Server) Serve() error {
	for s.in.Scan() {
		line := s.in.Bytes()
		if len(line) == 0 {
			continue
		}
		if resp, _ := s.HandleLine(line); resp != nil {
			s.writeLine(resp)
		}
	}
	return s.in.Err()
}

// HandleLine processes one raw JSON-RPC message and returns the response, or
// nil for notifications (which by definition receive no response).
// Unparseable input is NOT a notification: it yields a JSON-RPC -32700
// parse-error response (with a null id, per the spec) together with ErrParse,
// so transports can tell a malformed request apart from a notification. Used
// by both transports.
func (s *Server) HandleLine(line []byte) (*response, error) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return fail(json.RawMessage("null"), codeParseError, "parse error: "+err.Error()), ErrParse
	}
	return s.dispatch(req), nil
}

// HandleHTTP processes one JSON-RPC request body and returns response bytes
// (empty for notifications). For an unparseable body it returns the JSON-RPC
// -32700 parse-error payload together with ErrParse, so the HTTP transport
// can answer 4xx instead of a silent 204.
func (s *Server) HandleHTTP(body []byte) ([]byte, error) {
	resp, err := s.HandleLine(body)
	if resp == nil {
		return nil, err
	}
	data, _ := json.Marshal(resp)
	return data, err
}

func (s *Server) dispatch(req request) *response {
	switch req.Method {
	case "initialize":
		return ok(req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": serverName, "version": serverVersion},
		})
	case "notifications/initialized", "notifications/cancelled":
		return nil
	case "ping":
		return ok(req.ID, map[string]any{})
	case "tools/list":
		return ok(req.ID, map[string]any{"tools": toolList()})
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if s.readOnly && isMutating(p.Name) {
			return fail(req.ID, -32000, "read-only mode: tool "+p.Name+" is disabled (server started with --mcp-read-only)")
		}
		if s.rt != nil && s.rt.App.AgentPolicy.HasPolicy() {
			if err := policy.AuthorizeTool(s.rt.App.AgentPolicy, p.Name, p.Arguments); err != nil {
				return fail(req.ID, policy.RPCCodePolicyDenied, err.Error())
			}
		}
		return s.handleToolCall(req)
	default:
		if len(req.ID) > 0 {
			return fail(req.ID, -32601, "method not found: "+req.Method)
		}
		return nil
	}
}

func ok(id json.RawMessage, result any) *response {
	return &response{JSONRPC: "2.0", ID: id, Result: result}
}

func fail(id json.RawMessage, code int, msg string) *response {
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

func (s *Server) writeLine(r *response) {
	data, err := json.Marshal(r)
	if err != nil {
		return
	}
	fmt.Fprintf(s.out, "%s\n", data)
}
