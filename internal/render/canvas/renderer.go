// Package canvas is QORM's native rendering engine: layout → scene graph →
// display list → Renderer → Surface (see planning/adr/0007). This file defines
// the Renderer contract.
package canvas

import (
	"image"

	"github.com/qorm/platform/internal/op"
)

// Renderer consumes a recorded display list and produces pixels into a
// caller-owned target buffer. SoftwareRenderer is the first implementation;
// GPU and dirty-rectangle renderers will be added without changing callers
// (the Renderer Contract from ADR-0004 / technical-architecture-plan).
type Renderer interface {
	// Render evaluates ops and draws them into target, starting from a clean
	// white frame. The target's bounds define the viewport.
	Render(ops *op.Ops, target *image.RGBA)
}
