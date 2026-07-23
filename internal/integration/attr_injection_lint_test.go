// CI regression lint for the round-6..9 bug class: an author- or state-bound
// value interpolated RAW into a quoted HTML attribute (id=, name=, style=,
// fill=, stroke=, data-*, href=, value=, type=, onclick=, ...) in the renderer,
// where a double quote in the value TERMINATES the attribute and injects
// markup. Go's %q does NOT save you here — it renders a quote as \" and the
// HTML parser treats the backslash as a literal character, so the quote still
// closes the attribute. The value must be entity-encoded (html.EscapeString /
// attrID / styleAttr) or be a quote-free constant.
//
// This test is a PURE STATIC SCAN of internal/render/*.go (non-test): it never
// builds or runs the renderer, needs no network, and is deterministic and fast,
// so it runs in `go test ./...` and CI for free. For every fmt.Fprintf /
// fmt.Sprintf whose format literal interpolates a % verb into an HTML attribute
// value, it classifies the argument expression that fills the verb:
//
//	SAFE-ESCAPED    the argument is a call to a known escaping helper
//	                (html.EscapeString, attrID, jsStringID, styleAttr, safeURL)
//	                or to a local helper whose value is provably escaped/constant
//	                (boxCSS, textCSS, containerCSS, num, flexAlign, segStyle,
//	                actionColor, borderIf, ... — resolved recursively, one level
//	                of helper indirection at a time), possibly joined with "+" to
//	                other safe values.
//	SAFE-CONSTANT   the argument is a numeric/bool format verb (%d %g %f %t ...,
//	                whose output can never contain a quote), a numeric/bool/string
//	                literal with no double quote, a strconv formatter, a package
//	                const, or a local variable whose every assigned value is safe.
//	SAFE-ALLOWLISTED the argument matches an explicit, comment-justified entry in
//	                attrAllowlist below (provably safe, but not caught by the rules
//	                above — e.g. a function parameter every caller fills with a
//	                constant, or a value regex-constrained upstream).
//	UNSAFE          anything else — the test FAILS, printing file:line and the
//	                offending expression.
//
// HOW TO SATISFY THIS LINT when you add a new widget attribute:
//
//  1. PREFERRED — escape the value at the emission site:
//     fmt.Fprintf(&r.sb, `<div id=%q title=%q>`, attrID(n.ID), html.EscapeString(v))
//     attrID/html.EscapeString/styleAttr entity-encode the value so a quote can
//     no longer break out (the browser decodes entities back, so it is
//     transparent to clients). Numeric values are fine unescaped (%d/%g).
//  2. If the value is a quote-free constant or derived only from constants, the
//     SAFE-CONSTANT rule already passes it — do nothing.
//  3. ONLY if the value is provably safe but the rule cannot see it (e.g. a
//     parameter every caller fills with a constant), add an entry to
//     attrAllowlist with a one-line reason. Do NOT allowlist a real hole — if a
//     genuinely un-escaped author/bound value reaches an attribute, ESCAPE IT.
//
// The detector is self-proving: see the subtests that plant a deliberate raw
// interpolation (must be flagged) and a correctly-escaped one (must pass) in a
// throwaway temp dir and run the SAME classifier over them.
package integration

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// ---- escaping/constant helper classification sets ----

// attrEscapers are functions whose RESULT is safe to embed verbatim in a quoted
// attribute: they entity-encode (or scheme-validate) their input so it can carry
// no raw double quote. Matched by callee name.
var attrEscapers = map[string]bool{
	"html.EscapeString": true,
	"attrID":            true,
	"jsStringID":        true,
	"styleAttr":         true,
	"safeURL":           true,
}

// attrNumericFuncs are strconv formatters whose output is digits/sign/bool and
// therefore can never contain a quote or tag character.
var attrNumericFuncs = map[string]bool{
	"strconv.Itoa": true, "strconv.FormatInt": true, "strconv.FormatUint": true,
	"strconv.FormatFloat": true, "strconv.FormatBool": true, "strconv.FormatComplex": true,
	"strconv.AppendInt": true, "strconv.AppendUint": true, "strconv.AppendFloat": true,
	"strconv.AppendBool": true,
}

// attrNumericVerb reports whether a fmt verb formats a number/bool, so its
// formatted output can never contain a quote or tag character (reliable in
// vet-clean code, where %d/%g/... are only fed numbers/bools).
func attrNumericVerb(v byte) bool {
	switch v {
	case 'd', 'g', 'G', 'f', 'F', 'e', 'E', 't', 'b', 'o', 'x', 'X', 'U':
		return true
	}
	return false
}

// ---- package model ----

type attrPkgInfo struct {
	funcs  map[string]*ast.FuncDecl // by name: top-level funcs AND methods (keyed on method name)
	consts map[string]ast.Expr      // package-level const name -> value expr
}

// attrScope is the innermost enclosing function (declaration or literal), used
// to resolve local variable assignments.
type attrScope struct {
	body    *ast.BlockStmt
	params  *ast.FieldList
	results *ast.FieldList
}

// attrClassifier carries per-scan state (so concurrent scans of the real tree
// and of throwaway snippet dirs do not share memo tables).
type attrClassifier struct {
	pk   *attrPkgInfo
	fset *token.FileSet
	memo map[string]int  // producer name -> 1 safe / -1 unsafe
	busy map[string]bool // cycle guard for producer recursion
}

func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		if x, ok := f.X.(*ast.Ident); ok {
			return x.Name + "." + f.Sel.Name
		}
		return f.Sel.Name
	}
	return ""
}

func (c *attrClassifier) exprStr(e ast.Expr) string {
	var b bytes.Buffer
	printer.Fprint(&b, c.fset, e)
	return b.String()
}

// ---- format-verb scanning ----

// attrVerbHit is one format verb sitting inside an HTML attribute value.
type attrVerbHit struct {
	verb  byte   // s, q, d, ...
	attr  string // attribute name ("" => not an attribute interpolation)
	quote byte   // '"', '\'', 'q' (self-quoted by %q), or 0 (unquoted)
	argN  int    // 0-based index of the value argument
}

// parseVerb parses a fmt verb starting at s[i]=='%'. It returns the verb byte,
// the index just past the verb, and how many extra arguments a '*' width/
// precision consumes.
func attrParseVerb(s string, i int) (byte, int, int) {
	i++ // past '%'
	if i >= len(s) {
		return 0, i, 0
	}
	if s[i] == '%' {
		return '%', i + 1, 0 // literal percent, not a verb
	}
	stars := 0
	if s[i] == '[' { // explicit arg index %[2]d
		for i < len(s) && s[i] != ']' {
			i++
		}
		if i < len(s) {
			i++
		}
	}
	for i < len(s) && strings.IndexByte("-+ #0", s[i]) >= 0 {
		i++ // flags
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++ // width
	}
	if i < len(s) && s[i] == '*' {
		stars++
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++ // precision
		}
		if i < len(s) && s[i] == '*' {
			stars++
			i++
		}
	}
	if i >= len(s) {
		return 0, i, stars
	}
	v := s[i]
	return v, i + 1, stars
}

// attrAllVerbs returns every real verb (excluding %%) with its value-arg index.
func attrAllVerbs(fs string) []attrVerbHit {
	var out []attrVerbHit
	argIdx := 0
	for i := 0; i < len(fs); i++ {
		if fs[i] != '%' {
			continue
		}
		v, next, stars := attrParseVerb(fs, i)
		if v == '%' || v == 0 {
			i = next - 1
			continue
		}
		out = append(out, attrVerbHit{verb: v, argN: argIdx + stars})
		argIdx += 1 + stars
		i = next - 1
	}
	return out
}

func attrIsAttrNameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '-' || c == '_' || c == ':'
}
func attrIsAttrName(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// attrIsBoundary reports whether the byte before an attribute name is a
// tag-context separator (so CSS `prop:value` colons and base64 `=` padding are
// not mistaken for attribute assignments).
func attrIsBoundary(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '<' || c == '"' || c == '\'' || c == '/'
}

// attrVerbs walks a format string tracking whether the cursor is inside a
// double-quoted, single-quoted, self-quoted (%q) or unquoted attribute value,
// and returns each verb that sits inside one. Verbs in text context (or in the
// bare "%s" gap BETWEEN attributes, e.g. a11y(n)) carry attr=="" and are ignored
// by the caller — only interpolations INTO an attribute value are in scope.
func attrVerbs(s string) []attrVerbHit {
	var hits []attrVerbHit
	mode := byte('t') // t=text, d=double-quoted, s=single-quoted, u=unquoted
	attr := ""
	argIdx := 0
	for i := 0; i < len(s); {
		c := s[i]
		switch mode {
		case 't':
			if c == '%' {
				v, next, stars := attrParseVerb(s, i)
				if v != '%' && v != 0 {
					hits = append(hits, attrVerbHit{verb: v, quote: 0, argN: argIdx + stars})
					argIdx += 1 + stars
				}
				i = next
				continue
			}
			if c == '=' && i > 0 {
				j := i
				for j > 0 && attrIsAttrNameChar(s[j-1]) {
					j--
				}
				name := s[j:i]
				prev := byte(' ')
				if j > 0 {
					prev = s[j-1]
				}
				if attrIsAttrName(name) && attrIsBoundary(prev) && i+1 < len(s) {
					switch s[i+1] {
					case '"':
						mode, attr = 'd', name
						i += 2
						continue
					case '\'':
						mode, attr = 's', name
						i += 2
						continue
					default:
						mode, attr = 'u', name
						i++
						continue
					}
				}
			}
			i++
		case 'd':
			if c == '"' {
				mode, attr = 't', ""
				i++
				continue
			}
			if c == '%' {
				v, next, stars := attrParseVerb(s, i)
				if v != '%' && v != 0 {
					hits = append(hits, attrVerbHit{verb: v, attr: attr, quote: '"', argN: argIdx + stars})
					argIdx += 1 + stars
				}
				i = next
				continue
			}
			i++
		case 's':
			if c == '\'' {
				mode, attr = 't', ""
				i++
				continue
			}
			if c == '%' {
				v, next, stars := attrParseVerb(s, i)
				if v != '%' && v != 0 {
					hits = append(hits, attrVerbHit{verb: v, attr: attr, quote: '\'', argN: argIdx + stars})
					argIdx += 1 + stars
				}
				i = next
				continue
			}
			i++
		case 'u':
			if c == ' ' || c == '\t' || c == '\n' || c == '>' || c == '<' {
				mode, attr = 't', ""
				i++
				continue
			}
			if c == '%' {
				v, next, stars := attrParseVerb(s, i)
				if v != '%' && v != 0 {
					q := byte(0)
					if v == 'q' {
						q = 'q' // %q supplies the surrounding quotes itself
					}
					hits = append(hits, attrVerbHit{verb: v, attr: attr, quote: q, argN: argIdx + stars})
					argIdx += 1 + stars
					if v == 'q' {
						mode, attr = 't', "" // %q fully contains the value; attribute ends
					}
				}
				i = next
				continue
			}
			i++
		}
	}
	return hits
}

// ---- expression classification ----

// attrSafeProducer reports whether the local function `name` returns ONLY safe
// values — i.e. every return statement yields escaped/constant expressions. Such
// a helper (boxCSS, textCSS, containerCSS, num, flexAlign, segStyle, ...) may be
// interpolated into an attribute verbatim. Resolution is recursive (helpers
// calling helpers) with a cycle guard, so this is the "one level of local helper
// indirection" the design calls for, applied as far as the call chain goes.
func (c *attrClassifier) attrSafeProducer(name string) bool {
	if v, ok := c.memo[name]; ok {
		return v == 1
	}
	if c.busy[name] {
		return false
	}
	decl, ok := c.pk.funcs[name]
	if !ok || decl.Body == nil {
		c.memo[name] = -1
		return false
	}
	c.busy[name] = true
	sc := &attrScope{body: decl.Body}
	if decl.Type != nil {
		sc.params, sc.results = decl.Type.Params, decl.Type.Results
	}
	safe, found := true, false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if _, isFL := n.(*ast.FuncLit); isFL {
			return false // nested closure: different scope, not this function's value
		}
		ret, isRet := n.(*ast.ReturnStmt)
		if !isRet {
			return true
		}
		found = true
		if len(ret.Results) == 0 {
			safe = false // naked return of a named result: cannot verify statically
			return true
		}
		for _, r := range ret.Results {
			if !c.classify(r, sc, 0) {
				safe = false
			}
		}
		return true
	})
	if !found {
		safe = false
	}
	delete(c.busy, name)
	if safe {
		c.memo[name] = 1
	} else {
		c.memo[name] = -1
	}
	return safe
}

func attrIsParam(sc *attrScope, name string) bool {
	if sc == nil {
		return false
	}
	for _, fl := range []*ast.FieldList{sc.params, sc.results} {
		if fl == nil {
			continue
		}
		for _, field := range fl.List {
			for _, nm := range field.Names {
				if nm.Name == name {
					return true
				}
			}
		}
	}
	return false
}

// attrAllAssigns returns every RHS assigned to `name` within sc.body (:=, =, +=,
// incl. positionally-matched tuple assignments a,b := X,Y; not descending into
// nested func literals). ok=false when name is a parameter or has no plain
// assignment (range var, multi-value single-call return), which callers treat as
// UNRESOLVED and therefore conservatively unsafe. Collecting every assignment is
// sound-for-flagging under shadowing: if any value a variable can take is unsafe
// the variable is judged unsafe, so this can only add false positives, never hide
// a real hole.
func attrAllAssigns(sc *attrScope, name string) ([]ast.Expr, bool) {
	if sc == nil || sc.body == nil || attrIsParam(sc, name) {
		return nil, false
	}
	var rhs []ast.Expr
	ast.Inspect(sc.body, func(n ast.Node) bool {
		if _, isFL := n.(*ast.FuncLit); isFL {
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != len(as.Rhs) {
			return true
		}
		for k, lhs := range as.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
				rhs = append(rhs, as.Rhs[k])
			}
		}
		return true
	})
	if len(rhs) == 0 {
		return nil, false
	}
	return rhs, true
}

// attrSprintfSafe reports whether a fmt.Sprintf result is safe to embed in a
// quoted attribute: the format literal introduces no double quote of its own and
// every non-numeric verb is filled by an argument that is itself safe.
func (c *attrClassifier) attrSprintfSafe(fs string, args []ast.Expr, sc *attrScope, depth int) bool {
	if strings.Contains(fs, `"`) {
		return false
	}
	for _, vh := range attrAllVerbs(fs) {
		if attrNumericVerb(vh.verb) {
			continue
		}
		if vh.argN >= len(args) || !c.classify(args[vh.argN], sc, depth) {
			return false
		}
	}
	return true
}

// classify reports whether an argument expression is a SAFE value to interpolate
// into a quoted attribute (escaped or quote-free constant). It is deliberately
// conservative: anything it cannot prove safe returns false and is left to the
// allowlist or flagged.
func (c *attrClassifier) classify(e ast.Expr, sc *attrScope, depth int) bool {
	if depth > 16 {
		return false
	}
	switch x := e.(type) {
	case *ast.ParenExpr:
		return c.classify(x.X, sc, depth+1)
	case *ast.BasicLit:
		switch x.Kind {
		case token.INT, token.FLOAT, token.CHAR, token.IMAG:
			return true
		case token.STRING:
			s, err := strconv.Unquote(x.Value)
			if err != nil {
				return false
			}
			return !strings.Contains(s, `"`) // a quote-free literal cannot break out
		}
		return false
	case *ast.Ident:
		if x.Name == "true" || x.Name == "false" || x.Name == "nil" {
			return true
		}
		if rhsList, ok := attrAllAssigns(sc, x.Name); ok {
			all := true
			for _, rhs := range rhsList {
				if !c.classify(rhs, sc, depth+1) {
					all = false
				}
			}
			return all
		}
		if v, ok := c.pk.consts[x.Name]; ok { // package-level const
			return c.classify(v, nil, depth+1)
		}
		return false
	case *ast.BinaryExpr:
		if x.Op == token.ADD { // a concatenation is safe iff every operand is
			return c.classify(x.X, sc, depth+1) && c.classify(x.Y, sc, depth+1)
		}
		return false
	case *ast.CallExpr:
		name := calleeName(x.Fun)
		if attrEscapers[name] {
			return true // SAFE-ESCAPED
		}
		if attrNumericFuncs[name] {
			return true // SAFE-CONSTANT (numeric output)
		}
		if name == "fmt.Sprintf" {
			if len(x.Args) >= 1 {
				if lit, ok := x.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if fs, err := strconv.Unquote(lit.Value); err == nil {
						return c.attrSprintfSafe(fs, x.Args[1:], sc, depth+1)
					}
				}
			}
			return false
		}
		base := name
		if i := strings.IndexByte(name, '.'); i >= 0 {
			base = name[i+1:] // method call r.boxCSS(...) -> boxCSS
		}
		if _, ok := c.pk.funcs[base]; ok {
			return c.attrSafeProducer(base) // local helper returning only safe values
		}
		return false
	}
	return false
}

// ---- scanning a directory ----

// attrInterp is one detected attribute interpolation and its verdict.
type attrInterp struct {
	file  string // base name
	line  int
	attr  string
	verb  byte
	quote byte
	arg   string // argument expression, as printed
	safe  bool   // classified safe by the rules (before allowlist)
}

func scanAttrInterpolations(dir string) ([]attrInterp, error) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	pk := &attrPkgInfo{funcs: map[string]*ast.FuncDecl{}, consts: map[string]ast.Expr{}}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var files []*ast.File
	for _, n := range names {
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", n, err)
		}
		files = append(files, f)
		for _, d := range f.Decls {
			switch x := d.(type) {
			case *ast.FuncDecl:
				pk.funcs[x.Name.Name] = x
			case *ast.GenDecl:
				if x.Tok != token.CONST {
					continue
				}
				for _, spec := range x.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
						continue
					}
					pk.consts[vs.Names[0].Name] = vs.Values[0]
				}
			}
		}
	}

	c := &attrClassifier{pk: pk, fset: fset, memo: map[string]int{}, busy: map[string]bool{}}
	var out []attrInterp
	for _, f := range files {
		base := filepath.Base(fset.File(f.Pos()).Name())
		var scopes []*attrScope
		push := func(sc *attrScope) { scopes = append(scopes, sc) }
		pop := func() { scopes = scopes[:len(scopes)-1] }
		cur := func() *attrScope {
			if len(scopes) == 0 {
				return nil
			}
			return scopes[len(scopes)-1]
		}
		var walk func(n ast.Node) bool
		walk = func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				sc := &attrScope{body: x.Body}
				if x.Type != nil {
					sc.params, sc.results = x.Type.Params, x.Type.Results
				}
				push(sc)
				ast.Inspect(x.Body, walk)
				pop()
				return false
			case *ast.FuncLit:
				sc := &attrScope{body: x.Body}
				if x.Type != nil {
					sc.params, sc.results = x.Type.Params, x.Type.Results
				}
				push(sc)
				ast.Inspect(x.Body, walk)
				pop()
				return false
			case *ast.CallExpr:
				name := calleeName(x.Fun)
				if name != "fmt.Fprintf" && name != "fmt.Sprintf" {
					return true
				}
				fi := 0
				if name == "fmt.Fprintf" {
					fi = 1 // args[0] is the writer
				}
				if len(x.Args) <= fi {
					return true
				}
				lit, ok := x.Args[fi].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true // non-literal format: not statically analysable
				}
				fs, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				args := x.Args[fi+1:]
				pos := fset.Position(x.Pos())
				for _, vh := range attrVerbs(fs) {
					if vh.attr == "" {
						continue // text-context / between-attributes verb
					}
					argExpr := "<none>"
					safe := attrNumericVerb(vh.verb)
					if vh.argN < len(args) {
						argExpr = c.exprStr(args[vh.argN])
						if !safe {
							safe = c.classify(args[vh.argN], cur(), 0)
						}
					}
					out = append(out, attrInterp{
						file: base, line: pos.Line, attr: vh.attr,
						verb: vh.verb, quote: vh.quote, arg: argExpr, safe: safe,
					})
				}
				return true
			}
			return true
		}
		ast.Inspect(f, walk)
	}
	return out, nil
}

// ---- the comment-justified allowlist ----

// attrAllowEntry is a SAFE-ALLOWLISTED interpolation the rules cannot see. Every
// entry carries a one-line reason and MUST match at least one detected site (the
// test asserts this, so a stale entry — e.g. once the site gets properly escaped
// — fails loudly and is removed, keeping the list minimal). Matching is on the
// exact (file, attribute, argument-expression) triple, which is stable under line
// drift. These are provably safe, NOT real holes — see each reason.
type attrAllowEntry struct {
	file   string
	attr   string
	arg    string
	reason string
}

var attrAllowlist = []attrAllowEntry{
	{"render_animation.go", "style", "kf",
		"kf indexes the fixed motionKeyframe map of constant 'qa-*' keyframe names (default 'qa-fade'); it can never contain a quote"},
	{"render_animation.go", "style", "r.boxCSS(n) + anim",
		"anim = Sprintf('animation:%s %gms %s %gms %s both;', kf, dur, curve, delay, repeat): kf is a fixed keyframe-map constant, dur/delay are numeric %g, curve/repeat are styleAttr-escaped; boxCSS is escaped"},
	{"render_animation.go", "style", "r.boxCSS(n) + tf",
		"tf is 'transform:' + strings.Join(parts) of numeric-only rotate/scale/translate/skew (%g) fragments + constant punctuation; boxCSS is escaped"},
	{"render_data.go", "class", "cls",
		"this cls is the sort-indicator class: indCls ('qorm-sort-ind' constant) or indCls+' on'; the class=\"qdt-sel\" literal nearby is a distinct shadowed var used at a bare <tr%s>, not an attribute interpolation"},
	{"render_feedback.go", "style", "style",
		"alert bg/fg come from alertColors(variant) which returns constant CSS colors (var(--accent)/color-mix(...)); boxCSS is escaped; the icon SVG rides a text-context %s, not the attribute"},
	{"render_gesture.go", "class", "kind",
		"kind is a hwList/hwAdjust parameter; every call site in render.go passes a constant widget-name literal (bluetooth, wifi, nfc, ...)"},
	{"render_gesture.go", "onclick", "jsFn",
		"jsFn is a hwList/hwAdjust parameter; every call site passes a constant 'qorm*' bridge-handler name"},
	{"render_style.go", "data-state", "path",
		"path is constrained to [a-zA-Z0-9_.] by stateBindRe in boundPath, so it cannot contain a quote or tag character"},
	{"render_style.go", "fill", "color",
		"color is html.EscapeString'd at the top of chartBars/chartLine (before any fill interpolation)"},
	{"render_style.go", "stroke", "color",
		"color is html.EscapeString'd at the top of chartLine (before the stroke interpolation)"},
	{"render_style.go", "points", `strings.Join(pts, " ")`,
		"pts is built solely from Sprintf('%.1f,%.1f', x, y) numeric coordinate strings"},
	{"render_widgets.go", "style", "grow",
		"grow is a dateWheel parameter; every call site passes a constant flex-grow literal ('1', '1.3', '0.7')"},
	{"render_widgets.go", "aria-label", "aria",
		"aria is a navButton parameter; every call site passes a constant label ('Back', 'Close')"},
}

func attrAllowKey(file, attr, arg string) string { return file + "\x00" + attr + "\x00" + arg }

// ---- the test ----

const attrRenderDir = "../../internal/render"

// TestAttrInjectionLint guards the quoted-attribute interpolation bug class. See
// the file-top comment for how to satisfy it when adding a widget attribute.
func TestAttrInjectionLint(t *testing.T) {
	t.Run("render-package-clean", func(t *testing.T) {
		interps, err := scanAttrInterpolations(attrRenderDir)
		if err != nil {
			t.Fatalf("scanning %s: %v", attrRenderDir, err)
		}
		if len(interps) == 0 {
			t.Fatalf("no attribute interpolations found in %s — the scan is broken", attrRenderDir)
		}

		allow := map[string]string{} // key -> reason
		for _, e := range attrAllowlist {
			allow[attrAllowKey(e.file, e.attr, e.arg)] = e.reason
		}
		matched := map[string]int{} // key -> number of sites matched

		var unsafe []attrInterp
		nEscapedConst, nAllow := 0, 0
		for _, it := range interps {
			if it.safe {
				nEscapedConst++
				continue
			}
			key := attrAllowKey(it.file, it.attr, it.arg)
			if _, ok := allow[key]; ok {
				nAllow++
				matched[key]++
				continue
			}
			unsafe = append(unsafe, it)
		}

		for _, it := range unsafe {
			t.Errorf("%s:%d: UNSAFE attribute interpolation: %s interpolated into %s= (verb %%%c)\n"+
				"  a double quote in the value breaks out of the attribute and injects markup.\n"+
				"  FIX: escape it (e.g. html.EscapeString(...)/attrID(...)/styleAttr(...)) or, if it is\n"+
				"  provably safe but the rule cannot see it, add a justified entry to attrAllowlist.\n"+
				"  offending expression: %s",
				it.file, it.line, it.arg, it.attr, it.verb, it.arg)
		}

		// Every allowlist entry must still match a live site, or it is stale.
		for _, e := range attrAllowlist {
			key := attrAllowKey(e.file, e.attr, e.arg)
			if matched[key] == 0 {
				t.Errorf("stale allowlist entry (matches no detected site — remove it, or the site was fixed/renamed):\n  %s  attr=%s  arg=%s\n  reason: %s",
					e.file, e.attr, e.arg, e.reason)
			}
		}

		t.Logf("attribute interpolations scanned: %d (safe-by-rule %d, allowlisted %d, unsafe %d; allowlist size %d)",
			len(interps), nEscapedConst, nAllow, len(unsafe), len(attrAllowlist))
	})

	// Self-proof: the detector must flag a deliberate raw interpolation and pass a
	// correctly-escaped one, using the SAME classifier. This proves the lint cannot
	// be silently bypassed (e.g. by a broken scanner that flags nothing).
	t.Run("self-proof-raw-is-flagged", func(t *testing.T) {
		dir := t.TempDir()
		src := "package snippet\n\nimport \"fmt\"\n\n" +
			"// raw interpolates an untrusted value into a double-quoted attribute and a\n" +
			"// %%q attribute; both must be flagged.\n" +
			"func raw(userInput string) string {\n" +
			"\treturn fmt.Sprintf(`<div title=\"%s\" id=%q>`, userInput, userInput)\n" +
			"}\n"
		if err := os.WriteFile(filepath.Join(dir, "raw.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		interps, err := scanAttrInterpolations(dir)
		if err != nil {
			t.Fatalf("scanning planted snippet: %v", err)
		}
		var flaggedTitle, flaggedID bool
		for _, it := range interps {
			if !it.safe && it.attr == "title" && it.arg == "userInput" {
				flaggedTitle = true
			}
			if !it.safe && it.attr == "id" && it.arg == "userInput" {
				flaggedID = true
			}
		}
		if !flaggedTitle {
			t.Errorf("self-proof failed: the planted raw title=\"%%s\" interpolation was NOT flagged (interps=%+v)", interps)
		}
		if !flaggedID {
			t.Errorf("self-proof failed: the planted raw id=%%q interpolation was NOT flagged (interps=%+v)", interps)
		}
	})

	t.Run("self-proof-escaped-is-clean", func(t *testing.T) {
		dir := t.TempDir()
		src := "package snippet\n\nimport (\n\t\"fmt\"\n\t\"html\"\n)\n\n" +
			"// escaped entity-encodes the value so the quote cannot break out; neither\n" +
			"// interpolation may be flagged.\n" +
			"func escaped(userInput string) string {\n" +
			"\treturn fmt.Sprintf(`<div title=\"%s\" id=%q>`, html.EscapeString(userInput), html.EscapeString(userInput))\n" +
			"}\n"
		if err := os.WriteFile(filepath.Join(dir, "escaped.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		interps, err := scanAttrInterpolations(dir)
		if err != nil {
			t.Fatalf("scanning planted snippet: %v", err)
		}
		for _, it := range interps {
			if !it.safe {
				t.Errorf("self-proof failed: a correctly-escaped interpolation was flagged: %+v", it)
			}
		}
	})
}
