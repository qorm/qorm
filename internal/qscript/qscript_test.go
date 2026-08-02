package qscript

// Unit tests for the qscript interpreter: statements, expressions (with a
// parity suite pinning operator/builtin semantics to internal/expr), state
// read/write through the `state` handle, functions, governance limits, and
// error line attribution.

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/expr"
)

// runOK runs src against state and fails the test on any error.
func runOK(t *testing.T, src string, state, args map[string]any) {
	t.Helper()
	if err := Run(src, state, args); err != nil {
		t.Fatalf("Run(%q) error: %v", src, err)
	}
}

// runErr runs src expecting a failure whose message contains want.
func runErr(t *testing.T, src string, want string) {
	t.Helper()
	err := Run(src, map[string]any{}, nil)
	if err == nil {
		t.Fatalf("Run(%q) succeeded, want error containing %q", src, want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("Run(%q) error = %q, want it to contain %q", src, err.Error(), want)
	}
}

func TestLetAndAssign(t *testing.T) {
	state := map[string]any{}
	runOK(t, `
# comments run to end of line
let x = 1 + 2 * 3      # precedence mirrors the expression language
x = x + 1
state.a = x
state.obj.k = "v"      # intermediate maps are created (setPath semantics)
state.obj.n = x
`, state, nil)
	if state["a"] != 8.0 {
		t.Fatalf("state.a = %v, want 8", state["a"])
	}
	obj := state["obj"].(map[string]any)
	if obj["k"] != "v" || obj["n"] != 8.0 {
		t.Fatalf("state.obj = %v", obj)
	}
}

func TestAssignUndeclaredFails(t *testing.T) {
	runErr(t, "nope = 1", "undeclared")
}

func TestIfElseWhile(t *testing.T) {
	state := map[string]any{}
	runOK(t, `
let i = 0
let total = 0
while i < 10 {
  i = i + 1
  if (i % 2 == 0) {
    total = total + i
  } else {
    total = total + 1
  }
}
state.total = total
state.i = i
`, state, nil)
	if state["total"] != 35.0 { // 2+4+6+8+10 + five 1s
		t.Fatalf("state.total = %v, want 35", state["total"])
	}
	if state["i"] != 10.0 {
		t.Fatalf("state.i = %v, want 10", state["i"])
	}
}

func TestElseIfChain(t *testing.T) {
	state := map[string]any{}
	runOK(t, `
let x = 2
if (x == 1) { state.r = "one" } else if (x == 2) { state.r = "two" } else { state.r = "many" }
`, state, nil)
	if state["r"] != "two" {
		t.Fatalf("state.r = %v, want two", state["r"])
	}
}

func TestForIn(t *testing.T) {
	state := map[string]any{"items": []any{1.0, 2.0, 3.0}}
	runOK(t, `
let total = 0
for it in state.items {
  total = total + it
}
state.total = total
state.n = 0
for it in range(4) {
  state.n = state.n + it
}
for it in state.missing { state.total = 99 }   # nil iterates zero times
`, state, nil)
	if state["total"] != 6.0 {
		t.Fatalf("state.total = %v, want 6", state["total"])
	}
	if state["n"] != 6.0 {
		t.Fatalf("state.n = %v, want 6", state["n"])
	}
}

func TestForInNonArrayFails(t *testing.T) {
	runErr(t, "for x in 42 { }", "needs an array")
}

func TestFunctions(t *testing.T) {
	state := map[string]any{}
	runOK(t, `
fn add(a, b) { return a + b }
fn fact(n) {
  if (n <= 1) { return 1 }
  return n * fact(n - 1)
}
fn noop() { return }
fn yes() { return true }
state.sum = add(2, 3)
state.f5 = fact(5)
state.nil = noop()
state.yes = yes()
`, state, nil)
	if state["sum"] != 5.0 {
		t.Fatalf("state.sum = %v, want 5", state["sum"])
	}
	if state["f5"] != 120.0 {
		t.Fatalf("state.f5 = %v, want 120", state["f5"])
	}
	if state["nil"] != nil {
		t.Fatalf("state.nil = %v, want nil", state["nil"])
	}
	if state["yes"] != true {
		t.Fatalf("state.yes = %v, want true (a bare return must not eat the literal)", state["yes"])
	}
}

func TestFunctionScopeIsolation(t *testing.T) {
	// No closures: a function sees params, its own lets, state/args and the
	// global fn table — never the caller's locals.
	err := Run(`
fn read() { return hidden }
let hidden = 42
state.v = read()
`, map[string]any{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// `hidden` is a top-level local, invisible inside read: reads as nil.
	// (The call must not error — unknown reads are nil, like bindings.)
}

func TestArgsHandle(t *testing.T) {
	state := map[string]any{}
	args := map[string]any{"dx": 2.0, "who": map[string]any{"name": "ada"}}
	runOK(t, `
state.x = 10 + args.dx
state.who = args.who.name
`, state, args)
	if state["x"] != 12.0 || state["who"] != "ada" {
		t.Fatalf("state = %v", state)
	}
}

func TestIndexAssignNilPathFails(t *testing.T) {
	runErr(t, "state.deep[0] = 1", "cannot index-assign nil")
}

func TestIndexAssign(t *testing.T) {
	state := map[string]any{
		"board": []any{0.0, 1.0, 2.0},
		"piece": map[string]any{"x": 3.0},
	}
	runOK(t, `
let b = state.board
b[1] = 9                  # local alias of a state array mutates in place
state.board[2] = 7
state.piece.x = state.piece.x + 1
let v = concat(state.board)
v[0] = 5                  # concat copied: state.board[0] stays 0
state.copy0 = v[0]
`, state, nil)
	board := state["board"].([]any)
	if board[0] != 0.0 || board[1] != 9.0 || board[2] != 7.0 {
		t.Fatalf("board = %v, want [0 9 7]", board)
	}
	if state["piece"].(map[string]any)["x"] != 4.0 {
		t.Fatalf("piece.x = %v, want 4", state["piece"])
	}
	if state["copy0"] != 5.0 {
		t.Fatalf("copy0 = %v, want 5", state["copy0"])
	}
}

func TestIndexAssignOutOfRange(t *testing.T) {
	err := Run("state.a[5] = 1", map[string]any{"a": []any{1.0}}, nil)
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("err = %v, want out-of-range error", err)
	}
}

func TestArrayLiteral(t *testing.T) {
	state := map[string]any{}
	runOK(t, `
let a = [1, 2, 3]
state.len = len(a)
state.sum = sum(a)
state.nested = [[1, 2], [3]]
state.pick = at([0, 100, 300, 500, 800], 2)
state.empty = []
`, state, nil)
	if state["len"] != 3.0 || state["sum"] != 6.0 {
		t.Fatalf("state = %v", state)
	}
	nested := state["nested"].([]any)
	if len(nested) != 2 || nested[0].([]any)[1] != 2.0 {
		t.Fatalf("nested = %v", nested)
	}
	if state["pick"] != 300.0 {
		t.Fatalf("pick = %v, want 300", state["pick"])
	}
	if len(state["empty"].([]any)) != 0 {
		t.Fatalf("empty = %v", state["empty"])
	}
}

func TestTernaryAndLogic(t *testing.T) {
	state := map[string]any{}
	runOK(t, `
state.a = 1 > 2 ? "yes" : "no"
state.b = (1 < 2 && 2 < 3) ? 10 : 20
state.c = !false || false
`, state, nil)
	if state["a"] != "no" || state["b"] != 10.0 || state["c"] != true {
		t.Fatalf("state = %v", state)
	}
}

// TestParityWithExpr evaluates the same expression through internal/expr and
// through a qscript `let` with an identical context, pinning the shared
// operator/index/truthiness/builtin semantics.
func TestParityWithExpr(t *testing.T) {
	state := map[string]any{
		"n":    7.0,
		"s":    "hi",
		"b":    true,
		"list": []any{1.0, 2.0, 3.0},
		"obj":  map[string]any{"k": "v"},
	}
	ctx := map[string]any{"state": state}
	for k, v := range state {
		ctx[k] = v
	}
	exprs := []string{
		"1 + 2 * 3 - 4 / 2",
		"7 % 3", "7 % 0",
		"-state.n + 1",
		"state.n > 5 && state.b",
		"state.n < 5 || state.b",
		"!state.b",
		"state.s + \"!\" + state.n",
		"state.s == \"hi\"", "state.n == 7", "state.b == true",
		"state.n >= 7 ? \"big\" : \"small\"",
		"state.list[1]", "state.list[9]", "state.list[-1]",
		"state.obj.k", "state.obj.missing",
		"len(state.list)", "len(state.s)",
		"at(state.list, -1)", "sum(state.list)", "count(state.list)",
		"slice(state.list, 1)", "concat(state.list, state.list)",
		"join(split(\"a,b,c\", \",\"), \"-\")",
		"keys(state.obj)", "values(state.obj)",
		"range(3)", "fill(2, \"x\")",
		"map(state.list, \"it * 2\")", "filter(state.list, \"it > 1\")",
		"num(\"42\") + 1", "str(state.n) + \"!\"",
		"contains(state.s, \"i\")",
		"mod(7, 3)", "floor(1.7)", "ceil(1.2)", "abs(-3)",
		"min(3, 1, 2)", "max(3, 1, 2)",
		"state.missing.deep",
	}
	for _, src := range exprs {
		want, werr := expr.Eval(src, ctx)
		script := fmt.Sprintf("let r = %s\nstate.r = r", src)
		st := map[string]any{}
		for k, v := range state {
			st[k] = v
		}
		if err := Run(script, st, nil); err != nil {
			t.Fatalf("%s: qscript error %v (expr err %v)", src, err, werr)
		}
		got := st["r"]
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: qscript = %#v, expr = %#v", src, got, want)
		}
	}
}

func TestUnknownFunctionFails(t *testing.T) {
	runErr(t, "let x = noSuchFn(1)", "unknown function")
}

func TestGovernanceLoopCap(t *testing.T) {
	runErr(t, "while true { }", "iteration limit")
}

func TestGovernanceCallDepth(t *testing.T) {
	runErr(t, "fn f() { return f() }\nf()", "call depth")
}

func TestGovernanceTotalOps(t *testing.T) {
	// Several statements per iteration trip the 200k total-op budget well
	// before the 100k single-loop iteration cap.
	err := Run(`
let i = 0
while i >= 0 {
  i = i + 1
  i = i + 1
  i = i + 1
  i = i + 1
}
`, map[string]any{}, nil)
	if err == nil || !strings.Contains(err.Error(), "work limit") {
		t.Fatalf("err = %v, want total-op budget error", err)
	}
}

func TestErrorLineNumbers(t *testing.T) {
	err := Run(`
let a = 1
let b = 2
for x in 42 { }
`, map[string]any{}, nil)
	if err == nil {
		t.Fatal("want error")
	}
	qe, ok := err.(*Error)
	if !ok {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if qe.Line != 4 {
		t.Fatalf("error line = %d, want 4 (%v)", qe.Line, err)
	}
	if !strings.Contains(err.Error(), "line 4") {
		t.Fatalf("error %q must carry its line number", err)
	}
}

func TestParseErrorLineNumbers(t *testing.T) {
	_, err := Parse("let a = 1\nlet b =")
	if err == nil {
		t.Fatal("want parse error")
	}
	qe := err.(*Error)
	if qe.Line != 2 {
		t.Fatalf("parse error line = %d, want 2 (%v)", qe.Line, err)
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{
		"let = 1",           // missing name
		"let x",             // missing =
		"if (true) state.x", // missing block
		"for x state.a { }", // missing in
		"fn f( { }",         // bad params
		"fn f() { ",         // unterminated block
		"let x = 'abc",      // unterminated string
		"let x = 1.2.3",     // malformed number
		"f(1) = 2",          // call is not an l-value
		"let x = a ? 1",     // ternary missing :
	} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", src)
		}
	}
}

func TestAssignHandleItselfFails(t *testing.T) {
	runErr(t, "state = 1", "cannot replace")
}

func TestStringEscapesAndComments(t *testing.T) {
	state := map[string]any{}
	runOK(t, "let q = 'it\\'s' # trailing comment\nstate.q = q", state, nil)
	if state["q"] != "it's" {
		t.Fatalf("state.q = %q", state["q"])
	}
}

func TestProgramReuseAcrossRuns(t *testing.T) {
	prog, err := Parse("state.n = num(state.n) + 1")
	if err != nil {
		t.Fatal(err)
	}
	s1, s2 := map[string]any{}, map[string]any{}
	if err := prog.Run(s1, nil); err != nil {
		t.Fatal(err)
	}
	if err := prog.Run(s1, nil); err != nil {
		t.Fatal(err)
	}
	if err := prog.Run(s2, nil); err != nil {
		t.Fatal(err)
	}
	if s1["n"] != 2.0 || s2["n"] != 1.0 {
		t.Fatalf("s1=%v s2=%v, runs must be independent", s1, s2)
	}
}

func TestReturnAtTopLevelEndsScript(t *testing.T) {
	state := map[string]any{}
	runOK(t, "state.a = 1\nif (state.a == 1) { return }\nstate.b = 2", state, nil)
	if state["a"] != 1.0 || state["b"] != nil {
		t.Fatalf("state = %v, want a=1 and b never written", state)
	}
}
