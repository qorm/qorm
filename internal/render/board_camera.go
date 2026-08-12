package render

import (
	"math"
	"strconv"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// BoardCameraParams is the pure input for board follow-camera pan math.
// Semantics match canvas applyBoardCamera (internal/render/canvas/board_camera.go).
type BoardCameraParams struct {
	TargetX, TargetY float64
	CenterX, CenterY bool
	// ViewW/ViewH are the viewport size in physical px used for centering.
	ViewW, ViewH float64
	// DeadZone (px): left-anchored band for centerX follow (0 = always centre).
	DeadZone float64
	// MaxX/MaxY clamp the resolved pan when finite; math.Inf(1) = no clamp.
	// Callers that omit a max must pass +Inf (not 0).
	MaxX, MaxY float64
	// CurPanX is prior pan for dead-zone sticky behaviour (HTML paint may pass 0).
	CurPanX float64
}

// ComputeBoardCameraPan returns the content-group pan so the target lands per
// cameraCenter / dead-zone / cameraMax rules. Pure; no runtime or DOM.
func ComputeBoardCameraPan(p BoardCameraParams) (panX, panY float64) {
	cx, cy := p.TargetX, p.TargetY
	viewW, viewH := p.ViewW, p.ViewH
	if viewW <= 0 {
		viewW = 16
	}
	if viewH <= 0 {
		viewH = 12
	}

	if !p.CenterX && !p.CenterY {
		return -cx, -cy
	}

	desiredX, desiredY := 0.0, 0.0
	if p.CenterX {
		desiredX = -cx + viewW/2
	} else {
		desiredX = -cx
	}
	if p.CenterY {
		desiredY = -cy + viewH/2
	} else {
		desiredY = 0
	}

	if p.CenterX && p.DeadZone > 0 {
		curP := p.CurPanX
		if cx+curP > p.DeadZone {
			desiredX = p.DeadZone - cx
		} else {
			desiredX = curP
		}
	}

	if p.CenterX && !math.IsInf(p.MaxX, 1) {
		if desiredX > 0 {
			desiredX = 0
		} else if desiredX < -p.MaxX {
			desiredX = -p.MaxX
		}
	}
	if p.CenterY && !math.IsInf(p.MaxY, 1) {
		if desiredY > 0 {
			desiredY = 0
		} else if desiredY < -p.MaxY {
			desiredY = -p.MaxY
		}
	}
	return desiredX, desiredY
}

// resolveHTMLBoardCamera evaluates board camera* props into a pan.
// ok is false when cameraTarget is absent or unusable (caller falls through).
func resolveHTMLBoardCamera(n *model.Node, rt *runtime.Runtime) (panX, panY float64, ok bool) {
	if n == nil {
		return 0, 0, false
	}
	raw, has := n.Prop("cameraTarget")
	if !has {
		return 0, 0, false
	}
	target := resolveCameraTargetMap(raw, rt)
	if target == nil {
		return 0, 0, false
	}
	tx, okt := toFloatAny(target["x"])
	ty, oky := toFloatAny(target["y"])
	if !okt || !oky || math.IsNaN(tx) || math.IsNaN(ty) {
		return 0, 0, false
	}

	cell := 1.0
	if raw, ok := n.Prop("cameraCell"); ok {
		if v, ok := toFloatAny(raw); ok && v > 0 {
			cell = v
		}
	}

	screenW, screenH := htmlBoardScreen(n, rt)
	viewW, viewH := boardViewportPx(n, screenW, screenH)
	if _, hasViewport := n.Prop("cameraViewport"); hasViewport {
		viewW *= cell
		viewH *= cell
	}

	centerX, centerY := parseCameraCenter(n)
	dz := 0.0
	if raw, ok := n.Prop("cameraDeadZone"); ok {
		if v, ok := toFloatAny(raw); ok && v > 0 {
			dz = v
		}
	}
	maxX, maxY := math.Inf(1), math.Inf(1)
	if raw, ok := n.Prop("cameraMax"); ok {
		switch v := raw.(type) {
		case float64:
			maxX, maxY = v, v
		case int:
			maxX, maxY = float64(v), float64(v)
		case int64:
			maxX, maxY = float64(v), float64(v)
		case map[string]any:
			if x, ok := toFloatAny(v["x"]); ok {
				maxX = x
			}
			if y, ok := toFloatAny(v["y"]); ok {
				maxY = y
			}
		case string:
			if f, ok := toFloatAny(v); ok {
				maxX, maxY = f, f
			}
		}
	}

	panX, panY = ComputeBoardCameraPan(BoardCameraParams{
		TargetX:  tx,
		TargetY:  ty,
		CenterX:  centerX,
		CenterY:  centerY,
		ViewW:    viewW,
		ViewH:    viewH,
		DeadZone: dz,
		MaxX:     maxX,
		MaxY:     maxY,
		CurPanX:  0, // paint-only: no drag-pan ownership on the HTML path
	})
	return panX, panY, true
}

func parseCameraCenter(n *model.Node) (centerX, centerY bool) {
	raw, ok := n.Prop("cameraCenter")
	if !ok {
		return false, false
	}
	switch v := raw.(type) {
	case bool:
		return v, v
	case string:
		switch v {
		case "x":
			return true, false
		case "y":
			return false, true
		case "true", "1":
			return true, true
		}
	}
	return false, false
}

// htmlBoardScreen picks a physical screen size for camera viewport math.
// Prefer runtime viewport; else board style width/height; else 0 (headless defaults).
func htmlBoardScreen(n *model.Node, rt *runtime.Runtime) (w, h float64) {
	if rt != nil && rt.Viewport.W > 0 && rt.Viewport.H > 0 {
		return float64(rt.Viewport.W), float64(rt.Viewport.H)
	}
	if n != nil && n.Style != nil {
		if sw, ok := numOK(n.Style, "width"); ok && sw > 0 {
			if sh, ok := numOK(n.Style, "height"); ok && sh > 0 {
				return sw, sh
			}
		}
	}
	return 0, 0
}

// boardViewportPx mirrors canvas boardViewportFromScreen (px-in / px-out).
func boardViewportPx(n *model.Node, screenW, screenH float64) (float64, float64) {
	if screenW <= 0 || screenH <= 0 {
		if n != nil {
			if raw, ok := n.Prop("cameraViewport"); ok {
				if v, ok := toFloatAny(raw); ok && v > 0 {
					return v, v * 3 / 4
				}
			}
		}
		return 16, 12
	}
	if n != nil {
		if raw, ok := n.Prop("cameraViewport"); ok {
			if v, ok := toFloatAny(raw); ok && v > 0 {
				return v, v * screenH / screenW
			}
		}
	}
	return screenW, screenH
}

func resolveCameraTargetMap(raw any, rt *runtime.Runtime) map[string]any {
	switch v := raw.(type) {
	case map[string]any:
		return v
	case string:
		if v == "" {
			return nil
		}
		var ctx map[string]any
		if rt != nil {
			ctx = map[string]any{"state": rt.State}
		}
		out := runtime.EvalBinding(v, ctx)
		if m, ok := out.(map[string]any); ok {
			return m
		}
	}
	return nil
}

func toFloatAny(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		if x == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}
