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
		"qorm-skel",   // skeleton
		"qormAcc",     // accordion toggle wired
		"Section one", // accordion header
		"★",           // rating
		"Members",     // breadcrumb last crumb
		">design<",    // tag
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

// TestFlutterParityPhase1 covers the Flutter-parity phase-1 widgets: AppBar,
// ListTile, Wrap, button variants, and FloatingActionButton.
func TestFlutterParityPhase1(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"height:calc(44px",          // appbar (+ safe-area inset)
		">Flutter Widgets<",         // appbar title
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

// TestFlutterParityChips covers the Chip family + RangeSlider.
func TestFlutterParityChips(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"border-radius:16px", // chips
		">React<", "⚛️",      // choice chip + avatar
		"✓",                              // selected filter chip check
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

// TestFlutterParityGridStepper covers GridView + Material Stepper.
func TestFlutterParityGridStepper(t *testing.T) {
	app, err := loader.LoadDir(examplesDir(t, "components"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	html := render.Render(qrt.New(app)).HTML
	for _, m := range []string{
		"grid-template-columns:repeat(2,1fr)", // gridview
		">Item 1<", ">Item 4<",                // grid cells from data
		">Shipping<", ">shipping body<", // active step (wizStep=1) content
		"✓", // completed step 0 marker
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

// TestFlutterParityPageDropdown covers PageView + DropdownButton.
func TestFlutterParityPageDropdown(t *testing.T) {
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

// TestFlutterParityGestureAutocomplete covers GestureDetector + Autocomplete.
func TestFlutterParityGestureAutocomplete(t *testing.T) {
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

// TestFlutterParityFormField covers TextFormField (InputDecoration + reactive
// validation) and CircularProgressIndicator.
func TestFlutterParityFormField(t *testing.T) {
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

// TestFlutterParityBadgeDivider covers Badge-on-child + VerticalDivider.
func TestFlutterParityBadgeDivider(t *testing.T) {
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

// TestCupertinoDialogs covers CupertinoAlertDialog + CupertinoActionSheet.
func TestCupertinoDialogs(t *testing.T) {
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

// TestCupertinoListSection covers the iOS inset grouped list + tile chevron.
func TestCupertinoListSection(t *testing.T) {
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

// TestCupertinoSliderActivityPicker covers the iOS slider restyle,
// CupertinoActivityIndicator and CupertinoPicker.
func TestCupertinoSliderActivityPicker(t *testing.T) {
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

// TestContextMenuRefresh covers CupertinoContextMenu + RefreshIndicator.
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
