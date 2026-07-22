//go:build !(js && wasm)

package qormext

import "testing"

// captureCalls installs a recording evaluator via SetEvaluator and restores
// the previous one when the test ends, so tests stay isolated from each other
// despite the package-level evaluator.
func captureCalls(t *testing.T) *[]string {
	t.Helper()
	prev := evaluator
	var calls []string
	SetEvaluator(func(s string) { calls = append(calls, s) })
	t.Cleanup(func() { SetEvaluator(prev) })
	return &calls
}

// TestSetEvaluatorCallJS verifies the wired evaluator receives every script
// handed to CallJS, in order and unmodified.
func TestSetEvaluatorCallJS(t *testing.T) {
	calls := captureCalls(t)
	CallJS("one()")
	CallJS("two()")
	want := []string{"one()", "two()"}
	if len(*calls) != len(want) {
		t.Fatalf("evaluator received %d calls %v, want %d calls %v", len(*calls), *calls, len(want), want)
	}
	for i := range want {
		if (*calls)[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, (*calls)[i], want[i])
		}
	}
}

// TestCallJSWithoutEvaluator verifies CallJS is a safe no-op when no
// evaluator is wired, and that SetEvaluator(nil) detaches the previous one.
func TestCallJSWithoutEvaluator(t *testing.T) {
	prev := evaluator
	t.Cleanup(func() { SetEvaluator(prev) })

	var calls []string
	SetEvaluator(func(s string) { calls = append(calls, s) })
	CallJS("before()")
	if len(calls) != 1 {
		t.Fatalf("setup: evaluator got %d calls, want 1", len(calls))
	}

	SetEvaluator(nil)
	CallJS("dropped()") // must not panic and must not reach the old evaluator
	if len(calls) != 1 {
		t.Errorf("CallJS with a nil evaluator reached the previous evaluator: %v", calls)
	}
}

// TestEmit verifies Emit renders the qormEmit(event, dataJSON) call through
// the evaluator: dataJSON passes through verbatim, empty dataJSON becomes
// null, and the event name is quoted/escaped.
func TestEmit(t *testing.T) {
	calls := captureCalls(t)
	cases := []struct {
		event string
		data  string
		want  string
	}{
		{"foo", `{"a":1}`, `qormEmit("foo",{"a":1})`},
		{"bar", "", `qormEmit("bar",null)`},             // empty dataJSON becomes null
		{`say "hi"`, "", `qormEmit("say \"hi\"",null)`}, // event is quoted/escaped
		{"e", `[1,2]`, `qormEmit("e",[1,2])`},           // arbitrary JSON value passes through
	}
	for _, c := range cases {
		Emit(c.event, c.data)
	}
	if len(*calls) != len(cases) {
		t.Fatalf("evaluator received %d calls %v, want %d", len(*calls), *calls, len(cases))
	}
	for i, c := range cases {
		if (*calls)[i] != c.want {
			t.Errorf("Emit(%q, %q) produced %q, want %q", c.event, c.data, (*calls)[i], c.want)
		}
	}
}

// TestNative verifies Native renders qormToNative(op, dataJSON) through the
// evaluator: empty dataJSON becomes {}, and the op name is quoted/escaped.
func TestNative(t *testing.T) {
	calls := captureCalls(t)
	cases := []struct {
		op   string
		data string
		want string
	}{
		{"bluetoothScan", `{"x":1}`, `qormToNative("bluetoothScan",{"x":1})`},
		{"op", "", `qormToNative("op",{})`},             // empty dataJSON becomes {}
		{`a"b\c`, `{}`, `qormToNative("a\"b\\c",{})`},   // quote and backslash escaped
		{"hello中", `{}`, `qormToNative("hello中",{})`},   // non-ASCII passes through
		{"bad\nop", `{}`, `qormToNative("bad\nop",{})`}, // newline in op is escaped, not raw
	}
	for _, c := range cases {
		Native(c.op, c.data)
	}
	if len(*calls) != len(cases) {
		t.Fatalf("evaluator received %d calls %v, want %d", len(*calls), *calls, len(cases))
	}
	for i, c := range cases {
		if (*calls)[i] != c.want {
			t.Errorf("Native(%q, %q) produced %q, want %q", c.op, c.data, (*calls)[i], c.want)
		}
	}
}

// TestJSStr verifies jsStr wraps a string in double quotes and escapes
// everything that would otherwise terminate the literal, break it across
// lines, or be an illegal raw character inside it: quote, backslash,
// newline, carriage return, tab, backspace, form feed, the remaining C0
// control characters (as \u00XX, like a JSON escaper), and the U+2028/U+2029
// line separators.
func TestJSStr(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", `""`},
		{"abc", `"abc"`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{`"`, `"\""`},
		{`\`, `"\\"`},
		{`mix "\ end`, `"mix \"\\ end"`},
		{"é中", `"é中"`},
		{"a\nb", `"a\nb"`},                       // newline cannot appear raw in the literal
		{"a\rb", `"a\rb"`},                       // carriage return cannot appear raw either
		{"a\tb", `"a\tb"`},                       // tab uses the short escape
		{"a\b\fc", `"a\b\fc"`},                   // backspace and form feed use short escapes
		{"\x00\x01\x1f", `"\u0000\u0001\u001f"`}, // other C0 controls become \u00XX
		{"a\u2028b", `"a\u2028b"`},               // U+2028 line separator is a JS line terminator
		{"a\u2029b", `"a\u2029b"`},               // U+2029 paragraph separator likewise
	}
	for _, c := range cases {
		if got := jsStr(c.in); got != c.want {
			t.Errorf("jsStr(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}
