package integration

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// This file generates the API-reference pages of the docs site straight from the
// source of truth, so what a human or an AI reads is exactly what the runtime
// does. Four surfaces, each with an English page under api/ and a
// Chinese mirror under api/zh/ (the tables are language-neutral; only
// the surrounding prose differs):
//
//   props.md    — the declarative UI contract: node schema, common style props,
//                 and the per-widget prop table (extracted from internal/render).
//   actions.md  — the action/state contract: every step `type` and its fields
//                 (extracted from internal/runtime + internal/model).
//   http-api.md — the runtime's HTTP + SSE surface (extracted from internal/server).
//   go-api.md   — the public Go package pkg/qormext (extracted via go/doc).
//
// Regenerate all of them:
//   QORM_UPDATE_DOCS=1 go test ./internal/integration/ -run TestAPIRef

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// writeGenHeader emits the standard "auto-generated" doc header + intro, in the
// given language ("en" or "zh").
func writeGenHeader(b *strings.Builder, lang, title, intro, from string) {
	b.WriteString("# " + title + "\n\n")
	if lang == "zh" {
		b.WriteString("> 由源码自动生成(`TestAPIRef`),请勿手工编辑。下方的" + from + "从代码抽取,不会与实现漂移。\n\n")
	} else {
		b.WriteString("> Auto-generated from the source (`TestAPIRef`) — do not edit by hand. The " + from + " below is extracted from the code, so it can never drift.\n\n")
	}
	b.WriteString(intro + "\n\n")
}

// writeGenHeaderFrom is like writeGenHeader but names the source package.
func writeGenHeaderFrom(b *strings.Builder, lang, title, intro, pkg string) {
	b.WriteString("# " + title + "\n\n")
	if lang == "zh" {
		b.WriteString("> 由 `" + pkg + "` 自动生成(`TestAPIRef`),请勿手工编辑。\n\n")
	} else {
		b.WriteString("> Auto-generated from `" + pkg + "` (`TestAPIRef`) — do not edit by hand.\n\n")
	}
	b.WriteString(intro + "\n\n")
}

// syncDoc writes want to path when QORM_UPDATE_DOCS=1, else asserts equality.
func syncDoc(t *testing.T, path, want string) {
	t.Helper()
	if os.Getenv("QORM_UPDATE_DOCS") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Errorf("%s out of sync — run: QORM_UPDATE_DOCS=1 go test ./internal/integration/ -run TestAPIRef", path)
	}
}

// ---- shared render-source analysis (per-widget props) -----------------------

// keyArgIndex maps a prop-accessor helper to the arg position of its key string.
var keyArgIndex = map[string]int{
	"propStr": 1, "propStrOr": 1, "propNum": 1, "propBool": 1,
	"numProp": 1, "parseInvokeProp": 1, "boundArray": 1,
}

// stopMethods are renderer methods we neither attribute props to nor descend
// into: the common styled-box/text builders (their props are documented once as
// "common"), and the child-render recursion boundary (descending there would
// union every child widget's props into the parent).
var stopMethods = map[string]bool{
	"boxCSS": true, "textCSS": true, "containerCSS": true, "a11y": true,
	"resolveStyle": true, "resolveStyleVal": true,
	"node": true, "renderInner": true,
}

// widgetPropTable parses internal/render and returns, for each widget name from
// the node() switch, the widget-specific prop keys its renderer reads (the
// transitive closure of prop literals reachable from the renderer method,
// stopping at stopMethods).
func widgetPropTable(t *testing.T) [][2]string {
	t.Helper()
	const dir = "../../internal/render"
	fset := token.NewFileSet()
	directProps := map[string]map[string]bool{}
	calls := map[string]map[string]bool{}
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, de := range des {
		if !strings.HasSuffix(de.Name(), ".go") || strings.HasSuffix(de.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, de.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			dp := map[string]bool{}
			cl := map[string]bool{}
			ast.Inspect(fn.Body, func(nd ast.Node) bool {
				switch x := nd.(type) {
				case *ast.CallExpr:
					callee := ""
					switch fe := x.Fun.(type) {
					case *ast.Ident:
						callee = fe.Name
					case *ast.SelectorExpr:
						callee = fe.Sel.Name
						if id, ok := fe.X.(*ast.Ident); ok && id.Name == "r" {
							cl[fe.Sel.Name] = true
						}
					}
					if idx, ok := keyArgIndex[callee]; ok && len(x.Args) > idx {
						addLit(dp, x.Args[idx])
					}
					if se, ok := x.Fun.(*ast.SelectorExpr); ok && se.Sel.Name == "Prop" && len(x.Args) == 1 {
						addLit(dp, x.Args[0])
					}
				case *ast.IndexExpr:
					if se, ok := x.X.(*ast.SelectorExpr); ok && se.Sel.Name == "Props" {
						addLit(dp, x.Index)
					}
				}
				return true
			})
			directProps[fn.Name.Name] = dp
			calls[fn.Name.Name] = cl
		}
	}

	props := func(entry string) []string {
		seen := map[string]bool{}
		out := map[string]bool{}
		var walk func(m string)
		walk = func(m string) {
			if seen[m] || stopMethods[m] {
				return
			}
			seen[m] = true
			for k := range directProps[m] {
				out[k] = true
			}
			for c := range calls[m] {
				walk(c)
			}
		}
		walk(entry)
		var ks []string
		for k := range out {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		return ks
	}

	// node() switch: widget name -> renderer method.
	src, err := os.ReadFile(filepath.Join(dir, "render.go"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	start := strings.Index(s, "func (r *renderer) node(n *model.Node)")
	end := strings.Index(s[start:], "\n\tdefault:\n\t\tr.unknown(n)")
	if start < 0 || end < 0 {
		t.Fatal("could not locate the node() switch")
	}
	body := s[start : start+end]
	labelRe := regexp.MustCompile(`"([a-z0-9]+)"`)
	callRe := regexp.MustCompile(`r\.(\w+)\(`)
	lines := strings.Split(body, "\n")

	var rows [][2]string
	for i, ln := range lines {
		tl := strings.TrimSpace(ln)
		if !strings.HasPrefix(tl, "case ") || !strings.HasSuffix(tl, ":") {
			continue
		}
		labels := labelRe.FindAllStringSubmatch(tl, -1)
		if len(labels) == 0 {
			continue
		}
		canonical := labels[0][1]
		method := ""
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "" {
				continue
			}
			if m := callRe.FindStringSubmatch(lines[j]); m != nil {
				method = m[1]
			}
			break
		}
		p := props(method)
		cell := "—"
		if len(p) > 0 {
			cell = "`" + strings.Join(p, "` · `") + "`"
		}
		rows = append(rows, [2]string{canonical, cell})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
	return rows
}

func addLit(set map[string]bool, e ast.Expr) {
	if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		if v, err := strconv.Unquote(lit.Value); err == nil && v != "" {
			set[v] = true
		}
	}
}

// ---- action step vocabulary --------------------------------------------------

// stepTypes extracts the action step `type` values from the runtime dispatch.
func stepTypes(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("../../internal/runtime/runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	start := strings.Index(s, "func (r *Runtime) applyStep(")
	if start < 0 {
		t.Fatal("could not locate applyStep()")
	}
	// stop at the next top-level func so we only read applyStep's switch.
	rest := s[start+1:]
	if e := strings.Index(rest, "\nfunc "); e >= 0 {
		s = s[start : start+1+e]
	} else {
		s = s[start:]
	}
	caseRe := regexp.MustCompile(`(?m)^\tcase (.+):`)
	litRe := regexp.MustCompile(`"([a-zA-Z.]+)"`)
	var out []string
	seen := map[string]bool{}
	for _, m := range caseRe.FindAllStringSubmatch(s, -1) {
		for _, l := range litRe.FindAllStringSubmatch(m[1], -1) {
			if !seen[l[1]] {
				seen[l[1]] = true
				out = append(out, l[1])
			}
		}
	}
	return out
}
