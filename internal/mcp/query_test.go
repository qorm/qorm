package mcp

import (
	"path/filepath"
	"testing"

	"github.com/qorm/qorm/internal/loader"
)

func galleryRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "examples", "gallery")
}

func TestQueryByType(t *testing.T) {
	app, err := loader.LoadDir(galleryRoot(t))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	root := app.EntryRoot()

	cards := queryNodes(root, selector{Type: "card"})
	if len(cards) < 3 {
		t.Errorf("gallery should have several cards, got %d", len(cards))
	}
	for _, c := range cards {
		if c["type"] != "card" {
			t.Errorf("type filter leaked: %v", c["type"])
		}
		if _, ok := c["path"]; !ok {
			t.Error("match should carry an ancestor path")
		}
	}
}

func TestQueryByTextAndProp(t *testing.T) {
	app, _ := loader.LoadDir(galleryRoot(t))
	root := app.EntryRoot()

	// text match (case-insensitive) on the header greeting.
	if got := queryNodes(root, selector{TextContains: "hello"}); len(got) == 0 {
		t.Error("expected a node whose text contains 'hello'")
	}
	// hasProp: the select/radio nodes carry an "options" prop.
	if got := queryNodes(root, selector{HasProp: "options"}); len(got) < 2 {
		t.Errorf("expected >=2 nodes with an options prop, got %d", len(got))
	}
	// AND semantics: a button whose id contains 'docs' (the link) is a link, not button.
	if got := queryNodes(root, selector{Type: "button", IDContains: "docs"}); len(got) != 0 {
		t.Errorf("AND selector should not match, got %d", len(got))
	}
}
