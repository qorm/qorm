package policy

import (
	"encoding/json"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

func TestEffectiveLevelLegacyFull(t *testing.T) {
	if got := EffectiveLevel(model.AgentPolicy{}); got != LevelFull {
		t.Fatalf("empty policy want full, got %q", got)
	}
}

func TestEffectiveLevelPartialPolicyDefaultsPreview(t *testing.T) {
	p := model.AgentPolicy{Tools: map[string]bool{"qorm_dispatch": false}}
	if got := EffectiveLevel(p); got != LevelPreviewOnly {
		t.Fatalf("partial policy without level want preview-only, got %q", got)
	}
}

func TestLevelPreviewOnlyDeniesDispatch(t *testing.T) {
	p := model.AgentPolicy{Level: LevelPreviewOnly}
	if err := AuthorizeTool(p, "qorm_dispatch", nil); err == nil {
		t.Fatal("preview-only should deny dispatch")
	}
	if err := AuthorizeTool(p, "qorm_preview_patch", nil); err != nil {
		t.Fatalf("preview-only should allow preview_patch, got %v", err)
	}
}

func TestLevelOperateAllowsDispatch(t *testing.T) {
	p := model.AgentPolicy{Level: LevelOperate}
	if err := AuthorizeTool(p, "qorm_dispatch", nil); err != nil {
		t.Fatalf("operate should allow dispatch: %v", err)
	}
	if err := AuthorizeTool(p, "qorm_apply_patch", nil); err == nil {
		t.Fatal("operate should deny apply_patch")
	}
}

func TestLevelDesignDeniesWindow(t *testing.T) {
	p := model.AgentPolicy{Level: LevelDesign}
	if err := AuthorizeTool(p, "qorm_window", nil); err == nil {
		t.Fatal("design should deny qorm_window")
	}
	if err := AuthorizeTool(p, "qorm_apply_patch", nil); err != nil {
		t.Fatalf("design should allow apply_patch: %v", err)
	}
}

func TestToolOverrideAllow(t *testing.T) {
	p := model.AgentPolicy{
		Level: LevelPreviewOnly,
		Tools: map[string]bool{"qorm_dispatch": true},
	}
	if err := AuthorizeTool(p, "qorm_dispatch", nil); err != nil {
		t.Fatalf("override should allow dispatch: %v", err)
	}
}

func TestToolOverrideDeny(t *testing.T) {
	p := model.AgentPolicy{
		Level: LevelFull,
		Tools: map[string]bool{"qorm_window": false},
	}
	if err := AuthorizeTool(p, "qorm_window", nil); err == nil {
		t.Fatal("override false should deny window")
	}
}

func TestHostCallEvalDenied(t *testing.T) {
	p := model.AgentPolicy{
		Level: LevelFull,
		HostCall: model.HostCallPolicy{
			Allowed: true,
			Ops:     []string{"move"},
		},
	}
	args, _ := json.Marshal(map[string]string{"op": "eval", "js": "1+1"})
	if err := AuthorizeTool(p, "qorm_window", args); err == nil {
		t.Fatal("eval should be denied when not in hostCall.ops")
	}
}

func TestHostCallEvalAllowed(t *testing.T) {
	p := model.AgentPolicy{
		Level: LevelFull,
		HostCall: model.HostCallPolicy{
			Allowed: true,
			Ops:     []string{"eval"},
		},
	}
	args, _ := json.Marshal(map[string]string{"op": "eval", "js": "1+1"})
	if err := AuthorizeTool(p, "qorm_window", args); err != nil {
		t.Fatalf("eval should be allowed: %v", err)
	}
}

func TestManifestReadOnlyDeniesPreviewPatch(t *testing.T) {
	p := model.AgentPolicy{Level: LevelReadOnly}
	if err := AuthorizeTool(p, "qorm_preview_patch", nil); err == nil {
		t.Fatal("manifest read-only should deny preview_patch")
	}
}
