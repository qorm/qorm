package qscript

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ---- AST ----

type stmt interface{ stmtLine() int }

type letStmt struct {
	name string
	val  exprNode
	line int
}

// assignStmt's target is the restricted expression shape the parser accepts
// as an l-value: a (possibly dotted) identifier with any number of postfix
// index operations — x, state.a, state.obj.k, state.arr[i], a[i].b.
type assignStmt struct {
	target exprNode
	val    exprNode
	line   int
}

type ifStmt struct {
	cond exprNode
	then []stmt
	els  []stmt
	line int
}

type whileStmt struct {
	cond exprNode
	body []stmt
	line int
}

type forStmt struct {
	varName string
	in      exprNode
	body    []stmt
	line    int
}

type returnStmt struct {
	val  exprNode // nil = bare `return`
	line int
}

type exprStmt struct {
	e    exprNode
	line int
}

func (s *letStmt) stmtLine() int    { return s.line }
func (s *assignStmt) stmtLine() int { return s.line }
func (s *ifStmt) stmtLine() int     { return s.line }
func (s *whileStmt) stmtLine() int  { return s.line }
func (s *forStmt) stmtLine() int    { return s.line }
func (s *returnStmt) stmtLine() int { return s.line }
func (s *exprStmt) stmtLine() int   { return s.line }

type fnDecl struct {
	name   string
	params []string
	body   []stmt
	line   int
}

type exprNode interface{ exprLine() int }

type numLit struct {
	v    float64
	line int
}
type strLit struct {
	v    string
	line int
}
type boolLit struct {
	v    bool
	line int
}
type nullLit struct{ line int }
type ident struct {
	name string // possibly dotted (state.piece.x) — the lexer absorbs dots
	line int
}
type arrayLit struct {
	elems []exprNode
	line  int
}
type unary struct {
	op   string
	x    exprNode
	line int
}
type binary struct {
	op   string
	l, r exprNode
	line int
}
type ternary struct {
	cond, then, els exprNode
	line            int
}
type index struct {
	base, key exprNode
	line      int
}
type call struct {
	name string
	args []exprNode
	line int
}

func (e numLit) exprLine() int   { return e.line }
func (e strLit) exprLine() int   { return e.line }
func (e boolLit) exprLine() int  { return e.line }
func (e nullLit) exprLine() int  { return e.line }
func (e ident) exprLine() int    { return e.line }
func (e arrayLit) exprLine() int { return e.line }
func (e unary) exprLine() int    { return e.line }
func (e binary) exprLine() int   { return e.line }
func (e ternary) exprLine() int  { return e.line }
func (e index) exprLine() int    { return e.line }
func (e call) exprLine() int     { return e.line }

// reservedWords cannot be used as variable, function or loop-variable names.
var reservedWords = map[string]bool{
	"let": true, "if": true, "else": true, "for": true, "in": true,
	"while": true, "return": true, "fn": true, "state": true, "args": true,
	"true": true, "false": true, "null": true, "nil": true,
}

// ---- parser ----

type parser struct {
	toks  []token
	pos   int
	depth int
}

func parseProgram(toks []token) (*Program, error) {
	p := &parser{toks: toks}
	prog := &Program{fns: map[string]*fnDecl{}}
	for p.peek().kind != tEOF {
		if p.peek().kind == tIdent && p.peek().text == "fn" {
			fn, err := p.parseFn()
			if err != nil {
				return nil, err
			}
			if _, dup := prog.fns[fn.name]; dup {
				return nil, &Error{fn.line, fmt.Sprintf("function %q declared twice", fn.name)}
			}
			prog.fns[fn.name] = fn
			continue
		}
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		prog.body = append(prog.body, s)
	}
	return prog, nil
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

func (p *parser) expectOp(op string) error {
	if !p.matchOp(op) {
		return &Error{p.peek().line, fmt.Sprintf("expected %q, found %q", op, p.peek().text)}
	}
	return nil
}

// matchKeyword consumes an identifier token with the given spelling.
func (p *parser) matchKeyword(kw string) bool {
	if p.peek().kind == tIdent && p.peek().text == kw {
		p.pos++
		return true
	}
	return false
}

func (p *parser) parseFn() (*fnDecl, error) {
	kw := p.next() // "fn"
	t := p.peek()
	if t.kind != tIdent || reservedWords[t.text] {
		return nil, &Error{t.line, fmt.Sprintf("expected a function name after 'fn', found %q", t.text)}
	}
	p.next()
	fn := &fnDecl{name: t.text, line: kw.line}
	if err := p.expectOp("("); err != nil {
		return nil, err
	}
	if !(p.peek().kind == tOp && p.peek().text == ")") {
		for {
			a := p.peek()
			if a.kind != tIdent || reservedWords[a.text] || strings.Contains(a.text, ".") {
				return nil, &Error{a.line, fmt.Sprintf("expected a parameter name, found %q", a.text)}
			}
			p.next()
			fn.params = append(fn.params, a.text)
			if !p.matchOp(",") {
				break
			}
		}
	}
	if err := p.expectOp(")"); err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	fn.body = body
	return fn, nil
}

func (p *parser) parseBlock() ([]stmt, error) {
	if err := p.expectOp("{"); err != nil {
		return nil, err
	}
	var out []stmt
	for !(p.peek().kind == tOp && p.peek().text == "}") {
		if p.peek().kind == tEOF {
			return nil, &Error{p.peek().line, "unexpected end of script inside a block (missing '}')"}
		}
		if p.peek().kind == tIdent && p.peek().text == "fn" {
			return nil, &Error{p.peek().line, "functions can only be declared at the top level"}
		}
		s, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	p.next() // "}"
	return out, nil
}

func (p *parser) parseStmt() (stmt, error) {
	t := p.peek()
	if t.kind == tIdent {
		switch t.text {
		case "let":
			p.next()
			n := p.peek()
			if n.kind != tIdent || reservedWords[n.text] || strings.Contains(n.text, ".") {
				return nil, &Error{n.line, fmt.Sprintf("expected a variable name after 'let', found %q", n.text)}
			}
			p.next()
			if err := p.expectOp("="); err != nil {
				return nil, err
			}
			v, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			return &letStmt{name: n.text, val: v, line: t.line}, nil
		case "if":
			return p.parseIf()
		case "while":
			p.next()
			cond, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			body, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			return &whileStmt{cond: cond, body: body, line: t.line}, nil
		case "for":
			p.next()
			n := p.peek()
			if n.kind != tIdent || reservedWords[n.text] || strings.Contains(n.text, ".") {
				return nil, &Error{n.line, fmt.Sprintf("expected a loop variable name after 'for', found %q", n.text)}
			}
			p.next()
			if !p.matchKeyword("in") {
				return nil, &Error{p.peek().line, fmt.Sprintf("expected 'in' after the loop variable, found %q", p.peek().text)}
			}
			in, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			body, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			return &forStmt{varName: n.text, in: in, body: body, line: t.line}, nil
		case "return":
			p.next()
			// A bare `return` is followed by whatever cannot continue an
			// expression: a block close, the end of the script, or a statement
			// keyword (let/if/else/for/while/return/fn). true/false/null/nil
			// START an expression, so `return true` keeps its value.
			if (p.peek().kind == tOp && p.peek().text == "}") || p.peek().kind == tEOF {
				return &returnStmt{line: t.line}, nil
			}
			if p.peek().kind == tIdent {
				switch p.peek().text {
				case "let", "if", "else", "for", "in", "while", "return", "fn":
					return &returnStmt{line: t.line}, nil
				}
			}
			v, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			return &returnStmt{val: v, line: t.line}, nil
		}
	}
	// Expression statement or assignment: parse a unary-level expression
	// (primary + postfix index chain); a following bare `=` makes it an
	// assignment target, otherwise it is the start of a full expression.
	head, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	if p.peek().kind == tOp && p.peek().text == "=" {
		p.next()
		if !isLValue(head) {
			return nil, &Error{head.exprLine(), "the left side of '=' must be a variable, a state path or an index (e.g. x, state.a, state.arr[i])"}
		}
		v, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return &assignStmt{target: head, val: v, line: head.exprLine()}, nil
	}
	// The head was only parsed at unary level; resume the binary chain at
	// every precedence level (high to low) so `f(x) + 1` and `a * b + c`
	// continue with the same shape parseBinary would have produced.
	rest := head
	for level := len(precedence) - 1; level >= 0; level-- {
		rest, err = p.parseBinaryTail(rest, level)
		if err != nil {
			return nil, err
		}
	}
	rest, err = p.parseExprTail(rest)
	if err != nil {
		return nil, err
	}
	return &exprStmt{e: rest, line: head.exprLine()}, nil
}

func (p *parser) parseIf() (stmt, error) {
	kw := p.next() // "if"
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	then, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	st := &ifStmt{cond: cond, then: then, line: kw.line}
	if p.matchKeyword("else") {
		if p.peek().kind == tIdent && p.peek().text == "if" {
			nested, err := p.parseIf() // else-if chain
			if err != nil {
				return nil, err
			}
			st.els = []stmt{nested}
		} else {
			els, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			st.els = els
		}
	}
	return st, nil
}

// isLValue reports whether e has the assignment-target shape: a (possibly
// dotted) identifier, or one with postfix index operations applied. The
// `state`/`args` handles are legal roots (state.a = ...); statement keywords
// and literals are not.
func isLValue(e exprNode) bool {
	switch t := e.(type) {
	case ident:
		switch strings.Split(t.name, ".")[0] {
		case "let", "if", "else", "for", "in", "while", "return", "fn",
			"true", "false", "null", "nil":
			return false
		}
		return true
	case index:
		return isLValue(t.base)
	}
	return false
}

// ---- expression grammar (mirrors internal/exprNode, plus array literals) ----

func (p *parser) parseExpr() (exprNode, error) {
	p.depth++
	if p.depth > maxParseDepth {
		return nil, &Error{p.peek().line, fmt.Sprintf("expression too deeply nested (max %d)", maxParseDepth)}
	}
	defer func() { p.depth-- }()
	return p.parseTernary()
}

func (p *parser) parseTernary() (exprNode, error) {
	cond, err := p.parseBinary(0)
	if err != nil {
		return nil, err
	}
	return p.parseExprTail(cond)
}

// parseExprTail continues an already-parsed unary-or-wider expression with
// the operators that may follow it (ternary arms). Statement parsing splits
// here so it can peek for the assignment `=` first.
func (p *parser) parseExprTail(cond exprNode) (exprNode, error) {
	if p.matchOp("?") {
		line := cond.exprLine()
		then, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if !p.matchOp(":") {
			return nil, &Error{p.peek().line, "expected ':' in ternary"}
		}
		els, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return ternary{cond, then, els, line}, nil
	}
	return cond, nil
}

// operator precedence levels, low to high — the same table as internal/exprNode.
var precedence = [][]string{
	{"||"},
	{"&&"},
	{"==", "!="},
	{"<", "<=", ">", ">="},
	{"+", "-"},
	{"*", "/", "%"},
}

func (p *parser) parseBinary(level int) (exprNode, error) {
	if level >= len(precedence) {
		return p.parseUnary()
	}
	left, err := p.parseBinary(level + 1)
	if err != nil {
		return nil, err
	}
	return p.parseBinaryTail(left, level)
}

// parseBinaryTail continues an already-parsed left operand with the operator
// chain of one precedence level — the loop half of parseBinary, shared with
// the expression-statement path that resumes a unary-level head.
func (p *parser) parseBinaryTail(left exprNode, level int) (exprNode, error) {
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
		left = binary{matched, left, right, left.exprLine()}
	}
}

func (p *parser) parseUnary() (exprNode, error) {
	if p.peek().kind == tOp && (p.peek().text == "!" || p.peek().text == "-") {
		op := p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return unary{op.text, x, op.line}, nil
	}
	prim, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	return p.parsePostfix(prim)
}

// parsePostfix applies `[exprNode]` indexing and `.name` member access resuming
// after a bracket — the same shapes internal/exprNode accepts.
func (p *parser) parsePostfix(base exprNode) (exprNode, error) {
	for {
		switch {
		case p.matchOp("["):
			key, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.matchOp("]") {
				return nil, &Error{p.peek().line, "expected ']' after index expression"}
			}
			base = index{base, key, base.exprLine()}
		case p.matchOp("."):
			t := p.peek()
			if t.kind != tIdent {
				return nil, &Error{t.line, "expected identifier after '.'"}
			}
			p.next()
			for _, part := range strings.Split(t.text, ".") {
				base = index{base, strLit{part, t.line}, base.exprLine()}
			}
		default:
			return base, nil
		}
	}
}

func (p *parser) parsePrimary() (exprNode, error) {
	t := p.peek()
	switch t.kind {
	case tNumber:
		p.next()
		// Same contract as internal/exprNode: a malformed literal (the lexer
		// absorbs dots greedily) is a parse error, never a silent 0; a
		// range-overflow literal keeps its float value.
		f, err := strconv.ParseFloat(t.text, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return nil, &Error{t.line, fmt.Sprintf("invalid number literal %q", t.text)}
		}
		return numLit{f, t.line}, nil
	case tString:
		p.next()
		return strLit{t.text, t.line}, nil
	case tIdent:
		p.next()
		switch t.text {
		case "true":
			return boolLit{true, t.line}, nil
		case "false":
			return boolLit{false, t.line}, nil
		case "null", "nil":
			return nullLit{t.line}, nil
		}
		if p.peek().kind == tOp && p.peek().text == "(" {
			p.next()
			var args []exprNode
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
				return nil, &Error{p.peek().line, "expected ')' after arguments"}
			}
			return call{name: t.text, args: args, line: t.line}, nil
		}
		return ident{t.text, t.line}, nil
	case tOp:
		if t.text == "(" {
			p.next()
			inner, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.matchOp(")") {
				return nil, &Error{p.peek().line, "expected ')'"}
			}
			return inner, nil
		}
		if t.text == "[" { // array literal — the one form bindings do not have
			p.next()
			var elems []exprNode
			if !(p.peek().kind == tOp && p.peek().text == "]") {
				for {
					e, err := p.parseExpr()
					if err != nil {
						return nil, err
					}
					elems = append(elems, e)
					if !p.matchOp(",") {
						break
					}
				}
			}
			if !p.matchOp("]") {
				return nil, &Error{p.peek().line, "expected ']' after array elements"}
			}
			return arrayLit{elems, t.line}, nil
		}
	}
	return nil, &Error{t.line, fmt.Sprintf("unexpected token %q", t.text)}
}
