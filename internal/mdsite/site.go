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
	group   string // grouping label within the language ("" for root)
	lang    string // "en" or "zh"
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
		// A page under zh/ is Chinese; group it by its path within that language
		// so the zh sidebar mirrors the en one instead of one flat "zh" bucket.
		sep := string(filepath.Separator)
		lang, navRel := "en", rel
		if rel == "zh" || strings.HasPrefix(rel, "zh"+sep) {
			lang, navRel = "zh", strings.TrimPrefix(rel, "zh"+sep)
		}
		group := ""
		if parts := strings.SplitN(navRel, sep, 2); len(parts) == 2 {
			group = parts[0]
		}
		pages = append(pages, page{htmlRel: htmlRel, title: firstHeading(string(data), rel), group: group, lang: lang})
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
		nav := navHTML(pagesForLang(pages, p.lang), p.htmlRel)
		out := filepath.Join(outDir, p.htmlRel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(out, []byte(pageHTML(p.title, langSwitchHTML(p, pages), nav, body)), 0o644); err != nil {
			return 0, err
		}
	}

	// index.html: reuse an existing index/README page, else a generated landing.
	if !hasPage(pages, "index.html") && !hasPage(pages, "README.html") {
		nav := navHTML(pagesForLang(pages, "en"), "index.html")
		landing := "<h1>QORM Documentation</h1>\n<p>Select a page from the sidebar.</p>\n"
		if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte(pageHTML("QORM Docs", "", nav, landing)), 0o644); err != nil {
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

// pagesForLang returns only the pages in the given language, so each language's
// sidebar lists just its own pages.
func pagesForLang(pages []page, lang string) []page {
	var out []page
	for _, p := range pages {
		if p.lang == lang {
			out = append(out, p)
		}
	}
	return out
}

// langSwitchHTML builds the EN / 中文 toggle for a page, linking each language to
// this page's counterpart when it exists (zh/foo.html <-> foo.html).
func langSwitchHTML(p page, pages []page) string {
	sep := string(filepath.Separator)
	enRel, zhRel := p.htmlRel, "zh"+sep+p.htmlRel
	if p.lang == "zh" {
		enRel = strings.TrimPrefix(p.htmlRel, "zh"+sep)
		zhRel = p.htmlRel
	}
	dir := filepath.Dir(p.htmlRel)
	item := func(label, rel, code string) string {
		if p.lang == code {
			return `<span class="on">` + label + `</span>`
		}
		if hasPage(pages, rel) {
			link, _ := filepath.Rel(dir, rel)
			return `<a href="` + filepath.ToSlash(link) + `">` + label + `</a>`
		}
		return `<span class="off">` + label + `</span>`
	}
	return `<div class="lang">` + item("EN", enRel, "en") + item("中文", zhRel, "zh") + `</div>`
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

func pageHTML(title, langSwitch, nav, body string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s · QORM docs</title>
<link rel="icon" href="/assets/logo.svg">
<style>
  :root{ --ground:#f4f4f6; --surface:#fff; --raise:#fbfbfd; --ink:#15161a; --muted:#63666f; --faint:#8b8f98; --line:#e7e8ec; --accent:#0a84ff; --accent-ink:#0a6ed1; }
  @media (prefers-color-scheme:dark){ :root{ --ground:#0b0c0f; --surface:#15171c; --raise:#1b1e24; --ink:#eef0f3; --muted:#9a9ea8; --faint:#71757e; --line:#25272e; --accent:#0a84ff; --accent-ink:#4aa8ff; } }
  :root[data-theme="light"]{ --ground:#f4f4f6; --surface:#fff; --raise:#fbfbfd; --ink:#15161a; --muted:#63666f; --faint:#8b8f98; --line:#e7e8ec; --accent:#0a84ff; --accent-ink:#0a6ed1; }
  :root[data-theme="dark"]{ --ground:#0b0c0f; --surface:#15171c; --raise:#1b1e24; --ink:#eef0f3; --muted:#9a9ea8; --faint:#71757e; --line:#25272e; --accent:#0a84ff; --accent-ink:#4aa8ff; }
  *{box-sizing:border-box}
  body{margin:0;font-family:-apple-system,BlinkMacSystemFont,"SF Pro Text","Segoe UI",system-ui,sans-serif;color:var(--ink);background:var(--ground);line-height:1.62;letter-spacing:-.011em;-webkit-font-smoothing:antialiased}
  a{color:var(--accent-ink);text-decoration:none} a:hover{text-decoration:underline}
  code,pre{font-family:ui-monospace,"SF Mono",Menlo,Consolas,monospace}
  header.top{position:sticky;top:0;z-index:30;height:56px;display:flex;align-items:center;gap:14px;padding:0 22px;background:color-mix(in srgb,var(--ground) 82%%,transparent);backdrop-filter:saturate(180%%) blur(20px);-webkit-backdrop-filter:saturate(180%%) blur(20px);border-bottom:.5px solid var(--line)}
  header.top .brand{display:flex;align-items:center;gap:9px;font-weight:700;font-size:16px;letter-spacing:-.02em;color:var(--ink)}
  header.top .brand img{width:24px;height:24px}
  @media (prefers-color-scheme:dark){ header.top .brand img{filter:invert(1) brightness(1.7)} }
  :root[data-theme="dark"] header.top .brand img{filter:invert(1) brightness(1.7)}
  :root[data-theme="light"] header.top .brand img{filter:none}
  header.top .doc{color:var(--faint);font-weight:600;font-size:15px}
  header.top .sp{flex:1}
  header.top a.tl{color:var(--muted);font-size:14px;font-weight:500} header.top a.tl:hover{color:var(--ink);text-decoration:none}
  .tbtn{width:32px;height:32px;border-radius:8px;border:.5px solid var(--line);background:var(--surface);color:var(--muted);cursor:pointer;display:inline-flex;align-items:center;justify-content:center}
  .tbtn:hover{color:var(--ink)} .tbtn svg{width:16px;height:16px}
  header.top .lang{display:inline-flex;align-items:center;border:.5px solid var(--line);border-radius:8px;overflow:hidden;font-size:13px;font-weight:600}
  header.top .lang a,header.top .lang span{padding:5px 10px;color:var(--muted)}
  header.top .lang a:hover{color:var(--ink);text-decoration:none;background:var(--surface)}
  header.top .lang .on{background:var(--accent);color:#fff}
  header.top .lang .off{color:var(--faint);opacity:.55}
  .shell{display:flex;max-width:1180px;margin:0 auto}
  aside{width:262px;min-width:262px;height:calc(100vh - 56px);overflow:auto;position:sticky;top:56px;padding:22px 8px 48px 22px}
  aside .nav-group{text-transform:uppercase;font-size:11px;font-weight:700;color:var(--faint);margin:20px 0 6px;letter-spacing:.05em}
  aside ul{list-style:none;margin:0;padding:0}
  aside li a{display:block;padding:6px 12px;color:var(--muted);border-radius:8px;font-size:14px;font-weight:500}
  aside li a:hover{background:var(--surface);color:var(--ink);text-decoration:none}
  aside li a.active{background:color-mix(in srgb,var(--accent) 14%%,transparent);color:var(--accent-ink);font-weight:600}
  main{flex:1;min-width:0;max-width:800px;padding:38px 46px 90px}
  main>*:first-child{margin-top:0}
  main h1{font-size:33px;font-weight:800;letter-spacing:-.03em;margin:0 0 .5em}
  main h2{font-size:23px;font-weight:700;letter-spacing:-.02em;margin:1.7em 0 .5em;padding-bottom:.3em;border-bottom:.5px solid var(--line)}
  main h3{font-size:18px;font-weight:700;margin:1.5em 0 .4em}
  main code{background:var(--raise);border:.5px solid var(--line);padding:.12em .4em;border-radius:6px;font-size:.88em}
  main pre{background:var(--raise);border:.5px solid var(--line);padding:14px 16px;border-radius:12px;overflow:auto;font-size:13.5px;line-height:1.7}
  main pre code{background:none;border:none;padding:0}
  main table{border-collapse:collapse;width:100%%;margin:1em 0;display:block;overflow-x:auto;font-size:14px}
  main th,main td{border:.5px solid var(--line);padding:8px 13px;text-align:left}
  main th{background:var(--surface);font-weight:600}
  main blockquote{border-left:3px solid var(--accent);margin:1.1em 0;padding:.5em 1.1em;color:var(--muted);background:var(--surface);border-radius:0 10px 10px 0}
  main img{max-width:100%%}
  @media (max-width:800px){ aside{display:none} main{padding:26px 22px} }
</style>
</head>
<body>
<header class="top">
  <a class="brand" href="/"><img src="/assets/logo.svg" alt="QORM"><span>QORM</span></a>
  <span class="doc">docs</span>
  <span class="sp"></span>
  <a class="tl" href="/">Home</a>
  <a class="tl" href="https://github.com/qorm/qorm">GitHub</a>
  %s
  <button class="tbtn" id="theme" aria-label="Theme"></button>
</header>
<div class="shell">
  <aside>%s</aside>
  <main>%s</main>
</div>
<script>
  (function(){var r=document.documentElement,b=document.getElementById('theme');
   var sun='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M2 12h2m16 0h2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M19.1 4.9l-1.4 1.4M6.3 17.7l-1.4 1.4"/></svg>';
   var moon='<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>';
   function c(){return r.getAttribute('data-theme')||(matchMedia('(prefers-color-scheme:dark)').matches?'dark':'light');}
   function p(){b.innerHTML=c()==='dark'?sun:moon;} p();
   b.onclick=function(){r.setAttribute('data-theme',c()==='dark'?'light':'dark');p();};})();
</script>
</body>
</html>
`, html.EscapeString(title), langSwitch, nav, body)
}
