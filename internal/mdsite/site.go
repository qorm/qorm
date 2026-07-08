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
	navRel  string // language-neutral path, e.g. "tutorials/first-scene.html"
	title   string
	group   string // grouping label within the language ("" for root)
	lang    string // "en" or "zh"
}

// BuildSite renders every markdown file under docsDir into a linked static HTML
// site under outDir, and returns the number of pages written. siteName is the
// short label shown next to the logo (e.g. "docs" or "api").
func BuildSite(docsDir, outDir, siteName string) (int, error) {
	if siteName == "" {
		siteName = "docs"
	}
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
		navHTML := filepath.ToSlash(strings.TrimSuffix(navRel, ".md") + ".html")
		pages = append(pages, page{htmlRel: htmlRel, navRel: navHTML, title: firstHeading(string(data), rel), group: group, lang: lang})
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
		gi, gj := order(groupOrder, pages[i].group), order(groupOrder, pages[j].group)
		if gi != gj {
			return gi < gj
		}
		if pages[i].group != pages[j].group { // unordered groups: alphabetical
			return pages[i].group < pages[j].group
		}
		pi, pj := order(pageOrder, pages[i].navRel), order(pageOrder, pages[j].navRel)
		if pi != pj {
			return pi < pj
		}
		if pages[i].navRel != pages[j].navRel {
			return pages[i].navRel < pages[j].navRel
		}
		return pages[i].htmlRel < pages[j].htmlRel
	})

	for _, p := range pages {
		data, _ := os.ReadFile(srcOf[p.htmlRel])
		body := RenderMarkdown(string(data))
		nav := sidebarHTML(pages, p.lang, p.htmlRel)
		out := filepath.Join(outDir, p.htmlRel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(out, []byte(pageHTML(p.title, p.lang, siteName, langSwitchHTML(p, pages), nav, body)), 0o644); err != nil {
			return 0, err
		}
	}

	// Every directory that has a README.html gets an index.html copy, so a bare
	// directory URL (/docs/, /docs/zh/, /api/zh/, …) resolves instead of 404ing.
	for _, p := range pages {
		if filepath.Base(p.htmlRel) != "README.html" {
			continue
		}
		idx := filepath.Join(filepath.Dir(p.htmlRel), "index.html")
		if hasPage(pages, filepath.ToSlash(idx)) {
			continue // a real index page already owns this slot
		}
		if data, err := os.ReadFile(filepath.Join(outDir, p.htmlRel)); err == nil {
			_ = os.WriteFile(filepath.Join(outDir, idx), data, 0o644)
		}
	}
	// If the root has neither an index nor a README, drop a minimal landing there.
	if !hasPage(pages, "index.html") && !hasPage(pages, "README.html") {
		nav := sidebarHTML(pages, "en", "index.html")
		landing := "<h1>QORM Documentation</h1>\n<p>Select a page from the sidebar.</p>\n"
		if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte(pageHTML("QORM", "en", siteName, "", nav, landing)), 0o644); err != nil {
			return 0, err
		}
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

// langSwitchHTML builds the EN / 中文 toggle for a page, linking each language to
// this page's counterpart when it exists (zh/foo.html <-> foo.html). The
// data-lang attributes let the client remember the reader's language choice.
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
			return `<span class="on" data-lang="` + code + `">` + label + `</span>`
		}
		if hasPage(pages, rel) {
			link, _ := filepath.Rel(dir, rel)
			return `<a href="` + filepath.ToSlash(link) + `" data-lang="` + code + `">` + label + `</a>`
		}
		return `<span class="off" data-lang="` + code + `">` + label + `</span>`
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

// groupOrder and pageOrder give the sidebar an easy-to-hard reading order
// instead of alphabetical. Anything unlisted sorts after the listed ones
// (alphabetically), so adding a page never breaks the build — it just appends.
var groupOrder = map[string]int{
	"": 0, "tutorials": 1, "examples": 2, "reference": 3,
	"platforms": 4, "agent": 5, "security": 6,
}

var pageOrder = orderList(
	// overview (docs root) — gentlest first
	"README.html", "project-structure.html", "build-with-ai.html",
	"collaboration.html", "verification.html",
	// api root — the reference, in learning order
	"props.html", "widgets.html", "actions.html", "gestures.html",
	"animation.html", "navigation.html", "http-api.html", "go-api.html",
	// tutorials — the guided path
	"tutorials/getting-started.html", "tutorials/first-scene.html",
	"tutorials/first-action.html", "tutorials/first-component.html",
	"tutorials/first-platform-pack.html",
	// examples — simple to rich
	"examples/counter.html", "examples/todo.html", "examples/login.html",
	"examples/dashboard.html",
	// platforms — common targets to advanced
	"platforms/web.html", "platforms/mobile.html", "platforms/desktop.html",
	"platforms/miniapp.html", "platforms/native-middlelayer.html",
	"platforms/capabilities.html", "platforms/support-matrix.html",
	// agent — basics to per-agent packs
	"agent/permissions.html", "agent/mcp-tools.html", "agent/skills.html",
	"agent/claude-pack.html", "agent/codex-pack.html", "agent/openclaw-pack.html",
	// security — model to mechanics
	"security/security-model.html", "security/permission-model.html",
	"security/bundle-signing.html",
)

func orderList(keys ...string) map[string]int {
	m := make(map[string]int, len(keys))
	for i, k := range keys {
		m[k] = i
	}
	return m
}

// order returns the weight for key, or a large value (after all listed keys).
func order(m map[string]int, key string) int {
	if v, ok := m[key]; ok {
		return v
	}
	return 1 << 20
}

// groupLabels localises the sidebar section headings (derived from directory
// names) so the Chinese sidebar reads in Chinese too.
var groupLabels = map[string]map[string]string{
	"":          {"en": "overview", "zh": "概览"},
	"reference": {"en": "reference", "zh": "参考"},
	"platforms": {"en": "platforms", "zh": "平台"},
	"agent":     {"en": "agent", "zh": "智能体"},
	"security":  {"en": "security", "zh": "安全"},
	"examples":  {"en": "examples", "zh": "示例"},
	"tutorials": {"en": "tutorials", "zh": "教程"},
}

func groupLabel(group, lang string) string {
	if m, ok := groupLabels[group]; ok {
		if v := m[lang]; v != "" {
			return v
		}
		if v := m["en"]; v != "" {
			return v
		}
	}
	if group == "" {
		return "overview"
	}
	return group
}

// sidebarHTML builds a grouped sidebar for the given language. The structure is
// driven by the English page set (the superset), so no page ever vanishes from
// the sidebar: each row links to the page's translation when one exists, and
// otherwise falls back to the English page, tagged "EN". This is what keeps
// language navigation from silently dropping the reader back into English.
func sidebarHTML(all []page, lang, current string) string {
	sep := string(filepath.Separator)
	zhByRel := map[string]page{} // en-relative html path -> zh page
	var base []page              // english pages define the sidebar order
	for _, p := range all {
		if p.lang == "zh" {
			zhByRel[strings.TrimPrefix(p.htmlRel, "zh"+sep)] = p
		} else {
			base = append(base, p)
		}
	}
	dir := filepath.Dir(current)
	var b strings.Builder
	lastGroup := "\x00"
	for _, p := range base {
		if p.group != lastGroup {
			if lastGroup != "\x00" {
				b.WriteString("</ul>")
			}
			fmt.Fprintf(&b, `<div class="nav-group">%s</div><ul>`, html.EscapeString(groupLabel(p.group, lang)))
			lastGroup = p.group
		}
		target, tag := p, ""
		if lang == "zh" {
			if z, ok := zhByRel[p.htmlRel]; ok {
				target = z
			} else {
				tag = ` <span class="entag">EN</span>` // no translation yet
			}
		}
		link, _ := filepath.Rel(dir, target.htmlRel)
		cls := ""
		if target.htmlRel == current {
			cls = ` class="active"`
		}
		fmt.Fprintf(&b, `<li><a href="%s"%s>%s%s</a></li>`, filepath.ToSlash(link), cls, html.EscapeString(target.title), tag)
	}
	b.WriteString("</ul>")
	return b.String()
}

func pageHTML(title, lang, siteName, langSwitch, nav, body string) string {
	if lang == "" {
		lang = "en"
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="%s">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s · QORM</title>
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
  aside li a .entag{font-size:9.5px;font-weight:700;letter-spacing:.04em;color:var(--faint);border:.5px solid var(--line);border-radius:4px;padding:0 4px;margin-left:6px;vertical-align:1px}
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
  <span class="doc">%s</span>
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
  (function(){/* remember the reader's language across pages + both sites */
   try{var box=document.querySelector('header .lang');if(!box)return;
     var cur=document.documentElement.lang;
     box.querySelectorAll('[data-lang]').forEach(function(el){
       if(el.tagName==='A')el.addEventListener('click',function(){try{localStorage.setItem('qorm-lang',el.getAttribute('data-lang'));}catch(e){}});});
     var pref;try{pref=localStorage.getItem('qorm-lang');}catch(e){}
     if(pref&&pref!==cur){var alt=box.querySelector('a[data-lang="'+pref+'"]');if(alt)location.replace(alt.getAttribute('href'));}
   }catch(e){}})();
</script>
</body>
</html>
`, lang, html.EscapeString(title), siteName, langSwitch, nav, body)
}
