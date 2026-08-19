package canvas

import (
	"encoding/json"
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

func TestCollectMeasureReportsIDs(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "text", ID: "hello", Text: "Hello canvas"},
		{Type: "button", ID: "cta", Label: "Go"},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 300))
	e.DrawFrame(surf)

	raw := e.CollectMeasure()
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("CollectMeasure JSON: %v (%s)", err, raw)
	}
	byID := map[string]map[string]any{}
	for _, r := range rows {
		id, _ := r["id"].(string)
		byID[id] = r
	}
	for _, id := range []string{"root", "hello", "cta"} {
		r, ok := byID[id]
		if !ok {
			t.Fatalf("missing measured id %q in %s", id, raw)
		}
		if r["visible"] != true {
			t.Errorf("%s visible = %v, want true", id, r["visible"])
		}
		if r["w"].(float64) <= 0 || r["h"].(float64) <= 0 {
			t.Errorf("%s box w/h = %v/%v", id, r["w"], r["h"])
		}
	}
	if got := byID["hello"]["text"]; got != "Hello canvas" {
		t.Errorf("hello text = %v", got)
	}
}

func TestMeasureSceneHeadless(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "text", ID: "t", Text: "x"},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	raw := MeasureScene(rt, 400, 200, 1)
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil || len(rows) == 0 {
		t.Fatalf("MeasureScene: %v %s", err, raw)
	}
}

func TestCollectMeasureLogicalHiDPI(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Style: map[string]any{
		"width": 200.0, "height": 100.0, "background": "#ff0000", "padding": 8.0,
	}, Children: []*model.Node{
		{Type: "text", ID: "hello", Text: "Hi", Style: map[string]any{"fontSize": 16.0, "color": "#00ff00"}},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	e := NewEngine(rt, SoftwareRenderer{})
	// Physical 800x400 at scale 2 → logical 400x200 stage.
	surf := NewHeadlessSurface(image.Pt(800, 400))
	// RenderInto uses size in physical px and scale.
	e.RenderInto(image.Pt(800, 400), 2, surf.Frame())

	phys := e.CollectMeasureOpts(MeasureOpts{Logical: false})
	logi := e.CollectMeasureOpts(MeasureOpts{Logical: true})
	var pr, lr []map[string]any
	if err := json.Unmarshal(phys, &pr); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(logi, &lr); err != nil {
		t.Fatal(err)
	}
	by := func(rows []map[string]any) map[string]map[string]any {
		m := map[string]map[string]any{}
		for _, r := range rows {
			m[r["id"].(string)] = r
		}
		return m
	}
	pb, lb := by(pr), by(lr)
	// Physical width ~2× logical for the same node.
	if pb["root"]["w"].(float64) < lb["root"]["w"].(float64)*1.5 {
		t.Fatalf("physical w=%v should be ~2x logical w=%v", pb["root"]["w"], lb["root"]["w"])
	}
	// Style fields present.
	if fs, _ := lb["hello"]["fontSize"].(string); fs == "" {
		t.Error("logical measure must include fontSize")
	}
	if bg, _ := lb["root"]["background"].(string); bg == "" || bg == "rgba(0, 0, 0, 0)" {
		t.Errorf("root background = %q, want red-ish", bg)
	}
	if st, ok := lb["__stage"]; !ok || st["w"].(float64) != 400 {
		t.Errorf("logical stage w = %v, want 400", st)
	}
}

func TestMeasureSceneSettlesEntrance(t *testing.T) {
	// A long fade entrance must not leave the node invisible after MeasureScene.
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "card", ID: "c", Props: map[string]any{"animation": "fade", "duration": 5000.0},
			Style: map[string]any{"width": 80.0, "height": 40.0, "background": "#00ff00"}},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	raw := MeasureScene(rt, 200, 200, 1)
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	var card map[string]any
	for _, r := range rows {
		if r["id"] == "c" {
			card = r
		}
	}
	if card == nil {
		t.Fatal("missing card row")
	}
	if card["visible"] != true {
		t.Errorf("settled measure must mark entrance node visible, got %v opacity=%v animating=%v",
			card["visible"], card["opacity"], card["animating"])
	}
	if card["animating"] == true {
		t.Error("MeasureScene must settle entrances so animating is false")
	}
}

func TestCollectMeasureEntranceInvisible(t *testing.T) {
	// Mid-fade (without settle): opacity gate makes visible=false at t≈0.
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "card", ID: "c", Props: map[string]any{"animation": "fade", "duration": 5000.0},
			Style: map[string]any{"width": 80.0, "height": 40.0, "background": "#00ff00"}},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(200, 200))
	e.DrawFrame(surf) // entrance just started
	raw := e.CollectMeasureOpts(MeasureOpts{Logical: true})
	var rows []map[string]any
	_ = json.Unmarshal(raw, &rows)
	var card map[string]any
	for _, r := range rows {
		if r["id"] == "c" {
			card = r
		}
	}
	if card == nil {
		t.Fatal("missing card")
	}
	// At t=0 of a fade, opacity is 0 → not visible; animating true.
	if card["animating"] != true {
		t.Errorf("live mid-entrance should set animating, row=%v", card)
	}
}

func TestCollectMeasureEnrichedStyles(t *testing.T) {
	root := &model.Node{Type: "column", ID: "box", Style: map[string]any{
		"width": 120.0, "height": 60.0, "background": "#112233",
		"borderRadius": 8.0, "padding": 10.0,
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 300))
	e.DrawFrame(surf)
	raw := e.CollectMeasureOpts(MeasureOpts{Logical: true})
	var rows []map[string]any
	_ = json.Unmarshal(raw, &rows)
	var box map[string]any
	for _, r := range rows {
		if r["id"] == "box" {
			box = r
		}
	}
	if box == nil {
		t.Fatal("missing box row")
	}
	for _, k := range []string{"background", "padding", "borderRadius", "opacity"} {
		if box[k] == nil || box[k] == "" {
			t.Errorf("missing style field %q in %v", k, box)
		}
	}
}

func measuredByID(t *testing.T, raw []byte) map[string]map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("measure JSON: %v\n%s", err, raw)
	}
	out := map[string]map[string]any{}
	for _, row := range rows {
		if id, _ := row["id"].(string); id != "" {
			out[id] = row
		}
	}
	return out
}

func TestCollectMeasureDerivedAccessibilitySemantics(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Style: map[string]any{
		"width": 320.0, "background": "#ffffff",
	}, Children: []*model.Node{
		{Type: "button", ID: "save", Label: "Save", Props: map[string]any{"class": "locked"}},
		{Type: "checkbox", ID: "wifi", Label: "Wi-Fi", Props: map[string]any{
			"checked": "{{ state.wifi }}", "ariaLabel": "{{ state.wifiName }}",
		}},
		{Type: "input", ID: "email", Placeholder: "Email", Value: "{{ state.email }}",
			Props: map[string]any{"required": "{{ state.required }}"}},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}, Styles: []model.StyleRule{
		{Kind: model.StyleRuleClass, Name: "locked", Style: map[string]any{"disabled": "{{ state.locked }}"}},
	}}
	rt := runtime.New(app)
	rt.State["locked"] = true
	rt.State["wifi"] = true
	rt.State["wifiName"] = "Wireless network"
	rt.State["email"] = "a@example.com"
	rt.State["required"] = true

	rows := measuredByID(t, MeasureScene(rt, 320, 240, 1))
	if got := rows["root"]["role"]; got != "main" {
		t.Errorf("root role = %v, want main", got)
	}
	if got := rows["save"]["role"]; got != "button" {
		t.Errorf("save role = %v, want button", got)
	}
	if got := rows["save"]["accessibleName"]; got != "Save" {
		t.Errorf("visible label accessibleName = %v, want Save", got)
	}
	if got := rows["save"]["ariaLabel"]; got != "" {
		t.Errorf("visible label must not masquerade as explicit ariaLabel: %v", got)
	}
	if got := rows["save"]["disabled"]; got != true {
		t.Errorf("QSS/bound disabled = %v, want true", got)
	}
	if got := rows["wifi"]["role"]; got != "checkbox" {
		t.Errorf("checkbox role = %v", got)
	}
	if got := rows["wifi"]["ariaLabel"]; got != "Wireless network" {
		t.Errorf("bound explicit ariaLabel = %v", got)
	}
	if got := rows["wifi"]["checked"]; got != true {
		t.Errorf("bound checked = %v, want true", got)
	}
	state, _ := rows["email"]["semanticState"].(map[string]any)
	if state["required"] != true || state["value"] != "a@example.com" {
		t.Errorf("textbox semantic state = %#v", state)
	}
}

func TestCollectMeasureSecureInputNeverLeaksValue(t *testing.T) {
	password := &model.Node{Type: "input", ID: "secure_field", Placeholder: "Password",
		Value: "{{ state.secret }}", Props: map[string]any{"secure": "{{ state.secure }}"}}
	root := &model.Node{Type: "column", ID: "root", Style: map[string]any{"background": "#ffffff"},
		Children: []*model.Node{password}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	rt.State["secret"] = "swordfish-credential"
	rt.State["secure"] = true
	raw := MeasureScene(rt, 320, 120, 1)
	if strings.Contains(string(raw), "swordfish-credential") {
		t.Fatalf("secure input leaked its value through measurement:\n%s", raw)
	}
	row := measuredByID(t, raw)["secure_field"]
	if got := row["text"]; got != "" {
		t.Errorf("secure input measured text = %q, want empty", got)
	}
	state, _ := row["semanticState"].(map[string]any)
	if state["protected"] != true {
		t.Errorf("secure semantic state = %#v, want protected:true", state)
	}
	if _, exists := state["value"]; exists {
		t.Errorf("secure semantic state must omit value: %#v", state)
	}
	if _, exists := row["value"]; exists {
		t.Errorf("secure measurement row must omit flattened value: %#v", row)
	}
}

func TestCollectMeasureUsesMountedWhenBranchSemantics(t *testing.T) {
	thenNode := &model.Node{Type: "button", ID: "shared", Label: "Active action"}
	elseNode := &model.Node{Type: "image", ID: "shared", Props: map[string]any{
		"alt": "Inactive artwork", "src": "missing.png",
	}}
	root := &model.Node{Type: "column", ID: "root", Style: map[string]any{"background": "#ffffff"},
		Children: []*model.Node{{Type: "when", ID: "choice", Condition: "{{ state.active }}", Then: thenNode, Else: elseNode}}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})

	rt.State["active"] = true
	row := measuredByID(t, MeasureScene(rt, 320, 120, 1))["shared"]
	if row["role"] != "button" || row["accessibleName"] != "Active action" {
		t.Fatalf("mounted then semantics overwritten by inactive else: %#v", row)
	}
	rt.State["active"] = false
	row = measuredByID(t, MeasureScene(rt, 320, 120, 1))["shared"]
	if row["role"] != "img" || row["accessibleName"] != "Inactive artwork" {
		t.Fatalf("mounted else semantics wrong: %#v", row)
	}
}

func TestCollectMeasureWCAGContrastAndUnavailableReasons(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Style: map[string]any{
		"width": 320.0, "background": "#ffffff",
	}, Children: []*model.Node{
		{Type: "text", ID: "black", Text: "black", Style: map[string]any{"color": "#000000"}},
		{Type: "box", ID: "alpha", Style: map[string]any{"background": "#00000080"}, Children: []*model.Node{
			{Type: "text", ID: "on_alpha", Text: "white", Style: map[string]any{"color": "#ffffff"}},
		}},
		{Type: "box", ID: "gradient", Style: map[string]any{
			"background": "linear-gradient(90deg, #000000, #ffffff)",
		}, Children: []*model.Node{
			{Type: "text", ID: "on_gradient", Text: "unknown", Style: map[string]any{"color": "#ffffff"}},
		}},
		{Type: "image", ID: "picture", Props: map[string]any{"src": "missing.png"}},
		{Type: "box", ID: "dimmed", Style: map[string]any{
			"background": "#000000", "opacity": 0.5,
		}, Children: []*model.Node{
			{Type: "box", ID: "opaque_child", Style: map[string]any{"background": "#ffffff"}, Children: []*model.Node{
				{Type: "text", ID: "under_opacity", Text: "still unknown", Style: map[string]any{"color": "#000000"}},
			}},
		}},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	rows := measuredByID(t, MeasureScene(rt, 320, 240, 1))
	if got, _ := rows["black"]["contrast"].(float64); math.Abs(got-21) > 0.01 {
		t.Errorf("black-on-white contrast = %v, want 21", got)
	}
	if got := rows["black"]["effectiveBackground"]; got != "rgb(255, 255, 255)" {
		t.Errorf("black effective ancestor background = %v", got)
	}
	if got, _ := rows["on_alpha"]["contrast"].(float64); got <= 3.9 || got >= 4.1 {
		t.Errorf("white on composited 50%% black/white = %v, want about 4.0", got)
	}
	if _, ok := rows["on_gradient"]["contrast"]; ok {
		t.Errorf("gradient contrast must be unavailable, got %v", rows["on_gradient"]["contrast"])
	}
	if reason, _ := rows["on_gradient"]["contrastUnavailable"].(string); !strings.Contains(reason, "gradient") {
		t.Errorf("gradient unavailable reason = %q", reason)
	}
	if reason, _ := rows["picture"]["contrastUnavailable"].(string); !strings.Contains(reason, "raster") {
		t.Errorf("image unavailable reason = %q", reason)
	}
	if reason, _ := rows["under_opacity"]["contrastUnavailable"].(string); !strings.Contains(reason, "opacity") {
		t.Errorf("ancestor subtree opacity must remain unavailable through opaque child: %q", reason)
	}
}

func TestCollectMeasureHostLimits(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Style: map[string]any{
		"width": 320.0, "background": "#ffffff",
	}, Children: []*model.Node{
		{Type: "text", ID: "fancy", Text: "Hello", Style: map[string]any{
			"fontFamily": "Comic Sans", "fontSize": 16.0, "color": "#000000",
		}},
		{Type: "webview", ID: "embed", Props: map[string]any{"url": "https://example.com"},
			Style: map[string]any{"width": 200.0, "height": 120.0}},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	rows := measuredByID(t, MeasureScene(rt, 320, 240, 1))
	limits, _ := rows["fancy"]["hostLimits"].([]any)
	joined := fmt.Sprint(limits)
	if !strings.Contains(joined, "style.fontFamily ignored") {
		t.Errorf("fontFamily must surface as a hostLimit, got %v", rows["fancy"]["hostLimits"])
	}
	if _, ok := rows["fancy"]["hostLimits"]; !ok {
		t.Fatal("fancy row missing hostLimits")
	}
	wlimits, _ := rows["embed"]["hostLimits"].([]any)
	if !strings.Contains(fmt.Sprint(wlimits), "webview placeholder") {
		t.Errorf("webview must surface placeholder hostLimit, got %v", rows["embed"]["hostLimits"])
	}
}

func TestCollectMeasureContrastUnavailableWithoutOpaqueBackdrop(t *testing.T) {
	root := &model.Node{Type: "text", ID: "floating", Text: "No known host backdrop",
		Style: map[string]any{"color": "#000000"}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	row := measuredByID(t, MeasureScene(rt, 320, 100, 1))["floating"]
	if _, ok := row["contrast"]; ok {
		t.Fatalf("transparent root must not claim contrast: %v", row["contrast"])
	}
	if got := row["contrastUnavailable"]; got != "no opaque ancestor background" {
		t.Errorf("transparent-root reason = %v", got)
	}
}

func TestCollectMeasureListVirtualization(t *testing.T) {
	items := make([]any, 500)
	for i := range items {
		items[i] = map[string]any{"label": fmt.Sprintf("%d", i)}
	}
	list := &model.Node{
		Type: "list", ID: "rows", Data: "{{state.items}}",
		Props:    map[string]any{"virtualize": "window", "itemHeight": 20.0},
		Template: &model.Node{Type: "text", Props: map[string]any{"text": "{{index}}"}},
	}
	sv := &model.Node{
		Type: "scroll", ID: "sv",
		Style:    map[string]any{"width": 200.0, "height": 200.0},
		Children: []*model.Node{list},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{sv}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	rt.State["items"] = items
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)
	e.HandlePointer(PointerInput{Type: PointerMove, X: 100, Y: 50})
	if !e.HandleScroll(ScrollInput{DY: 2000}) {
		t.Fatal("scroll must be consumed")
	}
	e.DrawFrame(surf)

	rows := measuredByID(t, e.CollectMeasure())
	row, ok := rows["rows"]
	if !ok {
		t.Fatalf("missing list row rows in %v", rows)
	}
	lv, ok := row["listVirtualization"].(map[string]any)
	if !ok {
		t.Fatalf("rows missing listVirtualization: %v", row)
	}
	if lv["windowed"] != true {
		t.Errorf("windowed = %v, want true", lv["windowed"])
	}
	if int(lv["total"].(float64)) != 500 {
		t.Errorf("total = %v, want 500", lv["total"])
	}
	rendered := int(lv["rendered"].(float64))
	if rendered < 10 || rendered > 25 {
		t.Errorf("rendered = %v, want ~19", rendered)
	}
	start := int(lv["start"].(float64))
	if start < 90 || start > 100 {
		t.Errorf("start = %v, want ~96", start)
	}
}
