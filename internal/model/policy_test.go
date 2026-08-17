package model

import "testing"

func TestAgentPolicyHasPolicy(t *testing.T) {
	if (AgentPolicy{}).HasPolicy() {
		t.Fatal("empty policy should not HasPolicy")
	}
	if !(AgentPolicy{Level: "operate"}).HasPolicy() {
		t.Fatal("level should count as policy")
	}
	if !(AgentPolicy{Tools: map[string]bool{"qorm_dispatch": true}}).HasPolicy() {
		t.Fatal("tools map should count as policy")
	}
}

func TestCapabilitiesPolicyHasPolicy(t *testing.T) {
	if (CapabilitiesPolicy{}).HasPolicy() {
		t.Fatal("empty capabilities should not HasPolicy")
	}
	if !(CapabilitiesPolicy{Mode: "used-only"}).HasPolicy() {
		t.Fatal("mode should count as policy")
	}
}

func TestAppBreakpoints(t *testing.T) {
	if got := (*App)(nil).Breakpoints(); got["md"] != DefaultBreakpoints["md"] {
		t.Fatalf("nil app should use defaults: %#v", got)
	}
	app := &App{}
	if got := app.Breakpoints(); got["lg"] != 1024 {
		t.Fatalf("empty map should use defaults: %#v", got)
	}
	app.BreakpointWidths = map[string]int{"md": 900}
	if got := app.Breakpoints(); got["md"] != 900 || len(got) != 1 {
		t.Fatalf("custom breakpoints replace defaults: %#v", got)
	}
}
