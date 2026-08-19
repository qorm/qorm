package loader

import (
	"testing"

	"github.com/qorm/platform/internal/model"
)

func TestSceneErrorBoundaryHelpers(t *testing.T) {
	if got := parseSceneErrorBoundary(nil, nil, "App", "x"); got != nil {
		t.Fatalf("nil boundary = %#v, want nil", got)
	}
	diags := []string{}
	if got := parseSceneErrorBoundary(map[string]any{}, &diags, "Scene", "main"); got != nil {
		t.Fatalf("empty boundary = %#v, want nil", got)
	}
	if len(diags) == 0 {
		t.Fatal("missing diagnostic for empty boundary")
	}
	if raw := sceneErrorBoundaryToJSON(nil); raw != nil {
		t.Fatalf("sceneErrorBoundaryToJSON(nil) = %#v, want nil", raw)
	}
	if raw := nodeErrorBoundaryToJSON(nil); raw != nil {
		t.Fatalf("nodeErrorBoundaryToJSON(nil) = %#v, want nil", raw)
	}
	if raw := nodeErrorBoundaryToJSON(&model.NodeErrorBoundary{}); raw != nil {
		t.Fatalf("nodeErrorBoundaryToJSON(empty) = %#v, want nil", raw)
	}
}
