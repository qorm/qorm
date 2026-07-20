// Package integration exercises the whole runtime against the real example
// apps: load -> render -> dispatch action -> re-render, mirroring what the
// server does on a button press.
package integration

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

func examplesDir(t *testing.T, app string) string {
	t.Helper()
	return filepath.Join("..", "..", "examples", app)
}

func findButton(n *model.Node, id string) *model.Node {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if got := findButton(c, id); got != nil {
			return got
		}
	}
	return nil
}

func TestCounterRendersAndIncrements(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "counter"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)

	html := render.Render(rt).HTML
	if !strings.Contains(html, ">COUNTER<") {
		t.Errorf("expected COUNTER title in:\n%s", html)
	}
	if !strings.Contains(html, ">0<") {
		t.Errorf("expected initial bound count 0")
	}
	if !strings.Contains(html, "<button") {
		t.Errorf("expected buttons to render")
	}

	// Press "+" twice through the real onPress handler (args are re-evaluated
	// against current state each time, exactly as the server does).
	plus := findButton(app.EntryRoot(), "btn_plus")
	if plus == nil || plus.OnPress == nil {
		t.Fatal("btn_plus with onPress not found")
	}
	for i := 0; i < 2; i++ {
		rt.Dispatch(plus.OnPress.Name, rt.EvalArgs(plus.OnPress.Args))
	}
	if got := rt.State["count"]; got != float64(2) {
		t.Fatalf("after 2 increments want count=2, got %v", got)
	}

	minus := findButton(app.EntryRoot(), "btn_minus")
	rt.Dispatch(minus.OnPress.Name, rt.EvalArgs(minus.OnPress.Args))
	if got := rt.State["count"]; got != float64(1) {
		t.Fatalf("after decrement want count=1, got %v", got)
	}
	if !strings.Contains(render.Render(rt).HTML, ">1<") {
		t.Errorf("re-render should show count 1")
	}
}

func TestTodoRendersListAndAdds(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "todo"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)

	// The list template renders each item's text.
	if html := render.Render(rt).HTML; !strings.Contains(html, "Master QORM Layout") {
		t.Errorf("expected initial todo item to render")
	}

	// Simulate typing into the input and pressing Add.
	rt.State["inputValue"] = "Ship Go runtime"
	add := findButton(app.EntryRoot(), "add_btn")
	if add == nil || add.OnPress == nil {
		t.Fatal("add_btn with onPress not found")
	}
	rt.Dispatch(add.OnPress.Name, rt.EvalArgs(add.OnPress.Args))

	items, _ := rt.State["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("want 3 items after add, got %d", len(items))
	}
	if !strings.Contains(render.Render(rt).HTML, "Ship Go runtime") {
		t.Errorf("new item should render in the list")
	}
	if rt.State["inputValue"] != "" {
		t.Errorf("input should be cleared after add, got %q", rt.State["inputValue"])
	}
}

func TestExamplesRenderWithoutPanic(t *testing.T) {
	for _, name := range []string{"counter", "todo", "login"} {
		app, err := loader.LoadDir(examplesDir(t, name))
		if err != nil {
			t.Fatalf("%s load: %v", name, err)
		}
		html := render.Render(qrt.New(app)).HTML
		if len(html) == 0 {
			t.Errorf("%s rendered empty", name)
		}
	}
}

func TestGalleryWidgetCoverage(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "gallery"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, marker := range []string{"<select", "<textarea", `type="radio"`, `type="checkbox"`, `type="range"`, "qorm-spin", "grid-template-columns", "role=\"progressbar\"", "<svg", "<rect", "<polyline"} {
		if !strings.Contains(html, marker) {
			t.Errorf("gallery should render %q", marker)
		}
	}
}

func TestI18nSwitchingAndFallback(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "i18n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)

	// Default locale (en) renders English.
	if !strings.Contains(render.Render(rt).HTML, "Hello, world") {
		t.Error("default locale should render English title")
	}

	// Switch locale via the button's action; UI re-resolves to Chinese.
	btn := findButton(app.EntryRoot(), "lang")
	if btn == nil || btn.OnPress == nil {
		t.Fatal("missing language toggle button")
	}
	rt.Dispatch(btn.OnPress.Name, rt.EvalArgs(btn.OnPress.Args))
	if rt.State["locale"] != "zh" {
		t.Fatalf("toggle should set locale to zh, got %v", rt.State["locale"])
	}
	if !strings.Contains(render.Render(rt).HTML, "你好，世界") {
		t.Error("after switch, title should render in Chinese")
	}

	// A key missing in zh falls back to the default (en) translation.
	if got := rt.Catalog()["onlyEn"]; got != "fallback-value" {
		t.Errorf("missing zh key should fall back to en, got %v", got)
	}
}

func TestI18nParamsAndPlural(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "i18n"))
	rt := qrt.New(app)
	rt.State["name"] = "Ada"

	// param interpolation from state
	if got := rt.Catalog()["greeting"]; got != "Welcome, Ada!" {
		t.Errorf("param fill failed: %v", got)
	}
	// plural: exact =0, one, other (English)
	rt.State["count"] = float64(0)
	if got := rt.Catalog()["cart"]; got != "Your cart is empty" {
		t.Errorf("plural =0 failed: %v", got)
	}
	rt.State["count"] = float64(1)
	if got := rt.Catalog()["cart"]; got != "1 item in cart" {
		t.Errorf("plural one failed: %v", got)
	}
	rt.State["count"] = float64(5)
	if got := rt.Catalog()["cart"]; got != "5 items in cart" {
		t.Errorf("plural other failed: %v", got)
	}
	// same, in Chinese (other-only), with # substitution
	rt.State["locale"] = "zh"
	rt.State["count"] = float64(3)
	if got := rt.Catalog()["cart"]; got != "购物车有 3 件商品" {
		t.Errorf("zh plural failed: %v", got)
	}
}

func TestI18nRTL(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "i18n"))
	rt := qrt.New(app)

	// LTR locale: no direction override.
	if strings.Contains(render.Render(rt).HTML, "direction:rtl") {
		t.Error("en should not render RTL")
	}
	// Arabic: root flips to RTL and shows Arabic text.
	rt.State["locale"] = "ar"
	html := render.Render(rt).HTML
	if !strings.Contains(html, "direction:rtl") {
		t.Error("ar should render the root direction:rtl")
	}
	if !strings.Contains(html, "مرحبا بالعالم") {
		t.Error("ar title should render")
	}
	if !qrt.IsRTL("he-IL") || qrt.IsRTL("en-US") {
		t.Error("IsRTL locale detection wrong")
	}
}

func TestFormValidation(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "form"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)
	render.Render(rt) // touch

	htmlFor := func(email, pw string) string {
		rt.State["email"] = email
		rt.State["password"] = pw
		return render.Render(rt).HTML
	}
	const emailErr = "valid email address"
	const pwErr = "at least 6 characters"
	const ready = "ready to submit"

	// empty: no errors, not ready
	h := htmlFor("", "")
	if strings.Contains(h, emailErr) || strings.Contains(h, pwErr) || strings.Contains(h, ready) {
		t.Error("empty form should show neither errors nor ready")
	}
	// invalid: both errors, not ready
	h = htmlFor("bad", "12")
	if !strings.Contains(h, emailErr) || !strings.Contains(h, pwErr) {
		t.Error("invalid input should surface both validation errors")
	}
	if strings.Contains(h, ready) {
		t.Error("invalid form should not be ready")
	}
	// valid: no errors, ready
	h = htmlFor("ada@example.com", "secret9")
	if strings.Contains(h, emailErr) || strings.Contains(h, pwErr) {
		t.Error("valid input should clear errors")
	}
	if !strings.Contains(h, ready) {
		t.Error("valid form should show ready")
	}
}

func TestVirtualizedList(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "biglist"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)

	// Feed a large data set.
	const n = 5000
	items := make([]any, n)
	for i := 0; i < n; i++ {
		items[i] = map[string]any{"name": "User" + strconv.Itoa(i), "tag": "t" + strconv.Itoa(i%5)}
	}
	rt.State["items"] = items

	html := render.Render(rt).HTML
	if got := strings.Count(html, "content-visibility:auto"); got != n {
		t.Errorf("expected %d virtualized item wrappers, got %d", n, got)
	}
	if !strings.Contains(html, "User0") || !strings.Contains(html, "User4999") {
		t.Error("all items (first and last) should be present in the DOM")
	}
}

func TestComponentBatch1(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"<table", "qorm-table", "Name", ">Ada<", // table + bound rows
		`role="dialog"`, ">Welcome<", // modal (open)
		`role="alert"`, ">Saved<", // alert
		"qorm-skel",     // skeleton
		"qormAcc",       // accordion toggle wired
		"Section one",   // accordion header
		"M12 3l2.6 5.3", // rating star icon (SVG path)
		"Members",       // breadcrumb last crumb
		">design<",      // tag
	} {
		if !strings.Contains(html, m) {
			t.Errorf("components showcase should render %q", m)
		}
	}
}

func TestModalHiddenWhenClosed(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	rt := qrt.New(app)
	rt.State["showModal"] = false
	if strings.Contains(render.Render(rt).HTML, `role="dialog"`) {
		t.Error("modal should not render when open is false")
	}
}

func TestComponentBatch2(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"qormMenu", "qorm-menu-panel", // menu
		"<details", "main.go", // tree (nested)
		"scroll-snap-type", ">Slide 1<", // carousel
		`data-tooltip="A helpful hint"`, // tooltip
		"onclick=\"qorm(",               // pagination buttons wired
		">Shipped<",                     // timeline item
	} {
		if !strings.Contains(html, m) {
			t.Errorf("components showcase (batch 2) should render %q", m)
		}
	}
}

func TestComponentBatch3(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		">Revenue<", ">$12.4k<", // stat
		"qorm-seg", ">Monthly<", `name="seg"`, // segmented (bound radios)
		">No files yet<",                             // empty state
		"grid-template-columns:auto 1fr", ">Renews<", // descriptions
	} {
		if !strings.Contains(html, m) {
			t.Errorf("components showcase (batch 3) should render %q", m)
		}
	}
	// field: required marker + help text
	if !strings.Contains(html, "color:#ef4444;\"> *") {
		t.Error("field should show a required marker")
	}
}

// TestTaskBoardEndToEnd exercises a realistic app: add / toggle / filter /
// remove / sort — through the render + action pipeline, proving the widget and
// action vocabulary composes.
func TestTaskBoardEndToEnd(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "tasks"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)
	fire := func(action string, args map[string]any) { rt.Dispatch(action, args) }
	tasks := func() []any { return rt.State["tasks"].([]any) }

	if len(tasks()) != 2 {
		t.Fatalf("start with 2 tasks, got %d", len(tasks()))
	}
	// add
	rt.State["newTitle"] = "Fuzz the parser"
	fire("addTask", map[string]any{"title": "Fuzz the parser"})
	if len(tasks()) != 3 || rt.State["newTitle"] != "" {
		t.Fatalf("add failed: %d tasks, newTitle=%q", len(tasks()), rt.State["newTitle"])
	}
	if !strings.Contains(render.Render(rt).HTML, "Fuzz the parser") {
		t.Error("new task should render")
	}
	// toggle the new task done
	fire("toggleTask", map[string]any{"id": "Fuzz the parser"})
	// filter=done should hide the (not-done) "Ship native renderer"
	rt.State["filter"] = "done"
	h := render.Render(rt).HTML
	if strings.Contains(h, "Ship native renderer") {
		t.Error("active task should be hidden under the 'done' filter")
	}
	if !strings.Contains(h, "Fuzz the parser") || !strings.Contains(h, "Write the docs") {
		t.Error("done tasks should be visible under the 'done' filter")
	}
	// remove
	fire("removeTask", map[string]any{"id": "Fuzz the parser"})
	if len(tasks()) != 2 {
		t.Fatalf("remove failed: %d tasks", len(tasks()))
	}
	// sort A→Z by title
	fire("sortTasks", nil)
	first := tasks()[0].(map[string]any)["title"]
	if first != "Ship native renderer" { // "S" < "W"
		t.Errorf("sort asc: first should be 'Ship native renderer', got %v", first)
	}
}

func TestSortableTable(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)
	// header renders a clickable sort button
	if !strings.Contains(render.Render(rt).HTML, "⇅") {
		t.Error("sortable table should render clickable headers")
	}
	// sorting by the 'name' column reorders rows (Zoe, Ada, Linus -> Ada, Linus, Zoe)
	rt.Dispatch("sortRows", map[string]any{"column": "name"})
	rows := rt.State["rows"].([]any)
	if rows[0].(map[string]any)["name"] != "Ada" {
		t.Errorf("after sort by name, first row should be Ada, got %v", rows[0])
	}
}

// TestWidgetParityPhase1 covers the phase-1 widgets: app bar, list tile, wrap,
// button variants, and the floating action button.
func TestWidgetParityPhase1(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"height:calc(44px",          // appbar (+ safe-area inset)
		">Widget Gallery<",          // appbar title
		">Documents<", ">14 files<", // listtile title+subtitle
		"flex-wrap:wrap",                 // wrap
		"border:1px solid var(--accent)", // outlined button variant
		"box-shadow:0 2px 5px",           // elevated button variant
	} {
		if !strings.Contains(html, m) {
			t.Errorf("phase-1 showcase should render %q", m)
		}
	}
	// two circular elements: icon button + fab
	if strings.Count(html, "border-radius:50%") < 2 {
		t.Error("expected icon button + fab to be circular")
	}
}

// TestWidgetParityChips covers the chip family (choice/filter/input) + RangeSlider.
func TestWidgetParityChips(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"border-radius:16px",             // chips
		">Design<",                       // choice chip label
		"M12 3l2.6 5.3",                  // choice chip avatar (star icon SVG path)
		"M4 12l5 5L20 6",                 // selected filter chip check icon (SVG path)
		"qorm-range-lo", "qorm-range-hi", // range slider thumbs
	} {
		if !strings.Contains(html, m) {
			t.Errorf("chips/rangeslider showcase should render %q", m)
		}
	}
	// filled segment between lo(20) and hi(70) = 50% wide
	if !strings.Contains(html, "width:50%") {
		t.Error("rangeslider fill should span lo..hi")
	}
}

// TestWidgetParityGridStepper covers the grid view + the stepper widget.
func TestWidgetParityGridStepper(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"grid-template-columns:repeat(2,1fr)", // gridview
		">Item 1<", ">Item 4<",                // grid cells from data
		">Shipping<", ">shipping body<", // active step (wizStep=1) content
		"M4 12l5 5L20 6", // completed step marker (check icon SVG path)
	} {
		if !strings.Contains(html, m) {
			t.Errorf("gridview/stepper showcase should render %q", m)
		}
	}
	// inactive steps' bodies stay hidden
	if strings.Contains(html, ">payment body<") {
		t.Error("inactive step content should not render")
	}
}

// TestWidgetParityPageDropdown covers the page view + dropdown button widgets.
func TestWidgetParityPageDropdown(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"qorm-pageview", "scroll-snap-type:x mandatory", // pageview
		">Page One<", ">Page Two<", // pages
		"qormMenu(this)", `role="option"`, // dropdown button + menu items
		">By Name<", // current selection label shown
	} {
		if !strings.Contains(html, m) {
			t.Errorf("pageview/dropdown showcase should render %q", m)
		}
	}
}

// TestWidgetParityGestureAutocomplete covers the gesture detector + autocomplete widgets.
func TestWidgetParityGestureAutocomplete(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		`ondblclick="qorm(`,                    // double-tap wired
		"qormLong(document.getElementById",     // long-press wired
		`<datalist id="ac-ac"`, `list="ac-ac"`, // autocomplete datalist
		`value="Apricot"`, // a suggestion option
	} {
		if !strings.Contains(html, m) {
			t.Errorf("gesture/autocomplete showcase should render %q", m)
		}
	}
}

// TestWidgetParityFormField covers the text form field (input decoration +
// reactive validation) and the circular progress indicator.
func TestWidgetParityFormField(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)
	// components starts with email="" → invalid → error + red border shown
	html := render.Render(rt).HTML
	for _, m := range []string{
		">Email<",                  // label
		"Enter a valid email",      // reactive error (empty email is invalid)
		"border:1px solid #ef4444", // reddened border
		"0/40",                     // counter len/maxLength
		"stroke-dashoffset",        // determinate circular progress arc
	} {
		if !strings.Contains(html, m) {
			t.Errorf("formfield showcase should render %q", m)
		}
	}
	// set a valid email → error clears (reactive validation via expr)
	rt.State["email"] = "a@b.com"
	if strings.Contains(render.Render(rt).HTML, "Enter a valid email") {
		t.Error("valid email should clear the validation error")
	}
}

// TestWidgetParityBadgeDivider covers the badge-on-child + vertical divider widgets.
func TestWidgetParityBadgeDivider(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"position:absolute;top:-6px;right:-6px", // corner badge on child
		">3<",                                   // count
		"width:1px;align-self:stretch",          // vertical divider
	} {
		if !strings.Contains(html, m) {
			t.Errorf("badge/divider showcase should render %q", m)
		}
	}
}

// TestDialogs covers the alert dialog + action sheet widgets.
func TestDialogs(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		">Delete file?<", ">This cannot be undone.<", // alert dialog
		"width:270px",          // iOS alert width
		"color:var(--danger)",  // destructive action
		"align-items:flex-end", // action sheet anchored bottom
		">Choose an option<",   // sheet title
	} {
		if !strings.Contains(html, m) {
			t.Errorf("cupertino dialogs should render %q", m)
		}
	}
}

// TestListSection covers the iOS inset grouped list + tile chevron.
func TestListSection(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"text-transform:uppercase", ">Settings<", // section header
		"background:var(--surface);border-radius:10px",       // grouped card
		"height:.5px;background:var(--sep);margin-left:16px", // inset separators
		">›<", // iOS disclosure chevron on tappable tiles
	} {
		if !strings.Contains(html, m) {
			t.Errorf("listsection showcase should render %q", m)
		}
	}
}

// TestSliderActivityPicker covers the iOS slider restyle, the activity
// indicator and the picker.
func TestSliderActivityPicker(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		`class="qorm-slider"`, "--pct:", // iOS slider + track fill (via CSS var on the track pseudo-element)
		`class="qorm-activity"`, "rotate(315 10 10)", // activity indicator spokes
		"scroll-snap-type:y mandatory", "scroll-snap-align:center", // picker wheel
		">Tokyo<", // picker option (selected)
	} {
		if !strings.Contains(html, m) {
			t.Errorf("slider/activity/picker showcase should render %q", m)
		}
	}
}

// TestDismissible covers swipe-to-dismiss wiring.
func TestDismissible(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"qorm-dismiss-content",              // swipeable content layer
		"background:var(--danger)",          // destructive reveal
		"qormSwipe(document.getElementById", // swipe handler wired
		">Swipe me left to delete<",         // the row
	} {
		if !strings.Contains(html, m) {
			t.Errorf("dismissible showcase should render %q", m)
		}
	}
}

// TestContextMenuRefresh covers the context menu + refresh indicator widgets.
func TestContextMenuRefresh(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"qorm-ctx-panel", "qormCtx(document.getElementById", ">Long-press for menu<", // context menu
		"qorm-refresh-spin", "qormRefresh(document.getElementById", // pull-to-refresh
	} {
		if !strings.Contains(html, m) {
			t.Errorf("contextmenu/refresh showcase should render %q", m)
		}
	}
}

// TestAnimatedWidgets covers AnimatedContainer + AnimatedOpacity (implicit
// animation: a bound style change transitions).
func TestAnimatedWidgets(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	rt := qrt.New(app)
	html := render.Render(rt).HTML
	if !strings.Contains(html, "transition:all 400ms") || !strings.Contains(html, "width:100px") {
		t.Error("animatedcontainer should emit transition + start width")
	}
	if !strings.Contains(html, "transition:opacity 400ms") {
		t.Error("animatedopacity should emit an opacity transition")
	}
	// flip state → the bound style changes (the transition animates it)
	rt.State["big"] = true
	if !strings.Contains(render.Render(rt).HTML, "width:200px") {
		t.Error("changing state should update the animated width")
	}
}

// TestMotion covers the named-animation catalog + bindable effect switching.
func TestMotion(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	rt := qrt.New(app)
	if !strings.Contains(render.Render(rt).HTML, "animation:qa-bounce 600ms") {
		t.Error("motion should map effect=bounce to qa-bounce")
	}
	// switching state switches the animation implementation (agent-driven)
	for eff, kf := range map[string]string{"fade": "qa-fade", "flip": "qa-flip", "slideleft": "qa-slideleft"} {
		rt.State["effect"] = eff
		if !strings.Contains(render.Render(rt).HTML, "animation:"+kf) {
			t.Errorf("effect=%s should map to %s", eff, kf)
		}
	}
}

// TestTransformAspectRich covers Transform, AspectRatio and RichText.
func TestTransformAspectRich(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	rt := qrt.New(app)
	html := render.Render(rt).HTML
	for _, m := range []string{
		"transform:rotate(15deg)", "aspect-ratio:1.6",
		"font-style:italic", "text-decoration:underline", "color:#007aff;font-weight:700",
	} {
		if !strings.Contains(html, m) {
			t.Errorf("transform/aspect/richtext should render %q", m)
		}
	}
	rt.State["angle"] = float64(90)
	if !strings.Contains(render.Render(rt).HTML, "transform:rotate(90deg)") {
		t.Error("bound rotate should follow state")
	}
}

// TestNavLargeTitleSelectable covers the iOS large-title bar, NavigationRail
// and SelectableText.
func TestNavLargeTitleSelectable(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"font-size:34px;font-weight:700", ">Library<", ">128 items<", // large title
		"border-right:.5px solid",                          // navigation rail
		"user-select:text", ">Select and copy this text.<", // selectable text
	} {
		if !strings.Contains(html, m) {
			t.Errorf("nav/largetitle/selectable should render %q", m)
		}
	}
}

// TestBackCloseButton covers BackButton/CloseButton: they default to the URL
// router (history.back() → popstate → /navigate), name themselves for a11y when
// icon-only, and hand control to an explicit onPress action when given one.
func TestBackCloseButton(t *testing.T) {
	root := &model.Node{Type: "scaffold", ID: "r", Children: []*model.Node{
		{Type: "backbutton", ID: "back", Label: "Settings"},
		{Type: "closebutton", ID: "close"},
		{Type: "backbutton", ID: "custom", OnPress: &model.Invoke{Name: "dismiss"}},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{"dismiss": {ID: "dismiss"}}}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		`id="back"`, `onclick="history.back()"`, `aria-label="Back"`, `<span>Settings</span>`, // default back + label
		`id="close"`, `aria-label="Close"`, // default close
	} {
		if !strings.Contains(html, m) {
			t.Errorf("back/close button should render %q\n%s", m, html)
		}
	}
	// an explicit onPress overrides the default history.back()
	i := strings.Index(html, `id="custom"`)
	if i < 0 || strings.Contains(html[i:i+200], "history.back()") {
		t.Error("backbutton with onPress must dispatch the action, not history.back()")
	}
}

// TestFormOffstage covers Form (submit-gating via a real <form> + onPress) and
// Offstage (renders the subtree but hides it from paint, default hidden).
func TestFormOffstage(t *testing.T) {
	root := &model.Node{Type: "scaffold", ID: "r", Children: []*model.Node{
		{Type: "form", ID: "f", OnPress: &model.Invoke{Name: "save"}, Children: []*model.Node{
			{Type: "input", ID: "email"},
		}},
		{Type: "offstage", ID: "hid", Children: []*model.Node{{Type: "text", ID: "secret", Text: "peekaboo"}}},
		{Type: "offstage", ID: "shown", Props: map[string]any{"offstage": "false"},
			Children: []*model.Node{{Type: "text", ID: "vis", Text: "onstage"}}},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{"save": {ID: "save"}}}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		`<form id="f"`, `onsubmit="qorm(`, `;return false"`, `id="email"`, // form submits its action, no reload
		`>peekaboo<`, // offstage keeps the subtree in the DOM (ids stay wired)...
	} {
		if !strings.Contains(html, m) {
			t.Errorf("form/offstage should render %q\n%s", m, html)
		}
	}
	// default offstage is hidden; offstage:false paints normally
	hid := html[strings.Index(html, `id="hid"`):]
	if !strings.Contains(hid[:120], "display:none") {
		t.Error("offstage default should hide the subtree from paint")
	}
	shown := html[strings.Index(html, `id="shown"`):]
	if strings.Contains(shown[:120], "display:none") {
		t.Error("offstage:false should paint the subtree")
	}
}

// TestIndexedStack covers IndexedStack: all children mount (their ids stay in
// the DOM) but only the bound index paints; the rest are display:none.
func TestIndexedStack(t *testing.T) {
	root := &model.Node{Type: "scaffold", ID: "r", Children: []*model.Node{
		{Type: "indexedstack", ID: "wiz", Props: map[string]any{"index": "{{state.step}}"}, Children: []*model.Node{
			{Type: "text", ID: "s0", Text: "step-zero"},
			{Type: "text", ID: "s1", Text: "step-one"},
		}},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		GlobalState: model.GlobalState{Initial: map[string]any{"step": float64(1)}}}
	rt := qrt.New(app)
	rt.State["step"] = float64(1)
	html := render.Render(rt).HTML
	// both children present (state preserved), only index 1 painted
	for _, m := range []string{">step-zero<", ">step-one<"} {
		if !strings.Contains(html, m) {
			t.Errorf("indexedstack should mount every child, missing %q", m)
		}
	}
	z := html[strings.Index(html, ">step-zero<")-80 : strings.Index(html, ">step-zero<")]
	if !strings.Contains(z, "display:none") {
		t.Error("indexedstack should hide the non-selected child (index 0)")
	}
	o := html[strings.Index(html, ">step-one<")-80 : strings.Index(html, ">step-one<")]
	if strings.Contains(o, "display:none") {
		t.Error("indexedstack should paint the selected child (index 1)")
	}
}

// TestNavDrawerBottomBarLimited covers the last server-renderable catalog items:
// NavigationDrawer (destination list, active highlight, onChange), BottomAppBar
// (bottom toolbar of children), and LimitedBox (maxWidth/maxHeight cap).
func TestNavDrawerBottomBarLimited(t *testing.T) {
	root := &model.Node{Type: "scaffold", ID: "r", Children: []*model.Node{
		{Type: "navigationdrawer", ID: "nd", Value: "home",
			Props:    map[string]any{"items": `{{state.dest}}`},
			OnChange: &model.Invoke{Name: "go", Args: map[string]string{}}},
		{Type: "bottomappbar", ID: "bar", Children: []*model.Node{{Type: "text", ID: "t", Text: "toolbar"}}},
		{Type: "limitedbox", ID: "lb", Style: map[string]any{"maxWidth": float64(320)},
			Children: []*model.Node{{Type: "text", ID: "c", Text: "capped"}}},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{"go": {ID: "go"}}}
	rt := qrt.New(app)
	rt.State["dest"] = []any{
		map[string]any{"value": "home", "label": "Home", "icon": "house"},
		map[string]any{"value": "settings", "label": "Settings", "icon": "gear"},
	}
	html := render.Render(rt).HTML
	for _, m := range []string{
		`id="nd"`, `>Home</button>`, `>Settings</button>`, `onclick="qorm(`, // drawer destinations wired
		`id="bar"`, `role="toolbar"`, `>toolbar<`, // bottom app bar
		`id="lb"`, `max-width:320px`, `>capped<`, // limited box cap
	} {
		if !strings.Contains(html, m) {
			t.Errorf("navdrawer/bottombar/limitedbox should render %q\n%s", m, html)
		}
	}
	// active destination (home) is highlighted, the other is not
	home := html[strings.Index(html, `>Home</button>`)-260 : strings.Index(html, `>Home</button>`)]
	if !strings.Contains(home, "var(--accent)") {
		t.Error("navigationdrawer should highlight the active destination")
	}
}

// TestDraggableDragTarget covers Draggable (carries a string payload, wired via
// qormDraggable) and DragTarget (drop zone wired via qormDragTarget to an onDrop
// handler). The actual drag interaction is browser-verified; this asserts the
// server emits the payload + handler wiring.
func TestDraggableDragTarget(t *testing.T) {
	root := &model.Node{Type: "scaffold", ID: "r", Children: []*model.Node{
		{Type: "draggable", ID: "card", Props: map[string]any{"data": "u-101"},
			Children: []*model.Node{{Type: "text", ID: "t", Text: "drag me"}}},
		{Type: "dragtarget", ID: "bin", OnPress: &model.Invoke{Name: "drop"},
			Children: []*model.Node{{Type: "text", ID: "z", Text: "drop here"}}},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{"drop": {ID: "drop"}}}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		`class="qorm-draggable" data-qorm-drag="u-101"`, `qormDragInit`, // payload on the card, delegated init
		`class="qorm-droptarget" data-qorm-drop="`, // drop zone carries its handler index
		`>drag me<`, `>drop here<`,
	} {
		if !strings.Contains(html, m) {
			t.Errorf("draggable/dragtarget should render %q\n%s", m, html)
		}
	}
}

// TestRenderItemUniqueIDs guards the fix for JS-wired widgets inside a list:
// each item's Dismissible must get a unique id + matching swipe script, so
// swipe-to-delete works on every row (canonical inbox pattern), not just the
// first.
func TestRenderItemUniqueIDs(t *testing.T) {
	app, _ := loader.LoadDir(examplesDir(t, "components"))
	html := render.Render(qrt.New(app)).HTML
	// two inbox rows → irow-0 and irow-1, both wired
	for _, id := range []string{`id="irow-0"`, `id="irow-1"`, `getElementById("irow-0"`, `getElementById("irow-1"`} {
		if !strings.Contains(html, id) {
			t.Errorf("dismissible-in-list should emit %q", id)
		}
	}
}

// TestWidgetParityDataTable covers the datatable's selection (single row +
// select-all) and its pagination linkage through the slice() binding.
func TestWidgetParityDataTable(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)
	html := render.Render(rt).HTML
	for _, m := range []string{
		`class="qorm-datatable"`, "qdt-check", "qdt-sort", // table, selection checkboxes, sortable headers
		">Moss<", // a page-1 row
	} {
		if !strings.Contains(html, m) {
			t.Errorf("datatable showcase should render %q", m)
		}
	}
	// 12 rows at 10 per page: row 11 waits for page 2
	if strings.Contains(html, ">Margaret<") {
		t.Error("page 1 should not render row 11")
	}
	// single-row toggle selects, then deselects
	rt.Dispatch("dtToggle", map[string]any{"key": "r3"})
	if got := rt.State["dtSel"].([]any); len(got) != 1 || got[0] != "r3" {
		t.Fatalf("row toggle: got %v", rt.State["dtSel"])
	}
	if !strings.Contains(render.Render(rt).HTML, `class="qdt-sel"`) {
		t.Error("the selected row should carry the qdt-sel class")
	}
	rt.Dispatch("dtToggle", map[string]any{"key": "r3"})
	if len(rt.State["dtSel"].([]any)) != 0 {
		t.Error("toggling the same row again should deselect it")
	}
	// the header checkbox toggles the whole selection
	rt.Dispatch("dtToggle", map[string]any{"key": "__all__"})
	if got := rt.State["dtSel"].([]any); len(got) != 12 {
		t.Fatalf("select-all: got %d keys", len(got))
	}
	rt.Dispatch("dtToggle", map[string]any{"key": "__all__"})
	if len(rt.State["dtSel"].([]any)) != 0 {
		t.Error("select-all again should clear the selection")
	}
	// pagination dispatches {page} → the slice window moves to rows 11-12
	rt.Dispatch("dtPage", map[string]any{"page": "2"})
	if rt.State["dtPage"] != float64(2) {
		t.Fatalf("dtPage dispatch: got %v", rt.State["dtPage"])
	}
	p2 := render.Render(rt).HTML
	if !strings.Contains(p2, ">Margaret<") || strings.Contains(p2, ">Moss<") {
		t.Error("page 2 should show rows 11-12 only")
	}
	// sorting by name reorders rows (Moss first → Ada first)
	rt.Dispatch("dtSort", map[string]any{"column": "name"})
	rows := rt.State["dtRows"].([]any)
	if rows[0].(map[string]any)["name"] != "Ada" {
		t.Errorf("after sort by name, first row should be Ada, got %v", rows[0])
	}
	if rt.State["dtSort"] != "name" {
		t.Error("dtSort should record the sorted column")
	}
}

// TestStateReset covers the state.reset step: the showcase's Reset button
// restores every globalState key to its manifest initial value.
func TestStateReset(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)
	if !strings.Contains(render.Render(rt).HTML, ">Reset form<") {
		t.Fatal("reset button should render")
	}
	// dirty a handful of keys, then dispatch the reset action
	rt.State["email"] = "a@b.com"
	rt.State["city"] = "Paris"
	rt.State["dtSel"] = []any{"r1", "r2"}
	rt.Dispatch("resetForm", nil)
	if rt.State["email"] != "" || rt.State["city"] != "Tokyo" {
		t.Errorf("reset should restore initial scalars, got email=%v city=%v", rt.State["email"], rt.State["city"])
	}
	if got := rt.State["dtSel"].([]any); len(got) != 0 {
		t.Errorf("reset should restore initial arrays, got dtSel=%v", got)
	}
}

// TestWidgetParityTimePicker covers the timepicker's iOS-style hour/minute
// wheels (minuteStep spacing) and its {value} dispatch.
func TestWidgetParityTimePicker(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)
	html := render.Render(rt).HTML
	for _, m := range []string{
		`id="tp"`,
		"scroll-snap-type:y mandatory", // wheel columns
		">09<", ">30<",                 // the current value 09:30 on the wheels
		">55<", // minuteStep=5 still reaches 55
	} {
		if !strings.Contains(html, m) {
			t.Errorf("timepicker showcase should render %q", m)
		}
	}
	if strings.Contains(html, ">57<") {
		t.Error("minuteStep=5 should not offer a 57 minute")
	}
	// clicking a wheel item dispatches the full HH:MM value
	rt.Dispatch("setTime", map[string]any{"value": "14:45"})
	if rt.State["time"] != "14:45" {
		t.Errorf("setTime dispatch: got %v", rt.State["time"])
	}
	if !strings.Contains(render.Render(rt).HTML, ">45<") {
		t.Error("re-render should show the new minute")
	}
}

// TestWidgetParityMenuItems covers the menu's items prop: action rows with
// icons that dispatch their onPress, and a disabled row without a handler.
func TestWidgetParityMenuItems(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		`id="mni"`, ">File ▾<", // trigger
		">New Tab<", ">Share<", // enabled items
		"M12 5v14M5 12h14", // plus icon SVG path on the first item
	} {
		if !strings.Contains(html, m) {
			t.Errorf("menu items showcase should render %q", m)
		}
	}
	// the disabled row renders dimmed and carries no click handler (search
	// within the items-based menu — the children-based one has a "Delete" too)
	mni := html[strings.Index(html, `id="mni"`):]
	di := strings.Index(mni, ">Delete<")
	dseg := mni[strings.LastIndex(mni[:di], `<div style=`):di]
	if !strings.Contains(dseg, "opacity:.45") {
		t.Error("disabled menu item should render dimmed")
	}
	if strings.Contains(dseg, "onclick") {
		t.Error("disabled menu item must not dispatch")
	}
	// an enabled row dispatches its onPress via qorm(h)
	ni := strings.Index(mni, ">New Tab<")
	nseg := mni[strings.LastIndex(mni[:ni], `<div style=`):ni]
	if !strings.Contains(nseg, `onclick="qorm(`) {
		t.Error("enabled menu item should dispatch its onPress")
	}
}

// TestWidgetParityBoundAutocomplete covers autocomplete options bound to a
// state array (static arrays still work — see TestWidgetParityGestureAutocomplete).
func TestWidgetParityBoundAutocomplete(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)
	html := render.Render(rt).HTML
	for _, m := range []string{
		`<datalist id="acb-ac"`, `list="acb-ac"`, // bound autocomplete datalist
		`value="Cameroon"`, // a suggestion from state.countries
	} {
		if !strings.Contains(html, m) {
			t.Errorf("bound autocomplete showcase should render %q", m)
		}
	}
	// the datalist follows state
	rt.State["countries"] = append(rt.State["countries"].([]any), "Chad")
	if !strings.Contains(render.Render(rt).HTML, `value="Chad"`) {
		t.Error("options should re-render from state")
	}
}

// TestWidgetParityIgnorePointer covers IgnorePointer/AbsorbPointer: the subtree
// renders (ids stay wired) but is transparent to pointer events, and the
// wrapper stays out of layout.
func TestWidgetParityIgnorePointer(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		`id="ignp"`, "display:contents;pointer-events:none", // the wrapper
		`id="ignb"`, ">Cannot click me<", // the child still renders
	} {
		if !strings.Contains(html, m) {
			t.Errorf("ignorepointer showcase should render %q", m)
		}
	}
}

// TestDatePickerModal covers the date-dialog recipe: a modal whose open is
// bound to state, hosting a datepicker with Done/Cancel actions.
func TestDatePickerModal(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)
	// closed by default — no modal markup at all
	if strings.Contains(render.Render(rt).HTML, `id="dpm"`) {
		t.Error("date modal should stay closed while showDateDlg is false")
	}
	// the trigger opens it; the datepicker wheels render inside
	rt.Dispatch("openDateDlg", nil)
	html := render.Render(rt).HTML
	for _, m := range []string{
		`id="dpm"`, `role="dialog"`, ">Pick a date<", // modal open
		`id="dpd"`, "scroll-snap-type:y mandatory", // the datepicker wheels
		">Done<", ">Cancel<",
	} {
		if !strings.Contains(html, m) {
			t.Errorf("open date modal should render %q", m)
		}
	}
	// Done closes it again
	rt.Dispatch("closeDateDlg", nil)
	if strings.Contains(render.Render(rt).HTML, `id="dpm"`) {
		t.Error("Done should close the date modal")
	}
}

// TestWidgetParitySearchBar covers the searchbar: the two-way-bound input, the
// anchored results panel rendered from a bound items array (label/detail/icon),
// the client-side filter/close hooks, and onSelect dispatching the chosen
// entry's {label} as a plain string.
func TestWidgetParitySearchBar(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)
	html := render.Render(rt).HTML
	for _, m := range []string{
		`id="sb"`, `class="qorm-search"`, "qorm-search-panel", // input + anchored panel
		`placeholder="Search countries…"`,                                     // hint
		`data-state="query"`,                                                  // value two-way bound
		`oninput="qormSearch(this)"`, `onkeydown="qormSearchKey(this,event)"`, // filter + Escape
		`data-label="Cameroon"`, ">Africa<", // an entry from the bound array + its detail
		"qormSearchPick", // entry click fills + dispatches
		"M3 12h18",       // the globe icon on the Canada entry
	} {
		if !strings.Contains(html, m) {
			t.Errorf("searchbar showcase should render %q", m)
		}
	}
	// the panel follows the bound items array
	rt.State["countryItems"] = append(rt.State["countryItems"].([]any), map[string]any{"label": "Chad", "detail": "Africa"})
	if !strings.Contains(render.Render(rt).HTML, `data-label="Chad"`) {
		t.Error("items should re-render from state")
	}
	// picking an entry dispatches onSelect with its {label} — a plain string
	rt.Dispatch("setCountry", map[string]any{"label": "Chile"})
	if rt.State["country"] != "Chile" {
		t.Errorf("onSelect should set state.country, got %v", rt.State["country"])
	}
	if rt.State["query"] != "Chile" {
		t.Errorf("picking should fill the query text, got %v", rt.State["query"])
	}
	// the selection shows up in the other state-bound control (data flows
	// through state only, never widget-to-widget)
	if !strings.Contains(render.Render(rt).HTML, `value="Chile"`) {
		t.Error("the country-bound autocomplete should reflect the selection")
	}
}

// TestWidgetParitySegmentedMultiple covers segmented with multiple:true
// (ToggleButtons): the bound value path holds an array of selected values,
// membership marks the pressed options, and clicking dispatches onChange with
// {value} — the fmtToggle action's state.toggle flips membership both ways.
func TestWidgetParitySegmentedMultiple(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)
	html := render.Render(rt).HTML
	segm := html[strings.Index(html, `id="segm"`):]
	segm = segm[:strings.Index(segm, `</div>`)]
	for _, m := range []string{
		`role="group"`,
		`aria-pressed="true"`, // bold is selected initially
		`onclick="qorm(`,      // options dispatch (no data-state: the array must not be scalar-squashed)
		">Bold<", ">Italic<", ">Underline<",
	} {
		if !strings.Contains(segm, m) {
			t.Errorf("multi segmented showcase should render %q", m)
		}
	}
	if strings.Contains(segm, `type="radio"`) || strings.Contains(segm, "data-state") {
		t.Error("multiple selection must not use scalar radio/state-fold wiring")
	}
	pressed := strings.Count(segm, `aria-pressed="true"`)
	if pressed != 1 {
		t.Errorf("initially exactly one option (bold) should be pressed, got %d", pressed)
	}
	// toggle italic on, bold off — membership flips both ways through state
	rt.Dispatch("fmtToggle", map[string]any{"value": "italic"})
	rt.Dispatch("fmtToggle", map[string]any{"value": "bold"})
	got := rt.State["formats"].([]any)
	if len(got) != 1 || got[0] != "italic" {
		t.Fatalf("after toggling italic on and bold off, formats = %v", got)
	}
	segm2 := render.Render(rt).HTML
	segm2 = segm2[strings.Index(segm2, `id="segm"`):]
	segm2 = segm2[:strings.Index(segm2, `</div>`)]
	pressedOf := func(label string) string {
		i := strings.Index(segm2, ">"+label+"<")
		span := segm2[strings.LastIndex(segm2[:i], `<span role="button"`):i]
		if strings.Contains(span, `aria-pressed="true"`) {
			return "true"
		}
		return "false"
	}
	if pressedOf("Bold") != "false" {
		t.Error("bold should render unpressed after being toggled off")
	}
	if pressedOf("Italic") != "true" {
		t.Error("italic should render pressed after being toggled on")
	}
	// the single-select segmented beside it still works (unchanged behavior)
	if !strings.Contains(render.Render(rt).HTML, `role="radiogroup"`) {
		t.Error("single-select segmented should keep its radio wiring")
	}
}

// TestWidgetParityTableColumnWidths covers the column `width` prop on both
// table and datatable: numbers become px, CSS strings pass through, and a
// <colgroup> sizes the columns (datatable's checkbox column stays unsized).
func TestWidgetParityTableColumnWidths(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	// table: Name 180px (number → px), Role 30% (CSS string passthrough)
	tbl := html[strings.Index(html, `id="tbl"`):]
	tbl = tbl[:strings.Index(tbl, "<thead>")]
	for _, m := range []string{
		"<colgroup>",
		`<col style="width:180px">`,
		`<col style="width:30%">`,
	} {
		if !strings.Contains(tbl, m) {
			t.Errorf("table with column widths should render %q", m)
		}
	}
	// datatable: ID column 72px; the leading checkbox column is an unsized <col>
	dt := html[strings.Index(html, `id="dt"`):]
	dt = dt[:strings.Index(dt, "<thead>")]
	if !strings.Contains(dt, "<colgroup><col><col style=\"width:72px\">") {
		t.Errorf("datatable should size the ID column and leave the checkbox column unsized, got: %s", dt)
	}
}
