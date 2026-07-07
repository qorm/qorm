package mdsite

import "testing"

// FuzzRenderMarkdown ensures the markdown converter never panics.
func FuzzRenderMarkdown(f *testing.F) {
	for _, s := range []string{
		"# h", "```\ncode", "| a | b |\n|---|---|\n| 1 |", "- x\n- y",
		"[l](u)", "**b* *i**", "> q", "---", "`c`", "", "|||", "#", "######### x",
		"1. a\n2. b", "\x00\x01", "{{nested}}",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		_ = RenderMarkdown(src) // must not panic
	})
}
