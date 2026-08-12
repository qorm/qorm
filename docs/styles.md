# QSS — QORM Style Sheets

QSS is QORM's stylesheet language: a CSS-like rule syntax for sharing styles
across scenes, without repeating inline `style` blocks on every node.

A stylesheet lives in `styles/<id>.qss` and is loaded by the runtime at load
time. The Tetris example (`examples/tetris/styles/app.qss`) is a complete,
runnable reference — a game's entire look driven by one stylesheet.

## Rule syntax

```qss
# This is a comment

/* selectors: type, class, id */
button { borderRadius: 12 }
.accent { background: var(--primary); color: var(--on-primary) }
#submit { fontSize: 16 }

/* nested objects stay inline on the node, like style values */
.card { margin: { top: 8, bottom: 8 } }

/* {{bindings}} evaluate like inline style values */
.statValue { color: {{ state.dark ? "#fff" : "#111" }} }
```

- **Type selectors** (`button`, `text`, `box`, …) match nodes of that type.
- **Class selectors** (`.accent`) match nodes whose `class` prop lists that
  name (space-separated; a class named later in the prop wins).
- **ID selectors** (`#submit`) match a node's `id`.
- **`#` comments** — numbers stay numbers, strings and `{{bindings}}` evaluate
  like inline style values.

## Cascade

Style resolution is a cascade, later sources winning key by key:

```text
theme component default < type rule < class rule < id rule < inline `style`
```

- Within one class name, declaration order wins; the node's own `class` list
  order wins between classes.
- Inline `style` on the node always beats every rule.

## Using a stylesheet

Scene nodes reference rules with a `class` prop:

```json
{ "type": "text", "text": "TETRIS", "class": "title" }
```

## Rendering

Both backends apply QSS with the same cascade (theme component default <
type < class < id < inline). The native canvas backend (macOS default pure-Go
window; games WASM with `qorm_canvas`) merges matching rules in its measure
pass; the HTML path merges them into each node's emitted inline CSS
(`boxCSS` / `textCSS`). Widget chrome defaults on the HTML path (button
variants, shell theme vars) sit under QSS the way canvas theme component
defaults do.

## Diagnostics

Parse errors are load-time diagnostics naming BOTH the file and the line
(`[Stylesheet: app] app.qss:3: …`). Unknown style keys (against
`render.KnownStyleKeys`) are warnings. The rules parsed before and after an
error still load, exactly like a scene keeps loading alongside its own
diagnostics — one bad rule never blanks the app.

## Loader contract

- A `.qss` file is collected only inside a `styles/` directory.
- Its id is the file name (minus `.qss`): `styles/app.qss` → stylesheet `app`.
- Duplicate ids are an error diagnostic on directory load (first wins) and a
  hard refusal from `qorm build`.
- The raw source is kept on the app so the serializer writes each sheet back
  verbatim — the same fixed-point property component documents have.
