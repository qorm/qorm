package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// TestFeedbackWidgetBranches covers the secondary branches of the feedback
// widgets: tag removal, menu item args, circular-progress clamping, dialog
// action args, the default dismiss wiring and badge count edge cases.
func TestFeedbackWidgetBranches(t *testing.T) {
	open := map[string]any{"show": true}

	t.Run("tag-remove-button", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "tag", ID: "tg", Label: "beta", OnPress: &model.Invoke{Name: "remove"}})
		if !strings.Contains(res.HTML, "×") || !strings.Contains(res.HTML, `onclick="qorm(`) || len(res.Handlers) != 1 {
			t.Errorf("tag with onPress should render a wired remove button: %+v\n%s", res.Handlers, res.HTML)
		}
	})

	t.Run("menu-item-onpress-args", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "menu", ID: "mn", Label: "M", Props: map[string]any{"items": []any{
			map[string]any{"label": "Do", "onPress": map[string]any{"name": "do", "args": map[string]any{"id": 5}}},
			"odd-non-map",
		}}})
		if len(res.Handlers) != 1 || res.Handlers[0].Name != "do" || res.Handlers[0].Args["id"] != "5" {
			t.Errorf("menu item onPress should register with its args: %+v", res.Handlers)
		}
	})

	t.Run("circular-progress-clamps-high", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "circularprogress", ID: "cp", Props: map[string]any{"value": "2"}})
		// frac clamps to 1 -> the arc is fully drawn (offset 0)
		if !strings.Contains(res.HTML, `stroke-dashoffset="0"`) {
			t.Errorf("value>1 should clamp to a full ring (offset 0):\n%s", res.HTML)
		}
	})
	t.Run("circular-progress-clamps-low", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "circularprogress", ID: "cp2", Props: map[string]any{"value": "-1"}})
		// frac clamps to 0 -> offset equals the full circumference (empty ring)
		if !strings.Contains(res.HTML, `stroke-dashoffset="125.663706"`) {
			t.Errorf("value<0 should clamp to an empty ring (offset = circumference):\n%s", res.HTML)
		}
	})

	t.Run("dialog-action-onpress-args", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "alertdialog", ID: "ad", Props: map[string]any{
			"open":    "true",
			"actions": []any{map[string]any{"label": "OK", "onPress": map[string]any{"name": "ok", "args": map[string]any{"id": 9}}}},
		}})
		if len(res.Handlers) != 1 || res.Handlers[0].Args["id"] != "9" {
			t.Errorf("dialog action onPress should carry its args: %+v", res.Handlers)
		}
	})

	t.Run("alertdialog-cancel-default-dismiss", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "alertdialog", ID: "ad2", Props: map[string]any{
				"open":    "{{ state.show }}",
				"actions": []any{map[string]any{"label": "Cancel", "style": "cancel"}},
			}}, open)
		found := false
		for _, h := range res.Handlers {
			if h.Name == runtime.BuiltinDismiss && h.Args["path"] == "show" {
				found = true
			}
		}
		if !found {
			t.Errorf("un-wired cancel should default to __dismiss on the open path: %+v", res.Handlers)
		}
	})

	t.Run("actionsheet-dismiss-and-cancel", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "actionsheet", ID: "as", Props: map[string]any{
				"open":    "{{ state.show }}",
				"actions": []any{map[string]any{"label": "A"}},
				"cancel":  []any{map[string]any{"label": "Cancel"}},
			}}, open)
		if !strings.Contains(res.HTML, "data-dismiss-h=") {
			t.Errorf("open actionsheet should wire a backdrop dismiss:\n%s", res.HTML)
		}
		found := false
		for _, h := range res.Handlers {
			if h.Name == runtime.BuiltinDismiss && h.Args["path"] == "show" {
				found = true
			}
		}
		if !found {
			t.Errorf("actionsheet cancel should default to __dismiss: %+v", res.Handlers)
		}
	})

	t.Run("modal-dismissable-false", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "modal", ID: "md", Props: map[string]any{"open": "{{ state.show }}", "dismissable": false}, Children: textKids("b")},
			open)
		if strings.Contains(res.HTML, "data-dismiss-h=") {
			t.Errorf("dismissable:false modal must not wire a dismiss handler:\n%s", res.HTML)
		}
		for _, h := range res.Handlers {
			if h.Name == runtime.BuiltinDismiss {
				t.Errorf("dismissable:false should register no __dismiss: %+v", res.Handlers)
			}
		}
	})

	t.Run("badge-zero-hidden-unless-showzero", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "badge", ID: "bd", Label: "0", Children: textKids("x")})
		if strings.Contains(res.HTML, "top:-6px") {
			t.Errorf("a '0' count over a child should be hidden by default:\n%s", res.HTML)
		}
		res = renderWidget(t, &model.Node{Type: "badge", ID: "bd2", Label: "0", Props: map[string]any{"showZero": "true"}, Children: textKids("x")})
		if !strings.Contains(res.HTML, "top:-6px") || !strings.Contains(res.HTML, ">0</span>") {
			t.Errorf("showZero should reveal the '0' count:\n%s", res.HTML)
		}
	})
	t.Run("badge-smallsize-and-color", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "badge", ID: "bd3", Label: "5", Props: map[string]any{"smallSize": "true", "color": "blue"}, Children: textKids("x")})
		if !strings.Contains(res.HTML, "width:8px;height:8px") || !strings.Contains(res.HTML, "background:blue") {
			t.Errorf("badge smallSize/color should apply:\n%s", res.HTML)
		}
	})

	t.Run("alert-warning-variant", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "alert", ID: "al", Props: map[string]any{"variant": "warning"}, Text: "Careful"})
		if !strings.Contains(res.HTML, "var(--warning)") {
			t.Errorf("warning alert should use the warning color:\n%s", res.HTML)
		}
	})
}

// TestInputWidgetBranches covers the secondary branches of the input widgets:
// inputType, the checked prop, chip avatar/delete, dropdown hint/onChange,
// bound autocomplete options, searchbar onSelect and textFormField adornments.
func TestInputWidgetBranches(t *testing.T) {
	t.Run("input-type-prop", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "input", ID: "em", Props: map[string]any{"inputType": "email"}})
		if !strings.Contains(res.HTML, `type="email"`) {
			t.Errorf("inputType prop should set the input type:\n%s", res.HTML)
		}
	})

	t.Run("checkbox-checked-prop", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "checkbox", ID: "ck", Label: "Y", Props: map[string]any{"checked": "true"}})
		if !strings.Contains(res.HTML, " checked") {
			t.Errorf("checked prop should check an unbound checkbox:\n%s", res.HTML)
		}
	})

	t.Run("field-error-branch", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "field", ID: "fd", Props: map[string]any{"label": "Email", "error": "Invalid"}})
		if !strings.Contains(res.HTML, "Invalid") || !strings.Contains(res.HTML, "#ef4444") {
			t.Errorf("field error should render in red:\n%s", res.HTML)
		}
	})

	t.Run("chip-avatar-and-delete", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "inputchip", ID: "ch", Label: "Tag", Props: map[string]any{"avatar": "star"}, OnChange: &model.Invoke{Name: "del"}})
		if !strings.Contains(res.HTML, iconSVG("star", 15)) {
			t.Errorf("chip avatar should resolve to an svg:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, "event.stopPropagation()") || len(res.Handlers) != 1 {
			t.Errorf("inputchip onChange should wire a stop-propagation delete: %+v\n%s", res.Handlers, res.HTML)
		}
	})
	t.Run("chip-showcheck", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "choicechip", ID: "ch2", Label: "C", Props: map[string]any{"selected": "true", "showCheck": "true"}})
		if !strings.Contains(res.HTML, iconSVG("check", 12)) {
			t.Errorf("selected chip with showCheck should show a check icon:\n%s", res.HTML)
		}
	})

	t.Run("dropdown-hint-fallback", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "dropdownbutton", ID: "dd", Props: map[string]any{"hint": "Pick one",
			"options": []any{map[string]any{"value": "a", "label": "Apple"}}}})
		if !strings.Contains(res.HTML, "Pick one") {
			t.Errorf("empty dropdown should show its hint:\n%s", res.HTML)
		}
	})
	t.Run("dropdown-onchange", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "dropdownbutton", ID: "dd2", Value: "{{ state.c }}",
				Props:    map[string]any{"options": []any{map[string]any{"value": "a", "label": "Apple"}, map[string]any{"value": "b", "label": "Banana"}}},
				OnChange: &model.Invoke{Name: "set"}},
			map[string]any{"c": "a"})
		vals := map[string]bool{}
		for _, h := range res.Handlers {
			vals[h.Args["value"]] = true
		}
		if !vals["a"] || !vals["b"] {
			t.Errorf("dropdown onChange should dispatch each option value, got %v", vals)
		}
		// the selected option is visually marked
		if !strings.Contains(res.HTML, "background:var(--fill);font-weight:600;") {
			t.Errorf("dropdown should mark the selected option:\n%s", res.HTML)
		}
	})

	t.Run("autocomplete-bound-options", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "autocomplete", ID: "acp", Value: "{{ state.q }}", Props: map[string]any{"options": "{{ state.opts }}"}},
			map[string]any{"q": "", "opts": []any{"Red", "Green"}})
		if !strings.Contains(res.HTML, `<option value="Red">`) || !strings.Contains(res.HTML, `<option value="Green">`) {
			t.Errorf("autocomplete should render bound options:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, `data-state="q"`) {
			t.Errorf("autocomplete value should two-way bind:\n%s", res.HTML)
		}
	})

	t.Run("searchbar-onselect-bare-string-icon", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "searchbar", ID: "sb", Props: map[string]any{
			"onSelect": map[string]any{"name": "pick"},
			"items": []any{
				"PlainString",
				map[string]any{"label": "WithIcon", "icon": "search"},
			},
		}})
		if !strings.Contains(res.HTML, "qormSearchPick(this,") {
			t.Errorf("searchbar items should wire onSelect via qormSearchPick:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, "PlainString") {
			t.Errorf("searchbar should accept a bare string item:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, iconSVG("search", 15)) {
			t.Errorf("searchbar item icon should resolve to svg:\n%s", res.HTML)
		}
		// the picked label is carried as a handler arg
		labels := map[string]bool{}
		for _, h := range res.Handlers {
			labels[h.Args["label"]] = true
		}
		if !labels["PlainString"] || !labels["WithIcon"] {
			t.Errorf("searchbar onSelect should carry each item label, got %v", labels)
		}
	})

	t.Run("textformfield-prefix-suffix-helper", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "textformfield", ID: "tf", Props: map[string]any{
			"label": "Site", "prefix": "@", "suffix": ".com", "helper": "Your site"}, Value: "name"})
		for _, w := range []string{"@", ".com", "Your site"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("textformfield lacks %q:\n%s", w, res.HTML)
			}
		}
		// no error -> the border stays the default separator color
		if strings.Contains(res.HTML, "border:1px solid #ef4444") {
			t.Errorf("valid field should not use the error border:\n%s", res.HTML)
		}
	})
}

// TestGestureWidgetBranches covers the secondary branches of the gesture/input
// tiles: controlTile radio/checkbox + subtitle, dismissible/dragTarget onPress
// fallbacks, and swipeActions icons without a wired action.
func TestGestureWidgetBranches(t *testing.T) {
	t.Run("radiolisttile-checked", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "radiolisttile", ID: "rlt", Label: "Opt", Value: "{{ state.c }}", Props: map[string]any{"value": "x"}},
			map[string]any{"c": "x"})
		if !strings.Contains(res.HTML, `type="radio"`) || !strings.Contains(res.HTML, " checked") {
			t.Errorf("radiolisttile matching the bound value should be checked:\n%s", res.HTML)
		}
	})
	t.Run("checkboxlisttile-subtitle", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "checkboxlisttile", ID: "clt", Label: "Title", Props: map[string]any{"subtitle": "Sub"}})
		if !strings.Contains(res.HTML, "Sub") {
			t.Errorf("checkboxlisttile should render its subtitle:\n%s", res.HTML)
		}
	})

	t.Run("dismissible-onpress-fallback", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "dismissible", ID: "dm", OnPress: &model.Invoke{Name: "gone"}, Children: textKids("row")})
		if !strings.Contains(res.HTML, "qormSwipe(") || len(res.Handlers) != 1 || res.Handlers[0].Name != "gone" {
			t.Errorf("dismissible should fall back to onPress: %+v\n%s", res.Handlers, res.HTML)
		}
	})
	t.Run("dismissible-icon-fallback-text", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "dismissible", ID: "dm2", Props: map[string]any{"icon": "notarealicon"}, Children: textKids("row")})
		if !strings.Contains(res.HTML, "notarealicon") {
			t.Errorf("unknown dismissible icon should fall back to text:\n%s", res.HTML)
		}
	})

	t.Run("dragtarget-onpress-fallback", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "dragtarget", ID: "dt", OnPress: &model.Invoke{Name: "dropped"}, Children: textKids("x")})
		if !strings.Contains(res.HTML, "data-qorm-drop=") || len(res.Handlers) != 1 || res.Handlers[0].Name != "dropped" {
			t.Errorf("dragtarget should fall back to onPress for the drop handler: %+v\n%s", res.Handlers, res.HTML)
		}
	})

	t.Run("swipeactions-icon-only-action", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "swipeactions", ID: "swa", Props: map[string]any{"actions": []any{
			map[string]any{"icon": "trash", "color": "#ff3b30"}, // no name -> no onclick
		}}, Children: textKids("row")})
		if !strings.Contains(res.HTML, iconSVG("trash", 20)) {
			t.Errorf("swipeactions action icon should resolve to svg:\n%s", res.HTML)
		}
		if len(res.Handlers) != 0 {
			t.Errorf("a name-less swipe action should register no handler: %+v", res.Handlers)
		}
	})
}

// TestMediaAndIconBranches covers chart's sparkline/empty-data paths, the
// rune-safe avatar initials, icon glyph fallback and the iconSVG size default.
func TestMediaAndIconBranches(t *testing.T) {
	t.Run("chart-sparkline", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "chart", ID: "ch", Props: map[string]any{"data": []any{float64(1), float64(2)}, "chartType": "sparkline"}})
		if !strings.Contains(res.HTML, `stroke-width="1.5"`) {
			t.Errorf("sparkline should use the thinner stroke:\n%s", res.HTML)
		}
	})
	t.Run("chart-non-array-data-empty", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "chart", ID: "ch2", Props: map[string]any{"data": float64(5)}})
		if !strings.Contains(res.HTML, "<svg") || strings.Contains(res.HTML, "<rect") {
			t.Errorf("non-array chart data should render an empty svg (no bars):\n%s", res.HTML)
		}
	})

	t.Run("avatar-initials-rune-safe", func(t *testing.T) {
		// A multibyte name must truncate by rune, not byte (a byte slice would
		// corrupt the second glyph).
		res := renderWidget(t, &model.Node{Type: "avatar", ID: "av", Props: map[string]any{"name": "日本語"}})
		if !strings.Contains(res.HTML, "日本") {
			t.Errorf("avatar initials should take the first two runes intact:\n%s", res.HTML)
		}
	})

	t.Run("icon-glyph-and-text-fallback", func(t *testing.T) {
		if res := renderWidget(t, &model.Node{Type: "icon", ID: "ic", Props: map[string]any{"glyph": "star"}}); !strings.Contains(res.HTML, "<svg") {
			t.Errorf("icon should fall back to the glyph prop:\n%s", res.HTML)
		}
		if res := renderWidget(t, &model.Node{Type: "icon", ID: "ic2", Text: "heart"}); !strings.Contains(res.HTML, "<svg") {
			t.Errorf("icon should fall back to its text:\n%s", res.HTML)
		}
	})

	t.Run("iconsvg-size-default", func(t *testing.T) {
		if svg := iconSVG("check", 0); !strings.Contains(svg, `width="24"`) {
			t.Errorf("iconSVG with size<=0 should default to 24, got %q", svg)
		}
		if iconSVG("definitely-not-an-icon", 12) != "" {
			t.Errorf("iconSVG(unknown) should be empty")
		}
	})

	t.Run("motion-and-wrap-unknown-effect-fade", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "motion", ID: "mo", Props: map[string]any{"animation": "notarealeffect"}, Children: textKids("x")})
		if !strings.Contains(res.HTML, "animation:qa-fade") {
			t.Errorf("unknown motion effect should fall back to qa-fade:\n%s", res.HTML)
		}
		res = renderWidget(t, &model.Node{Type: "card", ID: "cd", Props: map[string]any{"animation": "alsobogus"}, Children: textKids("x")})
		if !strings.Contains(res.HTML, "animation:qa-fade") {
			t.Errorf("unknown universal-wrap effect should fall back to qa-fade:\n%s", res.HTML)
		}
	})
}
