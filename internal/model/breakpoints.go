package model

// DefaultBreakpoints are the responsive width thresholds (px) used when the
// manifest does not declare its own. Keys are exposed to expressions as
// breakpoint.<name> booleans (true when viewport.width >= threshold).
var DefaultBreakpoints = map[string]int{
	"sm": 640,
	"md": 768,
	"lg": 1024,
	"xl": 1280,
}

// Breakpoints returns the app's breakpoint map, falling back to defaults.
func (a *App) Breakpoints() map[string]int {
	if a == nil || len(a.BreakpointWidths) == 0 {
		return DefaultBreakpoints
	}
	out := make(map[string]int, len(a.BreakpointWidths))
	for k, v := range a.BreakpointWidths {
		out[k] = v
	}
	return out
}
