package mdsite

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type page struct {
	htmlRel string // path relative to out dir, e.g. "spec/ir-spec.html"
	title   string
	group   string // top-level dir, "" for root
}

// BuildSite renders every markdown file under docsDir into a linked static HTML
// site under outDir, and returns the number of pages written.
func BuildSite(docsDir, outDir string) (int, error) {
	var pages []page
	srcOf := map[string]string{} // htmlRel -> source md path

	err := filepath.WalkDir(docsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		rel, _ := filepath.Rel(docsDir, path)
		htmlRel := strings.TrimSuffix(rel, ".md") + ".html"
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		group := ""
		if parts := strings.SplitN(rel, string(filepath.Separator), 2); len(parts) == 2 {
			group = parts[0]
		}
		pages = append(pages, page{htmlRel: htmlRel, title: firstHeading(string(data), rel), group: group})
		srcOf[htmlRel] = path
		return nil
	})
	if err != nil {
		return 0, err
	}
	if len(pages) == 0 {
		return 0, fmt.Errorf("no markdown files under %s", docsDir)
	}
	sort.Slice(pages, func(i, j int) bool {
		if pages[i].group != pages[j].group {
			return pages[i].group < pages[j].group
		}
		return pages[i].htmlRel < pages[j].htmlRel
	})

	for _, p := range pages {
		data, _ := os.ReadFile(srcOf[p.htmlRel])
		body := RenderMarkdown(string(data))
		nav := navHTML(pages, p.htmlRel)
		out := filepath.Join(outDir, p.htmlRel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(out, []byte(pageHTML(p.title, nav, body)), 0o644); err != nil {
			return 0, err
		}
	}

	// index.html: reuse an existing index/README page, else a generated landing.
	if !hasPage(pages, "index.html") && !hasPage(pages, "README.html") {
		nav := navHTML(pages, "index.html")
		landing := "<h1>QORM Documentation</h1>\n<p>Select a page from the sidebar.</p>\n"
		if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte(pageHTML("QORM Docs", nav, landing)), 0o644); err != nil {
			return 0, err
		}
	} else if hasPage(pages, "README.html") && !hasPage(pages, "index.html") {
		data, _ := os.ReadFile(filepath.Join(outDir, "README.html"))
		_ = os.WriteFile(filepath.Join(outDir, "index.html"), data, 0o644)
	}
	return len(pages), nil
}

func hasPage(pages []page, htmlRel string) bool {
	for _, p := range pages {
		if p.htmlRel == htmlRel {
			return true
		}
	}
	return false
}

// firstHeading returns the first ATX heading text, or the file name.
func firstHeading(src, rel string) string {
	for _, line := range strings.Split(src, "\n") {
		if lvl := headingLevel(line); lvl > 0 {
			return strings.TrimSpace(line[lvl:])
		}
	}
	return strings.TrimSuffix(filepath.Base(rel), ".md")
}

// navHTML builds a grouped sidebar with links relative to the current page.
func navHTML(pages []page, current string) string {
	dir := filepath.Dir(current)
	var b strings.Builder
	lastGroup := "\x00"
	for _, p := range pages {
		if p.group != lastGroup {
			if lastGroup != "\x00" {
				b.WriteString("</ul>")
			}
			label := p.group
			if label == "" {
				label = "overview"
			}
			fmt.Fprintf(&b, `<div class="nav-group">%s</div><ul>`, html.EscapeString(label))
			lastGroup = p.group
		}
		link, _ := filepath.Rel(dir, p.htmlRel)
		cls := ""
		if p.htmlRel == current {
			cls = ` class="active"`
		}
		fmt.Fprintf(&b, `<li><a href="%s"%s>%s</a></li>`, link, cls, html.EscapeString(p.title))
	}
	b.WriteString("</ul>")
	return b.String()
}

func pageHTML(title, nav, body string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s · QORM</title>
<style>
  :root { --fg:#1f2328; --muted:#656d76; --bg:#fff; --side:#f6f8fa; --accent:#2e7df6; --border:#d0d7de; --code:#f6f8fa; }
  @media (prefers-color-scheme: dark) { :root { --fg:#e6edf3; --muted:#8b949e; --bg:#0d1117; --side:#161b22; --accent:#58a6ff; --border:#30363d; --code:#161b22; } }
  * { box-sizing:border-box; }
  body { margin:0; display:flex; font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif; color:var(--fg); background:var(--bg); }
  aside { width:280px; min-width:280px; height:100vh; overflow:auto; background:var(--side); border-right:1px solid var(--border); padding:20px; position:sticky; top:0; }
  aside .brand { font-weight:800; font-size:18px; margin-bottom:16px; }
  aside .nav-group { text-transform:uppercase; font-size:11px; color:var(--muted); margin:16px 0 6px; letter-spacing:.04em; }
  aside ul { list-style:none; margin:0; padding:0; }
  aside li a { display:block; padding:4px 8px; color:var(--fg); text-decoration:none; border-radius:6px; font-size:14px; }
  aside li a:hover { background:var(--border); }
  aside li a.active { background:var(--accent); color:#fff; }
  main { flex:1; max-width:820px; padding:40px 48px; overflow:auto; height:100vh; }
  main h1,h2,h3 { line-height:1.25; }
  main h1 { border-bottom:1px solid var(--border); padding-bottom:.3em; }
  main h2 { border-bottom:1px solid var(--border); padding-bottom:.3em; margin-top:1.5em; }
  main a { color:var(--accent); }
  main code { background:var(--code); padding:.15em .35em; border-radius:5px; font-size:.9em; }
  main pre { background:var(--code); padding:14px; border-radius:8px; overflow:auto; }
  main pre code { background:none; padding:0; }
  main table { border-collapse:collapse; width:100%%; margin:1em 0; display:block; overflow-x:auto; }
  main th,td { border:1px solid var(--border); padding:6px 12px; text-align:left; }
  main th { background:var(--side); }
  main blockquote { border-left:3px solid var(--accent); margin:0; padding:.2em 1em; color:var(--muted); }
</style>
</head>
<body>
<aside><div class="brand">QORM</div>%s</aside>
<main>%s</main>
</body>
</html>
`, html.EscapeString(title), nav, body)
}
