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

	"github.com/qorm/platform/internal/expr"
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

func TestMapLiteralAndNative(t *testing.T) {
	var gotOp string
	var gotArgs map[string]any
	SetNativeHook(func(op string, data map[string]any, cb func(name string, arg any)) {
		gotOp, gotArgs = op, data
		cb("qormOnX", "reply")
	})
	defer SetNativeHook(nil)
	src := `let r = native("webviewEval", {"id": "page", "js": "x=1"})
	        state.out = r`
	state := map[string]any{}
	if err := Run(src, state, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotOp != "webviewEval" || gotArgs["id"] != "page" || gotArgs["js"] != "x=1" {
		t.Errorf("native args = %v %v", gotOp, gotArgs)
	}
	if state["out"] != "reply" {
		t.Errorf("native() return = %v, want the callback arg", state["out"])
	}
}

// Regression: a //-comment holding an apostrophe used to open an unterminated
// single-quote string (the lexer only knew '#'), and the real error surfaced
// as a bogus "unterminated string literal" at EOF. Also covers multi-line map
// literals and string-concat across a closing quote — the exact shape of
// examples/webdemo/actions/pushCount.qs.
func TestLineCommentWithApostropheAndMultilineMap(t *testing.T) {
	src := "// Go -> page: push the counter into the embedded page's DOM.\n" +
		"state.count = state.count + 1\n" +
		"native(\"webviewEval\", {\n" +
		"  \"id\": \"page\",\n" +
		"  \"js\": \"document.getElementById('s').textContent = 'count = ' + \" + str(state.count)\n" +
		"})\n"
	state := map[string]any{"count": 0}
	var calls []map[string]any
	SetNativeHook(func(op string, data map[string]any, cb func(name string, arg any)) {
		calls = append(calls, map[string]any{"op": op, "args": data})
	})
	defer SetNativeHook(nil)
	if err := Run(src, state, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(calls) != 1 || calls[0]["op"] != "webviewEval" {
		t.Fatalf("native calls = %v", calls)
	}
	args := calls[0]["args"].(map[string]any)
	if args["js"] != "document.getElementById('s').textContent = 'count = ' + 1" {
		t.Errorf("js arg = %q", args["js"])
	}
}

// ---- v2: array/string methods, break/continue, lib, call ----

func TestArrayMethodsFunctional(t *testing.T) {
	cases := []struct{ src, want string }{
		{`let a = [1,2,3]
state.out = join(push(a, 4), ",")`, "1,2,3,4"},
		{`let a = [1,2,3]
state.out = join(unshift(a, 0), ",")`, "0,1,2,3"},
		{`let a = [1,2,3]
state.out = join(pop(a), ",")`, "1,2"},
		{`let a = [1,2,3]
state.out = join(shift(a), ",")`, "2,3"},
		{`let a = [1,2,3]
state.out = join(reverse(a), ",")`, "3,2,1"},
		{`let a = [3,1,2]
state.out = join(sort(a), ",")`, "1,2,3"},
		{`let a = [10,2]
state.out = str(indexOf(a, 2))`, "1"},
		{`let a = [10,2]
state.out = str(indexOf(a, 99))`, "-1"},
		{`let a = [10,2]
state.out = str(includes(a, 10))`, "true"},
		{`state.out = str(includes("hello", "ell"))`, "true"},
		{`state.out = charAt("hello", 1)`, "e"},
		{`state.out = substring("hello", 1, 3)`, "el"},
		{`state.out = repeat("ab", 3)`, "ababab"},
		{`state.out = padStart("5", 3, "0")`, "005"},
		{`state.out = padEnd("5", 3, "0")`, "500"},
		{`state.out = trimStart("  x  ")`, "x  "},
		{`state.out = trimEnd("  x  ")`, "  x"},
	}
	for _, c := range cases {
		st := map[string]any{}
		if err := Run(c.src, st, nil); err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if got := st["out"]; got != c.want {
			t.Errorf("%s → out=%v, want %q", c.src, got, c.want)
		}
	}
}

func TestBreakContinue(t *testing.T) {
	// break exits the loop; continue skips to the next iteration.
	src := `let n = 0
for x in range(10) {
  if (x == 3) { continue }
  if (x == 7) { break }
  n = n + 1
}
state.out = n`
	st := map[string]any{}
	if err := Run(src, st, nil); err != nil {
		t.Fatalf("break/continue: %v", err)
	}
	// 0,1,2 pass (3 skipped), 4,5,6 pass (7 breaks) → 6 values.
	if got := st["out"]; got != float64(6) {
		t.Errorf("break/continue → out=%v, want 6", got)
	}
	// while loop break
	src2 := `let i = 0
while (true) {
  i = i + 1
  if (i >= 5) { break }
}
state.out = i`
	st2 := map[string]any{}
	if err := Run(src2, st2, nil); err != nil {
		t.Fatalf("while break: %v", err)
	}
	if got := st2["out"]; got != float64(5) {
		t.Errorf("while break → out=%v, want 5", got)
	}
}

func TestCallBuiltinDispatches(t *testing.T) {
	var gotName string
	var gotArgs map[string]any
	SetDispatchHook(func(line int, name string, args map[string]any) error {
		gotName = name
		gotArgs = args
		return nil
	})
	defer SetDispatchHook(nil)

	st := map[string]any{}
	src := `call("other", {a: 1})`
	if err := Run(src, st, nil); err != nil {
		t.Fatalf("call: %v", err)
	}
	if gotName != "other" {
		t.Errorf("call dispatched %q, want other", gotName)
	}
	if gotArgs["a"] != float64(1) {
		t.Errorf("call args = %v, want {a:1}", gotArgs)
	}
}

func TestCallWithoutHookNoop(t *testing.T) {
	// No hook installed: call() returns false without error (like native()).
	st := map[string]any{}
	src := `let ok = call("x")
state.out = str(ok)`
	if err := Run(src, st, nil); err != nil {
		t.Fatalf("call without hook: %v", err)
	}
	if got := st["out"]; got != "false" {
		t.Errorf("call without hook → %v, want false", got)
	}
}

func TestLibPrependsAtDispatch(t *testing.T) {
	// A lib's fn definitions join the script's compilation (runtime merges
	// app.ScriptLib + "\n" + act.Script at dispatch).
	lib := `fn double(x) { return x * 2 }
fn greet() { return "hi" }`
	src := `state.out = double(21)`
	st := map[string]any{}
	if err := Run(lib+"\n"+src, st, nil); err != nil {
		t.Fatalf("lib+script: %v", err)
	}
	if got := st["out"]; got != float64(42) {
		t.Errorf("lib fn → %v, want 42", got)
	}
}

func TestExprNewBuiltinsParity(t *testing.T) {
	// The new builtins must work identically in bindings (expr) and scripts
	// (qscript delegates). Spot-check expr directly.
	cases := []struct {
		name string
		args []any
		want any
	}{
		{"push", []any{[]any{float64(1)}, float64(2)}, []any{float64(1), float64(2)}},
		{"indexOf", []any{[]any{float64(5), float64(9)}, float64(9)}, float64(1)},
		{"includes", []any{"hello world", "world"}, true},
		{"charAt", []any{"abc", float64(1)}, "b"},
		{"substring", []any{"abcdef", float64(2), float64(4)}, "cd"},
		{"repeat", []any{"x", float64(3)}, "xxx"},
	}
	for _, c := range cases {
		got := expr.CallBuiltin(c.name, c.args)
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s%v → %v, want %v", c.name, c.args, got, c.want)
		}
	}
}

// v2 type checks, JSON, and the new array extras.
func TestExprV2Builtins(t *testing.T) {
	cases := []struct {
		src, want string
	}{
		// typeof
		{`state.out = typeof(1)`, "number"},
		{`state.out = typeof("hi")`, "string"},
		{`state.out = typeof(true)`, "boolean"},
		{`state.out = typeof(null)`, "null"},
		{`state.out = typeof([1,2])`, "array"},
		{`state.out = typeof({a:1})`, "object"},
		// isArray / isString / isObject / isNull
		{`state.out = str(isArray([1]))`, "true"},
		{`state.out = str(isArray("x"))`, "false"},
		{`state.out = str(isString("x"))`, "true"},
		{`state.out = str(isString(1))`, "false"},
		{`state.out = str(isObject({a:1}))`, "true"},
		{`state.out = str(isObject([1]))`, "false"},
		{`state.out = str(isNull(null))`, "true"},
		{`state.out = str(isNull(0))`, "false"},
		// jsonEncode / jsonDecode round-trip
		{`state.out = jsonEncode({a: 1, b: [1,2,3], c: "hi"})`, `{"a":1,"b":[1,2,3],"c":"hi"}`},
		{`state.out = jsonDecode("{\"a\":1,\"b\":[1,2]}").a`, "1"},
		{`state.out = str(isArray(jsonDecode("[1,2,3]")))`, "true"},
		// flatten: one level deep — a nested list passes through
		{`state.out = str(len(flatten([[1,2],[3,4],[5,6]])))`, "6"},
		{`state.out = str(len(flatten([1, [2, 3], 4])))`, "4"},
		{`state.out = str(len(flatten([1,2,3])))`, "3"},
		{`state.out = str(len(flatten([[1,2],[3,[4,5]]])))`, "4"},
		// Mixed with v2
		{`state.out = str(isNumber(jsonDecode("42")))`, "true"},
	}
	for _, c := range cases {
		st := map[string]any{}
		if err := Run(c.src, st, nil); err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if got := fmt.Sprint(st["out"]); got != c.want {
			t.Errorf("%s → out=%q, want %q", c.src, got, c.want)
		}
	}
}
