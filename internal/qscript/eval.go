package qscript

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/qorm/qorm/internal/expr"
)

// ---- interpreter ----
//
// Values are float64, string, bool, nil, []any, map[string]any — exactly the
// expression language's value set. Operator, truthiness, equality, indexing
// and builtin semantics mirror internal/expr (builtins are delegated to
// expr.CallBuiltin); the parity is pinned by qscript_test.go's TestParity*.

// scope is one function invocation's variable set: parameters and `let`
// locals share it (there is no block scoping — a `let` inside a loop body
// simply re-sets the same slot each iteration), and `for` re-uses it for the
// loop variable. Functions see nothing of their caller's scope.
type scope map[string]any

type interp struct {
	fns   map[string]*fnDecl
	state map[string]any
	args  map[string]any
	ops   int // total-op counter for maxTotalOps
}

// bump charges one operation (a statement, one loop iteration, one call)
// against the run's total budget.
func (i *interp) bump(line int) error {
	i.ops++
	if i.ops > maxTotalOps {
		return &Error{line, fmt.Sprintf("script work limit exceeded (max %d operations)", maxTotalOps)}
	}
	return nil
}

// exec runs a statement list. The second return is true when a `return`
// fired; the third is the loop-control signal — 0 = normal, 1 = break,
// 2 = continue — propagated from a nested loop body so a loop's own
// handler can distinguish its break/continue from a function return.
// depth counts nested fn calls (maxCallDepth); block nesting is parse-bounded.
func (i *interp) exec(stmts []stmt, sc scope, depth int) (any, bool, int, error) {
	for _, s := range stmts {
		if err := i.bump(s.stmtLine()); err != nil {
			return nil, false, 0, err
		}
		v, ret, flow, err := i.execStmt(s, sc, depth)
		if err != nil || ret || flow != 0 {
			return v, ret, flow, err
		}
	}
	return nil, false, 0, nil
}

func (i *interp) execStmt(s stmt, sc scope, depth int) (any, bool, int, error) {
	switch t := s.(type) {
	case *letStmt:
		v, err := i.eval(t.val, sc, depth)
		if err != nil {
			return nil, false, 0, err
		}
		sc[t.name] = v
	case *assignStmt:
		v, err := i.eval(t.val, sc, depth)
		if err != nil {
			return nil, false, 0, err
		}
		if err := i.assign(t.target, v, sc, depth); err != nil {
			return nil, false, 0, err
		}
	case *exprStmt:
		if _, err := i.eval(t.e, sc, depth); err != nil {
			return nil, false, 0, err
		}
	case *ifStmt:
		c, err := i.eval(t.cond, sc, depth)
		if err != nil {
			return nil, false, 0, err
		}
		if expr.Truthy(c) {
			return i.exec(t.then, sc, depth)
		}
		return i.exec(t.els, sc, depth)
	case *whileStmt:
		iters := 0
		for {
			c, err := i.eval(t.cond, sc, depth)
			if err != nil {
				return nil, false, 0, err
			}
			if !expr.Truthy(c) {
				break
			}
			iters++
			if iters > maxLoopIters {
				return nil, false, 0, &Error{t.line, fmt.Sprintf("loop iteration limit exceeded (max %d)", maxLoopIters)}
			}
			if err := i.bump(t.line); err != nil {
				return nil, false, 0, err
			}
			v, ret, flow, err := i.exec(t.body, sc, depth)
			if err != nil {
				return v, false, 0, err
			}
			if ret {
				return v, true, 0, nil
			}
			if flow == 1 { // break: leave the loop, signal consumed
				break
			}
			// flow == 2 (continue): just move to the next iteration
		}
	case *forStmt:
		in, err := i.eval(t.in, sc, depth)
		if err != nil {
			return nil, false, 0, err
		}
		if in == nil {
			break // iterating nothing is a no-op, like a `forEach` over a missing path
		}
		arr, ok := in.([]any)
		if !ok {
			return nil, false, 0, &Error{t.line, fmt.Sprintf("for-in needs an array to iterate, got %s", typeName(in))}
		}
		if len(arr) > maxLoopIters {
			return nil, false, 0, &Error{t.line, fmt.Sprintf("loop iteration limit exceeded (max %d)", maxLoopIters)}
		}
		for _, el := range arr {
			if err := i.bump(t.line); err != nil {
				return nil, false, 0, err
			}
			sc[t.varName] = el
			v, ret, flow, err := i.exec(t.body, sc, depth)
			if err != nil {
				return v, false, 0, err
			}
			if ret {
				return v, true, 0, nil
			}
			if flow == 1 { // break
				break
			}
			// flow == 2 (continue): next iteration
		}
	case *breakStmt:
		return nil, false, 1, nil
	case *continueStmt:
		return nil, false, 2, nil
	case *returnStmt:
		if t.val == nil {
			return nil, true, 0, nil
		}
		v, err := i.eval(t.val, sc, depth)
		if err != nil {
			return nil, false, 0, err
		}
		return v, true, 0, nil
	}
	return nil, false, 0, nil
}

// assign writes v at the l-value target: a local (`x`), a state path
// (`state.a`, `state.obj.k`), or an indexed element (`state.arr[i]`, `m.k`
// where m is a local map). Dotted paths create intermediate maps exactly like
// the runtime's setPath; index writes mutate the container in place.
func (i *interp) assign(target exprNode, v any, sc scope, depth int) error {
	switch t := target.(type) {
	case ident:
		parts := strings.Split(t.name, ".")
		root := parts[0]
		if root == "state" || root == "args" {
			if len(parts) == 1 {
				return &Error{t.line, fmt.Sprintf("cannot replace the %q handle itself; assign a path under it (e.g. %s.x = ...)", root, root)}
			}
			base := i.state
			if root == "args" {
				base = i.args
			}
			setMapPath(base, parts[1:], v)
			return nil
		}
		cur, ok := sc[root]
		if !ok {
			return &Error{t.line, fmt.Sprintf("assignment to undeclared variable %q (declare it first with let)", root)}
		}
		if len(parts) == 1 {
			sc[root] = v
			return nil
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return &Error{t.line, fmt.Sprintf("%s is %s, not an object — cannot set %q on it", root, typeName(cur), parts[1])}
		}
		setMapPath(m, parts[1:], v)
		return nil
	case index:
		base, err := i.eval(t.base, sc, depth)
		if err != nil {
			return err
		}
		key, err := i.eval(t.key, sc, depth)
		if err != nil {
			return err
		}
		switch c := base.(type) {
		case map[string]any:
			c[expr.Stringify(key)] = v
			return nil
		case []any:
			f := math.Trunc(toNum(key))
			if !(f >= 0 && f < float64(len(c))) {
				return &Error{t.line, fmt.Sprintf("array index %s out of range (length %d)", expr.Stringify(key), len(c))}
			}
			c[int(f)] = v
			return nil
		}
		return &Error{t.line, fmt.Sprintf("cannot index-assign %s (needs an array or object)", typeName(base))}
	}
	return &Error{target.exprLine(), "invalid assignment target"}
}

// setMapPath walks m along parts, creating intermediate maps, and sets the
// final key — the runtime's setPath semantics for dotted state paths.
func setMapPath(m map[string]any, parts []string, v any) {
	for _, p := range parts[:len(parts)-1] {
		next, ok := m[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			m[p] = next
		}
		m = next
	}
	m[parts[len(parts)-1]] = v
}

// ---- expression evaluation ----

func (i *interp) eval(e exprNode, sc scope, depth int) (any, error) {
	switch t := e.(type) {
	case numLit:
		return t.v, nil
	case strLit:
		return t.v, nil
	case boolLit:
		return t.v, nil
	case nullLit:
		return nil, nil
	case ident:
		return i.lookup(t.name, sc), nil
	case mapLit:
		out := make(map[string]any, len(t.keys))
		for k, key := range t.keys {
			v, err := i.eval(t.vals[k], sc, depth)
			if err != nil {
				return nil, err
			}
			out[key] = v
		}
		return out, nil
	case arrayLit:
		out := make([]any, len(t.elems))
		for k, el := range t.elems {
			v, err := i.eval(el, sc, depth)
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
		return out, nil
	case index:
		base, err := i.eval(t.base, sc, depth)
		if err != nil {
			return nil, err
		}
		key, err := i.eval(t.key, sc, depth)
		if err != nil {
			return nil, err
		}
		return indexValue(base, key), nil
	case unary:
		x, err := i.eval(t.x, sc, depth)
		if err != nil {
			return nil, err
		}
		if t.op == "!" {
			return !expr.Truthy(x), nil
		}
		return -toNum(x), nil
	case ternary:
		c, err := i.eval(t.cond, sc, depth)
		if err != nil {
			return nil, err
		}
		if expr.Truthy(c) {
			return i.eval(t.then, sc, depth)
		}
		return i.eval(t.els, sc, depth)
	case binary:
		return i.evalBinary(t, sc, depth)
	case call:
		return i.evalCall(t, sc, depth)
	}
	return nil, nil
}

func (i *interp) evalBinary(b binary, sc scope, depth int) (any, error) {
	switch b.op {
	case "&&":
		l, err := i.eval(b.l, sc, depth)
		if err != nil {
			return nil, err
		}
		if !expr.Truthy(l) {
			return false, nil
		}
		r, err := i.eval(b.r, sc, depth)
		return expr.Truthy(r), err
	case "||":
		l, err := i.eval(b.l, sc, depth)
		if err != nil {
			return nil, err
		}
		if expr.Truthy(l) {
			return true, nil
		}
		r, err := i.eval(b.r, sc, depth)
		return expr.Truthy(r), err
	}
	l, err := i.eval(b.l, sc, depth)
	if err != nil {
		return nil, err
	}
	r, err := i.eval(b.r, sc, depth)
	if err != nil {
		return nil, err
	}
	switch b.op {
	case "+":
		if isStr(l) || isStr(r) {
			return expr.Stringify(l) + expr.Stringify(r), nil
		}
		return toNum(l) + toNum(r), nil
	case "-":
		return toNum(l) - toNum(r), nil
	case "*":
		return toNum(l) * toNum(r), nil
	case "/":
		return toNum(l) / toNum(r), nil
	case "%":
		ri := int64(toNum(r))
		if ri == 0 {
			return 0.0, nil
		}
		return float64(int64(toNum(l)) % ri), nil
	case "==":
		return equals(l, r), nil
	case "!=":
		return !equals(l, r), nil
	case "<":
		return compare(l, r) < 0, nil
	case "<=":
		return compare(l, r) <= 0, nil
	case ">":
		return compare(l, r) > 0, nil
	case ">=":
		return compare(l, r) >= 0, nil
	}
	return nil, nil
}

// builtinNames is the documented v1 builtin set — the names delegated to
// expr.CallBuiltin. A call to anything else (and not a script fn) is an
// authoring error reported with its line, instead of expr's silent nil: in a
// script a typo'd function name is a logic bug, not a render blemish.
var builtinNames = map[string]bool{
	"len": true, "trim": true, "upper": true, "lower": true,
	"contains": true, "startsWith": true, "endsWith": true, "replace": true,
	"matches": true, "str": true, "num": true, "number": true, "int": true,
	"abs": true, "round": true, "floor": true, "ceil": true, "mod": true,
	"sin": true, "cos": true, "tan": true, "atan2": true, "sqrt": true,
	"min": true, "max": true, "not": true, "empty": true,
	"default": true, "coalesce": true,
	"range": true, "fill": true, "concat": true, "slice": true,
	"at": true, "first": true, "last": true, "sum": true, "avg": true,
	"count": true, "join": true, "split": true,
	"keys": true, "values": true, "map": true, "filter": true, "format": true,
	"now": true, "call": true,
	// Audio (WAV playback routed through the runtime's audio handler).
	"playSound": true, "playMusic": true, "stopMusic": true,
	// Array methods (v2): functional — each returns a NEW list.
	"push": true, "unshift": true, "pop": true, "shift": true,
	"reverse": true, "sort": true, "indexOf": true, "includes": true,
	"flatten": true,
	// String methods (v2).
	"charAt": true, "substring": true, "repeat": true,
	"padStart": true, "padEnd": true, "trimStart": true, "trimEnd": true,
	// Type checks and JSON (v2).
	"typeof": true, "isArray": true, "isList": true, "isString": true,
	"isNumber": true, "isBool": true, "isObject": true, "isNull": true,
	"jsonEncode": true, "JSON.stringify": true, "jsonDecode": true, "JSON.parse": true,
}

// nativeHook is the optional bridge to host-native ops (hardware widgets,
// the webview overlay, …). The host installs it once (canvas.SetNativeInvoker
// shapes it); scripts call native(op, args). It stays a package var (no
// canvas import — runtime → qscript → canvas would cycle).
var nativeHook func(op string, data map[string]any, cb func(name string, arg any))

// dispatchHook is the optional bridge for the call() builtin: the runtime
// installs it so a script can dispatch sibling actions by name. Returns an
// error (with the caller's script line) when the dispatched action fails —
// the runtime's invoke-depth governance applies to call() recursion.
var dispatchHook func(line int, name string, args map[string]any) error

// SetDispatchHook installs the host's action-dispatch bridge for the call()
// builtin (nil = call() returns false without dispatching).
func SetDispatchHook(fn func(line int, name string, args map[string]any) error) {
	dispatchHook = fn
}

// SetNativeHook installs the host's native-op bridge for the native()
// builtin (nil = native() fails with an authoring error).
func SetNativeHook(fn func(op string, data map[string]any, cb func(name string, arg any))) {
	nativeHook = fn
}

// callNative runs one native op through the hook and returns the first
// callback argument (true when the op calls back with nothing).
func (i *interp) callNative(line int, op string, args map[string]any) (any, error) {
	if nativeHook == nil {
		return nil, &Error{line, "native(): no native bridge installed on this host"}
	}
	var ret any = true
	nativeHook(op, args, func(name string, arg any) { ret = arg })
	return ret, nil
}

func (i *interp) evalCall(c call, sc scope, depth int) (any, error) {
	args := make([]any, len(c.args))
	for k, a := range c.args {
		v, err := i.eval(a, sc, depth)
		if err != nil {
			return nil, err
		}
		args[k] = v
	}
	if fn, ok := i.fns[c.name]; ok {
		if depth+1 > maxCallDepth {
			return nil, &Error{c.line, fmt.Sprintf("function call depth exceeded (max %d)", maxCallDepth)}
		}
		if err := i.bump(c.line); err != nil {
			return nil, err
		}
		inner := scope{}
		for k, name := range fn.params {
			if k < len(args) {
				inner[name] = args[k]
			} else {
				inner[name] = nil // missing arguments read as nil, like bindings
			}
		}
		v, _, _, err := i.exec(fn.body, inner, depth+1)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
	if c.name == "call" {
		// call("actionId" [, argsMap]): dispatch another action through the
		// host's dispatch hook (installed by the runtime). Scripts compose by
		// firing sibling actions — the recursion governance is the runtime's
		// invoke-depth machinery. Returns true when a hook is installed and
		// the action ran, false when no hook is present.
		if len(args) < 1 || len(args) > 2 {
			return nil, &Error{c.line, "call(action [, args]) takes 1-2 arguments"}
		}
		name, _ := args[0].(string)
		if name == "" {
			return nil, &Error{c.line, "call(): action name must be a non-empty string"}
		}
		m := map[string]any{}
		if len(args) == 2 {
			if mm, ok := args[1].(map[string]any); ok {
				m = mm
			}
		}
		if dispatchHook == nil {
			return false, nil // no host bridge: a no-op, like native() without one
		}
		return true, dispatchHook(c.line, name, m)
	}
	if c.name == "native" {
		// native(op [, argsMap]): run one host-native op through the installed
		// bridge — hardware widgets, webview eval, custom native ops.
		if len(args) < 1 || len(args) > 2 {
			return nil, &Error{c.line, "native(op [, args]) takes 1-2 arguments"}
		}
		op, _ := args[0].(string)
		if op == "" {
			return nil, &Error{c.line, "native(): op must be a non-empty string"}
		}
		m := map[string]any{}
		if len(args) == 2 {
			if mm, ok := args[1].(map[string]any); ok {
				m = mm
			}
		}
		return i.callNative(c.line, op, m)
	}
	if builtinNames[c.name] {
		return expr.CallBuiltin(c.name, args), nil
	}
	return nil, &Error{c.line, fmt.Sprintf("unknown function %q (not a script fn, not a builtin)", c.name)}
}

// lookup resolves a possibly-dotted identifier: the root is a local, the
// `state` or `args` handle, or nil; the dotted tail walks maps exactly like
// the expression language's lookup.
func (i *interp) lookup(name string, sc scope) any {
	parts := strings.Split(name, ".")
	var cur any
	switch parts[0] {
	case "state":
		cur = i.state
	case "args":
		cur = i.args
	default:
		cur = sc[parts[0]]
	}
	for _, p := range parts[1:] {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}

// ---- value helpers (mirror internal/expr's unexported rules) ----

func isStr(v any) bool { _, ok := v.(string); return ok }

func toNum(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case bool:
		if t {
			return 1
		}
		return 0
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	}
	return 0
}

func equals(l, r any) bool {
	if isStr(l) || isStr(r) {
		return expr.Stringify(l) == expr.Stringify(r)
	}
	if _, ok := l.(bool); ok {
		return expr.Truthy(l) == expr.Truthy(r)
	}
	if _, ok := r.(bool); ok {
		return expr.Truthy(l) == expr.Truthy(r)
	}
	return toNum(l) == toNum(r)
}

func compare(l, r any) int {
	if isStr(l) || isStr(r) {
		return strings.Compare(expr.Stringify(l), expr.Stringify(r))
	}
	a, b := toNum(l), toNum(r)
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// indexValue mirrors internal/expr's base[key] rules: an object is keyed by
// the stringified key, a list by the truncated numeric key; nil bases/keys,
// negative or out-of-range list indexes and missing keys all yield nil.
func indexValue(base, key any) any {
	if key == nil {
		return nil
	}
	switch c := base.(type) {
	case map[string]any:
		return c[expr.Stringify(key)]
	case []any:
		f := math.Trunc(toNum(key))
		if !(f >= 0 && f < float64(len(c))) {
			return nil
		}
		return c[int(f)]
	}
	return nil
}

// typeName names a value's kind for error messages.
func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "nil"
	case float64:
		return "a number"
	case string:
		return "a string"
	case bool:
		return "a bool"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	default:
		return fmt.Sprintf("%T", v)
	}
}
