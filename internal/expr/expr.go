// Package expr evaluates the small expression language embedded in QORM
// bindings and action steps, e.g. `count + 1`, `state.count`,
// `isLoggingIn ? "..." : "..."`, `email + "@" + domain`.
//
// Supported: number/string/bool/null literals, identifiers with dotted member
// access, postfix indexing base[expr] (list by number, object by string key;
// out-of-range or missing keys yield nil) with dotted member access resuming
// after a bracket (users[0].name), unary ! and -, binary * / % + - < <= > >=
// == !=, && ||, and the ternary ?:. Values are float64, string, bool, nil,
// []any, map[string]any.
package expr

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

// Guard rails against hostile bindings: an app is authored JSON, so a
// pathological `{{ ((((…)))) }}` must not stack-overflow the process. A legit
// binding expression is short and shallow.
const (
	maxExprLen   = 64 << 10 // 64 KB source cap
	maxExprDepth = 256      // parser recursion cap (also bounds the AST/eval depth)

	// Guard rails for string sub-expressions (the map/filter/count predicate
	// arguments, evaluated by evalSub). A predicate is itself an expression, so
	// it re-enters the evaluator; cyclic data (a list appended to itself by an
	// action) plus a self-referential predicate string could otherwise recurse
	// or branch without bound. Nesting counts into the same depth guard as the
	// parser (maxSubExprDepth = maxExprDepth), and total predicate evaluations
	// per top-level Eval are capped so exponential branching degrades to nil
	// instead of hanging a render.
	maxSubExprDepth = maxExprDepth
	maxSubExprEvals = 100_000
)

// parsed caches a parse result (a binding string is evaluated once per render,
// but the same handful of expressions render over and over, so parsing them each
// time is pure waste). The cache is bounded so a pathological app can't grow it.
type parsed struct {
	node node
	err  error
}

var (
	astCache sync.Map // src string -> parsed
	astCount atomic.Int64
)

const maxASTCache = 8192

func parse(src string) (node, error) {
	if v, ok := astCache.Load(src); ok {
		c := v.(parsed)
		return c.node, c.err
	}
	n, err := parseUncached(src)
	if astCount.Load() < maxASTCache {
		if _, loaded := astCache.LoadOrStore(src, parsed{n, err}); !loaded {
			astCount.Add(1)
		}
	}
	return n, err
}

func parseUncached(src string) (node, error) {
	if len(src) > maxExprLen {
		return nil, fmt.Errorf("expression too long (%d bytes, max %d)", len(src), maxExprLen)
	}
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	node, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, fmt.Errorf("unexpected token %q", p.peek().text)
	}
	return node, nil
}

// Eval parses (cached) and evaluates src against ctx.
func Eval(src string, ctx map[string]any) (any, error) {
	node, err := parse(src)
	if err != nil {
		return nil, err
	}
	return evalNode(node, ctx, &evalEnv{}), nil
}

// CloseIndex returns the offset of the first "}}" in s that lies outside a
// string literal, or -1 if there is none. Quote tracking mirrors the
// expression lexer (single or double quotes with backslash escapes) so a
// "}}" inside a binding's string literal — e.g. {{ '}}' }} — is not mistaken
// for the closing delimiter. It is the one shared way to find a binding's
// closing delimiter: the loader's static checks and the runtime's binding
// evaluation both scan with it, so a binding that validates at load time
// cannot be split differently at render time.
func CloseIndex(s string) int {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			switch {
			case c == '\\':
				i++ // skip the escaped character
			case c == quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '}':
			if i+1 < len(s) && s[i+1] == '}' {
				return i
			}
		}
	}
	return -1
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

// lex tokenizes s. It reports an error for malformed lexemes — an
// unterminated string literal, in particular — rather than coercing them into
// a plausible value: binding expressions come from authored, untrusted JSON,
// so malformed input must fail loudly.
func lex(s string) ([]token, error) {
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
			if j >= len(r) {
				return nil, fmt.Errorf("unterminated string literal (missing closing %q)", string(quote))
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
	return toks, nil
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

// index is postfix access: base[key] for an explicit bracket, and also the
// desugared form of `.name` member access that resumes after a bracket chain
// (users[0].name parses to index{index{users, 0}, "name"}). A plain dotted
// identifier (state.count) is still a single ident token, unchanged.
type index struct{ base, key node }

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
	prim, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	return p.parsePostfix(prim)
}

// parsePostfix applies postfix accessors to a parsed primary: `[expr]` indexes
// the base by an arbitrary expression, and a `.` token (which the lexer only
// emits when a dot is not absorbed into an identifier, e.g. right after `]` or
// `)`) resumes dotted member access. The loop is iterative, mirroring the
// binary-operator chains: chain length is bounded by the 64 KB source cap,
// while a *nested* index (a[a[a[...]]]) recurses through parseExpr and is
// bounded by the depth guard.
func (p *parser) parsePostfix(base node) (node, error) {
	for {
		switch {
		case p.matchOp("["):
			key, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.matchOp("]") {
				return nil, fmt.Errorf("expected ']' after index expression")
			}
			base = index{base, key}
		case p.matchOp("."):
			t := p.peek()
			if t.kind != tIdent {
				return nil, fmt.Errorf("expected identifier after '.'")
			}
			p.next()
			// The identifier token may itself be dotted (the lexer greedily
			// consumes dots): a[0].b.c arrives as one "b.c" token.
			for _, part := range strings.Split(t.text, ".") {
				base = index{base, strLit{part}}
			}
		default:
			return base, nil
		}
	}
}

func (p *parser) parsePrimary() (node, error) {
	t := p.peek()
	switch t.kind {
	case tNumber:
		p.next()
		// The lexer greedily consumes digits and dots into one lexeme, so a
		// malformed literal like "1.2.3" or "9.." arrives here intact; it must
		// be a parse error, never a silent 0 (bindings are authored JSON).
		// strconv.ErrRange (a well-formed literal overflowing to +/-Inf or
		// underflowing to 0) stays lenient, preserving the values the lexer
		// and evaluator have always produced for such literals.
		f, err := strconv.ParseFloat(t.text, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return nil, fmt.Errorf("invalid number literal %q", t.text)
		}
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

// evalEnv carries per-evaluation state shared across one Eval call's whole
// tree, including every string sub-expression it spawns (map/filter/count
// predicates). subDepth counts predicate nesting (stack guard); subEvals
// counts total predicate evaluations (work guard, so exponential branching
// over cyclic data degrades instead of hanging). See maxSubExprDepth /
// maxSubExprEvals.
type evalEnv struct {
	subDepth int
	subEvals int
}

func evalNode(n node, ctx map[string]any, env *evalEnv) any {
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
			args[i] = evalNode(a, ctx, env)
		}
		return callBuiltin(t.name, args, env)
	case index:
		return indexValue(evalNode(t.base, ctx, env), evalNode(t.key, ctx, env))
	case unary:
		x := evalNode(t.x, ctx, env)
		if t.op == "!" {
			return !truthy(x)
		}
		return -toNum(x)
	case ternary:
		if truthy(evalNode(t.cond, ctx, env)) {
			return evalNode(t.then, ctx, env)
		}
		return evalNode(t.els, ctx, env)
	case binary:
		return evalBinary(t, ctx, env)
	}
	return nil
}

func evalBinary(b binary, ctx map[string]any, env *evalEnv) any {
	switch b.op {
	case "&&":
		return truthy(evalNode(b.l, ctx, env)) && truthy(evalNode(b.r, ctx, env))
	case "||":
		return truthy(evalNode(b.l, ctx, env)) || truthy(evalNode(b.r, ctx, env))
	}
	l := evalNode(b.l, ctx, env)
	r := evalNode(b.r, ctx, env)
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

// indexValue resolves base[key]: an object is keyed by the stringified key, a
// list by the truncated numeric key. A nil base or key, a non-indexable base,
// a negative / NaN / out-of-range list index, and a missing object key all
// yield nil — bindings never panic. (The at() builtin layers negative-from-end
// semantics on top of this; plain indexing stays strict about negatives so a
// coerced-to-0-then-negative key cannot silently alias the last element.)
func indexValue(base, key any) any {
	if key == nil {
		return nil
	}
	switch c := base.(type) {
	case map[string]any:
		return c[Stringify(key)]
	case []any:
		f := math.Trunc(toNum(key))
		// The float-domain comparison rejects NaN (both sides false) and
		// out-of-range values before the int conversion can misbehave.
		if !(f >= 0 && f < float64(len(c))) {
			return nil
		}
		return c[int(f)]
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

// Truthy reports whether a value is truthy under the expression language's
// rules — the same predicate `!`, `&&`, `||` and ternaries use: nil, false, 0,
// "" and empty arrays/objects are falsy; everything else is truthy. Exported
// so the runtime's `if` step branches exactly like a `{{ cond ? a : b }}`.
func Truthy(v any) bool { return truthy(v) }

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
