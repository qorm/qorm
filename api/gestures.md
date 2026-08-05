# Gestures

QORM ships touch/pointer gestures as widget props — no JavaScript to write.

| gesture | how |
|---|---|
| tap / double-tap / long-press | `onPress` / `onDoubleTap` / `onLongPress` on any node |
| swipe-to-dismiss | a `dismissible` widget with `onDismissed` |
| swipe-to-reveal actions | a `swipeactions` widget with an `actions` list |
| swipeable pages | a horizontal `scroll` (scroll-snap) |
| pull-to-refresh | a `scroll` with `onRefresh` |
| drag-to-reorder | a `list` with `reorderable: true` + `onReorder` |
| context menu | `contextMenu` on a node |
| drag-and-drop | `draggable` → `dragtarget` cross-panel drag |
| hover feedback | `hoverBackground`, `hoverOpacity`, `hoverScale` on any node |
| press feedback | `pressedBackground`, `pressedOpacity`, `pressedScale` on any node |
| animated transitions | `transition: "0.2s"` on any node animates interaction effects |
| scroll momentum | viewports coast after wheel/trackpad input with inertia |
| board pan momentum | infinite canvas coasts after drag release |

## Text input editing (native canvas backend)

The native canvas window implements a full text editing system:

- **Selection**: Shift+arrow extends, Cmd+A selects all, click places the caret,
  double-click selects a word, triple-click selects a line (textarea) or the whole
  field (single-line), drag selects text.
- **Clipboard**: Cmd+C copies the selection, Cmd+X cuts, Cmd+V pastes (multi-line
  values are folded to single-line for `<input>`). The system clipboard is
  accessed via `pbcopy`/`pbpaste` on macOS.
- **Undo/redo**: Cmd+Z undoes the last edit, Cmd+Shift+Z / Cmd+Y redoes. The
  undo stack is capped at 50 entries.
- **Navigation**: Left/Right by rune, Cmd+Left/Right by word, Home/End to line
  start/end (textarea) or buffer start/end (input), Up/Down by visual line
  (textarea only).
- **Delete**: Backspace deletes the rune before the caret, DeleteForward (Fn+Delete)
  deletes after; both replace the selection when non-empty.
- **Caret blink**: the insertion caret blinks at 500ms while the field is focused.
- **Secure input**: `"secure": true` masks the value with bullets while editing;
  the real buffer is what copy and state write-back use.
- **Number input**: `"inputType": "number"` restricts entry to digits, one `-` and
  one `.`; `min`, `max`, `step` props clamp the committed value.
- **Readonly**: `"readonly": true` keeps the field focusable and keyboard-visible
  but prevents editing.
- **IME composition**: uncommitted composing text is drawn with an underline after
  the buffer (the host's text-input-client seam fills it).

## Scroll momentum

The native canvas engine applies inertia to scroll viewports after wheel/trackpad
input stops:

- **Per-viewport velocity** tracked via exponential moving average (α=0.15) of
  consumed deltas, so continuous trackpad input builds smooth speed and discrete
  wheel events produce a subtle carry-forward.
- **Frame-rate independent friction**: velocity is decayed as `0.88^frames` each
  frame — a 16ms frame gets one friction step, a 32ms frame gets two.
- **Double-count prevention**: momentum is skipped when a scroll event arrived
  within the last 2ms (the event handler already applied its own delta).
- **Boundary handling**: velocity is zeroed when the offset hits the floor (0);
  the layout-time clamp (`scrollOffsetPos`) repairs any overshoot at the ceiling.
- The engine keeps animating while any viewport has active momentum, settling once
  all velocities drop below 0.3 px/frame.

## Board pan momentum

The infinite-canvas `board` root coasts after a drag release:

- **Velocity tracking**: during a blank-space drag, the engine tracks the
  displacement between consecutive moves and smooths it with EMA (α=0.2).
- **Coast phase**: on release, the tracked velocity is applied each frame and
  decayed with `0.92^frames` — a longer, floatier feel than scroll momentum
  (≈50 frames / 833ms to settle).
- **Cancellation**: a new blank-space press immediately cancels any in-flight
  coast, so the next drag starts from a clean state.

## Animated interaction transitions

Any node can declare a `transition` duration (`"0.2s"`, `"200ms"`) that animates
interaction-effect changes instead of snapping them:

- **Scale**: `"pressedScale": 0.95` + `"transition": "0.2s"` smoothly scales the
  node on press and back on release, via a `Float64Tween` through the theme's
  easing curve.
- **Background**: theme `pressedBackgroundColor` / per-node `"pressedBackground"`
  animate alongside the scale.
- **Opacity**: `"pressedOpacity"` / `"hoverOpacity"` join the same tween.

## Disabled visual dimming

Disabled nodes (`"disabled": true` in style) automatically render at 50% opacity
on the native canvas backend:

- **Non-widget nodes** (text, box, button, column, row, …) use the generic dimming.
- **Interactive widgets** (switch, checkbox, slider, select, …) handle their own
  disabled visuals and are excluded from generic dimming to prevent double-dimming.
- **Custom opacity**: `"disabledOpacity": 0.3` overrides the default 0.5.
- Disabled nodes also: block pointer activation, refuse focus, show a
  `not-allowed` cursor, and are excluded from the Tab order — all at the engine
  level with no per-widget logic needed.
