package mcp

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// tokenApp builds a minimal app with two enforced color tokens and one
// advisory (enforce:false) numeric token.
func tokenApp() *model.App {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "text", ID: "title", Text: "Hi"},
	}}
	return &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": root},
		DesignTokens: map[string]model.DesignToken{
			"color.primary": {Type: "color", Value: "#0a84ff", Enforce: true},
			"color.bg":      {Type: "color", Value: "#f2f2f7", Enforce: true},
			"spacing.md":    {Type: "number", Value: "16", Enforce: false},
		},
	}
}

func styleOp(color any) []PatchOp {
	return []PatchOp{{Op: "setProp", Target: "title", Key: "style", Value: map[string]any{"color": color}}}
}

// TestDesignTokenRejectsNonTokenColor asserts a color style set to a value that
// is not a declared enforced token is rejected, with the allowed values listed.
func TestDesignTokenRejectsNonTokenColor(t *testing.T) {
	app := tokenApp()
	err := applyPatch(app, styleOp("#ff0000"))
	if err == nil {
		t.Fatal("expected design token violation for a non-token color")
	}
	msg := err.Error()
	if !strings.Contains(msg, "design token violation") {
		t.Errorf("error should name the violation, got %q", msg)
	}
	if !strings.Contains(msg, "#0a84ff") || !strings.Contains(msg, "#f2f2f7") {
		t.Errorf("error should list allowed tokens, got %q", msg)
	}
	// The rejected value must not have landed.
	if n := findInScenes(app, "title"); n.Style != nil {
		t.Errorf("rejected style should not be applied, got %v", n.Style)
	}
}

// TestDesignTokenAllowsTokenColor asserts a token value passes, and that
// hex-case / '#'-prefix normalization makes "#0A84FF" equal the token.
func TestDesignTokenAllowsTokenColor(t *testing.T) {
	app := tokenApp()
	if err := applyPatch(app, styleOp("#0A84FF")); err != nil {
		t.Fatalf("uppercase token color should be allowed: %v", err)
	}
	if got := findInScenes(app, "title").Style["color"]; got != "#0A84FF" {
		t.Errorf("token color should be applied verbatim, got %v", got)
	}

	app2 := tokenApp()
	if err := applyPatch(app2, styleOp("#f2f2f7")); err != nil {
		t.Fatalf("declared token color should be allowed: %v", err)
	}
}

// TestDesignTokenIgnoresNonColorKeys asserts enforcement only touches color
// style keys — a spacing/padding style is never blocked even though a numeric
// token exists (its enforce is false and it isn't a color key anyway).
func TestDesignTokenIgnoresNonColorKeys(t *testing.T) {
	app := tokenApp()
	op := []PatchOp{{Op: "setProp", Target: "title", Key: "style", Value: map[string]any{"padding": 24, "fontSize": 99}}}
	if err := applyPatch(app, op); err != nil {
		t.Fatalf("non-color style keys must not be constrained: %v", err)
	}
}

// TestDesignTokenAdvisoryNotEnforced asserts an app whose only relevant token is
// enforce:false (or whose color tokens are all advisory) imposes no constraint.
func TestDesignTokenAdvisoryNotEnforced(t *testing.T) {
	app := tokenApp()
	// Flip the two color tokens to advisory: enforcement should now be off.
	app.DesignTokens["color.primary"] = model.DesignToken{Type: "color", Value: "#0a84ff", Enforce: false}
	app.DesignTokens["color.bg"] = model.DesignToken{Type: "color", Value: "#f2f2f7", Enforce: false}
	if err := applyPatch(app, styleOp("#ff0000")); err != nil {
		t.Fatalf("advisory-only tokens must not block edits: %v", err)
	}
}

// TestNoDesignTokensUnchanged asserts an app that declares no design tokens
// behaves exactly as before — any color is accepted.
func TestNoDesignTokensUnchanged(t *testing.T) {
	app := tokenApp()
	app.DesignTokens = nil
	if err := applyPatch(app, styleOp("#123456")); err != nil {
		t.Fatalf("no design tokens => no constraint, got %v", err)
	}
}

// TestDesignTokenEnforcedAtPreview asserts the constraint fires at preview time
// too (previewPatch runs applyPatch on a clone), so the agent sees the
// violation before attempting to commit.
func TestDesignTokenEnforcedAtPreview(t *testing.T) {
	s := &Server{rt: qrt.New(tokenApp())}
	res := s.previewPatch(styleOp("#ff0000"))
	if ok, _ := res["ok"].(bool); ok {
		t.Fatalf("preview of a non-token color should report a violation, got %v", res)
	}
	if e, _ := res["error"].(string); !strings.Contains(e, "design token violation") {
		t.Errorf("preview error should name the violation, got %q", e)
	}
}
