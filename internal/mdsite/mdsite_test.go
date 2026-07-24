package mdsite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderMarkdownSubset(t *testing.T) {
	md := "# Title\n\nHello **bold** and `code` and [link](https://x).\n\n" +
		"![alt](img/x.png)\n\n" +
		"- one\n- two\n\n" +
		"| A | B |\n|---|---|\n| 1 | 2 |\n\n" +
		"```\ncode block\n```\n"
	h := RenderMarkdown(md)
	for _, want := range []string{
		`<h1 id="title">Title</h1>`,
		"<strong>bold</strong>",
		"<code>code</code>",
		`<a href="https://x">link</a>`,
		`<img src="img/x.png" alt="alt" loading="lazy" decoding="async">`,
		"<ul>", "<li>one</li>",
		"<table>", "<th>A</th>", "<td>1</td>",
		"<pre><code>code block</code></pre>",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q in:\n%s", want, h)
		}
	}
	// Image syntax must render an <img>, not fall through to the link rule
	// (which used to leave a broken "!<a ...>").
	if strings.Contains(h, "!<a") {
		t.Error("image syntax rendered as a broken !<a> instead of <img>")
	}
	// HTML in prose must be escaped.
	if strings.Contains(RenderMarkdown("a <script> tag"), "<script>") {
		t.Error("raw HTML must be escaped")
	}
}

func TestBuildSite(t *testing.T) {
	docs := t.TempDir()
	os.MkdirAll(filepath.Join(docs, "spec"), 0o755)
	os.MkdirAll(filepath.Join(docs, "img"), 0o755)
	os.WriteFile(filepath.Join(docs, "index.md"), []byte("# Home\n\nWelcome.\n\n![pic](img/pic.png)\n"), 0o644)
	os.WriteFile(filepath.Join(docs, "spec", "ir.md"), []byte("# IR Spec\n\nDetails."), 0o644)
	os.WriteFile(filepath.Join(docs, "img", "pic.png"), []byte("PNGDATA"), 0o644)

	out := t.TempDir()
	n, err := BuildSite(docs, out, "docs")
	if err != nil {
		t.Fatalf("BuildSite: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 pages, got %d", n)
	}
	idx, _ := os.ReadFile(filepath.Join(out, "index.html"))
	if !strings.Contains(string(idx), "IR Spec") {
		t.Error("index nav should link to the spec page")
	}
	if !strings.Contains(string(idx), `<img src="img/pic.png"`) {
		t.Error("index should embed the image referenced by its markdown")
	}
	// Non-markdown files are copied through, preserving relative paths.
	pic, err := os.ReadFile(filepath.Join(out, "img", "pic.png"))
	if err != nil || string(pic) != "PNGDATA" {
		t.Errorf("static asset not copied verbatim: err=%v content=%q", err, pic)
	}
	// Markdown sources themselves are never copied to the output.
	if _, err := os.Stat(filepath.Join(out, "index.md")); !os.IsNotExist(err) {
		t.Error("markdown sources must not be copied to the output")
	}
	spec, err := os.ReadFile(filepath.Join(out, "spec", "ir.html"))
	if err != nil {
		t.Fatalf("spec page not written: %v", err)
	}
	// nav link from a subdir page must be relative (../index.html).
	if !strings.Contains(string(spec), "../index.html") {
		t.Error("subdir page should link up to root with a relative path")
	}
}

func TestFrontMatterMeta(t *testing.T) {
	docs := t.TempDir()
	os.WriteFile(filepath.Join(docs, "page.md"), []byte(
		"---\ntitle: Custom Title\ndescription: A hand-written description.\nog-image: img/og.png\n---\n# Heading\n\nBody text.\n"), 0o644)
	os.WriteFile(filepath.Join(docs, "plain.md"), []byte(
		"# Plain\n\nFirst paragraph here.\n\nSecond one.\n"), 0o644)

	out := t.TempDir()
	if _, err := BuildSite(docs, out, "docs"); err != nil {
		t.Fatalf("BuildSite: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(out, "page.html"))
	if err != nil {
		t.Fatalf("page.html not written: %v", err)
	}
	for _, want := range []string{
		"<title>Custom Title · QORM</title>",
		`<meta name="description" content="A hand-written description.">`,
		`<meta property="og:image" content="img/og.png">`,
		`<meta name="twitter:image" content="img/og.png">`,
		`<link rel="canonical" href="https://qorm.com/docs/page.html">`,
		`<link rel="alternate" hreflang="x-default" href="https://qorm.com/docs/page.html">`,
		`application/ld+json`,
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("page.html missing %q", want)
		}
	}
	// The front-matter block must never leak into the rendered body.
	if strings.Contains(string(page), "<hr>") || strings.Contains(string(page), "og-image:") {
		t.Error("front matter leaked into the rendered body")
	}
	// Without front matter, the first paragraph becomes the description.
	plain, _ := os.ReadFile(filepath.Join(out, "plain.html"))
	if !strings.Contains(string(plain), `<meta name="description" content="First paragraph here.">`) {
		t.Error("plain.html should fall back to the first paragraph as description")
	}
}
