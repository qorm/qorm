---
title: First Scene
description: A QORM scene is the UI entry point: a root node tree of text, layout, and interpolation that the renderer turns into a screen.
---

# First Scene

A scene is QORM's UI entry point. A scene has an `id` and a root node `root`; the node tree describes the UI. Text content goes in the `text` field, and `{{ state.x }}` interpolates global state.

## Minimal scene

```json
{
  "type": "scene",
  "id": "main",
  "root": { "type": "text", "id": "hello", "text": "Hello QORM" }
}
```

## Scene with layout

Container nodes (`column` / `row`) arrange their children using `style` (padding, spacing, background) and `layout` (size, alignment).

```json
{
  "type": "scene",
  "id": "main",
  "root": {
    "type": "column",
    "id": "root",
    "style":  { "padding": 24, "gap": 12 },
    "layout": { "width": "fill", "height": "fill", "align": "center" },
    "children": [
      { "type": "text",   "id": "title",  "text": "Title" },
      { "type": "button", "id": "submit", "text": "Submit", "onPress": "save" }
    ]
  }
}
```

- Use `text` (not `value`) for text; template interpolation like `"Welcome, {{ state.user }}"` also goes in `text`.
- Buttons use `onPress` to trigger actions (see [First Action](first-action.md)).
- For all available node types, see the [Widget catalog](/api/widgets.md).

![A rendered QORM scene with a title, text, and a button](img/first-scene.png)
*A scene renders to a screen — text nodes plus a button laid out by a column.*

## Lists and item scope

A `list` binds `data` to an array and repeats one `renderItem` template. Inside
that template the row is `item`, and its position is `index` (0-based) plus the
`first` / `last` flags.

```json
{
  "type": "list", "id": "rows", "data": "{{ state.items }}",
  "renderItem": {
    "type": "row", "children": [
      { "type": "text", "text": "{{ index + 1 }}." },
      { "type": "text", "text": "{{ item.title }}" }
    ]
  }
}
```

`index` / `first` / `last` are separate scope names, so a data field of the
same name is never shadowed: `{{ item.index }}` is your data, `{{ index }}` is
the loop position.

**Nested lists** rebind `item` on the inner loop. Give the outer list an `as`
alias to keep it reachable — `"as": "section"` binds `section` plus
`sectionIndex` / `sectionFirst` / `sectionLast`:

```json
{
  "type": "list", "data": "{{ state.sections }}", "as": "section",
  "renderItem": {
    "type": "column", "children": [
      { "type": "text", "text": "{{ section.title }}" },
      { "type": "list", "data": "{{ section.rows }}",
        "renderItem": { "type": "text", "text": "{{ section.title }} / {{ item }}" } }
    ]
  }
}
```

An invalid or reserved alias falls back to `item` with a load-time warning.
`gridview` takes the same `as` and the same scope names.

### List props worth knowing

| Prop | What it does |
|---|---|
| `separator` | draws a hairline between items — `true`, or `{ "height": 1, "inset": 16, "color": "…" }`. Never after the last item |
| `pageSize` + `page` | built-in pagination: only one window of `data` renders. `page` is 1-based and clamps into range, so an overshooting page shows the last one instead of a blank screen |
| `groupBy` + `sectionHeader` | splits **consecutive runs** of an equal key into sections with a header label. The renderer never reorders your data — sort it first |
| `sticky` + `stickyTop` | section headers stick while scrolling (`sticky` defaults to `true`); `stickyTop` parks them below an appbar |
| `onRefresh` | pull-to-refresh; dispatches the named action |
| `reorderable` + `onReorder` | press-hold drag to reorder |

```json
{
  "type": "list", "id": "contacts", "data": "{{ state.contacts }}",
  "groupBy": "letter", "sectionHeader": "{{ item.letter }}", "stickyTop": 44,
  "separator": true, "pageSize": 50, "page": "{{ state.page }}",
  "onRefresh": "reloadContacts",
  "renderItem": { "type": "listtile", "text": "{{ item.name }}" }
}
```

`index` stays global to the whole `data`, not to the current page, so numbering
keeps counting across pages. `groupBy` and `reorderable` are not combined
(section headers would misindex the drag), and `reorderable` takes precedence
over `onRefresh` (both would claim the same drag gesture).

Note that `virtualize` is a rendering hint (`content-visibility`), not real
windowing — the full item markup is still produced. Use `pageSize` when you
need the DOM itself to stay small.

## Interactive styling

Pseudo-state style keys describe how a node reacts, with no JavaScript and no
extra nodes:

```json
{
  "type": "button", "id": "save", "text": "Save",
  "style": {
    "hoverBackground": "#0a84ff", "hoverColor": "#ffffff",
    "pressedScale": 0.97,
    "focusBorderColor": "var(--accent)",
    "disabled": "{{ state.saving }}", "disabledOpacity": 0.4
  }
}
```

The eight keys are `hoverBackground`, `hoverColor`, `hoverOpacity`,
`pressedScale`, `pressedOpacity`, `focusBorderColor`, `disabled` and
`disabledOpacity`. Hover styling only applies on devices that really hover, so
a tap can never strand a node in a hover state; `disabled` also emits
`aria-disabled`.

`width` and `height` accept a number of pixels, `"fill"`, or a string with a
unit — `"50%"`, `"30vw"`, `"40vh"`, `"120px"`. Layering and flex behaviour are
available as `zIndex`, `alignSelf` and `flexShrink`.

## Images

`src` and `alt` are interpolated, so an image works inside a `renderItem`.
Three props cover the states a remote image goes through:

```json
{ "type": "image", "id": "hero",
  "src": "{{ item.photo }}", "alt": "{{ item.caption }}",
  "placeholder": "#e5e5ea",
  "fallback": "/assets/missing.png",
  "fit": "cover" }
```

- `placeholder` is what shows while the image loads — a CSS color, or a
  low-resolution URL painted as a blur-up background.
- `fallback` replaces the image if it fails to load. The swap happens once, so
  a broken fallback cannot loop.
- Lazy loading is **on by default**; set `"lazy": false` to opt out for an
  above-the-fold image.

`placeholder` and `fallback` are static values, not bindings.

## Choosing a widget at runtime

A node's `type` may itself be a binding, resolved per render against the
current scope. One list template can then render a different widget per row
instead of stacking `if` branches:

```json
{
  "type": "list", "id": "feed", "data": "{{ state.messages }}",
  "renderItem": { "type": "{{ item.kind }}", "text": "{{ item.text }}", "src": "{{ item.src }}" }
}
```

With `kind` of `"text"` or `"image"` the same row renders a text node or an
image. If the binding resolves to nothing, or to a name no widget answers to,
the node degrades to the renderer's unknown-node placeholder and the diagnostic
names the offending expression — so a typo is visible rather than silent.

For the expression language itself, see [Expressions](../expressions.md).
