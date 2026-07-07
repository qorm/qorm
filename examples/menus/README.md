# Menus example

Demonstrates QORM's three desktop menu surfaces. Each is declared in
`qorm.json` and reports selections onto the event bus — your app listens with
`qormOn(event, fn)` and does whatever it wants with the id. This example just
prints the last selection; the actual behaviour is up to the app.

## 1. System menu bar — `platforms.desktop.menu`

Groups that sit between the App and Edit menus. Items take an SF-Symbol `icon`,
a `shortcut` (`cmd+n`, `cmd+shift+s`, …), a nested `items` submenu, and
`separator`. Selecting one fires `qormEmit('menu', {id})`.

```json
"menu": [
  { "title": "File", "items": [
    { "id": "new",  "title": "New",   "icon": "doc.badge.plus", "shortcut": "cmd+n" },
    { "separator": true },
    { "id": "export", "title": "Export", "icon": "square.and.arrow.up", "items": [
      { "id": "export-pdf", "title": "as PDF" }
    ]}
  ]}
]
```

## 2. Tray icon — `platforms.desktop.tray`

A menu-bar status item with its own menu (same item shape, SF-Symbol icons +
submenus). Selecting fires `qormEmit('tray', {id})`. The reserved id `"quit"`
terminates the app.

```json
"tray": { "tip": "Menus demo", "items": [
  { "id": "tray-show", "title": "Show Window", "icon": "macwindow" },
  { "separator": true },
  { "id": "quit", "title": "Quit", "icon": "power" }
]}
```

## 3. Right-click menu — the `contextmenu` widget

Wrap any content in a `contextmenu` node with an `items` array. Right-clicking
the content opens a cursor-positioned menu with built-in SVG icons (QORM icon
names — `copy`, `clipboard`, `share`, `x`, …), submenus, and separators.
Selecting fires `qormEmit('context', {id})`. Style the panel with `menuStyle`.

```json
{ "type": "contextmenu", "id": "ctx", "items": [
  { "id": "copy", "title": "Copy", "icon": "copy" },
  { "id": "share", "title": "Share", "icon": "share", "items": [ … ] }
], "children": [ … the right-clickable content … ] }
```

## Receiving events (`native/web.js`)

```js
qormOn('menu',    function(d){ /* d.id from the menu bar */ });
qormOn('tray',    function(d){ /* d.id from the tray      */ });
qormOn('context', function(d){ /* d.id from right-click   */ });
```

Selections also appear in the desktop Activity-log window as `app: menu <id>`.

Run: `qorm run examples/menus` (desktop). Native menus (bar + tray) are
macOS/Windows/Linux desktop features; the right-click menu also works on web.
