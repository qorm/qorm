package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/godbus/dbus/v5"
)

// encodePNG builds a 2x1 PNG with known, non-premultiplied pixel values.
func encodePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xFF})
	img.SetNRGBA(1, 0, color.NRGBA{R: 0xAA, G: 0xBB, B: 0xCC, A: 0x80})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestPngToARGB32(t *testing.T) {
	px, err := pngToARGB32(encodePNG(t))
	if err != nil {
		t.Fatalf("pngToARGB32: %v", err)
	}
	if px.Width != 2 || px.Height != 1 {
		t.Fatalf("size = %dx%d, want 2x1", px.Width, px.Height)
	}
	want := []byte{
		0xFF, 0x11, 0x22, 0x33, // pixel 0: A,R,G,B
		0x80, 0xAA, 0xBB, 0xCC, // pixel 1: alpha preserved, not premultiplied
	}
	if !bytes.Equal(px.Data, want) {
		t.Fatalf("data = %x, want %x", px.Data, want)
	}
}

func TestPngToARGB32Invalid(t *testing.T) {
	if _, err := pngToARGB32([]byte("not a png")); err == nil {
		t.Fatal("expected an error for invalid PNG data")
	}
}

func TestNewTrayMenuIDs(t *testing.T) {
	m := newTrayMenu([]string{"Activity Log", "Open in Browser", "Quit QORM"})
	if len(m.root.children) != 3 {
		t.Fatalf("children = %d, want 3", len(m.root.children))
	}
	for i, it := range m.root.children {
		if it.id != int32(i+1) {
			t.Errorf("item %d id = %d, want %d", i, it.id, i+1)
		}
		if m.byID[it.id] != it {
			t.Errorf("byID[%d] does not index item %d", it.id, i)
		}
	}
	if m.byID[0] != m.root {
		t.Error("byID[0] is not the root")
	}
}

func TestTrayMenuFromJSON(t *testing.T) {
	// The AppTrayJSON wire format (model.TrayConfig).
	src := `{"icon":"star","tip":"My App","items":[
		{"id":"open","title":"Open"},
		{"separator":true},
		{"id":"more","title":"More","items":[{"id":"about","title":"About"}]},
		{"id":"quit","title":"Quit"}]}`
	m, err := trayMenuFromJSON(src)
	if err != nil {
		t.Fatalf("trayMenuFromJSON: %v", err)
	}
	if len(m.root.children) != 4 {
		t.Fatalf("top-level items = %d, want 4", len(m.root.children))
	}
	open := m.root.children[0]
	if open.strID != "open" || open.label != "Open" || open.id != 1 {
		t.Errorf("item 0 = %+v, want id=1 strID=open label=Open", open)
	}
	if !m.root.children[1].separator {
		t.Error("item 1 should be a separator")
	}
	more := m.root.children[2]
	if len(more.children) != 1 || more.children[0].strID != "about" {
		t.Errorf("submenu not parsed: %+v", more)
	}
	// depth-first ids: open=1, sep=2, more=3, about=4, quit=5
	if about := m.byID[4]; about == nil || about.strID != "about" {
		t.Errorf("byID[4] = %+v, want the about item", m.byID[4])
	}
	if quit := m.byID[5]; quit == nil || quit.strID != "quit" {
		t.Errorf("byID[5] = %+v, want the quit item", m.byID[5])
	}
}

func TestTrayMenuFromJSONErrors(t *testing.T) {
	if _, err := trayMenuFromJSON("not json"); err == nil {
		t.Error("expected an error for malformed JSON")
	}
	if _, err := trayMenuFromJSON(`{"items":[]}`); err == nil {
		t.Error("expected an error for an empty menu")
	}
}

func TestMenuProps(t *testing.T) {
	m, err := trayMenuFromJSON(`{"items":[
		{"id":"a","title":"A"},
		{"separator":true},
		{"id":"sub","title":"Sub","items":[{"id":"b","title":"B"}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if v := m.root.menuProps()["children-display"]; v.Value() != "submenu" {
		t.Errorf("root children-display = %v, want submenu", v)
	}
	a := m.byID[1].menuProps()
	if a["label"].Value() != "A" || a["enabled"].Value() != true || a["visible"].Value() != true {
		t.Errorf("plain item props = %v", a)
	}
	if _, ok := a["children-display"]; ok {
		t.Error("leaf item must not announce a submenu")
	}
	if sep := m.byID[2].menuProps(); sep["type"].Value() != "separator" {
		t.Errorf("separator props = %v", sep)
	}
	if sub := m.byID[3].menuProps(); sub["children-display"].Value() != "submenu" {
		t.Errorf("parent item props = %v", sub)
	}
}

func TestLayoutNodeDepth(t *testing.T) {
	m, err := trayMenuFromJSON(`{"items":[{"id":"sub","title":"Sub","items":[{"id":"leaf","title":"Leaf"}]}]}`)
	if err != nil {
		t.Fatal(err)
	}

	// depth 0: node only, no children.
	if n := m.root.layoutNode(0); len(n.Children) != 0 {
		t.Errorf("depth 0: children = %d, want 0", len(n.Children))
	}

	// depth 1: direct children only.
	n := m.root.layoutNode(1)
	if len(n.Children) != 1 {
		t.Fatalf("depth 1: children = %d, want 1", len(n.Children))
	}
	sub, ok := n.Children[0].Value().(menuLayout)
	if !ok {
		t.Fatalf("child is %T, want menuLayout", n.Children[0].Value())
	}
	if sub.ID != 1 || len(sub.Children) != 0 {
		t.Errorf("depth 1 child = id %d with %d children, want id 1 with 0", sub.ID, len(sub.Children))
	}

	// depth -1: the whole tree.
	n = m.root.layoutNode(-1)
	sub = n.Children[0].Value().(menuLayout)
	if len(sub.Children) != 1 {
		t.Fatalf("depth -1: grandchildren = %d, want 1", len(sub.Children))
	}
	leaf := sub.Children[0].Value().(menuLayout)
	if leaf.ID != 2 || leaf.Properties["label"].Value() != "Leaf" {
		t.Errorf("leaf = %+v", leaf)
	}
}

// The layout node must marshal with the dbusmenu wire signature (ia{sv}av).
func TestLayoutSignature(t *testing.T) {
	n := newTrayMenu([]string{"One"}).root.layoutNode(-1)
	if sig := dbus.SignatureOf(n); sig.String() != "(ia{sv}av)" {
		t.Errorf("layout signature = %s, want (ia{sv}av)", sig)
	}
	if sig := dbus.SignatureOf([]sniPixmap{}); sig.String() != "a(iiay)" {
		t.Errorf("pixmap signature = %s, want a(iiay)", sig)
	}
	if sig := dbus.SignatureOf(sniToolTip{}); sig.String() != "(sa(iiay)ss)" {
		t.Errorf("tooltip signature = %s, want (sa(iiay)ss)", sig)
	}
}

func TestGroupProperties(t *testing.T) {
	m := newTrayMenu([]string{"A", "B"})
	all := m.groupProperties(nil)
	if len(all) != 3 { // root + 2 items
		t.Fatalf("all rows = %d, want 3", len(all))
	}
	some := m.groupProperties([]int32{2, 99})
	if len(some) != 1 || some[0].ID != 2 || some[0].Properties["label"].Value() != "B" {
		t.Errorf("filtered rows = %+v, want just item 2 (B)", some)
	}
}
