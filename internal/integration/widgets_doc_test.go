package integration

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// widgetCatalog extracts the canonical widget type + its aliases + renderer from
// the render node() switch, so the doc is generated from the ONE source of truth
// (the switch itself) and can never drift.
func widgetCatalog(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("../../internal/render/render.go")
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

	type group struct{ canonical, aliases, renderer string }
	var groups []group
	for i, ln := range lines {
		tl := strings.TrimSpace(ln)
		if !strings.HasPrefix(tl, "case ") || !strings.HasSuffix(tl, ":") {
			continue
		}
		labels := labelRe.FindAllStringSubmatch(tl, -1)
		if len(labels) == 0 {
			continue
		}
		var names []string
		for _, m := range labels {
			names = append(names, m[1])
		}
		renderer := ""
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "" {
				continue
			}
			if m := callRe.FindStringSubmatch(lines[j]); m != nil {
				renderer = m[1]
			}
			break
		}
		g := group{canonical: names[0], renderer: renderer}
		if len(names) > 1 {
			g.aliases = strings.Join(names[1:], ", ")
		}
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].canonical < groups[j].canonical })

	var b strings.Builder
	b.WriteString("# Widget Catalog\n\n")
	b.WriteString("> Auto-generated from the node() switch in `internal/render/render.go` (`TestWidgetCatalogInSync`) — do not edit by hand.\n")
	b.WriteString("> Auto-generated from the render switch — do not edit by hand.\n\n")
	b.WriteString("Each widget lists its **canonical name** first; the rest are equivalent aliases. Prefer the canonical name when writing apps.\n\n")
	b.WriteString("| Canonical | Aliases | Renderer |\n|---|---|---|\n")
	for _, g := range groups {
		al := g.aliases
		if al == "" {
			al = "—"
		}
		b.WriteString("| `" + g.canonical + "` | " + al + " | `" + g.renderer + "` |\n")
	}
	return b.String()
}

// TestWidgetCatalogInSync keeps docs/reference/widgets.md generated from the
// render switch — so the canonical widget names + aliases a human/AI reads are
// exactly what the renderer handles. Regenerate: QORM_UPDATE_DOCS=1 go test
// ./internal/integration/ -run TestWidgetCatalogInSync
func TestWidgetCatalogInSync(t *testing.T) {
	const path = "../../docs/reference/widgets.md"
	want := widgetCatalog(t)
	if os.Getenv("QORM_UPDATE_DOCS") == "1" {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Errorf("docs/reference/widgets.md out of sync — run: QORM_UPDATE_DOCS=1 go test ./internal/integration/ -run TestWidgetCatalogInSync")
	}
}
