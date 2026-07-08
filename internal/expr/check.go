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
		for _, a := range t.args {
			c.infer(a) // still check inside arguments
		}
		return typeUnknown // builtin results are never assumed
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
	}
	return "?"
}
