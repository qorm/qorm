package qss

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

func mustParse(t *testing.T, src string) []model.StyleRule {
	t.Helper()
	rules, errs := Parse(src)
	if len(errs) > 0 {
		t.Fatalf("Parse(%q) errors: %v", src, errs)
	}
	return rules
}

// The three selector shapes: widget type, class, id.
func TestParseSelectorKinds(t *testing.T) {
	rules := mustParse(t, `
button { background: var(--primary); borderRadius: 12; fontWeight: 600 }
.accent { background: #007AFF }
#submit { height: 44; width: fill }
`)
	if len(rules) != 3 {
		t.Fatalf("got %d rules, want 3", len(rules))
	}
	want := []struct {
		kind, name string
	}{
		{model.StyleRuleType, "button"},
		{model.StyleRuleClass, "accent"},
		{model.StyleRuleID, "submit"},
	}
	for i, w := range want {
		if rules[i].Kind != w.kind || rules[i].Name != w.name {
			t.Errorf("rule %d = {%q %q}, want {%q %q}", i, rules[i].Kind, rules[i].Name, w.kind, w.name)
		}
	}
	if got := rules[0].Style["borderRadius"]; got != float64(12) {
		t.Errorf("borderRadius = %v (%T), want float64(12)", got, got)
	}
	if got := rules[0].Style["background"]; got != "var(--primary)" {
		t.Errorf("background = %v, want var(--primary)", got)
	}
	if got := rules[1].Style["background"]; got != "#007AFF" {
		t.Errorf("class background = %v, want #007AFF (a # inside a value is not a comment)", got)
	}
	if got := rules[2].Style["width"]; got != "fill" {
		t.Errorf("width = %v, want the string fill", got)
	}
	if got := rules[2].Style["height"]; got != float64(44) {
		t.Errorf("height = %v (%T), want float64(44)", got, got)
	}
}

// A rule may span lines; declarations separate on `;` OR newline.
func TestParseMultilineRule(t *testing.T) {
	rules := mustParse(t, `button {
	background: var(--primary)
	borderRadius: 12;
	fontWeight: 600
}
`)
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if len(rules[0].Style) != 3 {
		t.Fatalf("style keys = %v, want 3 entries", rules[0].Style)
	}
}

// Comments: a full-line `#`, a `#` inside a rule body, and the `#submit` /
// `# 注释` disambiguation (an id selector is `#` + identifier; anything else
// is a comment). Inside a VALUE a `#` is literal text (hex colors).
func TestParseComments(t *testing.T) {
	rules := mustParse(t, `# leading comment
button { # comment after the brace
	background: #007AFF
	# comment line inside a body
}
#idRule { height: 4 }
# another comment
`)
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2 (%v)", len(rules), rules)
	}
	if rules[1].Kind != model.StyleRuleID || rules[1].Name != "idRule" {
		t.Fatalf("rule 1 = {%q %q}, want {id idRule}", rules[1].Kind, rules[1].Name)
	}
	if got := rules[0].Style["background"]; got != "#007AFF" {
		t.Fatalf("background = %q, want #007AFF", got)
	}
}

// A binding runs to its terminator verbatim: `;`, `}` and newlines inside
// {{ }} or inside quotes belong to the value.
func TestParseBindingValue(t *testing.T) {
	rules := mustParse(t, `.cell { width: {{ state.cond ? 'a;b' : 14 }}; background: {{ at(state.colors, item) }} }
`)
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if got := rules[0].Style["width"]; got != "{{ state.cond ? 'a;b' : 14 }}" {
		t.Fatalf("width = %q, want the binding verbatim", got)
	}
	if got := rules[0].Style["background"]; got != "{{ at(state.colors, item) }}" {
		t.Fatalf("background = %q, want the binding verbatim", got)
	}
}

// Value typing: bare numbers become float64 (JSON parity), quoted strings are
// unquoted, everything else stays the raw string.
func TestParseValueTyping(t *testing.T) {
	rules := mustParse(t, `.x { fontSize: 16; opacity: 0.5; width: "fill"; textAlign: center }
`)
	st := rules[0].Style
	if got := st["fontSize"]; got != float64(16) {
		t.Errorf("fontSize = %v (%T), want float64(16)", got, got)
	}
	if got := st["opacity"]; got != float64(0.5) {
		t.Errorf("opacity = %v (%T), want float64(0.5)", got, got)
	}
	if got := st["width"]; got != "fill" {
		t.Errorf("width = %q, want fill (quotes stripped)", got)
	}
	if got := st["textAlign"]; got != "center" {
		t.Errorf("textAlign = %q, want center", got)
	}
}

// Syntax errors are diagnostics carrying the 1-based source line; the parser
// recovers per rule, so a good rule after a bad one still loads.
func TestParseDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		src  string
		line int
	}{
		{"selector without brace", "button\n", 1},
		{"key without colon", "button { background var(--primary) }", 1},
		{"missing value", "button { background: }", 1},
		{"unterminated rule", "button {\n\tbackground: red;\n\tfontSize: 3\n", 1},
		{"bad selector char", "{ background: red }", 1},
		{"empty selector name", ". { background: red }", 1},
		{"nested object value", "button { margin: {top: 10} }", 1},
		{"error line is the declaration's line", "button { background: red }\n\n.accent {\nfontSize 3\n}\n", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse(tc.src)
			if len(errs) == 0 {
				t.Fatalf("Parse(%q) reported no error", tc.src)
			}
			if errs[0].Line != tc.line {
				t.Fatalf("error line = %d, want %d (errs: %v)", errs[0].Line, tc.line, errs)
			}
		})
	}
}

// Recovery: one malformed rule must not swallow the following good ones.
func TestParseRecoversAfterBadRule(t *testing.T) {
	rules, errs := Parse("button { background red }\n.accent { height: 4 }\n")
	if len(errs) == 0 {
		t.Fatal("expected an error for the malformed rule")
	}
	if len(rules) != 1 || rules[0].Name != "accent" {
		t.Fatalf("rules = %v, want only .accent to survive", rules)
	}
}
