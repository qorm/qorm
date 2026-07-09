// DBus tray protocol data structures — the pure (no-bus) half of the Linux
// StatusNotifierItem tray in tray_linux.go: PNG → ARGB32 pixmap conversion, the
// tray menu model, and com.canonical.dbusmenu layout construction. dbusmenu has
// no formal spec; the wire shapes follow the de-facto protocol as implemented
// by libdbusmenu and fyne-io/systray. Kept build-tag-free (godbus is pure Go)
// so the protocol logic is unit-testable on any host.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"image/color"
	"image/png"

	"github.com/godbus/dbus/v5"
)

// sniPixmap is one icon frame as org.kde.StatusNotifierItem wants it:
// ARGB32 in network byte order, DBus signature (iiay).
type sniPixmap struct {
	Width  int32
	Height int32
	Data   []byte
}

// sniToolTip is the StatusNotifierItem ToolTip property, signature (sa(iiay)ss).
type sniToolTip struct {
	IconName    string
	IconPixmaps []sniPixmap
	Title       string
	Description string
}

// pngToARGB32 decodes a PNG into an SNI pixmap: 4 bytes per pixel in A,R,G,B
// order (ARGB32 in network byte order), non-premultiplied.
func pngToARGB32(data []byte) (sniPixmap, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return sniPixmap{}, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return sniPixmap{}, errors.New("empty image")
	}
	out := make([]byte, 0, w*h*4)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			out = append(out, c.A, c.R, c.G, c.B)
		}
	}
	return sniPixmap{Width: int32(w), Height: int32(h), Data: out}, nil
}

// trayItem is one node of the tray menu tree. IDs are the int32 handles the
// dbusmenu protocol addresses items by; the root is always id 0. strID carries
// the app-level menu id (JSON menus) routed to traySelected.
type trayItem struct {
	id        int32
	strID     string // app menu id ("" for plain string menus)
	label     string
	separator bool
	children  []*trayItem
}

// trayMenu is the menu tree plus the id index the dbusmenu methods look up in.
type trayMenu struct {
	root *trayItem
	byID map[int32]*trayItem
}

// newTrayMenu builds a flat menu from plain labels (the built-in tray). Item
// ids are 1..n in order, so id-1 recovers the onClick index.
func newTrayMenu(labels []string) *trayMenu {
	m := &trayMenu{root: &trayItem{id: 0}}
	m.byID = map[int32]*trayItem{0: m.root}
	for i, l := range labels {
		it := &trayItem{id: int32(i + 1), label: l}
		m.root.children = append(m.root.children, it)
		m.byID[it.id] = it
	}
	return m
}

// trayMenuJSONItem mirrors model.MenuItem's JSON shape (the AppTrayJSON wire
// format); icon/shortcut/role are macOS-only and ignored here.
type trayMenuJSONItem struct {
	ID        string             `json:"id"`
	Title     string             `json:"title"`
	Separator bool               `json:"separator"`
	Items     []trayMenuJSONItem `json:"items"`
}

// trayMenuFromJSON builds the menu tree from the app's tray config JSON
// ({"items":[{id,title,separator,items…}]}), assigning depth-first int32 ids.
func trayMenuFromJSON(menuJSON string) (*trayMenu, error) {
	var cfg struct {
		Items []trayMenuJSONItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(menuJSON), &cfg); err != nil {
		return nil, err
	}
	if len(cfg.Items) == 0 {
		return nil, errors.New("tray menu has no items")
	}
	m := &trayMenu{root: &trayItem{id: 0}}
	m.byID = map[int32]*trayItem{0: m.root}
	next := int32(1)
	var add func(parent *trayItem, items []trayMenuJSONItem)
	add = func(parent *trayItem, items []trayMenuJSONItem) {
		for _, ji := range items {
			it := &trayItem{id: next, strID: ji.ID, label: ji.Title, separator: ji.Separator}
			next++
			parent.children = append(parent.children, it)
			m.byID[it.id] = it
			add(it, ji.Items)
		}
	}
	add(m.root, cfg.Items)
	return m, nil
}

// menuLayout is one com.canonical.dbusmenu layout node: (id, properties,
// children), signature (ia{sv}av) — the shape GetLayout returns.
type menuLayout struct {
	ID         int32
	Properties map[string]dbus.Variant
	Children   []dbus.Variant
}

// menuItemProps pairs an item id with its properties, signature (ia{sv}) —
// one GetGroupProperties result row.
type menuItemProps struct {
	ID         int32
	Properties map[string]dbus.Variant
}

// menuProps returns the dbusmenu properties of one item (label / enabled /
// visible, "type":"separator" for separators, "children-display":"submenu" for
// parents). The root node only announces its submenu.
func (it *trayItem) menuProps() map[string]dbus.Variant {
	if it.separator {
		return map[string]dbus.Variant{"type": dbus.MakeVariant("separator")}
	}
	if it.id == 0 {
		return map[string]dbus.Variant{"children-display": dbus.MakeVariant("submenu")}
	}
	p := map[string]dbus.Variant{
		"label":   dbus.MakeVariant(it.label),
		"enabled": dbus.MakeVariant(true),
		"visible": dbus.MakeVariant(true),
	}
	if len(it.children) > 0 {
		p["children-display"] = dbus.MakeVariant("submenu")
	}
	return p
}

// layoutNode builds the GetLayout tree rooted at this item. depth is the
// protocol's recursionDepth: -1 = all descendants, 0 = this node only,
// n = n levels of children.
func (it *trayItem) layoutNode(depth int32) menuLayout {
	n := menuLayout{ID: it.id, Properties: it.menuProps(), Children: []dbus.Variant{}}
	if depth == 0 {
		return n
	}
	d := depth
	if d > 0 {
		d--
	}
	for _, c := range it.children {
		n.Children = append(n.Children, dbus.MakeVariant(c.layoutNode(d)))
	}
	return n
}

// groupProperties returns the GetGroupProperties rows for ids (empty ids = all
// items, per the dbusmenu convention). Unknown ids are skipped.
func (m *trayMenu) groupProperties(ids []int32) []menuItemProps {
	if len(ids) == 0 {
		var walk func(it *trayItem)
		out := []menuItemProps{}
		walk = func(it *trayItem) {
			out = append(out, menuItemProps{ID: it.id, Properties: it.menuProps()})
			for _, c := range it.children {
				walk(c)
			}
		}
		walk(m.root)
		return out
	}
	out := make([]menuItemProps, 0, len(ids))
	for _, id := range ids {
		if it, ok := m.byID[id]; ok {
			out = append(out, menuItemProps{ID: it.id, Properties: it.menuProps()})
		}
	}
	return out
}
