package expr

// Static type checking for binding expressions. The loader calls Check with
// the app's declared state schema so an authoring mistake like
// `{{ state.count - 1 }}` over a string-typed `count` is reported at load
// time instead of silently evaluating to a wrong value (toNum coerces
// non-numeric strings to 0).

import (
	"fmt"
	"strconv"
	"strings"
)

// Inferred type names. typeUnknown never produces a mismatch: anything the
// checker cannot prove (builtin results, comparisons, unlisted identifiers)
// is given the benefit of the doubt, so false positives stay impossible.
const (
	typeUnknown = "unknown"
	typeNumber  = "number"
	typeString  = "string"
	typeBool    = "bool"
	typeArray   = "array"
	typeObject  = "object"
	// typeExpr is a parameter-spec-only type (never inferred): the argument
	// must be a string carrying a sub-expression (map/filter/count). A string
	// literal in that position is additionally parsed and checked in place.
	typeExpr = "expr"
)

// Mismatch is one static type error found in an expression.
type Mismatch struct {
	Expr   string // the full expression source that was checked
	Detail string // e.g. `state.count is string, used as number`
}

// Check parses src (using the shared expression parser) and reports operands
// whose declared type cannot be used numerically. vars maps a dotted
// identifier to its declared type (e.g. vars["state.count"] = "number", from
// the manifest's globalState schema); identifiers not present are unknown and
// never reported. Sources that fail to parse yield no mismatches — syntax is
// a separate concern.
//
// Rules (matched to evalBinary's semantics):
//   - `-` `*` `/` `%` and unary `-` require numeric operands: a string,
//     array, or object operand is a mismatch (toNum silently coerces them
//     to 0 at runtime). Bools are legal (toNum: true=1, false=0).
//   - `+` never reports: a string operand means concatenation by design.
//   - Comparisons, logic, ternaries, and builtin calls infer unknown and are
//     never reported (their subexpressions are still checked).
//   - Indexing base[key] infers unknown; when the base is declared array, a
//     key whose declared type toNum would silently zero (string/array/object)
//     is reported.
//   - The collection/format builtins listed in callSigs get arity and
//     argument-type checks; a literal map/filter/count sub-expression is
//     parsed and checked in place. Anything not provably wrong passes.
func Check(src string, vars map[string]string) []Mismatch {
	n, err := parse(src)
	if err != nil {
		return nil
	}
	c := &checker{src: src, vars: vars}
	c.infer(n)
	return c.mismatches
}

type checker struct {
	src        string
	vars       map[string]string
	mismatches []Mismatch
}

// normalizeType maps a schema's declared type string onto the checker's type
// names; anything unrecognized is unknown (and therefore never reported).
func normalizeType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "number", "num", "int", "integer", "float", "double":
		return typeNumber
	case "string", "str", "text":
		return typeString
	case "bool", "boolean":
		return typeBool
	case "array", "list":
		return typeArray
	case "object", "map":
		return typeObject
	}
	return typeUnknown
}

// infer walks the AST bottom-up, returning the node's inferred type and
// recording mismatches for numeric operators applied to non-numeric operands.
func (c *checker) infer(n node) string {
	switch t := n.(type) {
	case numLit:
		return typeNumber
	case strLit:
		return typeString
	case boolLit:
		return typeBool
	case nullLit:
		return typeUnknown
	case ident:
		if declared, ok := c.vars[t.name]; ok {
			return normalizeType(declared)
		}
		return typeUnknown
	case call:
		argTypes := make([]string, len(t.args))
		for i, a := range t.args {
			argTypes[i] = c.infer(a) // still check inside arguments
		}
		c.checkCall(t, argTypes)
		return typeUnknown // builtin results are never assumed
	case index:
		bt := c.infer(t.base)
		kt := c.infer(t.key)
		if bt == typeArray {
			switch kt {
			case typeString, typeArray, typeObject:
				c.mismatches = append(c.mismatches, Mismatch{
					Expr:   c.src,
					Detail: fmt.Sprintf("%s is %s, used as list index", exprText(t.key), kt),
				})
			}
		}
		return typeUnknown // element types are not modeled
	case unary:
		xt := c.infer(t.x)
		if t.op == "!" {
			return typeBool
		}
		c.requireNumeric(t.x, xt) // unary minus
		return typeNumber
	case ternary:
		c.infer(t.cond)
		c.infer(t.then)
		c.infer(t.els)
		return typeUnknown // branches may differ; don't guess
	case binary:
		lt := c.infer(t.l)
		rt := c.infer(t.r)
		switch t.op {
		case "-", "*", "/", "%":
			c.requireNumeric(t.l, lt)
			c.requireNumeric(t.r, rt)
			return typeNumber
		case "+":
			// evalBinary: any string operand concatenates, so + is always legal.
			if lt == typeString || rt == typeString {
				return typeString
			}
			if lt == typeNumber && rt == typeNumber {
				return typeNumber
			}
			return typeUnknown
		}
		return typeUnknown // comparisons and logic
	}
	return typeUnknown
}

// callSig is the static contract of one checked builtin: an arity range
// (max -1 = variadic) and a wanted type per leading parameter position.
type callSig struct {
	min, max int
	params   []string
}

// callSigs lists the builtins the checker validates. Only the collection /
// format builtins are here: their misuse degrades to nil/empty at runtime, so
// an authoring mistake (sum of a string, filter with a number) would otherwise
// vanish silently. The older builtins keep their historical no-check behavior.
var callSigs = map[string]callSig{
	"at":     {2, 2, []string{typeArray, typeNumber}},
	"first":  {1, 1, []string{typeArray}},
	"last":   {1, 1, []string{typeArray}},
	"sum":    {1, 1, []string{typeArray}},
	"avg":    {1, 1, []string{typeArray}},
	"count":  {1, 2, []string{typeArray, typeExpr}},
	"join":   {2, 2, []string{typeArray, typeString}},
	"split":  {2, 2, []string{typeString, typeString}},
	"keys":   {1, 1, []string{typeObject}},
	"values": {1, 1, []string{typeObject}},
	"map":    {2, 2, []string{typeArray, typeExpr}},
	"filter": {2, 2, []string{typeArray, typeExpr}},
	"format": {1, -1, []string{typeString}},
}

// checkCall validates a call against callSigs: arity first (wrong arity makes
// positional type checks meaningless, so it reports and stops), then each
// declared parameter, then — for a literal sub-expression argument — a parse
// and a recursive Check of the literal itself.
func (c *checker) checkCall(t call, argTypes []string) {
	sig, ok := callSigs[t.name]
	if !ok {
		return
	}
	if n := len(t.args); n < sig.min || (sig.max >= 0 && n > sig.max) {
		c.mismatches = append(c.mismatches, Mismatch{Expr: c.src, Detail: arityDetail(t.name, sig, n)})
		return
	}
	for i, want := range sig.params {
		if i >= len(t.args) {
			break
		}
		if !argCompatible(want, argTypes[i]) {
			wantDesc := want
			if want == typeExpr {
				wantDesc = "a string sub-expression"
			}
			c.mismatches = append(c.mismatches, Mismatch{
				Expr: c.src,
				Detail: fmt.Sprintf("%s is %s, %s() argument %d wants %s",
					exprText(t.args[i]), argTypes[i], t.name, i+1, wantDesc),
			})
			continue
		}
		if want == typeExpr {
			if lit, ok := t.args[i].(strLit); ok {
				c.checkSubExpr(t.name, i+1, lit.v)
			}
		}
	}
}

// checkSubExpr statically validates a literal map/filter/count sub-expression:
// it must parse, and its own contents are re-checked (with no declared vars —
// `it` is the only runtime binding and its shape is unknown, so only provable
// mistakes such as literal misuse or bad nested calls can surface). Nesting
// terminates because each literal is strictly shorter than its quoting outer
// source.
func (c *checker) checkSubExpr(fn string, pos int, src string) {
	if _, err := parse(src); err != nil {
		c.mismatches = append(c.mismatches, Mismatch{
			Expr:   c.src,
			Detail: fmt.Sprintf("%s() argument %d sub-expression %q does not parse: %v", fn, pos, src, err),
		})
		return
	}
	for _, m := range Check(src, nil) {
		c.mismatches = append(c.mismatches, Mismatch{
			Expr:   c.src,
			Detail: fmt.Sprintf("in %s() sub-expression %q: %s", fn, src, m.Detail),
		})
	}
}

// arityDetail words an arity mismatch for the three signature shapes.
func arityDetail(name string, sig callSig, n int) string {
	switch {
	case sig.max < 0:
		return fmt.Sprintf("%s() takes at least %d argument%s, got %d", name, sig.min, plural(sig.min), n)
	case sig.min == sig.max:
		return fmt.Sprintf("%s() takes %d argument%s, got %d", name, sig.min, plural(sig.min), n)
	default:
		return fmt.Sprintf("%s() takes %d to %d arguments, got %d", name, sig.min, sig.max, n)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// argCompatible reports whether an inferred argument type can satisfy a
// wanted parameter type. Unknown is always compatible (the no-false-positive
// contract); bools and numbers stringify losslessly, so a string parameter
// only rejects arrays and objects.
func argCompatible(want, got string) bool {
	if got == typeUnknown {
		return true
	}
	switch want {
	case typeNumber:
		return got == typeNumber || got == typeBool
	case typeArray:
		return got == typeArray
	case typeObject:
		return got == typeObject
	case typeString:
		return got != typeArray && got != typeObject
	default: // typeExpr
		return got == typeString
	}
}

// requireNumeric records a mismatch when an operand of a numeric operator has
// a type that toNum would silently zero out.
func (c *checker) requireNumeric(n node, typ string) {
	switch typ {
	case typeString, typeArray, typeObject:
		c.mismatches = append(c.mismatches, Mismatch{
			Expr:   c.src,
			Detail: fmt.Sprintf("%s is %s, used as number", exprText(n), typ),
		})
	}
}

// exprText reconstructs a readable form of an AST node for diagnostics.
func exprText(n node) string {
	switch t := n.(type) {
	case numLit:
		return Stringify(t.v)
	case strLit:
		return strconv.Quote(t.v)
	case boolLit:
		if t.v {
			return "true"
		}
		return "false"
	case nullLit:
		return "null"
	case ident:
		return t.name
	case unary:
		return t.op + exprText(t.x)
	case binary:
		return exprText(t.l) + " " + t.op + " " + exprText(t.r)
	case ternary:
		return exprText(t.cond) + " ? " + exprText(t.then) + " : " + exprText(t.els)
	case call:
		args := make([]string, len(t.args))
		for i, a := range t.args {
			args[i] = exprText(a)
		}
		return t.name + "(" + strings.Join(args, ", ") + ")"
	case index:
		return exprText(t.base) + "[" + exprText(t.key) + "]"
	}
	return "?"
}
