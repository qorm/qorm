package mdsite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderMarkdownSubset(t *testing.T) {
	md := "# Title\n\nHello **bold** and `code` and [link](https://x).\n\n" +
		"- one\n- two\n\n" +
		"| A | B |\n|---|---|\n| 1 | 2 |\n\n" +
		"```\ncode block\n```\n"
	h := RenderMarkdown(md)
	for _, want := range []string{
		`<h1 id="title">Title</h1>`,
		"<strong>bold</strong>",
		"<code>code</code>",
		`<a href="https://x">link</a>`,
		"<ul>", "<li>one</li>",
		"<table>", "<th>A</th>", "<td>1</td>",
		"<pre><code>code block</code></pre>",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q in:\n%s", want, h)
		}
	}
	// HTML in prose must be escaped.
	if strings.Contains(RenderMarkdown("a <script> tag"), "<script>") {
		t.Error("raw HTML must be escaped")
	}
}

func TestBuildSite(t *testing.T) {
	docs := t.TempDir()
	os.MkdirAll(filepath.Join(docs, "spec"), 0o755)
	os.WriteFile(filepath.Join(docs, "index.md"), []byte("# Home\n\nWelcome."), 0o644)
	os.WriteFile(filepath.Join(docs, "spec", "ir.md"), []byte("# IR Spec\n\nDetails."), 0o644)

	out := t.TempDir()
	n, err := BuildSite(docs, out)
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
	spec, err := os.ReadFile(filepath.Join(out, "spec", "ir.html"))
	if err != nil {
		t.Fatalf("spec page not written: %v", err)
	}
	// nav link from a subdir page must be relative (../index.html).
	if !strings.Contains(string(spec), "../index.html") {
		t.Error("subdir page should link up to root with a relative path")
	}
}
