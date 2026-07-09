package loader

import (
	"path/filepath"
	"testing"
)

// TestDesignTokensParsed verifies the loader reads a manifest's designTokens
// into App.DesignTokens, coercing numeric values to their string form and
// carrying the enforce flag through.
func TestDesignTokensParsed(t *testing.T) {
	app, err := LoadDir(filepath.Join("..", "..", "examples", "gallery"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(app.DesignTokens) == 0 {
		t.Fatal("expected design tokens to be parsed from the gallery manifest")
	}

	primary, ok := app.DesignTokens["color.primary"]
	if !ok {
		t.Fatal("missing token color.primary")
	}
	if primary.Type != "color" || primary.Value != "#0a84ff" || !primary.Enforce {
		t.Errorf("color.primary = %+v, want {color #0a84ff true}", primary)
	}

	// Numeric token values are stored as strings; enforce:false carries through.
	spacing, ok := app.DesignTokens["spacing.md"]
	if !ok {
		t.Fatal("missing token spacing.md")
	}
	if spacing.Type != "number" || spacing.Value != "16" || spacing.Enforce {
		t.Errorf("spacing.md = %+v, want {number 16 false}", spacing)
	}
}

// TestNoDesignTokens confirms an app without the field simply has none (no
// panic, empty map/nil), preserving backward compatibility.
func TestNoDesignTokens(t *testing.T) {
	app, err := LoadDir(filepath.Join("..", "..", "examples", "counter"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(app.DesignTokens) != 0 {
		t.Errorf("counter should declare no design tokens, got %v", app.DesignTokens)
	}
}
