package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// TestWidgetBranchesAndProps drives the secondary branches of widgets that the
// main table tests leave uncovered: prop-driven variants, fallbacks and the
// onChange wiring of the wheel/destination widgets.
func TestWidgetBranchesAndProps(t *testing.T) {
	open := map[string]any{"show": true}

	t.Run("appbar-leading-icon", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "appbar", ID: "ab", Label: "T", Props: map[string]any{"leading": "star"}})
		if !strings.Contains(res.HTML, iconSVG("star", 20)) {
			t.Errorf("appbar leading icon should resolve to svg:\n%s", res.HTML)
		}
	})
	t.Run("appbar-leading-text", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "appbar", ID: "ab2", Label: "T", Props: map[string]any{"leading": "Menu"}})
		if !strings.Contains(res.HTML, "Menu") {
			t.Errorf("appbar leading text should render:\n%s", res.HTML)
		}
	})
	t.Run("appbar-custom-background", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "appbar", ID: "ab3", Label: "T", Props: map[string]any{"background": "blue"}})
		if !strings.Contains(res.HTML, "background:blue;") {
			t.Errorf("appbar background prop should apply:\n%s", res.HTML)
		}
	})

	t.Run("fab-extended", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "fab", ID: "fb", Label: "Add", Props: map[string]any{"extended": "true"}})
		if !strings.Contains(res.HTML, "border-radius:24px") || !strings.Contains(res.HTML, ">Add</button>") {
			t.Errorf("extended fab should be a pill with the label:\n%s", res.HTML)
		}
	})

	t.Run("spacer-size", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "spacer", ID: "sp", Style: map[string]any{"size": float64(20)}})
		if !strings.Contains(res.HTML, "width:20px;height:20px;flex-shrink:0;") {
			t.Errorf("sized spacer should be a fixed box:\n%s", res.HTML)
		}
	})

	t.Run("drawer-left-side", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "drawer", ID: "dr", Props: map[string]any{"open": "{{ state.show }}", "side": "left"}, Children: textKids("p")},
			open)
		if !strings.Contains(res.HTML, "left:0;top:0;bottom:0;") {
			t.Errorf("left drawer should anchor left:\n%s", res.HTML)
		}
	})

	t.Run("gridview-autofill", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "gridview", ID: "gv", Data: "{{ state.items }}",
				Props:    map[string]any{"minItemWidth": float64(150), "spacing": float64(4)},
				Template: &model.Node{Type: "text", ID: "c", Text: "{{ item.t }}"}},
			map[string]any{"items": []any{map[string]any{"t": "X"}}})
		if !strings.Contains(res.HTML, "repeat(auto-fill,minmax(150px,1fr))") || !strings.Contains(res.HTML, "gap:4px;") {
			t.Errorf("gridview autofill template wrong:\n%s", res.HTML)
		}
	})

	t.Run("camera-with-value-and-change", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "camera", ID: "cm", Value: "{{ state.pic }}", OnChange: &model.Invoke{Name: "save"}},
			map[string]any{"pic": "data:image/png;base64,AAA"})
		if !strings.Contains(res.HTML, `src="data:image/png;base64,AAA"`) || !strings.Contains(res.HTML, "display:block") {
			t.Errorf("camera with a captured value should show the preview:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, `data-state="pic"`) || !strings.Contains(res.HTML, "data-h=") {
			t.Errorf("camera should sync state and wire onChange:\n%s", res.HTML)
		}
	})

	t.Run("datepicker-clamps-reversed-years", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "datepicker", ID: "dp", Value: "2026-07-15",
			Props: map[string]any{"minYear": float64(2030), "maxYear": float64(2020)}})
		// maxY < minY is clamped to minY, so 2030 is the only (selected) year.
		if !strings.Contains(res.HTML, "2030") || strings.Contains(res.HTML, ">2020<") {
			t.Errorf("reversed min/max year should clamp:\n%s", res.HTML)
		}
	})
	t.Run("datepicker-onchange-wires-wheels", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "datepicker", ID: "dp2", Value: "2026-07-15",
			OnChange: &model.Invoke{Name: "setDate"}})
		if !strings.Contains(res.HTML, `onclick="qorm(`) || len(res.Handlers) == 0 {
			t.Errorf("datepicker onChange should wire each wheel item:\n%s", res.HTML)
		}
		// 12 months + 31 days + (2035-2020+1)=16 years = 59 selectable items
		if len(res.Handlers) != 59 {
			t.Errorf("datepicker should register 59 wheel handlers, got %d", len(res.Handlers))
		}
		for _, h := range res.Handlers {
			if h.Name != "setDate" || h.Args["value"] == "" {
				t.Errorf("each wheel handler should carry a recomposed date value: %+v", h)
			}
		}
	})

	t.Run("timepicker-minute-step", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "timepicker", ID: "tp", Value: "09:30", Props: map[string]any{"minuteStep": float64(15)}})
		// The minute wheel steps 0,15,30,45. 30 and 45 can only be minute marks
		// (the hour wheel stops at 23), so they prove the stepped wheel rendered.
		for _, w := range []string{">30<", ">45<"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("timepicker minuteStep=15 lacks minute label %q:\n%s", w, res.HTML)
			}
		}
		// Off-step minutes (29, 59 — both impossible as hours) must NOT render.
		for _, w := range []string{">29<", ">59<"} {
			if strings.Contains(res.HTML, w) {
				t.Errorf("minuteStep=15 should not render off-step minute %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("picker-onchange", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "picker", ID: "pk", Value: "S",
			Props:    map[string]any{"options": []any{map[string]any{"value": "S", "label": "Small"}, map[string]any{"value": "L", "label": "Large"}}},
			OnChange: &model.Invoke{Name: "setSize"}})
		if len(res.Handlers) != 2 {
			t.Errorf("picker onChange should register one handler per option, got %d", len(res.Handlers))
		}
		vals := map[string]bool{}
		for _, h := range res.Handlers {
			vals[h.Args["value"]] = true
		}
		if !vals["S"] || !vals["L"] {
			t.Errorf("picker handlers should carry each option value, got %v", vals)
		}
	})

	t.Run("richtext-weight-underline", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "richtext", ID: "rx", Props: map[string]any{"spans": []any{
			map[string]any{"text": "Bold", "fontWeight": float64(700), "underline": true},
			"not-a-map", // odd element shape is skipped, not fatal
		}}})
		if !strings.Contains(res.HTML, "font-weight:700;") || !strings.Contains(res.HTML, "text-decoration:underline;") {
			t.Errorf("richtext span should honour fontWeight+underline:\n%s", res.HTML)
		}
	})

	t.Run("largetitle-actions", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "largetitle", ID: "lt", Label: "Big",
			Children: []*model.Node{{Type: "button", ID: "act", Label: "Edit"}}})
		if !strings.Contains(res.HTML, "Edit") {
			t.Errorf("largetitle should render its action children:\n%s", res.HTML)
		}
	})

	t.Run("navigationrail-onchange", func(t *testing.T) {
		items := []any{map[string]any{"value": "h", "label": "Home", "icon": "home"}, map[string]any{"value": "s", "label": "Search"}}
		res := renderWidgetState(t,
			&model.Node{Type: "navigationrail", ID: "nr", Value: "{{ state.tab }}", Props: map[string]any{"items": items},
				OnChange: &model.Invoke{Name: "go"}},
			map[string]any{"tab": "h"})
		if len(res.Handlers) != 2 {
			t.Errorf("navigationrail onChange should register per item, got %d", len(res.Handlers))
		}
	})

	t.Run("navigationdrawer-onchange", func(t *testing.T) {
		items := []any{map[string]any{"value": "h", "label": "Home"}, map[string]any{"value": "s", "label": "Search"}}
		res := renderWidgetState(t,
			&model.Node{Type: "navigationdrawer", ID: "nd", Value: "{{ state.tab }}", Props: map[string]any{"items": items},
				OnChange: &model.Invoke{Name: "go", Args: map[string]string{"src": "drawer"}}},
			map[string]any{"tab": "s"})
		if len(res.Handlers) != 2 {
			t.Errorf("navigationdrawer onChange should register per item, got %d", len(res.Handlers))
		}
		// the extra authored arg is preserved alongside the per-item value
		for _, h := range res.Handlers {
			if h.Args["src"] != "drawer" || h.Args["value"] == "" {
				t.Errorf("navigationdrawer handler should merge authored args + value: %+v", h)
			}
		}
	})

	t.Run("bottomnav-icon-fallback-and-args", func(t *testing.T) {
		items := []any{map[string]any{"value": "x", "label": "Weird", "icon": "notarealicon"}}
		res := renderWidgetState(t,
			&model.Node{Type: "bottomnav", ID: "bn", Value: "{{ state.tab }}", Props: map[string]any{"items": items},
				OnChange: &model.Invoke{Name: "go", Args: map[string]string{"from": "nav"}}},
			map[string]any{"tab": "x"})
		if !strings.Contains(res.HTML, "notarealicon") {
			t.Errorf("unknown nav icon should fall back to escaped text:\n%s", res.HTML)
		}
		if len(res.Handlers) != 1 || res.Handlers[0].Args["from"] != "nav" || res.Handlers[0].Args["value"] != "x" {
			t.Errorf("bottomnav handler should merge authored args + item value: %+v", res.Handlers)
		}
	})

	t.Run("backbutton-onpress-and-label", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "backbutton", ID: "bb", Label: "Back", OnPress: &model.Invoke{Name: "pop"}})
		if strings.Contains(res.HTML, "history.back()") {
			t.Errorf("backbutton with onPress should not use history.back():\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, `onclick="qorm(`) || !strings.Contains(res.HTML, "<span>Back</span>") {
			t.Errorf("backbutton should wire onPress and show its label:\n%s", res.HTML)
		}
	})
	t.Run("closebutton-custom-aria", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "closebutton", ID: "cb", Props: map[string]any{"ariaLabel": "Dismiss"}})
		if !strings.Contains(res.HTML, `aria-label="Dismiss"`) || strings.Contains(res.HTML, `aria-label="Close"`) {
			t.Errorf("closebutton should honour an authored ariaLabel:\n%s", res.HTML)
		}
	})

	t.Run("form-onpress-submit", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "form", ID: "fm", OnPress: &model.Invoke{Name: "submit"}, Children: textKids("x")})
		if !strings.Contains(res.HTML, `onsubmit="qorm(`) || !strings.Contains(res.HTML, "return false") {
			t.Errorf("form with onPress should submit via qorm and block reload:\n%s", res.HTML)
		}
	})

	t.Run("offstage-false-shows", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "offstage", ID: "os", Props: map[string]any{"offstage": "false"}, Children: textKids("VISIBLE")})
		if !strings.Contains(res.HTML, "VISIBLE") || strings.Contains(res.HTML, "display:none") {
			t.Errorf("offstage=false should reveal the child without display:none:\n%s", res.HTML)
		}
	})

	t.Run("limitedbox-style-and-prop", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "limitedbox", ID: "lb", Style: map[string]any{"maxWidth": float64(100), "maxHeight": float64(50)}, Children: textKids("x")})
		if !strings.Contains(res.HTML, "max-width:100px;") || !strings.Contains(res.HTML, "max-height:50px;") {
			t.Errorf("limitedbox should read max sizes from style:\n%s", res.HTML)
		}
		// style wins; with no style the prop is used
		res = renderWidget(t, &model.Node{Type: "limitedbox", ID: "lb2", Props: map[string]any{"maxHeight": float64(80)}, Children: textKids("x")})
		if !strings.Contains(res.HTML, "max-height:80px;") {
			t.Errorf("limitedbox should fall back to the maxHeight prop:\n%s", res.HTML)
		}
	})

	t.Run("scaffold-no-appbar-safe-top", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "scaffold", ID: "sf", Children: []*model.Node{{Type: "text", ID: "b", Text: "BODY"}}})
		if !strings.Contains(res.HTML, "padding-top:var(--safe-top") {
			t.Errorf("scaffold without an appbar must apply the top safe inset:\n%s", res.HTML)
		}
		// with an appbar the body must NOT carry the top inset (the appbar owns it)
		res = renderWidget(t, &model.Node{Type: "scaffold", ID: "sf2", Children: []*model.Node{
			{Type: "appbar", ID: "ab", Label: "T"}, {Type: "text", ID: "b2", Text: "BODY"}}})
		if strings.Contains(res.HTML, "padding-top:var(--safe-top") {
			t.Errorf("scaffold with an appbar should not pad the body top:\n%s", res.HTML)
		}
	})

	t.Run("contextmenu-items-submenu-separator-icon", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "contextmenu", ID: "cx",
			Props: map[string]any{"items": []any{
				map[string]any{"id": "e", "title": "Edit", "icon": "copy"},
				map[string]any{"separator": true},
				map[string]any{"title": "More", "items": []any{map[string]any{"id": "s", "title": "SubItem"}}},
				"odd-non-map", // skipped, not fatal
			}},
			Children: textKids("target")})
		for _, w := range []string{`data-id="e"`, iconSVG("copy", 15), "height:.5px;background:var(--sep)", "qorm-ctxmenu-sub", "SubItem", iconSVG("chevron-right", 13)} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("contextmenu items lack %q:\n%s", w, res.HTML)
			}
		}
	})
	t.Run("contextmenu-menu-style", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "contextmenu", ID: "cx2",
			Props:    map[string]any{"items": []any{map[string]any{"id": "e", "title": "Edit"}}, "menuStyle": "background:hotpink;"},
			Children: textKids("t")})
		if !strings.Contains(res.HTML, "background:hotpink;") {
			t.Errorf("contextmenu menuStyle should apply to the panel:\n%s", res.HTML)
		}
	})

	t.Run("refreshindicator-onpress-fallback", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "refreshindicator", ID: "ri", OnPress: &model.Invoke{Name: "reload"}, Children: textKids("c")})
		if !strings.Contains(res.HTML, "qormRefresh(") || len(res.Handlers) != 1 || res.Handlers[0].Name != "reload" {
			t.Errorf("refreshindicator should fall back to onPress for the refresh handler: %+v\n%s", res.Handlers, res.HTML)
		}
	})

	t.Run("screens-and-desktop-widgets", func(t *testing.T) {
		if res := renderWidget(t, &model.Node{Type: "screens", ID: "scr"}); !strings.Contains(res.HTML, "qorm-screens-out") {
			t.Errorf("screens lacks its output region:\n%s", res.HTML)
		}
		if res := renderWidget(t, &model.Node{Type: "loginitem", ID: "li", Label: "Launch"}); !strings.Contains(res.HTML, "Launch") {
			t.Errorf("loginitem should use the authored label:\n%s", res.HTML)
		}
	})
}

// TestTransformBinding covers transform's bindable values, which also exercise
// numProp's string-binding branch (a {{ }} prop evaluating to a number).
func TestTransformBinding(t *testing.T) {
	t.Run("bound-rotate", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "transform", ID: "tf", Props: map[string]any{"rotate": "{{ state.deg }}"}, Children: textKids("x")},
			map[string]any{"deg": float64(90)})
		if !strings.Contains(res.HTML, "rotate(90deg)") {
			t.Errorf("bound rotate should resolve to 90deg:\n%s", res.HTML)
		}
	})
	t.Run("translate-skew-scales", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "transform", ID: "tf2",
			Props:    map[string]any{"translateX": float64(5), "translateY": float64(6), "skew": float64(10), "scaleX": float64(2), "scaleY": float64(3)},
			Children: textKids("x")})
		for _, w := range []string{"translate(5px,6px)", "skew(10deg)", "scaleX(2)", "scaleY(3)"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("transform lacks %q:\n%s", w, res.HTML)
			}
		}
	})
	t.Run("translate-x-only", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "transform", ID: "tf3", Props: map[string]any{"translateX": float64(7)}, Children: textKids("x")})
		if !strings.Contains(res.HTML, "translate(7px,0px)") {
			t.Errorf("translateX alone should default Y to 0:\n%s", res.HTML)
		}
	})
	t.Run("no-transform-props", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "transform", ID: "tf4", Children: textKids("x")})
		if strings.Contains(res.HTML, "transform:") {
			t.Errorf("transform with no props should emit no transform declaration:\n%s", res.HTML)
		}
	})
}

// TestWidgetAliasTypes guards that the Flutter/Cupertino alias names in the
// renderer switch dispatch to the same widget as their canonical type, so an app
// authoring e.g. "cupertinopicker" is not reported unknown.
func TestWidgetAliasTypes(t *testing.T) {
	cases := []struct {
		alias string
		node  *model.Node
		want  string
	}{
		{"cupertinopicker", &model.Node{Type: "cupertinopicker", ID: "a1", Props: map[string]any{"options": []any{"S"}}}, "height:180px"},
		{"cupertinodatepicker", &model.Node{Type: "cupertinodatepicker", ID: "a2", Value: "2026-07-15"}, "Jan"},
		{"cupertinotimepicker", &model.Node{Type: "cupertinotimepicker", ID: "a3", Value: "09:30"}, "height:180px"},
		{"sliverappbar", &model.Node{Type: "sliverappbar", ID: "a4", Label: "Big"}, "font-size:34px"},
		{"cupertinolargetitle", &model.Node{Type: "cupertinolargetitle", ID: "a5", Label: "Big"}, "font-size:34px"},
		{"absorbpointer", &model.Node{Type: "absorbpointer", ID: "a6", Children: textKids("x")}, "pointer-events:none"},
		{"floatingactionbutton", &model.Node{Type: "floatingactionbutton", ID: "a7"}, "qorm-tap"},
		{"navigationbar", &model.Node{Type: "navigationbar", ID: "a8", Props: map[string]any{"items": []any{map[string]any{"value": "h", "label": "H"}}}}, "qorm-bottomnav"},
		{"bottomnavigationbar", &model.Node{Type: "bottomnavigationbar", ID: "a9", Props: map[string]any{"items": []any{map[string]any{"value": "h", "label": "H"}}}}, "qorm-bottomnav"},
		{"dropdown", &model.Node{Type: "dropdown", ID: "a10", Props: map[string]any{"options": []any{"x"}}}, "<select"},
		{"wakelock", &model.Node{Type: "wakelock", ID: "a11"}, "qorm-keepawake"},
		{"geolocation", &model.Node{Type: "geolocation", ID: "a12"}, "qorm-location"},
		{"flashlight", &model.Node{Type: "flashlight", ID: "a13"}, "qorm-torch"},
		{"keychain", &model.Node{Type: "keychain", ID: "a14"}, "qorm-securestorage"},
		{"metric", &model.Node{Type: "metric", ID: "a15", Props: map[string]any{"value": "1"}}, "1"},
		{"formfield", &model.Node{Type: "formfield", ID: "a16", Props: map[string]any{"label": "L"}}, "L"},
		{"listitem", &model.Node{Type: "listitem", ID: "a17", Label: "T"}, "T"},
		{"cupertinolistsection", &model.Node{Type: "cupertinolistsection", ID: "a18", Props: map[string]any{"header": "H"}, Children: textKids("x")}, "H"},
		{"stepper", &model.Node{Type: "stepper", ID: "a19", Props: map[string]any{"steps": []any{"A"}}}, "A"},
		{"dialog", &model.Node{Type: "dialog", ID: "a20", Props: map[string]any{"open": "true"}, Children: textKids("body")}, `role="dialog"`},
		{"banner", &model.Node{Type: "banner", ID: "a21", Text: "hi"}, `role="alert"`},
		{"inkwell", &model.Node{Type: "inkwell", ID: "a22", OnPress: &model.Invoke{Name: "t"}, Children: textKids("x")}, `onclick="qorm(`},
		{"gesture", &model.Node{Type: "gesture", ID: "a23", Children: textKids("x")}, `id="a23"`},
		{"rotatedbox", &model.Node{Type: "rotatedbox", ID: "a24", Props: map[string]any{"rotate": float64(45)}, Children: textKids("x")}, "rotate(45deg)"},
		{"hero", &model.Node{Type: "hero", ID: "a25", Children: textKids("x")}, "animation:qa-scale"},
		{"speak", &model.Node{Type: "speak", ID: "a26"}, "qorm-tts"},
		{"heading", &model.Node{Type: "heading", ID: "a27"}, "qorm-compass"},
		{"safearea", &model.Node{Type: "safearea", ID: "a28"}, "qorm-insets"},
		{"openlink", &model.Node{Type: "openlink", ID: "a29"}, "qorm-openurl"},
		{"startatlogin", &model.Node{Type: "startatlogin", ID: "a30"}, "qorm-loginitem"},
		{"displays", &model.Node{Type: "displays", ID: "a31"}, "qorm-screens"},
		{"fingerprint", &model.Node{Type: "fingerprint", ID: "a32"}, "qorm-biometric"},
		{"faceid", &model.Node{Type: "faceid", ID: "a33"}, "qorm-biometric"},
		{"audiorecorder", &model.Node{Type: "audiorecorder", ID: "a34"}, "qorm-recorder"},
		{"barcode", &model.Node{Type: "barcode", ID: "a35"}, "qorm-qrscan"},
		{"file", &model.Node{Type: "file", ID: "a36"}, "qorm-filepicker"},
		{"photo", &model.Node{Type: "photo", ID: "a37"}, "qorm-photopicker"},
		{"modes", &model.Node{Type: "modes", ID: "a38"}, "qorm-systemmodes"},
		{"screencapture", &model.Node{Type: "screencapture", ID: "a39"}, "qorm-screenshot"},
		{"screenrecording", &model.Node{Type: "screenrecording", ID: "a40"}, "qorm-screenrecord"},
		{"speechinput", &model.Node{Type: "speechinput", ID: "a41"}, "qorm-stt"},
		{"slidingsegmentedcontrol", &model.Node{Type: "slidingsegmentedcontrol", ID: "a42", Props: map[string]any{"options": []any{"a"}}}, "qorm-seg"},
		{"swipeaction", &model.Node{Type: "swipeaction", ID: "a43", Props: map[string]any{"actions": []any{map[string]any{"label": "Del", "name": "d"}}}, Children: textKids("x")}, "qorm-swa"},
		{"cupertinocontextmenu", &model.Node{Type: "cupertinocontextmenu", ID: "a44", Props: map[string]any{"actions": []any{map[string]any{"label": "C"}}}, Children: textKids("x")}, "qorm-ctx"},
		{"expansionpanel", &model.Node{Type: "expansionpanel", ID: "a45", Label: "M", Children: textKids("x")}, "<details"},
		{"inputchip", &model.Node{Type: "inputchip", ID: "a46", Label: "T"}, "×"},
		{"choicechip", &model.Node{Type: "choicechip", ID: "a47", Label: "T"}, "T"},
		{"circularprogressindicator", &model.Node{Type: "circularprogressindicator", ID: "a48"}, "<svg"},
		{"cupertinoactivityindicator", &model.Node{Type: "cupertinoactivityindicator", ID: "a49"}, "qorm-activity"},
		{"keyvalue", &model.Node{Type: "keyvalue", ID: "a50", Props: map[string]any{"items": []any{map[string]any{"label": "k", "value": "v"}}}}, "v"},
		{"cupertinoactionsheet", &model.Node{Type: "cupertinoactionsheet", ID: "a51", Props: map[string]any{"open": "true", "actions": []any{map[string]any{"label": "A"}}}}, "qorm-sheet"},
		{"cupertinoalertdialog", &model.Node{Type: "cupertinoalertdialog", ID: "a52", Props: map[string]any{"open": "true", "title": "T"}}, "T"},
		{"checkboxlisttile-alias", &model.Node{Type: "checkboxlisttile", ID: "a53", Label: "C"}, `type="checkbox"`},
		{"radiolisttile", &model.Node{Type: "radiolisttile", ID: "a54", Label: "R", Props: map[string]any{"value": "v"}}, `type="radio"`},
		{"longpressdraggable", &model.Node{Type: "longpressdraggable", ID: "a55", Props: map[string]any{"data": "p"}, Children: textKids("x")}, "qorm-draggable"},
		{"droptarget", &model.Node{Type: "droptarget", ID: "a56", Children: textKids("x")}, "qorm-droptarget"},
		{"animatedpadding", &model.Node{Type: "animatedpadding", ID: "a57", Children: textKids("x")}, "transition:all"},
		{"animatedalign", &model.Node{Type: "animatedalign", ID: "a58", Children: textKids("x")}, "transition:all"},
		{"animatedpositioned", &model.Node{Type: "animatedpositioned", ID: "a59", Children: textKids("x")}, "transition:all"},
		{"animatedswitcher", &model.Node{Type: "animatedswitcher", ID: "a60", Children: textKids("x")}, "animation:qa-fade"},
		{"fadetransition", &model.Node{Type: "fadetransition", ID: "a61", Children: textKids("x")}, "animation:qa-fade"},
		{"slidetransition", &model.Node{Type: "slidetransition", ID: "a62", Children: textKids("x")}, "animation:qa-slideup"},
		{"scaletransition", &model.Node{Type: "scaletransition", ID: "a63", Children: textKids("x")}, "animation:qa-scale"},
		{"rotationtransition", &model.Node{Type: "rotationtransition", ID: "a64", Children: textKids("x")}, "animation:qa-rotate"},
		{"sizetransition", &model.Node{Type: "sizetransition", ID: "a65", Children: textKids("x")}, "animation:qa-size"},
	}
	for _, tc := range cases {
		t.Run(tc.alias, func(t *testing.T) {
			res := renderWidget(t, tc.node)
			if !strings.Contains(res.HTML, tc.want) {
				t.Errorf("alias %q lacks %q:\n%s", tc.alias, tc.want, res.HTML)
			}
			if len(res.Unknown) != 0 {
				t.Errorf("alias %q reported unknown: %v", tc.alias, res.Unknown)
			}
		})
	}
}
