// Package expr evaluates the small expression language embedded in QORM
// bindings and action steps, e.g. `count + 1`, `state.count`,
// `isLoggingIn ? "..." : "..."`, `email + "@" + domain`.
//
// Supported: number/string/bool/null literals, identifiers with dotted member
// access, unary ! and -, binary * / % + - < <= > >= == !=, && ||, and the
// ternary ?:. Values are float64, string, bool, nil, []any, map[string]any.
package expr

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Guard rails against hostile bindings: an app is authored JSON, so a
// pathological `{{ ((((…)))) }}` must not stack-overflow the process. A legit
// binding expression is short and shallow.
const (
	maxExprLen   = 64 << 10 // 64 KB source cap
	maxExprDepth = 256      // parser recursion cap (also bounds the AST/eval depth)
)

// Eval parses and evaluates src against ctx.
func Eval(src string, ctx map[string]any) (any, error) {
	if len(src) > maxExprLen {
		return nil, fmt.Errorf("expression too long (%d bytes, max %d)", len(src), maxExprLen)
	}
	p := &parser{toks: lex(src)}
	node, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, fmt.Errorf("unexpected token %q", p.peek().text)
	}
	return evalNode(node, ctx), nil
}

// ---- lexer ----

type tkind int

const (
	tEOF tkind = iota
	tNumber
	tString
	tIdent
	tOp
)

type token struct {
	kind tkind
	text string
}

func lex(s string) []token {
	var toks []token
	r := []rune(s)
	i := 0
	for i < len(r) {
		c := r[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case unicode.IsDigit(c) || (c == '.' && i+1 < len(r) && unicode.IsDigit(r[i+1])):
			j := i
			for j < len(r) && (unicode.IsDigit(r[j]) || r[j] == '.') {
				j++
			}
			toks = append(toks, token{tNumber, string(r[i:j])})
			i = j
		case c == '\'' || c == '"':
			quote := c
			j := i + 1
			var sb strings.Builder
			for j < len(r) && r[j] != quote {
				if r[j] == '\\' && j+1 < len(r) {
					j++
				}
				sb.WriteRune(r[j])
				j++
			}
			toks = append(toks, token{tString, sb.String()})
			i = j + 1
		case unicode.IsLetter(c) || c == '_' || c == '$':
			j := i
			for j < len(r) && (unicode.IsLetter(r[j]) || unicode.IsDigit(r[j]) || r[j] == '_' || r[j] == '$' || r[j] == '.') {
				j++
			}
			toks = append(toks, token{tIdent, string(r[i:j])})
			i = j
		default:
			// multi-char operators first
			two := ""
			if i+1 < len(r) {
				two = string(r[i : i+2])
			}
			switch two {
			case "&&", "||", "==", "!=", "<=", ">=":
				toks = append(toks, token{tOp, two})
				i += 2
			default:
				toks = append(toks, token{tOp, string(c)})
				i++
			}
		}
	}
	toks = append(toks, token{tEOF, ""})
	return toks
}

// ---- parser (produces a tiny AST of nodes) ----

type node interface{}

type numLit struct{ v float64 }
type strLit struct{ v string }
type boolLit struct{ v bool }
type nullLit struct{}
type ident struct{ name string }
type unary struct {
	op string
	x  node
}
type binary struct {
	op   string
	l, r node
}
type ternary struct{ cond, then, els node }
type call struct {
	name string
	args []node
}

type parser struct {
	toks  []token
	pos   int
	depth int
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }
func (p *parser) matchOp(op string) bool {
	if p.peek().kind == tOp && p.peek().text == op {
		p.pos++
		return true
	}
	return false
}

func (p *parser) parseExpr() (node, error) {
	p.depth++
	if p.depth > maxExprDepth {
		return nil, fmt.Errorf("expression too deeply nested (max %d)", maxExprDepth)
	}
	defer func() { p.depth-- }()
	return p.parseTernary()
}

func (p *parser) parseTernary() (node, error) {
	cond, err := p.parseBinary(0)
	if err != nil {
		return nil, err
	}
	if p.matchOp("?") {
		then, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.matchOp(":") {
			return nil, fmt.Errorf("expected ':' in ternary")
		}
		els, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return ternary{cond, then, els}, nil
	}
	return cond, nil
}

// operator precedence levels, low to high.
var precedence = [][]string{
	{"||"},
	{"&&"},
	{"==", "!="},
	{"<", "<=", ">", ">="},
	{"+", "-"},
	{"*", "/", "%"},
}

func (p *parser) parseBinary(level int) (node, error) {
	if level >= len(precedence) {
		return p.parseUnary()
	}
	left, err := p.parseBinary(level + 1)
	if err != nil {
		return nil, err
	}
	for {
		matched := ""
		for _, op := range precedence[level] {
			if p.peek().kind == tOp && p.peek().text == op {
				matched = op
				break
			}
		}
		if matched == "" {
			return left, nil
		}
		p.next()
		right, err := p.parseBinary(level + 1)
		if err != nil {
			return nil, err
		}
		left = binary{matched, left, right}
	}
}

func (p *parser) parseUnary() (node, error) {
	if p.peek().kind == tOp && (p.peek().text == "!" || p.peek().text == "-") {
		op := p.next().text
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return unary{op, x}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (node, error) {
	t := p.peek()
	switch t.kind {
	case tNumber:
		p.next()
		f, _ := strconv.ParseFloat(t.text, 64)
		return numLit{f}, nil
	case tString:
		p.next()
		return strLit{t.text}, nil
	case tIdent:
		p.next()
		switch t.text {
		case "true":
			return boolLit{true}, nil
		case "false":
			return boolLit{false}, nil
		case "null", "nil":
			return nullLit{}, nil
		}
		// function call: IDENT ( args )
		if p.peek().kind == tOp && p.peek().text == "(" {
			p.next()
			var args []node
			if !(p.peek().kind == tOp && p.peek().text == ")") {
				for {
					a, err := p.parseExpr()
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if !p.matchOp(",") {
						break
					}
				}
			}
			if !p.matchOp(")") {
				return nil, fmt.Errorf("expected ')' after arguments")
			}
			return call{name: t.text, args: args}, nil
		}
		return ident{t.text}, nil
	case tOp:
		if t.text == "(" {
			p.next()
			inner, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.matchOp(")") {
				return nil, fmt.Errorf("expected ')'")
			}
			return inner, nil
		}
	}
	return nil, fmt.Errorf("unexpected token %q", t.text)
}

// ---- evaluator ----

func evalNode(n node, ctx map[string]any) any {
	switch t := n.(type) {
	case numLit:
		return t.v
	case strLit:
		return t.v
	case boolLit:
		return t.v
	case nullLit:
		return nil
	case ident:
		return lookup(t.name, ctx)
	case call:
		args := make([]any, len(t.args))
		for i, a := range t.args {
			args[i] = evalNode(a, ctx)
		}
		return callBuiltin(t.name, args)
	case unary:
		x := evalNode(t.x, ctx)
		if t.op == "!" {
			return !truthy(x)
		}
		return -toNum(x)
	case ternary:
		if truthy(evalNode(t.cond, ctx)) {
			return evalNode(t.then, ctx)
		}
		return evalNode(t.els, ctx)
	case binary:
		return evalBinary(t, ctx)
	}
	return nil
}

func evalBinary(b binary, ctx map[string]any) any {
	switch b.op {
	case "&&":
		return truthy(evalNode(b.l, ctx)) && truthy(evalNode(b.r, ctx))
	case "||":
		return truthy(evalNode(b.l, ctx)) || truthy(evalNode(b.r, ctx))
	}
	l := evalNode(b.l, ctx)
	r := evalNode(b.r, ctx)
	switch b.op {
	case "+":
		if isStr(l) || isStr(r) {
			return Stringify(l) + Stringify(r)
		}
		return toNum(l) + toNum(r)
	case "-":
		return toNum(l) - toNum(r)
	case "*":
		return toNum(l) * toNum(r)
	case "/":
		return toNum(l) / toNum(r)
	case "%":
		ri := int64(toNum(r))
		if ri == 0 { // guard the truncated divisor, not just the float value
			return 0.0
		}
		return float64(int64(toNum(l)) % ri)
	case "==":
		return equals(l, r)
	case "!=":
		return !equals(l, r)
	case "<":
		return compare(l, r) < 0
	case "<=":
		return compare(l, r) <= 0
	case ">":
		return compare(l, r) > 0
	case ">=":
		return compare(l, r) >= 0
	}
	return nil
}

// lookup resolves a dotted identifier (e.g. "state.count") within ctx.
func lookup(name string, ctx map[string]any) any {
	parts := strings.Split(name, ".")
	var cur any = ctx[parts[0]]
	for _, p := range parts[1:] {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
	}
	return cur
}

// ---- value helpers ----

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

func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t != ""
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	}
	return v != nil
}

func equals(l, r any) bool {
	if isStr(l) || isStr(r) {
		return Stringify(l) == Stringify(r)
	}
	if _, ok := l.(bool); ok {
		return truthy(l) == truthy(r)
	}
	if _, ok := r.(bool); ok {
		return truthy(l) == truthy(r)
	}
	return toNum(l) == toNum(r)
}

func compare(l, r any) int {
	if isStr(l) || isStr(r) {
		return strings.Compare(Stringify(l), Stringify(r))
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

// Stringify renders a value for text interpolation.
func Stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}
