package geom

// BBox represents an axis-aligned bounding box
type BBox struct {
	MinX, MinY float64
	MaxX, MaxY float64
}

func NewBBox(x, y, w, h float64) BBox {
	return BBox{
		MinX: x,
		MinY: y,
		MaxX: x + w,
		MaxY: y + h,
	}
}

// Contains checks if a point is inside the bounding box
func (b BBox) Contains(p Point) bool {
	return p.X >= b.MinX && p.X <= b.MaxX && p.Y >= b.MinY && p.Y <= b.MaxY
}

// Intersects checks if two bounding boxes overlap
func (b BBox) Intersects(other BBox) bool {
	return b.MinX <= other.MaxX && b.MaxX >= other.MinX &&
		b.MinY <= other.MaxY && b.MaxY >= other.MinY
}
