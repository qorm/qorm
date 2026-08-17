package capability

import (
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
)

func TestStemsFromAppHardware(t *testing.T) {
	app, err := loader.LoadDir("../../examples/hardware")
	if err != nil {
		t.Fatal(err)
	}
	stems := StemsFromApp(app)
	if len(stems) == 0 {
		t.Fatal("hardware example should use capabilities")
	}
	found := false
	for _, s := range stems {
		if s == "location" || s == "camera" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected location/camera among stems, got %v", stems)
	}
}

func TestAuthorizeOpLegacyOpen(t *testing.T) {
	app := &model.App{}
	if err := AuthorizeOp(app, "nfcRead"); err != nil {
		t.Fatalf("legacy open app should allow ops: %v", err)
	}
}

func TestAuthorizeOpRequiredCaps(t *testing.T) {
	app := &model.App{
		RequiredCapabilities: []string{"location"},
	}
	if err := AuthorizeOp(app, "location"); err != nil {
		t.Fatalf("location should be allowed: %v", err)
	}
	if err := AuthorizeOp(app, "nfcRead"); err == nil {
		t.Fatal("nfcRead should be denied when not declared")
	}
}

func TestAuthorizeOpDeny(t *testing.T) {
	app := &model.App{
		RequiredCapabilities: []string{"location", "camera"},
		Capabilities: model.CapabilitiesPolicy{
			Deny: []string{"camera"},
		},
	}
	if err := AuthorizeOp(app, "location"); err != nil {
		t.Fatalf("location allowed: %v", err)
	}
	if err := AuthorizeOp(app, "recordVideo"); err == nil {
		t.Fatal("camera ops should be denied")
	}
}

func TestCapPolicyScriptLegacyNull(t *testing.T) {
	if got := CapPolicyScript(&model.App{}); got != "null" {
		t.Fatalf("legacy app want null, got %s", got)
	}
}

func TestCapPolicyScriptEnforcesJSON(t *testing.T) {
	app := &model.App{RequiredCapabilities: []string{"location"}}
	got := CapPolicyScript(app)
	if got == "null" || got == "" {
		t.Fatalf("expected JSON policy, got %q", got)
	}
	if !containsSubstring(got, `"enforce":true`) || !containsSubstring(got, `"location"`) {
		t.Fatalf("policy JSON unexpected: %s", got)
	}
}

func containsSubstring(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
