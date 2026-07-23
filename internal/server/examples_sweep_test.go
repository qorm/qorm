package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// exampleServer boots an example app through the real runtime + HTTP surface
// and returns the server, its loopback URL and the page-embedded human token
// (the GET / also primes the handler table, like a browser rendering the page).
func exampleServer(t *testing.T, name string) (*Server, string, string) {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", name))
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	s := New(qrt.New(app))
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	tok := pageEventToken(t, ts.URL)
	return s, ts.URL, tok
}

// sweepEvent posts a human event (handler index + folded inputs) exactly like
// the browser client and returns the re-rendered body with the new revision.
func sweepEvent(t *testing.T, base, tok string, h int, inputs map[string]any) (string, int64) {
	t.Helper()
	body := map[string]any{"h": h}
	if inputs != nil {
		body["inputs"] = inputs
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, base+"/event", strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Qorm-Token", tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post /event: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post /event h=%d: status %d", h, resp.StatusCode)
	}
	rev, _ := strconv.ParseInt(resp.Header.Get("X-Qorm-Rev"), 10, 64)
	var sb strings.Builder
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String(), rev
}

// handlerIdx finds the index of the first handler with the given action name
// whose args/scope satisfy match (nil = any). The index is what the rendered
// HTML's onclick="qorm(N)" refers to.
func handlerIdx(t *testing.T, s *Server, name string, match func(h render.Handler) bool) int {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, h := range s.handlers {
		if h.Name == name && (match == nil || match(h)) {
			return i
		}
	}
	t.Fatalf("no handler named %q in the rendered page (have %d handlers)", name, len(s.handlers))
	return -1
}

// TestSweepCounter drives the counter example end to end over loopback HTTP:
// human clicks mutate shared state and the re-rendered HTML reflects them.
func TestSweepCounter(t *testing.T) {
	s, base, tok := exampleServer(t, "counter")

	plus := handlerIdx(t, s, "increment", nil)
	minus := handlerIdx(t, s, "decrement", nil)

	html, rev1 := sweepEvent(t, base, tok, plus, nil)
	if !strings.Contains(html, ">1<") {
		t.Fatalf("count=1 missing after +:\n%s", html)
	}
	html, rev2 := sweepEvent(t, base, tok, plus, nil)
	if !strings.Contains(html, ">2<") {
		t.Fatalf("count=2 missing after second +:\n%s", html)
	}
	html, _ = sweepEvent(t, base, tok, minus, nil)
	if !strings.Contains(html, ">1<") {
		t.Fatalf("count=1 missing after -:\n%s", html)
	}
	if !(rev1 == 1 && rev2 == 2) {
		t.Fatalf("revisions must advance per event: %d, %d", rev1, rev2)
	}

	// The clicks are attributed to the human in the shared activity log.
	s.actMu.Lock()
	var found bool
	for _, e := range s.activity {
		if e.Source == "human" && e.Detail == "dispatch increment" {
			found = true
		}
	}
	s.actMu.Unlock()
	if !found {
		t.Fatal("human dispatch not recorded in the activity log")
	}
}

// TestSweepTodo drives the todo example: two-way input binding, adding to a
// data-bound list (re-render shows the new row), and a per-item list action.
func TestSweepTodo(t *testing.T) {
	s, base, tok := exampleServer(t, "todo")

	// Two-way binding: typing folds input values into state on every event.
	html, _ := sweepEvent(t, base, tok, -1, map[string]any{"inputValue": "Write the launch plan"})
	if !strings.Contains(html, `value="Write the launch plan"`) {
		t.Fatalf("bound input must re-render with the typed value:\n%s", html)
	}

	// Add: the button's args re-evaluate against fresh state, so the typed
	// value in the same request becomes the new item.
	add := handlerIdx(t, s, "addTodo", nil)
	html, _ = sweepEvent(t, base, tok, add, map[string]any{"inputValue": "Write the launch plan"})
	for _, want := range []string{"Master QORM Layout", "Build a premium app", "Write the launch plan"} {
		if !strings.Contains(html, want) {
			t.Fatalf("list re-render must show every item, missing %q:\n%s", want, html)
		}
	}
	s.mu.Lock()
	items, _ := s.rt.State["items"].([]any)
	inputAfter := s.rt.State["inputValue"]
	s.mu.Unlock()
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3 after addTodo", len(items))
	}
	if inputAfter != "" {
		t.Fatalf("inputValue must clear after adding, got %v", inputAfter)
	}

	// Toggle one specific row: each list item renders its OWN handler (scoped
	// to that item) — find the one for id=2 and flip its done flag.
	toggle := handlerIdx(t, s, "toggleTodo", func(h render.Handler) bool {
		item, _ := h.Scope["item"].(map[string]any)
		return item != nil && fmt.Sprint(item["id"]) == "2"
	})
	sweepEvent(t, base, tok, toggle, nil)
	s.mu.Lock()
	items, _ = s.rt.State["items"].([]any)
	row, _ := items[1].(map[string]any)
	done := row["done"]
	s.mu.Unlock()
	if done != true {
		t.Fatalf("toggleTodo must flip item 2's done to true, got %v", done)
	}
}

// TestSweepDashboard drives the dashboard example: the segmented control binds
// through state, and a reported viewport swap swaps the responsive `when`
// branches in the pushed re-render.
func TestSweepDashboard(t *testing.T) {
	s, base, tok := exampleServer(t, "dashboard")

	// Segmented single-choice is a state-bound radio group: picking "Year"
	// folds into state.range like the browser client does.
	html, _ := sweepEvent(t, base, tok, -1, map[string]any{"range": "Year"})
	s.mu.Lock()
	rng := s.rt.State["range"]
	s.mu.Unlock()
	if rng != "Year" {
		t.Fatalf("range = %v, want Year", rng)
	}
	if !strings.Contains(html, `value="Year" checked`) {
		t.Fatalf("segmented must re-render with Year checked:\n%s", html)
	}

	// Responsive branches: unknown viewport renders the narrow (else) branch;
	// reporting a wide viewport re-renders + pushes the grid branch.
	if html := renderCurrent(s); strings.Contains(html, `id="stats"`) {
		t.Fatalf("unknown viewport should render the narrow branch (no #stats grid):\n%s", html)
	}
	if code := postViewport(t, base, tok, 1440, 900); code != http.StatusNoContent {
		t.Fatalf("post /viewport: %d", code)
	}
	if html := renderCurrent(s); !strings.Contains(html, `id="stats"`) {
		t.Fatalf("1440x900 viewport must swap in the wide grid branch:\n%s", html)
	}
}

// TestSweepLogin drives the login example: the secure field renders as a
// password input, and the (action-less) login button dispatches cleanly as a
// no-op — the page re-renders intact instead of erroring.
func TestSweepLogin(t *testing.T) {
	s, base, tok := exampleServer(t, "login")

	html, _ := sweepEvent(t, base, tok, -1, nil)
	if !strings.Contains(html, `type="password"`) {
		t.Fatalf("the secure input must render as type=password:\n%s", html)
	}
	if !strings.Contains(html, "Welcome Back") {
		t.Fatalf("login page must render its title:\n%s", html)
	}

	login := handlerIdx(t, s, "performLogin", nil)
	html, _ = sweepEvent(t, base, tok, login, nil)
	if !strings.Contains(html, "Welcome Back") {
		t.Fatalf("dispatching the (unimplemented) login action must stay a clean no-op:\n%s", html)
	}
}

// TestSweepForm drives the form example: validation expressions run on the
// server during dispatch — empty fields surface field errors, valid input
// creates the account — and every status re-renders into the HTML.
func TestSweepForm(t *testing.T) {
	s, base, tok := exampleServer(t, "form")
	submit := handlerIdx(t, s, "submit", nil)

	// Empty submission: required-field errors.
	html, _ := sweepEvent(t, base, tok, submit, map[string]any{"email": "", "password": ""})
	for _, want := range []string{"Email is required", "Password is required", "Please fix the highlighted fields"} {
		if !strings.Contains(html, want) {
			t.Fatalf("empty submit must show %q:\n%s", want, html)
		}
	}

	// Short password: the length rule fires while the email error clears.
	html, _ = sweepEvent(t, base, tok, submit, map[string]any{"email": "ada@example.com", "password": "pw"})
	if !strings.Contains(html, "Use at least 6 characters") {
		t.Fatalf("short password must trigger the length rule:\n%s", html)
	}
	if strings.Contains(html, "Email is required") {
		t.Fatalf("a valid email must clear its field error:\n%s", html)
	}

	// Valid submission: account created.
	html, _ = sweepEvent(t, base, tok, submit, map[string]any{"email": "ada@example.com", "password": "secret123"})
	if !strings.Contains(html, "Account created") {
		t.Fatalf("valid submit must report Account created:\n%s", html)
	}
	s.mu.Lock()
	status := s.rt.State["status"]
	s.mu.Unlock()
	if status != "Account created" {
		t.Fatalf("status = %v", status)
	}
}

// TestSweepDragDrop simulates the browser's drag-and-drop gesture (qormPostDrop
// posts the drop handler + _dragData) against the dragdrop example's kanban:
// dropping card t1 on the "doing" column moves it there in shared state.
func TestSweepDragDrop(t *testing.T) {
	s, base, tok := exampleServer(t, "dragdrop")

	html, _ := sweepEvent(t, base, tok, -1, nil)
	if !strings.Contains(html, `data-qorm-drag="t1"`) {
		t.Fatalf("draggable cards must carry their drag payload:\n%s", html)
	}
	if !strings.Contains(html, "data-qorm-drop=") {
		t.Fatalf("drop targets must carry their handler index:\n%s", html)
	}

	dropDoing := handlerIdx(t, s, "moveDoing", nil)
	html, _ = sweepEvent(t, base, tok, dropDoing, map[string]any{"_dragData": "t1"})

	s.mu.Lock()
	items, _ := s.rt.State["items"].([]any)
	var col string
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m["id"] == "t1" {
			col, _ = m["col"].(string)
		}
	}
	s.mu.Unlock()
	if col != "doing" {
		t.Fatalf("dropping t1 on the doing target must set col=doing, got %q", col)
	}
	for _, label := range []string{"Design the icon", "Write the docs", "Ship v0.2.2", "Cut the release"} {
		if !strings.Contains(html, label) {
			t.Fatalf("the re-rendered board must keep every card, missing %q:\n%s", label, html)
		}
	}
}

// TestSweepReorder simulates the browser's drag-to-reorder gesture
// (qormPostReorder posts _reorderFrom/_reorderTo) against the reorder example:
// the list order persists in state via the state.move step.
func TestSweepReorder(t *testing.T) {
	s, base, tok := exampleServer(t, "reorder")

	labels := func() []string {
		s.mu.Lock()
		defer s.mu.Unlock()
		items, _ := s.rt.State["items"].([]any)
		out := make([]string, 0, len(items))
		for _, it := range items {
			m, _ := it.(map[string]any)
			out = append(out, fmt.Sprint(m["label"]))
		}
		return out
	}
	before := labels()

	reorder := handlerIdx(t, s, "onReorder", nil)
	html, _ := sweepEvent(t, base, tok, reorder, map[string]any{"_reorderFrom": 0, "_reorderTo": 2})

	// state.move removes at `from` and inserts at `to`, so the dragged item
	// ends up AT index 2 (matching the drop position the client computes):
	// [a b c d] -> [b c a d].
	after := labels()
	const from, to = 0, 2
	want := append([]string{}, before[from+1:to+1]...)
	want = append(want, before[from])
	want = append(want, before[to+1:]...)
	if !equalStrings(after, want) {
		t.Fatalf("reorder %d->%d: %v -> %v, want %v", from, to, before, after, want)
	}
	for _, l := range after {
		if !strings.Contains(html, l) {
			t.Fatalf("re-rendered list missing %q:\n%s", l, html)
		}
	}
}

// TestSweepComponentsOverlayDismiss drives the overlay default behavior: the
// modal's state-bound `open` gets a built-in __dismiss handler (backdrop tap /
// Escape on the client); dispatching it closes the overlay — no app action.
func TestSweepComponentsOverlayDismiss(t *testing.T) {
	s, base, tok := exampleServer(t, "components")

	// showModal starts true: the modal and its dismiss handler are rendered.
	html, _ := sweepEvent(t, base, tok, -1, nil)
	if !strings.Contains(html, "This is a modal dialog.") {
		t.Fatalf("the initially-open modal must render:\n%s", html)
	}
	dismiss := handlerIdx(t, s, "__dismiss", func(h render.Handler) bool {
		return h.Args["path"] == "showModal"
	})
	html, _ = sweepEvent(t, base, tok, dismiss, nil)
	if strings.Contains(html, "This is a modal dialog.") {
		t.Fatalf("__dismiss must close the modal:\n%s", html)
	}
	s.mu.Lock()
	open := s.rt.State["showModal"]
	s.mu.Unlock()
	if open != false {
		t.Fatalf("showModal = %v after __dismiss, want false", open)
	}
}

// TestSweepComponentsTableSort drives the table's built-in __sort: clicking a
// column sorts the bound rows ascending; clicking the sorted column again
// flips to descending — exactly what the header button dispatches.
func TestSweepComponentsTableSort(t *testing.T) {
	s, base, tok := exampleServer(t, "components")

	sortName := handlerIdx(t, s, "__sort", func(h render.Handler) bool {
		return h.Args["column"] == "name" && h.Args["data"] == "rows"
	})

	names := func() []string {
		s.mu.Lock()
		defer s.mu.Unlock()
		rows, _ := s.rt.State["rows"].([]any)
		out := make([]string, 0, len(rows))
		for _, r := range rows {
			m, _ := r.(map[string]any)
			out = append(out, fmt.Sprint(m["name"]))
		}
		return out
	}

	if _, _ = sweepEvent(t, base, tok, sortName, nil); !equalStrings(names(), []string{"Ada", "Linus", "Zoe"}) {
		t.Fatalf("first name-sort must be ascending, got %v", names())
	}
	html, _ := sweepEvent(t, base, tok, sortName, nil)
	if !equalStrings(names(), []string{"Zoe", "Linus", "Ada"}) {
		t.Fatalf("re-sorting the sorted column must flip to descending, got %v", names())
	}
	// The re-rendered table marks the sorted column + direction.
	if !strings.Contains(html, "qdt-sort") {
		t.Fatalf("sortable headers must render their sort buttons:\n%s", html)
	}
	s.mu.Lock()
	field, dir := s.rt.State["rowsSort"], s.rt.State["rowsDir"]
	s.mu.Unlock()
	if field != "name" || dir != "desc" {
		t.Fatalf("recorded sort = %v/%v, want name/desc", field, dir)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSweepNavigationDeepLink drives deep-link entry + back navigation over
// the navigation example: the URL's scene+params render immediately, and
// /navigate back returns to the entry scene.
func TestSweepNavigationDeepLink(t *testing.T) {
	s, base, tok := exampleServer(t, "navigation")

	resp, err := http.Get(base + "/?scene=profile&userId=u-42")
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 1<<16)
	n := 0
	for n < len(body) {
		m, err := resp.Body.Read(body[n:])
		n += m
		if err != nil {
			break
		}
	}
	resp.Body.Close()
	page := string(body[:n])
	if !strings.Contains(page, "User id: u-42") {
		t.Fatalf("deep link must render the profile with route.userId:\n%s", page)
	}

	if code, _ := doJSON(t, http.MethodPost, base+"/navigate", tok, "", `{"back":true}`); code != http.StatusNoContent {
		t.Fatalf("navigate back: %d", code)
	}
	s.mu.Lock()
	scene := s.rt.CurrentScene()
	s.mu.Unlock()
	if scene != "" && scene != "home" {
		t.Fatalf("back must return to the entry scene, got %q", scene)
	}
}

// TestSweepAllExamplesRenderClean is the headless equivalent of `qorm check`
// for the render layer: every scene of every example loads and renders with an
// empty Unknown list (no unrecognised widget types — the renderer's own
// self-verify surface), so the session's render edits shifted no app's output
// into an error state.
func TestSweepAllExamplesRenderClean(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("..", "..", "examples", "*", "qorm.json"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("enumerate examples: %v (%d)", err, len(dirs))
	}
	for _, manifest := range dirs {
		dir := filepath.Dir(manifest)
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			app, err := loader.LoadDir(dir)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			rt := qrt.New(app)
			for sceneID := range app.Scenes {
				res := render.RenderScene(rt, sceneID)
				if res.HTML == "" {
					t.Errorf("scene %q rendered empty HTML", sceneID)
				}
				if strings.Contains(res.HTML, "no scene to render") {
					t.Errorf("scene %q fell back to the empty render", sceneID)
				}
				if len(res.Unknown) > 0 {
					t.Errorf("scene %q has unrecognised widget types: %v", sceneID, res.Unknown)
				}
			}
		})
	}
}
