// CI regression lint for the round-6..9 bug class: an author- or state-bound
// value interpolated RAW into a quoted HTML attribute (id=, name=, style=,
// fill=, stroke=, data-*, href=, value=, type=, onclick=, ...), where a double
// quote in the value TERMINATES the attribute and injects markup. Go's %q does
// NOT save you here — it renders a quote as \" and the HTML parser treats the
// backslash as a literal character, so the quote still closes the attribute.
// The value must be entity-encoded (html.EscapeString / attrID / styleAttr) or
// be a quote-free constant.
//
// WHAT IS SCANNED: every directory in attrScanDirs — today internal/render AND
// internal/server. It used to be internal/render alone, and that gap is exactly
// how a stored-XSS hole lived undetected in the HTML shell: <html lang="%s">
// and class="qorm-theme-%s" took state.locale / state.theme unescaped, so any
// write to those state keys (an action, an http response, MCP qorm_set_state, a
// theme picker bound to an input) executed attacker JS in the app's origin on
// the next full page load — and could then read the event token and impersonate
// the human. The rule to catch it was always here; nothing ever pointed it at
// the package. ANY package that assembles markup must be listed.
//
// This test is a PURE STATIC SCAN of those directories' *.go (non-test): it
// never builds or runs the renderer, needs no network, and is deterministic and
// fast, so it runs in `go test ./...` and CI for free. For every fmt.Fprintf /
// fmt.Sprintf, it walks the format string tracking the HTML SOURCE POSITION of
// each verb and classifies the argument that fills it against the escaping
// discipline that position actually requires:
//
//	attr    a quoted/unquoted attribute value — entity encoding works, because
//	        the HTML parser decodes it back.
//	url     href=/src=/action= — needs BOTH a scheme check (safeURL) and entity
//	        encoding. safeURL ALONE is not a pass: it returns the URL untouched,
//	        so a quote inside an http:// URL still ends the attribute.
//	event   on*= — this is JAVASCRIPT that the parser entity-DECODES before the
//	        JS parser sees it, so html.EscapeString is not merely insufficient,
//	        it is self-defeating (&#39; decodes back to ' and closes the
//	        handler's string). Only jsStringID passes.
//	script  a literal <script> body — raw text, entities are never decoded, and
//	        "</" ends the element regardless of JS quoting. Only jsStringID.
//	style   a literal <style> body — raw-text CSS. Entity encoding is inert
//	        there and would not stop a ";" from opening another declaration
//	        anyway. Only a CSS-value allowlist (cssValue / cssStyleValue).
//	text    ordinary element content — NOT checked (see LIMITS).
//
// The verdicts:
//
//	SAFE-ESCAPED    the argument is a call to a helper that neutralises the
//	                value FOR THAT POSITION (html.EscapeString/attrID/styleAttr
//	                for attributes, jsStringID for JS, cssValue/cssStyleValue
//	                for CSS, themeClass/langTag for the normalized shell values)
//	                or to a local helper whose value is provably safe (boxCSS,
//	                textCSS, containerCSS, num, flexAlign, segStyle, ... —
//	                resolved recursively with a per-position memo), possibly
//	                joined with "+" to other safe values.
//	SAFE-CONSTANT   the argument is a numeric/bool format verb (%d %g %f %t ...,
//	                whose output can never contain a quote), a numeric/bool/string
//	                literal carrying none of the delimiters that position uses, a
//	                strconv formatter, a package const, or a local variable whose
//	                every assigned value is safe.
//	SAFE-ALLOWLISTED the argument matches an explicit, comment-justified entry in
//	                attrAllowlist below (provably safe, but not caught by the rules
//	                above — e.g. a function parameter every caller fills with a
//	                constant, or a value regex-constrained upstream). A reason
//	                must justify THE POSITION: for a <style> body "it has no
//	                quote" proves nothing, the question is ";" "{" "}" "</style".
//	UNSAFE          anything else — the test FAILS, printing file:line, the
//	                position, and the offending expression.
//
// A format string that is not a compile-time constant is itself reported: its
// verbs cannot be placed in any position, so nothing about the site can be
// proven. (Concatenated string LITERALS are folded and stay analysable.)
//
// LIMITS (known false negatives, deliberately left, so nobody mistakes a green
// run for proof of more than it checks):
//
//   - TEXT context is not checked. `<div>%s</div>` with a raw value is a real
//     XSS shape, but in this renderer the text-context %s is overwhelmingly an
//     already-rendered HTML fragment (children, a11y attribute runs, icon SVG),
//     so the rule would be almost entirely false positives and would need an
//     allowlist larger than the codebase. Text escaping is covered by
//     behavioural tests instead (internal/render/escaping_test.go).
//   - Markup built by string CONCATENATION or strings.Builder.WriteString
//     rather than a format verb is not seen — e.g.
//     b.WriteString(`<div title="` + v + `">`). Only fmt.Fprintf/fmt.Sprintf
//     are walked.
//   - Raw-text (<script>/<style>) tracking is within ONE format literal; an
//     element opened in one call and closed in another is not followed.
//
// HOW TO SATISFY THIS LINT when you add a new emission site:
//
//  1. PREFERRED — neutralise the value for its position at the emission site:
//     fmt.Fprintf(&r.sb, `<div id=%q title=%q>`, attrID(n.ID), html.EscapeString(v))
//     and jsStringID / cssValue in the JS and CSS positions respectively.
//  2. If the value is a quote-free constant or derived only from constants, the
//     SAFE-CONSTANT rule already passes it — do nothing.
//  3. ONLY if the value is provably safe but the rule cannot see it (e.g. a
//     parameter every caller fills with a constant), add an entry to
//     attrAllowlist with a reason that answers the question that position
//     raises. Do NOT allowlist a real hole.
//
// The detector is self-proving: the subtests plant one deliberately broken and
// one correctly-neutralised snippet per rule in a throwaway temp dir and run
// the SAME classifier over them, so a scanner that silently stops flagging
// anything fails loudly.
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
	"sync"
	"testing"
)

// ---- escaping/constant helper classification sets ----

// attrEscapers are functions whose RESULT is safe to embed verbatim in a quoted
// attribute: they entity-encode their input so it can carry no raw double
// quote. Matched by callee name.
//
// safeURL is deliberately NOT here. It validates a URL's SCHEME (rejecting
// javascript:/data:) and returns the URL otherwise untouched — it does not
// entity-encode anything, so `href=%q, safeURL(v)` still lets a double quote in
// the URL terminate the attribute. A scheme validator and an escaper are
// different jobs and href needs both: html.EscapeString(safeURL(v)). See
// attrURLValidators.
var attrEscapers = map[string]bool{
	"html.EscapeString": true,
	"attrID":            true,
	"jsStringID":        true,
	"styleAttr":         true,
	// Normalizers that constrain their output to a character set containing
	// none of & < > " ' — structurally stronger than escaping (see the doc
	// comments at each definition). The call sites escape on top of them
	// anyway; they are listed so a normalized value is not reported as
	// unresolvable when it is not additionally wrapped.
	"themeClass": true,
	"langTag":    true,
}

// attrURLValidators only check a URL's scheme. Recorded so the diagnostic can
// say WHY the site is still unsafe rather than just "unrecognised call".
var attrURLValidators = map[string]bool{"safeURL": true}

// attrJSEscapers produce a value safe to drop into a JS source position — a
// literal <script> body or an on*= event-handler attribute. Only jsStringID
// qualifies: it emits a complete quoted JS string literal AND neutralises the
// "</" sequence that would end the <script> element regardless of JS quoting.
//
// html.EscapeString is NOT a JS escaper. In an on*= attribute the browser
// decodes character references BEFORE handing the value to the JS parser, so
// html.EscapeString's own output re-materialises the dangerous characters:
// `&#39;` becomes `'` and closes the JS string literal the handler is built
// from. In a <script> BODY entities are not decoded at all, so escaping there
// silently corrupts the script instead of protecting it.
var attrJSEscapers = map[string]bool{"jsStringID": true}

// attrCSSFilters produce a value safe to drop into a CSS source position (a
// <style> element's body). They are strict allowlists over the CSS value
// grammar — entity encoding is useless in a <style> element, where character
// references are not decoded, and would not stop a ";" from opening a new
// declaration even if they were.
var attrCSSFilters = map[string]bool{
	"cssValue":      true, // render_data.go — <style> block colours
	"cssStyleValue": true, // render_style.go — style="…" attribute values
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

// attrConstString folds a constant string expression — a string literal, or
// several joined with "+" (the shape long HTML templates are written in) — into
// its value. ok=false for anything with a runtime component, which is what
// makes a format string un-analysable.
func attrConstString(e ast.Expr) (string, bool) {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return attrConstString(x.X)
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(x.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return "", false
		}
		l, okL := attrConstString(x.X)
		r, okR := attrConstString(x.Y)
		return l + r, okL && okR
	}
	return "", false
}

func (c *attrClassifier) exprStr(e ast.Expr) string {
	var b bytes.Buffer
	printer.Fprint(&b, c.fset, e)
	return b.String()
}

// ---- format-verb scanning ----

// attrCtx is the HTML source position a verb sits in. Each position decodes
// its content by different rules, so each needs a different escaper — using the
// wrong one is not a partial fix, it is no fix (see attrJSEscapers).
type attrCtx string

const (
	ctxText   attrCtx = "text"   // ordinary element content
	ctxAttr   attrCtx = "attr"   // inside a quoted/unquoted attribute value
	ctxEvent  attrCtx = "event"  // inside an on*= handler attribute (JS after entity decoding)
	ctxURL    attrCtx = "url"    // inside href=/src=/action= (navigating or fetching)
	ctxScript attrCtx = "script" // inside a literal <script> element body (JS, no entity decoding)
	ctxStyle  attrCtx = "style"  // inside a literal <style> element body (CSS, no entity decoding)
)

// attrVerbHit is one format verb and the HTML source position it lands in.
type attrVerbHit struct {
	verb  byte    // s, q, d, ...
	attr  string  // attribute name ("" => not an attribute interpolation)
	ctx   attrCtx // source position (see attrCtx)
	quote byte    // '"', '\'', 'q' (self-quoted by %q), or 0 (unquoted)
	argN  int     // 0-based index of the value argument
}

// attrCtxFor classifies an attribute NAME into the source position its value
// occupies. An on* attribute is a JS position that merely looks like an
// attribute; href/src/action are URL positions where the scheme also matters.
func attrCtxFor(name string) attrCtx {
	l := strings.ToLower(name)
	if strings.HasPrefix(l, "on") && len(l) > 2 {
		return ctxEvent
	}
	switch l {
	case "href", "src", "action", "formaction", "xlink:href", "data", "poster":
		return ctxURL
	}
	return ctxAttr
}

// attrRawTextElem reports whether a tag name opens an HTML raw-text element:
// everything up to the matching close tag is script or CSS source, NOT markup,
// and character references inside it are not decoded.
func attrRawTextElem(name string) (attrCtx, bool) {
	switch strings.ToLower(name) {
	case "script":
		return ctxScript, true
	case "style":
		return ctxStyle, true
	}
	return "", false
}

// attrScanRawText finds a `<script`/`<style` opening tag at s[i] and returns
// the context it opens plus the index just past the tag's ">". ok=false when
// s[i] does not open one.
func attrScanRawText(s string, i int) (attrCtx, int, bool) {
	if s[i] != '<' {
		return "", 0, false
	}
	j := i + 1
	for j < len(s) && attrIsAttrNameChar(s[j]) {
		j++
	}
	ctx, ok := attrRawTextElem(s[i+1 : j])
	if !ok {
		return "", 0, false
	}
	for j < len(s) && s[j] != '>' { // skip the tag's own attributes
		j++
	}
	if j >= len(s) {
		return ctx, len(s), true
	}
	return ctx, j + 1, true
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

// attrVerbs walks a format string tracking the HTML source position of the
// cursor — text, a double-/single-/self-quoted or unquoted attribute value, or
// the body of a literal <script>/<style> element — and returns every verb with
// the position it lands in. Verbs in plain text context (including the bare
// "%s" gap BETWEEN attributes, e.g. a11y(n)) carry ctxText and attr=="" and are
// skipped by the caller; every other position is checked.
//
// Raw-text tracking is WITHIN ONE format literal: a <script> opened in one
// Fprintf and closed in another is not followed (the repo writes each inline
// script as a single literal). That is a false-negative bound, not a false
// positive — the same conservative direction as the rest of the scanner.
func attrVerbs(s string) []attrVerbHit {
	var hits []attrVerbHit
	mode := byte('t') // t=text, d=double-quoted, s=single-quoted, u=unquoted, r=raw-text body
	attr := ""
	raw := ctxText // which raw-text element is open when mode=='r'
	argIdx := 0
	for i := 0; i < len(s); {
		c := s[i]
		switch mode {
		case 'r':
			// Inside <script>/<style>: only the matching close tag ends it.
			if c == '<' && i+1 < len(s) && s[i+1] == '/' {
				j := i + 2
				for j < len(s) && attrIsAttrNameChar(s[j]) {
					j++
				}
				if ctx, ok := attrRawTextElem(s[i+2 : j]); ok && ctx == raw {
					mode, raw = 't', ctxText
					i = j
					continue
				}
			}
			if c == '%' {
				v, next, stars := attrParseVerb(s, i)
				if v != '%' && v != 0 {
					// "<style-body>" / "<script-body>" is deliberately not a
					// legal attribute name, so a raw-text finding can never
					// share an allowlist key with an attribute of the same name.
					hits = append(hits, attrVerbHit{verb: v, ctx: raw, attr: "<" + string(raw) + "-body>", argN: argIdx + stars})
					argIdx += 1 + stars
				}
				i = next
				continue
			}
			i++
		case 't':
			if ctx, next, ok := attrScanRawText(s, i); ok {
				mode, raw = 'r', ctx
				i = next
				continue
			}
			if c == '%' {
				v, next, stars := attrParseVerb(s, i)
				if v != '%' && v != 0 {
					hits = append(hits, attrVerbHit{verb: v, ctx: ctxText, quote: 0, argN: argIdx + stars})
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
					hits = append(hits, attrVerbHit{verb: v, attr: attr, ctx: attrCtxFor(attr), quote: '"', argN: argIdx + stars})
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
					hits = append(hits, attrVerbHit{verb: v, attr: attr, ctx: attrCtxFor(attr), quote: '\'', argN: argIdx + stars})
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
					hits = append(hits, attrVerbHit{verb: v, attr: attr, ctx: attrCtxFor(attr), quote: q, argN: argIdx + stars})
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

// attrMode is which escaping discipline an expression is being judged against.
// It mirrors attrCtx, collapsed to the three rule sets that actually differ.
type attrMode string

const (
	modeHTML attrMode = "html" // attribute value / URL attribute: entity encoding
	modeJS   attrMode = "js"   // <script> body or on*= handler: jsStringID only
	modeCSS  attrMode = "css"  // <style> body: a CSS-value allowlist
)

func attrModeFor(ctx attrCtx) attrMode {
	switch ctx {
	case ctxEvent, ctxScript:
		return modeJS
	case ctxStyle:
		return modeCSS
	}
	return modeHTML
}

// attrSafeProducer reports whether the local function `name` returns ONLY safe
// values FOR THE GIVEN MODE — i.e. every return statement yields
// escaped/filtered/constant expressions. Such a helper (boxCSS, textCSS,
// containerCSS, num, flexAlign, segStyle, ...) may be interpolated verbatim.
// Resolution is recursive (helpers calling helpers) with a cycle guard, so this
// is the "one level of local helper indirection" the design calls for, applied
// as far as the call chain goes. The memo and cycle guard are keyed by mode as
// well as name: a helper that is safe for an attribute is not thereby safe in a
// <script> body.
func (c *attrClassifier) attrSafeProducer(mode attrMode, name string) bool {
	key := string(mode) + "\x00" + name
	if v, ok := c.memo[key]; ok {
		return v == 1
	}
	if c.busy[key] {
		return false
	}
	decl, ok := c.pk.funcs[name]
	if !ok || decl.Body == nil {
		c.memo[key] = -1
		return false
	}
	c.busy[key] = true
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
			if !c.classifyIn(mode, r, sc, 0) {
				safe = false
			}
		}
		return true
	})
	if !found {
		safe = false
	}
	delete(c.busy, key)
	if safe {
		c.memo[key] = 1
	} else {
		c.memo[key] = -1
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

// attrSprintfSafe reports whether a fmt.Sprintf result is safe to embed in the
// given mode: the format literal introduces no delimiter of its own and every
// non-numeric verb is filled by an argument that is itself safe for the mode.
func (c *attrClassifier) attrSprintfSafe(mode attrMode, fs string, args []ast.Expr, sc *attrScope, depth int) bool {
	if !attrLiteralOK(mode, fs) {
		return false
	}
	for _, vh := range attrAllVerbs(fs) {
		if attrNumericVerb(vh.verb) {
			continue
		}
		if vh.argN >= len(args) || !c.classifyIn(mode, args[vh.argN], sc, depth) {
			return false
		}
	}
	return true
}

// attrLiteralOK reports whether a SOURCE STRING LITERAL is inert in the given
// mode. A literal is author-written Go source, not runtime data, so the bar is
// only "it does not itself supply the delimiter that would let an adjacent
// interpolation escape".
func attrLiteralOK(mode attrMode, s string) bool {
	switch mode {
	case modeJS:
		// A quote or backslash would let a following value continue a JS
		// string the lint cannot see the end of; "</" ends the script element.
		return !strings.ContainsAny(s, "\"'\\") && !strings.Contains(s, "</")
	case modeCSS:
		// A brace or "<" would open a rule / end the style element around an
		// interpolation.
		return !strings.ContainsAny(s, "{}<")
	}
	return !strings.Contains(s, `"`) // a quote-free literal cannot break out
}

// classify is the HTML-attribute rule set — kept as the name the self-proof
// subtests and the rest of the scanner use.
func (c *attrClassifier) classify(e ast.Expr, sc *attrScope, depth int) bool {
	return c.classifyIn(modeHTML, e, sc, depth)
}

// classifyIn reports whether an argument expression is SAFE to interpolate at a
// source position governed by `mode`. It is deliberately conservative: anything
// it cannot prove safe returns false and is left to the allowlist or flagged.
//
// The three modes share the constant/variable/concatenation machinery and
// differ only in which CALLS count as neutralising, because the browser decodes
// each position differently:
//
//	modeHTML  entity encoding works (the parser decodes it back)
//	modeJS    entity encoding is WORSE than nothing (an on*= value is entity-
//	          decoded before the JS parser sees it, so html.EscapeString's
//	          &#39; becomes a real ' and closes the handler's string; inside a
//	          <script> body entities are not decoded at all). Only jsStringID.
//	modeCSS   entity encoding does nothing at all (a <style> body is raw text)
//	          and would not stop ";" anyway. Only a CSS-value allowlist.
func (c *attrClassifier) classifyIn(mode attrMode, e ast.Expr, sc *attrScope, depth int) bool {
	if depth > 16 {
		return false
	}
	switch x := e.(type) {
	case *ast.ParenExpr:
		return c.classifyIn(mode, x.X, sc, depth+1)
	case *ast.BasicLit:
		switch x.Kind {
		case token.INT, token.FLOAT, token.CHAR, token.IMAG:
			return true
		case token.STRING:
			s, err := strconv.Unquote(x.Value)
			if err != nil {
				return false
			}
			return attrLiteralOK(mode, s)
		}
		return false
	case *ast.Ident:
		if x.Name == "true" || x.Name == "false" || x.Name == "nil" {
			return true
		}
		if rhsList, ok := attrAllAssigns(sc, x.Name); ok {
			all := true
			for _, rhs := range rhsList {
				if !c.classifyIn(mode, rhs, sc, depth+1) {
					all = false
				}
			}
			return all
		}
		if v, ok := c.pk.consts[x.Name]; ok { // package-level const
			return c.classifyIn(mode, v, nil, depth+1)
		}
		return false
	case *ast.BinaryExpr:
		if x.Op == token.ADD { // a concatenation is safe iff every operand is
			return c.classifyIn(mode, x.X, sc, depth+1) && c.classifyIn(mode, x.Y, sc, depth+1)
		}
		return false
	case *ast.CallExpr:
		name := calleeName(x.Fun)
		switch mode {
		case modeJS:
			if attrJSEscapers[name] {
				return true
			}
		case modeCSS:
			if attrCSSFilters[name] {
				return true
			}
			// In CSS, html.EscapeString is a no-op the browser never undoes,
			// so safety must come from the value INSIDE it. Look through.
			if name == "html.EscapeString" && len(x.Args) == 1 {
				return c.classifyIn(modeCSS, x.Args[0], sc, depth+1)
			}
		default:
			if attrEscapers[name] {
				return true // SAFE-ESCAPED
			}
			if attrURLValidators[name] {
				// Scheme-checked but NOT encoded: a quote inside an allowed
				// http:// URL still terminates the attribute. Stop here rather
				// than falling through to the local-helper rule, which would
				// otherwise "prove" safeURL safe from the shape of its returns.
				return false
			}
		}
		if attrNumericFuncs[name] {
			return true // SAFE-CONSTANT (numeric output)
		}
		if name == "fmt.Sprintf" {
			if len(x.Args) >= 1 {
				if fs, ok := attrConstString(x.Args[0]); ok {
					return c.attrSprintfSafe(mode, fs, x.Args[1:], sc, depth+1)
				}
			}
			return false
		}
		base := name
		if i := strings.IndexByte(name, '.'); i >= 0 {
			base = name[i+1:] // method call r.boxCSS(...) -> boxCSS
		}
		if _, ok := c.pk.funcs[base]; ok {
			return c.attrSafeProducer(mode, base) // local helper, judged in this mode
		}
		return false
	}
	return false
}

// ---- scanning a directory ----

// attrInterp is one detected interpolation into an HTML source position, and
// its verdict.
type attrInterp struct {
	file  string // base name
	line  int
	attr  string // attribute name, or "script"/"style" for a raw-text body
	ctx   attrCtx
	verb  byte
	quote byte
	arg   string // argument expression, as printed
	safe  bool   // classified safe by the rules (before allowlist)
}

// attrFixHint is the per-context repair instruction printed with a finding.
func attrFixHint(ctx attrCtx) string {
	switch ctx {
	case ctxEvent:
		return "an on*= attribute is JAVASCRIPT that the parser entity-decodes FIRST, so html.EscapeString does not protect it (&#39; decodes to ' and closes the handler's string). FIX: build the value with jsStringID(...), or emit no dynamic value at all."
	case ctxScript:
		return "a <script> body is raw text: entities are NOT decoded, so html.EscapeString cannot help and \"</\" ends the element regardless of JS quoting. FIX: jsStringID(...)."
	case ctxStyle:
		return "a <style> body is raw text CSS: entity encoding is a no-op there and would not stop a \";\" from opening another declaration anyway. FIX: filter through cssValue(...)/cssStyleValue(...)."
	case ctxURL:
		return "a URL attribute needs BOTH a scheme check and entity encoding: safeURL(...) alone leaves a double quote in the URL free to terminate the attribute. FIX: html.EscapeString(safeURL(...))."
	default:
		return "a double quote in the value breaks out of the attribute and injects markup. FIX: escape it (html.EscapeString(...)/attrID(...)/styleAttr(...))."
	}
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
				pos := fset.Position(x.Pos())
				fs, ok := attrConstString(x.Args[fi])
				if !ok {
					// A format string with a runtime component cannot be walked
					// for context, so NOTHING about the site can be proven —
					// including that it is not assembling an attribute out of
					// untrusted data. Report it rather than silently passing
					// (the scanner used to `return true` here, which let a whole
					// emission site hide from the lint by building its format at
					// run time).
					out = append(out, attrInterp{
						file: base, line: pos.Line, attr: "<computed-format>",
						ctx: ctxAttr, verb: 's', arg: c.exprStr(x.Args[fi]), safe: false,
					})
					return true
				}
				args := x.Args[fi+1:]
				for _, vh := range attrVerbs(fs) {
					if vh.attr == "" {
						continue // text-context / between-attributes verb
					}
					argExpr := "<none>"
					safe := attrNumericVerb(vh.verb)
					if vh.argN < len(args) {
						argExpr = c.exprStr(args[vh.argN])
						if !safe {
							safe = c.classifyIn(attrModeFor(vh.ctx), args[vh.argN], cur(), 0)
						}
					}
					out = append(out, attrInterp{
						file: base, line: pos.Line, attr: vh.attr, ctx: vh.ctx,
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

	// ---- raw-text element bodies (<style>/<script>), reported since the lint
	// learned to track them. Each reason must answer the CSS/JS question, not
	// the quote question: in a <style> body the danger is ';' '{' '}' and
	// '</style'; in a <script> body it is '</' and an unterminated JS string.

	{"render_data.go", "<style-body>", "attrID(n.ID)",
		"CSS SELECTOR position in tabIndicator. The emitter returns early unless isIdent(n.ID) (render_data.go:474) — [A-Za-z_][A-Za-z0-9_]* only — so the id can contain no ';', '{', '}', '<' or whitespace and cannot end the rule, open another one, or close the <style> element. attrID's escaping is belt-and-braces here, not the argument"},
	{"server.go", "<style-body>", "themeCSS(rt)",
		"CSS body of the HTML shell = render.ThemeCSS (a package constant) + render.TokenCSS('#qorm-stage', manifest designTokens). TokenCSS is the only author-fed half and it strips ';' '{' '}' '<' '>' CR and LF from every token VALUE (sanitizeTokenValue) and maps every token NAME onto [A-Za-z0-9_-] (sanitizeTokenName), so a designToken can neither end its declaration, open a rule, nor close the <style> element"},
	{"server.go", "<style-body>", "fixedCSS",
		"CSS body of the fixed-window rule, built ONLY from int fields w.Width/w.Height via fmt with %d verbs (server.go:1443). The manifest's platforms.desktop.window width/height are parsed with int(asFloat(...)), so the value is digits only — no author string reaches this style body, and the literal rule text (width:Npx, min/max-width, height, body{align-items:center;padding:0}) is a package constant. Cannot contain ';', '{', '}', '<', quotes or '</' to end the declaration, open a rule, or close the <style> element"},
	{"server.go", "<script-body>", "qormAppJS(rev, tok)",
		"JS body of the HTML shell = the embedded app.js constant with exactly two placeholders substituted: __QORM_REV__ takes strconv.FormatInt of an int64 (digits and sign only) and __QORM_TOKEN__ takes s.eventToken, 16 crypto/rand bytes hex-encoded (genEventToken, server.go:167) — 32 characters of [0-9a-f]. Neither can contain a quote, a backslash or '</'; no app/author/agent data reaches this script body at all (every Page call site passes s.eventToken or nothing)"},
	{"server.go", "<script-body>", "qormKeyBindings(rt)",
		"JSON snippet declaring scene key/swipe bindings and handler indices for the HTML path. Key names come from SceneKeys/SceneKeyReleases maps and swipe dirs from SceneSwipes — the loader normalises them to lowercase identifiers and rejects non-ident names (swipe dirs are further restricted to left/right/up/down). Handler names come from render.RenderScene's handler table — action ids and invoke names, which are isIdent-checked identifiers. The entire structure is json.Marshal'd into a legal JSON literal (assignments: __qormKeys={...}; __qormKeyReleases={...}; __qormKeyToIdx={...}; __qormSwipes={...}). Every value is either a quoted JSON string (escaped by the stdlib) or a JSON number (handler index). None can contain '</', an unescaped quote, or escape the script tag"},
}

func attrAllowKey(file, attr, arg string) string { return file + "\x00" + attr + "\x00" + arg }

// attrMatchedAll accumulates allowlist hits across every scanned directory, so
// "stale entry" is judged once over the whole scan rather than per package.
var (
	attrMatchedMu  sync.Mutex
	attrMatchedAll = map[string]int{}
)

// ---- the test ----

// attrScanDirs are EVERY package that assembles HTML by hand. This used to be
// the single directory internal/render, which is precisely why a stored-XSS
// hole survived in internal/server for so long: the HTML *shell* (<html lang>,
// the stage's theme class, the console's iframe title, the offline manifest's
// meta content) is written in internal/server, and nothing scanned it. A
// package that emits markup and is not listed here is not covered — add it.
var attrScanDirs = []string{
	"../../internal/render",
	"../../internal/server",
}

const attrRenderDir = "../../internal/render"

// TestAttrInjectionLint guards the interpolation-into-HTML bug class. See the
// file-top comment for how to satisfy it when adding a widget attribute.
func TestAttrInjectionLint(t *testing.T) {
	for _, dir := range attrScanDirs {
		t.Run(filepath.Base(dir)+"-package-clean", func(t *testing.T) {
			interps, err := scanAttrInterpolations(dir)
			if err != nil {
				t.Fatalf("scanning %s: %v", dir, err)
			}
			if len(interps) == 0 {
				t.Fatalf("no interpolations found in %s — the scan is broken", dir)
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
				t.Errorf("%s:%d: UNSAFE %s interpolation: %s interpolated into %s (verb %%%c)\n"+
					"  %s\n"+
					"  If it is provably safe but the rule cannot see it, add a justified entry to\n"+
					"  attrAllowlist — the reason must explain why THIS CONTEXT is safe, not merely\n"+
					"  that the value carries no quote.\n"+
					"  offending expression: %s",
					it.file, it.line, it.ctx, it.arg, it.attr, it.verb, attrFixHint(it.ctx), it.arg)
			}

			t.Logf("%s: interpolations scanned: %d (safe-by-rule %d, allowlisted %d, unsafe %d)",
				dir, len(interps), nEscapedConst, nAllow, len(unsafe))

			// Record what this directory matched so the staleness check below
			// can be made across ALL directories at once.
			attrMatchedMu.Lock()
			for k, n := range matched {
				attrMatchedAll[k] += n
			}
			attrMatchedMu.Unlock()
		})
	}

	// Every allowlist entry must still match a live site in SOME scanned
	// directory, or it is stale.
	t.Run("allowlist-has-no-stale-entries", func(t *testing.T) {
		attrMatchedMu.Lock()
		defer attrMatchedMu.Unlock()
		for _, e := range attrAllowlist {
			if attrMatchedAll[attrAllowKey(e.file, e.attr, e.arg)] == 0 {
				t.Errorf("stale allowlist entry (matches no detected site — remove it, or the site was fixed/renamed):\n  %s  attr=%s  arg=%s\n  reason: %s",
					e.file, e.attr, e.arg, e.reason)
			}
		}
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

	// ---- self-proofs for the context rules the red team demonstrated the
	// scanner was blind to. Each plants the exact shape that slipped through
	// and asserts the SAME classifier now reaches the right verdict.

	// The hole that motivated scanning internal/server at all: the shell's
	// <html lang="%s"> and class="qorm-theme-%s" took rt.CurrentLocale() /
	// rt.CurrentTheme() — i.e. state.locale / state.theme, writable by an
	// action, an http response or MCP qorm_set_state — with no escaping and no
	// normalization, so the next full page load executed attacker JS in the
	// app's origin. The scanner ALWAYS had the rule to catch it; it simply
	// never looked at the package.
	t.Run("self-proof-shell-state-into-attribute-is-flagged", func(t *testing.T) {
		// rt is an interface here exactly as runtime.Runtime is a foreign type
		// in internal/server: the state accessors are NOT local functions the
		// classifier can look inside, so their result is unresolvable — and
		// therefore unsafe — which is the correct verdict.
		interps := scanSnippet(t, "shell.go", "package snippet\n\nimport \"fmt\"\n\n"+
			"type stateReader interface {\n\tCurrentLocale() string\n\tCurrentTheme() string\n}\n\n"+
			"func page(rt stateReader, body string) string {\n"+
			"\tlang := rt.CurrentLocale()\n"+
			"\ttheme := rt.CurrentTheme()\n"+
			"\treturn fmt.Sprintf(`<html lang=\"%s\"><div class=\"qorm-theme-%s\">%s</div></html>`, lang, theme, body)\n"+
			"}\n")
		assertFlagged(t, interps, "lang", "lang", ctxAttr)
		assertFlagged(t, interps, "class", "theme", ctxAttr)
	})

	// L3: safeURL only validates the SCHEME. A double quote inside an
	// http:// URL still terminates the attribute, so safeURL alone is not a
	// SAFE verdict — only html.EscapeString(safeURL(v)) is.
	t.Run("self-proof-safeURL-alone-is-flagged", func(t *testing.T) {
		interps := scanSnippet(t, "url.go", "package snippet\n\nimport (\n\t\"fmt\"\n\t\"html\"\n)\n\n"+
			"func safeURL(u string) string { return u }\n\n"+
			"func bad(u string) string  { return fmt.Sprintf(`<a href=%q>x</a>`, safeURL(u)) }\n"+
			"func good(u string) string { return fmt.Sprintf(`<a href=%q>x</a>`, html.EscapeString(safeURL(u))) }\n")
		assertFlagged(t, interps, "href", "safeURL(u)", ctxURL)
		assertClean(t, interps, "html.EscapeString(safeURL(u))")
	})

	// L4: an on*= attribute is JS that the HTML parser entity-DECODES first,
	// so html.EscapeString's &#39; turns back into a ' that closes the
	// handler's string literal. Only jsStringID is a real fix there.
	t.Run("self-proof-escaped-into-event-handler-is-flagged", func(t *testing.T) {
		interps := scanSnippet(t, "ev.go", "package snippet\n\nimport (\n\t\"fmt\"\n\t\"html\"\n)\n\n"+
			"func jsStringID(s string) string { return s }\n\n"+
			"func bad(id string) string  { return fmt.Sprintf(`<b onclick=\"qorm('%s')\">x</b>`, html.EscapeString(id)) }\n"+
			"func good(id string) string { return fmt.Sprintf(`<b onclick=\"qorm(%s)\">x</b>`, jsStringID(id)) }\n")
		assertFlagged(t, interps, "onclick", "html.EscapeString(id)", ctxEvent)
		assertClean(t, interps, "jsStringID(id)")
	})

	// L2: a <script> body is raw text — entities are never decoded there, and
	// "</" ends the element no matter how the JS is quoted.
	t.Run("self-proof-script-body-is-flagged", func(t *testing.T) {
		interps := scanSnippet(t, "sc.go", "package snippet\n\nimport (\n\t\"fmt\"\n\t\"html\"\n)\n\n"+
			"func jsStringID(s string) string { return s }\n\n"+
			"func bad(id string) string  { return fmt.Sprintf(`<script>go(%s)</script>`, html.EscapeString(id)) }\n"+
			"func good(id string) string { return fmt.Sprintf(`<script>go(%s)</script>`, jsStringID(id)) }\n")
		assertFlagged(t, interps, "<script-body>", "html.EscapeString(id)", ctxScript)
		assertClean(t, interps, "jsStringID(id)")
	})

	// L7: a <style> body is raw-text CSS — entity encoding is inert there, and
	// it would not stop a ";" from opening another declaration in any case.
	t.Run("self-proof-style-body-is-flagged", func(t *testing.T) {
		interps := scanSnippet(t, "st.go", "package snippet\n\nimport (\n\t\"fmt\"\n\t\"html\"\n)\n\n"+
			"func cssValue(s string) string { return s }\n\n"+
			"func bad(c string) string  { return fmt.Sprintf(`<style>#a{color:%s}</style>`, html.EscapeString(c)) }\n"+
			"func good(c string) string { return fmt.Sprintf(`<style>#a{color:%s}</style>`, cssValue(c)) }\n")
		assertFlagged(t, interps, "<style-body>", "html.EscapeString(c)", ctxStyle)
		assertClean(t, interps, "cssValue(c)")
	})

	// L6: a format string assembled at run time hides every verb in it from
	// the context walker, so the site cannot be judged and must be reported.
	// A format built by CONCATENATING LITERALS is still constant and must not
	// be reported (that is how the long templates in render_data.go read).
	t.Run("self-proof-computed-format-is-flagged", func(t *testing.T) {
		interps := scanSnippet(t, "fmtx.go", "package snippet\n\nimport \"fmt\"\n\n"+
			"func bad(f, v string) string  { return fmt.Sprintf(f, v) }\n"+
			"func good(v string) string    { return fmt.Sprintf(`<div `+`data-x=\"%s\">`, \"k\") }\n")
		found := false
		for _, it := range interps {
			if it.attr == "<computed-format>" && !it.safe {
				found = true
			}
			if it.attr == "data-x" && !it.safe {
				t.Errorf("self-proof failed: a format built from concatenated LITERALS must stay analysable, got %+v", it)
			}
		}
		if !found {
			t.Errorf("self-proof failed: a run-time format string was not reported (interps=%+v)", interps)
		}
	})
}

// scanSnippet parses one planted source file with the real classifier.
func scanSnippet(t *testing.T, name, src string) []attrInterp {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	interps, err := scanAttrInterpolations(dir)
	if err != nil {
		t.Fatalf("scanning planted snippet %s: %v", name, err)
	}
	return interps
}

// assertFlagged requires the planted (attr, arg) site to be reported UNSAFE in
// the expected source position.
func assertFlagged(t *testing.T, interps []attrInterp, attr, arg string, ctx attrCtx) {
	t.Helper()
	for _, it := range interps {
		if it.attr == attr && it.arg == arg {
			if it.safe {
				t.Errorf("self-proof failed: %s=%s was judged SAFE, must be flagged", attr, arg)
			}
			if it.ctx != ctx {
				t.Errorf("self-proof failed: %s=%s classified as ctx %q, want %q", attr, arg, it.ctx, ctx)
			}
			return
		}
	}
	t.Errorf("self-proof failed: no finding at all for %s=%s (interps=%+v)", attr, arg, interps)
}

// assertClean requires the planted correctly-neutralised argument to pass.
func assertClean(t *testing.T, interps []attrInterp, arg string) {
	t.Helper()
	seen := false
	for _, it := range interps {
		if it.arg != arg {
			continue
		}
		seen = true
		if !it.safe {
			t.Errorf("self-proof failed: correctly-neutralised %s was flagged: %+v", arg, it)
		}
	}
	if !seen {
		t.Errorf("self-proof failed: the correctly-neutralised site %s was not detected at all", arg)
	}
}
