// Package qss parses QORM Style Sheets (styles/*.qss) — the style leg of the
// structure/logic/style separation. A sheet is a list of rules:
//
//	# a comment
//	button { background: var(--primary); borderRadius: 12; fontWeight: 600 }
//	.accent { background: #007AFF }
//	#submit { height: 44; width: fill }
//
// Three selector shapes exist — a widget type name (`button`), a class
// (`.name`, matched against the space-separated names in a node's `class`
// prop) and an id (`#name`). A rule body holds the same key/value pairs a
// node's inline "style" object holds: numbers stay numbers, every other value
// stays a string (var(--x), a hex color, "fill", or a {{binding}} — evaluated
// at render time exactly like an inline style value). Declarations are
// separated by `;` or a newline, so one rule may span lines; `}` ends the
// rule. `#` starts a whole-line comment wherever a selector or a style key is
// expected — inside a value it is literal text, so a hex color needs no
// escaping — and an id selector (`#submit`) is told from a comment by the
// identifier character right after the `#`.
package qss

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/qorm/platform/internal/model"
)

// ParseError is one syntax error, located by its 1-based source line.
type ParseError struct {
	Line int
	Msg  string
}

func (e ParseError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

// maxErrors caps the diagnostics one malformed sheet can produce; recovery is
// per rule, so a genuinely broken file would otherwise report one error per
// line to the end of the file.
const maxErrors = 20

// Parse parses one sheet into its rules, in declaration order. Syntax errors
// are collected (not fatal): the parser recovers at the end of the offending
// rule and keeps going, so one bad rule does not hide the rest of the sheet.
func Parse(src string) ([]model.StyleRule, []ParseError) {
	p := &parser{src: []rune(src)}
	var rules []model.StyleRule
	for {
		p.skipSpace()
		if p.eof() {
			break
		}
		if p.peek() == '#' && !p.idSelectorAhead() {
			p.skipComment()
			continue
		}
		rule, ok := p.parseRule()
		if !ok {
			p.recover()
			if len(p.errs) >= maxErrors {
				break
			}
			continue
		}
		rules = append(rules, rule)
	}
	return rules, p.errs
}

type parser struct {
	src  []rune
	pos  int
	line int // 0-based; reported +1
	errs []ParseError
}

func (p *parser) eof() bool { return p.pos >= len(p.src) }

func (p *parser) peek() rune {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

func (p *parser) next() rune {
	c := p.peek()
	p.pos++
	if c == '\n' {
		p.line++
	}
	return c
}

func (p *parser) err(line int, format string, args ...any) {
	if len(p.errs) >= maxErrors {
		return
	}
	p.errs = append(p.errs, ParseError{Line: line + 1, Msg: fmt.Sprintf(format, args...)})
}

// skipSpace consumes whitespace (including newlines).
func (p *parser) skipSpace() {
	for !p.eof() && (p.peek() == ' ' || p.peek() == '\t' || p.peek() == '\r' || p.peek() == '\n') {
		p.next()
	}
}

// skipComment consumes a `#` comment through the end of its line.
func (p *parser) skipComment() {
	for !p.eof() && p.peek() != '\n' {
		p.next()
	}
}

// idSelectorAhead reports whether the `#` at the cursor opens an id selector
// (`#submit`) rather than a comment: a `#` immediately followed by an
// identifier-start character. `# 注释` and a lone `#` are comments.
func (p *parser) idSelectorAhead() bool {
	return p.pos+1 < len(p.src) && isIdentStart(p.src[p.pos+1])
}

func isIdentStart(c rune) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isIdentChar(c rune) bool {
	return isIdentStart(c) || c >= '0' && c <= '9' || c == '-'
}

// parseIdent consumes a maximal identifier run ([A-Za-z0-9_-]+).
func (p *parser) parseIdent() string {
	start := p.pos
	for !p.eof() && isIdentChar(p.peek()) {
		p.next()
	}
	return string(p.src[start:p.pos])
}

// parseRule parses `selector { decls }`. ok is false on a syntax error (the
// error is recorded; the caller recovers).
func (p *parser) parseRule() (model.StyleRule, bool) {
	line := p.line
	rule := model.StyleRule{Style: map[string]any{}}
	switch c := p.peek(); {
	case c == '.':
		p.next()
		rule.Kind = model.StyleRuleClass
		rule.Name = p.parseIdent()
	case c == '#':
		p.next()
		rule.Kind = model.StyleRuleID
		rule.Name = p.parseIdent()
	case isIdentStart(c):
		rule.Kind = model.StyleRuleType
		rule.Name = p.parseIdent()
	default:
		p.err(p.line, "expected a selector (a widget type like `button`, a class like `.name`, or an id like `#name`), got %q", string(c))
		return rule, false
	}
	if rule.Name == "" {
		p.err(line, "empty selector name")
		return rule, false
	}
	p.skipSpace()
	if p.eof() || p.peek() != '{' {
		p.err(line, "selector %q must be followed by `{`", selectorText(rule))
		return rule, false
	}
	p.next()
	for {
		p.skipSpace()
		if p.eof() {
			p.err(line, "rule %q is missing its closing `}`", selectorText(rule))
			return rule, false
		}
		if p.peek() == '}' {
			p.next()
			return rule, true
		}
		if p.peek() == '#' {
			p.skipComment() // inside a body `#` can only start a comment
			continue
		}
		done, ok := p.parseDecl(&rule)
		if !ok {
			return rule, false
		}
		if done {
			return rule, true
		}
	}
}

// parseDecl parses one `key : value` declaration, terminated by `;`, a
// newline, or the rule's closing `}` (which it consumes — reported as
// done=true so the body loop ends the rule). A value runs to its terminator
// verbatim: {{binding}} braces and quoted strings inside it shield any `;`,
// `}` or newline they contain.
func (p *parser) parseDecl(rule *model.StyleRule) (done, ok bool) {
	line := p.line
	if !isIdentStart(p.peek()) {
		p.err(line, "expected a style key in rule %q, got %q", selectorText(*rule), string(p.peek()))
		return false, false
	}
	key := p.parseIdent()
	p.skipSpaceInline()
	if p.eof() || p.peek() != ':' {
		p.err(line, "style key %q in rule %q must be followed by `:`", key, selectorText(*rule))
		return false, false
	}
	p.next()
	p.skipSpaceInline()
	value, closed, badObject := p.parseValue()
	if badObject {
		p.err(line, "style key %q in rule %q: nested object values are not supported in styles/*.qss — values are scalars (number, string, var(--x), {{binding}}); keep nested objects (e.g. margin: {top: …}) inline on the scene node", key, selectorText(*rule))
		return false, false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		p.err(line, "style key %q in rule %q has no value", key, selectorText(*rule))
		return false, false
	}
	rule.Style[key] = scalarValue(value)
	return closed, true
}

// parseValue consumes a declaration value up to its terminator (`;`, a
// newline, or `}` at binding depth 0 outside quotes). It consumes a `;`
// terminator, leaves a newline for the body loop's skipSpace, and consumes a
// `}` terminator — reported via the closed result so the caller can end the
// rule. A bare `{` (not the `{{` of a binding) means the author wrote a nested
// object value, which the format does not support: reported via badObject.
func (p *parser) parseValue() (value string, closed, badObject bool) {
	start := p.pos
	depth := 0 // {{ }} nesting
	var quote rune
	for !p.eof() {
		c := p.peek()
		if quote != 0 {
			if c == '\\' && p.pos+1 < len(p.src) {
				p.next()
				p.next()
				continue
			}
			if c == quote {
				quote = 0
			}
			p.next()
			continue
		}
		switch {
		case c == '\'' || c == '"':
			quote = c
			p.next()
		case c == '{' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '{':
			depth++
			p.next()
			p.next()
		case c == '{' && depth == 0:
			return "", false, true
		case c == '}' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '}' && depth > 0:
			depth--
			p.next()
			p.next()
		case depth == 0 && c == ';':
			v := string(p.src[start:p.pos])
			p.next()
			return v, false, false
		case depth == 0 && c == '\n':
			return string(p.src[start:p.pos]), false, false
		case depth == 0 && c == '}':
			v := string(p.src[start:p.pos])
			p.next()
			return v, true, false
		default:
			p.next()
		}
	}
	return string(p.src[start:p.pos]), false, false
}

// skipSpaceInline consumes spaces and tabs but stops at a newline (a newline
// terminates a declaration).
func (p *parser) skipSpaceInline() {
	for !p.eof() && (p.peek() == ' ' || p.peek() == '\t' || p.peek() == '\r') {
		p.next()
	}
}

// recover skips to just past the next `}` (or EOF) after a syntax error, so
// the next rule starts clean.
func (p *parser) recover() {
	for !p.eof() {
		if p.next() == '}' {
			return
		}
	}
}

// selectorText renders a rule's selector the way it was authored, for
// diagnostics.
func selectorText(r model.StyleRule) string {
	switch r.Kind {
	case model.StyleRuleClass:
		return "." + r.Name
	case model.StyleRuleID:
		return "#" + r.Name
	}
	return r.Name
}

// scalarValue gives a declaration value its JSON-parity type: a bare number
// becomes a float64 (what encoding/json yields for a numeric style value, so
// the renderer's type switches see the same shapes either way), a quoted
// string is unquoted, and everything else stays the raw string.
func scalarValue(v string) any {
	if len(v) >= 2 {
		if q := v[0]; (q == '\'' || q == '"') && v[len(v)-1] == q {
			return v[1 : len(v)-1]
		}
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil && looksNumeric(v) {
		return f
	}
	return v
}

// looksNumeric restricts ParseFloat to plain decimal spellings, so "fill" or
// "var(--x)" never reach it and "1e3" stays the string an author would read it
// as (ParseFloat would accept it, but no style key consumes exponential
// notation — better kept literal).
func looksNumeric(v string) bool {
	for i, c := range v {
		switch {
		case c >= '0' && c <= '9':
		case c == '-' && i == 0:
		case c == '.':
		default:
			return false
		}
	}
	return v != "" && v != "-" && v != "."
}
