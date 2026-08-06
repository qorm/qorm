// Package qscript implements the QORM script language — a small,
// deterministic, JS-lite interpreted language for app logic. Where scene
// JSON declares structure and data, a script action (action JSON "script")
// carries the logic that used to need a Go component or long step lists.
//
// # Language
//
// Statements:
//
//	let x = expr                     # declare a local
//	x = expr                         # assign a declared local / param
//	state.a = expr                   # write a state path (setPath semantics:
//	state.obj.k = expr               #   intermediate maps are created)
//	state.arr[i] = expr              # write one array element / map entry
//	if (expr) { ... } else { ... }   # else is optional; `else if` chains
//	for name in expr { ... }         # iterate the elements of an array
//	while expr { ... } { ... }       # (parens around the condition optional)
//	break / continue                 # leave / re-iterate the innermost loop
//	return expr?                     # inside fn; at top level ends the script.
//	                                 # A bare `return` is recognised before '}'
//	                                 # or end of script; anywhere else the text
//	                                 # after it is parsed as the return value.
//	expr                             # expression statement (e.g. a call)
//
// Functions:
//
//	fn name(a, b, ...) { ... }
//
// Calls look like builtins: name(1, state.x). There are NO closures: a
// function sees its parameters, its own `let` locals, the `state` and `args`
// handles, and the global function table — nothing from any caller's scope.
//
// Expressions mirror internal/expr exactly: number/string/bool/null
// literals, dotted identifiers (state.piece.x), postfix indexing (a[i],
// users[0].name), unary ! and -, binary * / % + - < <= > >= == != && ||,
// the ternary ?:, and the same builtin function set (len/slice/map/filter/
// sum/count/at/fill/range/concat/join/split/keys/values/num/str/contains/
// mod/floor/ceil/abs/min/max/...) plus the v2 array/string methods
// (push/unshift/pop/shift/reverse/sort/indexOf/includes/charAt/substring/
// repeat/padStart/padEnd/trimStart/trimEnd) — builtin calls are delegated to
// expr.CallBuiltin so script and binding semantics can never drift apart.
// Scripts add array literals [e1, e2, ...] (the binding language has none).
//
// Comments run from '#' to end of line. There are no semicolons; statements
// are self-delimiting. `state` is the injected read/write handle (reads are
// runtime state paths, writes follow the runtime's setPath semantics) and
// `args` is the injected dispatch-argument map. The words `let if else for
// in while return fn break continue state args true false null nil` are
// reserved.
//
// # Governance (a runaway script degrades to an error, never a hang)
//
//   - source length cap (64 KB) and parser depth cap (256), mirroring expr;
//   - maxTotalOps (200k) counted per executed statement, loop iteration and
//     function call across the whole run;
//   - maxLoopIters (100k) per single loop execution;
//   - maxCallDepth (64) on fn calls.
//
// Every violation returns an *Error carrying the script line number — the
// interpreter never panics, whatever the input.
//
// # Determinism
//
// There is no I/O and no external call surface: a script is a pure function
// of (state, args). The ONLY clock is the explicit `now()` builtin (Unix ms),
// and randomness otherwise enters only through state (e.g. an LCG kept in
// state.rng — see examples/tetris).
//
// # Composing actions: call() and the shared library
//
// A script may fire a sibling action with call("id" [, args]). The call is
// bridged by a host-installed dispatch hook (SetDispatchHook); the runtime
// wires it to its own Dispatch, so call() chains re-enter the normal
// invoke-depth governance and a failure surfaces as an *Error on the caller's
// line. Without a hook, call() is a no-op returning false. Shared logic lives
// in the reserved actions/lib.qs, which the loader collects (app.ScriptLib)
// and the runtime prepends to every action's source at dispatch.
package qscript

import "fmt"

// Governance limits. See the package comment for the rationale; the values
// are deliberately generous for real app logic (a tetris tick is ~2k ops)
// while still turning any runaway loop into a prompt error.
const (
	maxSrcLen     = 64 << 10 // 64 KB source cap (mirrors expr)
	maxParseDepth = 256      // parser recursion cap (mirrors expr)
	maxTotalOps   = 200_000  // statements + loop iterations + calls per run
	maxLoopIters  = 100_000  // iterations of one loop execution
	maxCallDepth  = 64       // nested fn calls
)

// Error is a script failure — parse or runtime — attributed to a source
// line (0 when no line applies, e.g. the source-length cap).
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

// Program is a parsed script, ready to run. It is read-only after Parse, so
// one Program may serve concurrent Run calls (each Run builds its own
// interpreter state).
type Program struct {
	fns  map[string]*fnDecl
	body []stmt
}

// Parse compiles src. A failure is an *Error with the offending line, so the
// loader can report it as a load-time diagnostic and an agent can fix the
// exact line.
func Parse(src string) (*Program, error) {
	if len(src) > maxSrcLen {
		return nil, &Error{Msg: fmt.Sprintf("script too long (%d bytes, max %d)", len(src), maxSrcLen)}
	}
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	return parseProgram(toks)
}

// Run executes the program against state with the given dispatch args (both
// may be nil). State writes land in place (setPath semantics); a runtime
// failure — governance limit, type error, unknown function — is returned as
// an *Error with the script line number.
func (p *Program) Run(state, args map[string]any) error {
	if state == nil {
		state = map[string]any{}
	}
	if args == nil {
		args = map[string]any{}
	}
	i := &interp{fns: p.fns, state: state, args: args}
	_, _, _, err := i.exec(p.body, scope{}, 0)
	return err
}

// Run parses and executes src in one call — the runtime's dispatch path.
func Run(src string, state, args map[string]any) error {
	p, err := Parse(src)
	if err != nil {
		return err
	}
	return p.Run(state, args)
}
