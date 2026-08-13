//go:build !desktop

package main

import (
	"encoding/json"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"

	"github.com/qorm/qorm/internal/measure"
	"github.com/qorm/qorm/internal/render/canvas"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// canvasFlow is a superset of the existing WebView flow contract. dispatch
// and setState retain their established spellings; the pure canvas driver also
// supports real press/type/key/scroll input and deterministic wait steps.
type canvasFlow struct {
	Steps []canvasFlowStep `json:"steps"`
}

type canvasFlowStep struct {
	Name   string           `json:"name"`
	Do     canvasFlowDo     `json:"do"`
	Checks []map[string]any `json:"checks"`
}

type canvasFlowDo struct {
	SetState *canvasStateOp `json:"setState"`
	State    *canvasStateOp `json:"state"`
	Dispatch string         `json:"dispatch"`
	Args     map[string]any `json:"args"`
	Press    any            `json:"press"`
	Type     any            `json:"type"`
	Key      any            `json:"key"`
	Scroll   any            `json:"scroll"`
	Wait     any            `json:"wait"`
}

type canvasStateOp struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
}

type canvasFlowDriver struct {
	rt      *qrt.Runtime
	engine  *canvas.Engine
	surface *canvas.HeadlessSurface
	logical bool
}

func newCanvasFlowDriver(rt *qrt.Runtime, width, height int, logical bool) *canvasFlowDriver {
	if width <= 0 {
		width = 400
	}
	if height <= 0 {
		height = 820
	}
	d := &canvasFlowDriver{
		rt: rt, engine: canvas.NewEngine(rt, canvas.SoftwareRenderer{}),
		surface: canvas.NewHeadlessSurface(image.Pt(width, height)), logical: logical,
	}
	// Mount the graph once, then settle mount-only entrances exactly like
	// MeasureScene. Timers are not advanced by this settling operation.
	d.engine.DrawFrame(d.surface)
	d.engine.SettleEntrances()
	d.drawDirty()
	return d
}

func (d *canvasFlowDriver) drawDirty() {
	// Actions such as timeline onComplete may dirty one follow-up frame. Bound
	// the drain: a repeating timer/animation deliberately never settles and a
	// check step must not spin forever.
	for i := 0; i < 8; i++ {
		if i > 0 && !d.engine.Dirty() {
			break
		}
		d.engine.DrawFrame(d.surface)
	}
}

func (d *canvasFlowDriver) rows() []byte {
	return d.engine.CollectMeasureOpts(canvas.MeasureOpts{Logical: d.logical})
}

func evalCanvasFlow(rt *qrt.Runtime, width, height int, logical bool, checksJSON []byte) ([]byte, error) {
	var flow canvasFlow
	if err := json.Unmarshal(checksJSON, &flow); err != nil {
		return nil, fmt.Errorf("bad flow JSON: %w", err)
	}
	if flow.Steps == nil {
		return nil, fmt.Errorf(`bad flow JSON: missing "steps" array`)
	}
	d := newCanvasFlowDriver(rt, width, height, logical)
	steps := make([]map[string]any, 0, len(flow.Steps))
	allPass := true
	for i, st := range flow.Steps {
		action, err := d.apply(st.Do)
		if err != nil {
			return nil, fmt.Errorf("flow step %d (%s): %w", i+1, st.Name, err)
		}
		d.drawDirty()
		cb, _ := json.Marshal(st.Checks)
		rep, err := measure.Eval(rt, d.rows(), cb)
		if err != nil {
			return nil, err
		}
		var rd map[string]any
		if err := json.Unmarshal(rep, &rd); err != nil {
			return nil, err
		}
		if ok, _ := rd["ok"].(bool); !ok {
			allPass = false
		}
		steps = append(steps, map[string]any{
			"step": i + 1, "name": st.Name, "action": action, "result": rd,
		})
	}
	return json.MarshalIndent(map[string]any{"app": rt.App.Name, "ok": allPass, "steps": steps}, "", "  ")
}

func (d *canvasFlowDriver) apply(op canvasFlowDo) (string, error) {
	n := 0
	if op.SetState != nil {
		n++
	}
	if op.State != nil {
		n++
	}
	if op.Dispatch != "" {
		n++
	}
	if op.Press != nil {
		n++
	}
	if op.Type != nil {
		n++
	}
	if op.Key != nil {
		n++
	}
	if op.Scroll != nil {
		n++
	}
	if op.Wait != nil {
		n++
	}
	if n == 0 {
		return "(none)", nil
	}
	if n != 1 {
		return "", fmt.Errorf("do must contain exactly one operation")
	}

	state := op.SetState
	if state == nil {
		state = op.State
	}
	if state != nil {
		if state.Path == "" {
			return "", fmt.Errorf("state path is required")
		}
		if !d.rt.SetStatePath(state.Path, state.Value) {
			return "", fmt.Errorf("state path %q is read-only", state.Path)
		}
		d.engine.MarkDirty()
		return fmt.Sprintf("set_state %s=%v", state.Path, state.Value), nil
	}
	if op.Dispatch != "" {
		d.rt.Dispatch(op.Dispatch, op.Args)
		d.engine.MarkDirty()
		return "dispatch " + op.Dispatch, nil
	}
	if op.Press != nil {
		id, err := flowID(op.Press)
		if err != nil {
			return "", fmt.Errorf("press: %w", err)
		}
		if err := d.reveal(id); err != nil {
			return "", err
		}
		b, _ := d.engine.BoundsByID(id)
		x, y := float64(b.Min.X+b.Max.X)/2, float64(b.Min.Y+b.Max.Y)/2
		d.engine.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: x, Y: y})
		d.engine.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: x, Y: y, Buttons: 1})
		d.engine.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: x, Y: y})
		return "press #" + id, nil
	}
	if op.Type != nil {
		id, text, clear, err := flowType(op.Type)
		if err != nil {
			return "", err
		}
		if err := d.focus(id, false); err != nil {
			return "", err
		}
		if clear {
			d.engine.HandleKey(canvas.KeyInput{Key: "a", Meta: true, Down: true})
			d.engine.HandleKey(canvas.KeyInput{Key: "a", Meta: true, Down: false})
			d.engine.HandleKey(canvas.KeyInput{Key: "backspace", Down: true})
			d.engine.HandleKey(canvas.KeyInput{Key: "backspace", Down: false})
		}
		for _, r := range text {
			d.engine.HandleKey(canvas.KeyInput{Key: "rune", Rune: r, Down: true})
			d.engine.HandleKey(canvas.KeyInput{Key: "rune", Rune: r, Down: false})
		}
		return fmt.Sprintf("type #%s %q", id, text), nil
	}
	if op.Key != nil {
		k, id, err := flowKey(op.Key)
		if err != nil {
			return "", err
		}
		if id != "" {
			if err := d.focus(id, true); err != nil {
				return "", err
			}
		}
		d.engine.HandleKey(k)
		if k.Down {
			k.Down = false
			d.engine.HandleKey(k)
		}
		return "key " + k.Key, nil
	}
	if op.Scroll != nil {
		id, dx, dy, ctrl, err := flowScroll(op.Scroll)
		if err != nil {
			return "", err
		}
		if err := d.reveal(id); err != nil {
			return "", err
		}
		b, _ := d.engine.BoundsByID(id)
		x, y := float64(b.Min.X+b.Max.X)/2, float64(b.Min.Y+b.Max.Y)/2
		d.engine.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: x, Y: y})
		if !d.engine.HandleScroll(canvas.ScrollInput{DX: dx, DY: dy, Ctrl: ctrl}) {
			return "", fmt.Errorf("scroll #%s consumed no delta", id)
		}
		return fmt.Sprintf("scroll #%s dx=%g dy=%g", id, dx, dy), nil
	}
	if op.Wait != nil {
		dur, err := flowDuration(op.Wait)
		if err != nil {
			return "", err
		}
		// First record any effect triggered by the preceding step, then move all
		// canvas deadlines forward deterministically and render the new instant.
		d.drawDirty()
		d.engine.AdvanceTime(dur)
		return "wait " + dur.String(), nil
	}
	return "", fmt.Errorf("unsupported operation")
}

func (d *canvasFlowDriver) reveal(id string) error {
	if !d.engine.RevealID(id) {
		return fmt.Errorf("target #%s not found", id)
	}
	d.drawDirty()
	if _, ok := d.engine.BoundsByID(id); !ok {
		return fmt.Errorf("target #%s not rendered", id)
	}
	return nil
}

func (d *canvasFlowDriver) focus(id string, keyboard bool) error {
	if err := d.reveal(id); err != nil {
		return err
	}
	if !d.engine.FocusID(id, keyboard) {
		return fmt.Errorf("target #%s cannot be focused", id)
	}
	d.drawDirty()
	return nil
}

func flowID(v any) (string, error) {
	if s, ok := v.(string); ok && s != "" {
		return strings.TrimPrefix(s, "#"), nil
	}
	if m, ok := v.(map[string]any); ok {
		if s, _ := m["id"].(string); s != "" {
			return strings.TrimPrefix(s, "#"), nil
		}
	}
	return "", fmt.Errorf("target id is required")
}

func flowType(v any) (id, text string, clear bool, err error) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", "", false, fmt.Errorf(`type must be {"id":…, "text":…}`)
	}
	id, err = flowID(m)
	if err != nil {
		return
	}
	text, ok = m["text"].(string)
	if !ok {
		err = fmt.Errorf("type text is required")
		return
	}
	clear, _ = m["clear"].(bool)
	return
}

func flowKey(v any) (canvas.KeyInput, string, error) {
	k := canvas.KeyInput{Down: true}
	id := ""
	switch x := v.(type) {
	case string:
		k.Key = x
	case map[string]any:
		k.Key, _ = x["key"].(string)
		id, _ = x["id"].(string)
		k.Shift, _ = x["shift"].(bool)
		k.Ctrl, _ = x["ctrl"].(bool)
		k.Alt, _ = x["alt"].(bool)
		k.Meta, _ = x["meta"].(bool)
	default:
		return k, "", fmt.Errorf(`key must be a string or {"key":…}`)
	}
	k.Key = normalizeFlowKey(k.Key)
	if k.Key == "" {
		return k, "", fmt.Errorf("key name is required")
	}
	return k, strings.TrimPrefix(id, "#"), nil
}

func normalizeFlowKey(k string) string {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "enter":
		return "return"
	case "esc":
		return "escape"
	case "arrowup":
		return "up"
	case "arrowdown":
		return "down"
	case "arrowleft":
		return "left"
	case "arrowright":
		return "right"
	default:
		return strings.ToLower(strings.TrimSpace(k))
	}
}

func flowScroll(v any) (id string, dx, dy float64, ctrl bool, err error) {
	m, ok := v.(map[string]any)
	if !ok {
		err = fmt.Errorf(`scroll must be {"id":…, "dy":…}`)
		return
	}
	id, err = flowID(m)
	if err != nil {
		return
	}
	dx, _ = numberAny(m["dx"])
	dy, _ = numberAny(m["dy"])
	ctrl, _ = m["ctrl"].(bool)
	if dx == 0 && dy == 0 {
		err = fmt.Errorf("scroll needs non-zero dx or dy")
	}
	return
}

func flowDuration(v any) (time.Duration, error) {
	if f, ok := numberAny(v); ok {
		if f < 0 {
			return 0, fmt.Errorf("wait must be non-negative")
		}
		return time.Duration(f * float64(time.Millisecond)), nil
	}
	if s, ok := v.(string); ok {
		if d, err := time.ParseDuration(s); err == nil && d >= 0 {
			return d, nil
		}
	}
	return 0, fmt.Errorf("wait must be milliseconds or a duration string")
}

func numberAny(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}
