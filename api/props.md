# Node & Widget Props

> Auto-generated from the source (`TestAPIRef`) — do not edit by hand. The props table below is extracted from the code, so it can never drift.

The declarative contract every QORM app is written against. A UI is a tree of **nodes**; each node is one JSON object.

## Node schema

Every node object may carry these top-level keys:

| Key | Type | Meaning |
|---|---|---|
| `type` | string | widget name — see the [widget catalog](widgets.md); may be a `{{ binding }}` (e.g. `{{ item.kind }}`), resolved against the current scope at render time so one template can render a different widget per item |
| `id` | string | stable node id (for state binding, patching, `data-state`) |
| `text` | string | text content (text/heading/paragraph nodes) |
| `label` | string | button / control label |
| `placeholder` | string | input placeholder |
| `value` | string | input or bound value; may contain `{{ binding }}` |
| `style` | object | visual style — see [common style props](#common-style-props) |
| `layout` | object | layout hints: `width` `height` `align` `justify` |
| `onPress` | action / string | press handler — an action id, or inline steps |
| `onChange` | action / string | change handler (inputs, toggles, sliders, selects) |
| `onKeyDown` | action / string | key-down handler — dispatched to the focused node first, then bubbling to the scene root; the pressed key is seeded as the `key` arg, read as `{{ key }}` (native canvas backend) |
| `onKeyUp` | action / string | key-up handler (same dispatch as `onKeyDown`) |
| `renderItem` | node | item template for a bound `list` |
| `data` | string | list data-binding expression (e.g. `state.todos`) |
| `children` | node[] | child nodes |
| `condition` | string | `when` nodes only: `{{ … }}` expression over `viewport.width` / `viewport.height` / `viewport.orientation` selecting `then` (truthy) or `else`; an unknown viewport (server first frame) is falsy |
| `then` | node | `when` nodes only: subtree rendered when `condition` is truthy |
| `else` | node | `when` nodes only: subtree rendered otherwise (unlike the `if` prop, which hides one node, `when` swaps two alternative subtrees) |
| `…` | any | any other key is a widget-specific **prop** (table below) |

## Common style props

Read by the shared renderer, so they work on any node that draws a box:

- **Box (`style`)** — `width` `height` `minWidth` `maxWidth` `minHeight` `maxHeight` `padding` `margin` `gap` `background` `gradient` `borderRadius` `borderWidth` `borderColor` `shadow` `opacity` `aspectRatio` `flexGrow` `flexShrink` `alignSelf` `zIndex` `position` `top` `right` `bottom` `left` `cursor` `transition`
- **Text (`style`)** — `color` `fontSize` `fontWeight` `fontFamily` `lineHeight` `letterSpacing` `fontStyle` `textDecoration` `textTransform` `textAlign` `lineClamp`
- **Pseudo-state (`style`)** — `hoverBackground` `hoverColor` `hoverOpacity` `pressedScale` `pressedOpacity` `focusBorderColor` `disabled` `disabledOpacity`
- **Backdrop (`style`)** — `backdropBlur` (frosted-glass radius in px, capped at 120; `0` turns the frost off on `appbar` / `largetitle`, which are frosted by default) `backdropTint` (the translucent fill the blur shows through; a browser without `backdrop-filter` falls back to a solid panel)
- **Layout (`layout`)** — `width` `height` `align` `justify` (`wrap` on containers, `columns` on `grid`, `orientation` on `scroll`)
- **Accessibility (top-level)** — `role` `ariaLabel` `title` `tooltip`

## Per-widget props

The widget-specific keys each renderer reads, on top of the common style props above. Auto-extracted from the `node()` switch in `internal/render` — a `—` means the widget takes only common props.

| Widget | Props |
|---|---|
| `accordion` | `active` · `single` · `title` |
| `actionsheet` | `dismissable` · `open` · `title` |
| `activityindicator` | `size` |
| `alert` | `title` · `variant` |
| `alertdialog` | `dismissable` · `message` · `open` · `title` |
| `animatedcontainer` | `curve` · `duration` |
| `animatedopacity` | `duration` · `opacity` |
| `appbar` | `background` · `leading` |
| `aspectratio` | `ratio` |
| `autocomplete` | `debounce` · `options` |
| `avatar` | `initials` · `name` · `size` · `src` |
| `backbutton` | — |
| `badge` | `color` · `showZero` · `smallSize` |
| `barometer` | `label` |
| `battery` | `label` |
| `biometric` | `label` |
| `bluetooth` | `label` |
| `bottomappbar` | — |
| `bottomnav` | `items` |
| `breadcrumb` | `items` · `separator` |
| `brightness` | — |
| `button` | `novalidate` · `submit` · `submitOnEnter` · `variant` |
| `calendar` | `label` |
| `camera` | `label` |
| `carousel` | `autoplay` · `indicators` |
| `chart` | `chartType` · `color` · `data` |
| `checkbox` | `checked` · `debounce` |
| `chip` | `avatar` · `selected` · `showCheck` |
| `circularprogress` | `color` · `size` · `stroke` · `value` |
| `clipboard` | `label` |
| `closebutton` | — |
| `compass` | `label` |
| `contacts` | `label` |
| `contextmenu` | `items` · `menuStyle` |
| `datatable` | `as` · `column` · `columns` · `data` · `detail` · `maxHeight` · `minWidth` · `rowKey` · `scrollX` · `selectable` · `selected` · `sortData` · `sortDir` · `sortField` · `stickyHeader` · `stickyTop` |
| `datepicker` | `maxYear` · `minYear` |
| `descriptions` | `items` |
| `deviceinfo` | `label` |
| `dismissible` | `icon` · `onDismissed` |
| `divider` | `orientation` |
| `dockbadge` | — |
| `draggable` | `data` |
| `dragtarget` | `onDrop` |
| `drawer` | `open` · `side` |
| `dropdownbutton` | `hint` · `options` |
| `empty` | `icon` · `title` |
| `expansiontile` | `initiallyExpanded` · `leading` |
| `fab` | `extended` |
| `field` | `error` · `help` · `label` · `required` |
| `filepicker` | `label` |
| `form` | `novalidate` |
| `gesturedetector` | `onDoubleTap` · `onLongPress` |
| `gridview` | `as` · `crossAxisCount` · `minItemWidth` · `page` · `pageSize` · `spacing` |
| `haptics` | `label` |
| `icon` | `glyph` · `icon` · `size` |
| `ignorepointer` | — |
| `image` | `alt` · `fallback` · `fit` · `lazy` · `placeholder` · `src` |
| `indexedstack` | `index` |
| `input` | `autocomplete` · `autofocus` · `debounce` · `inputMode` · `inputType` · `maxLength` · `pattern` · `readonly` · `required` · `requiredMessage` |
| `insets` | `label` |
| `keepawake` | `label` |
| `largetitle` | `background` · `collapsible` · `subtitle` |
| `limitedbox` | `maxHeight` · `maxWidth` |
| `link` | `href` |
| `list` | `as` · `groupBy` · `itemHeight` · `onRefresh` · `onReorder` · `overscan` · `page` · `pageSize` · `reorderable` · `sectionHeader` · `separator` · `sticky` · `stickyTop` · `virtualize` |
| `listsection` | `footer` · `header` |
| `listtile` | `chevron` · `leading` · `subtitle` · `trailing` |
| `location` | `label` |
| `loginitem` | — |
| `materialstepper` | `active` · `steps` |
| `menu` | `items` |
| `modal` | `dismissable` · `open` · `title` |
| `monthview` | `events` · `heading` · `max` · `min` · `month` · `onMonthChange` · `selected` · `showAdjacent` · `today` · `weekStart` · `weekdays` |
| `navigationdrawer` | `items` |
| `navigationrail` | `items` |
| `network` | `label` |
| `nfc` | `label` |
| `notify` | — |
| `offstage` | `offstage` |
| `openurl` | `label` |
| `orientation` | `label` |
| `pageview` | — |
| `pagination` | `page` · `total` |
| `pedometer` | `label` |
| `photopicker` | `label` |
| `picker` | `options` |
| `progress` | `color` |
| `proximity` | `label` |
| `qrscan` | `label` |
| `radio` | `debounce` · `options` |
| `rangeslider` | `debounce` · `high` · `low` · `max` · `min` · `step` |
| `rating` | `max` · `size` · `value` |
| `recorder` | `label` |
| `refreshindicator` | `onRefresh` |
| `richtext` | `spans` |
| `scaffold` | — |
| `screenrecord` | `label` |
| `screens` | — |
| `screenshot` | `label` |
| `searchbar` | `debounce` · `hint` · `items` · `onSelect` |
| `securestorage` | `label` |
| `segmented` | `debounce` · `multiple` · `options` |
| `select` | `debounce` · `options` |
| `selectabletext` | — |
| `sensors` | `label` |
| `share` | `label` |
| `sheet` | `backdrop` · `dismissable` · `handle` · `initialSnap` · `onClose` · `onSnap` · `open` · `snapPoints` · `title` |
| `skeleton` | — |
| `slider` | `debounce` · `max` · `min` · `step` |
| `slot` | `name` · `slot` |
| `snackbar` | `action` · `open` |
| `spacer` | — |
| `spinner` | `color` · `size` |
| `stat` | `delta` · `deltaType` · `label` · `value` |
| `steps` | `active` · `steps` |
| `storage` | `label` |
| `stt` | `label` |
| `swipeactions` | `actions` |
| `switchlisttile` | `subtitle` · `value` |
| `systemmodes` | `label` |
| `table` | `as` · `column` · `columns` · `data` · `detail` · `maxHeight` · `minWidth` · `scrollX` · `sortData` · `sortDir` · `sortField` · `stickyHeader` · `stickyTop` |
| `tabs` | `active` · `indicator` · `indicatorColor` · `lazy` · `scrollable` · `swipe` · `tabs` |
| `tag` | — |
| `text` | — |
| `textarea` | `autocomplete` · `autofocus` · `debounce` · `inputMode` · `maxLength` · `pattern` · `readonly` · `required` · `requiredMessage` · `rows` |
| `textformfield` | `autocomplete` · `autofocus` · `debounce` · `error` · `helper` · `inputMode` · `inputType` · `label` · `maxLength` · `pattern` · `prefix` · `readonly` · `required` · `requiredMessage` · `suffix` |
| `timeline` | `items` |
| `timepicker` | `minuteStep` |
| `timer` | — |
| `tooltip` | `focusable` · `maxWidth` · `placement` · `position` · `tooltip` |
| `torch` | `label` |
| `transform` | `rotate` · `scale` · `scaleX` · `scaleY` · `skew` · `translateX` · `translateY` |
| `tree` | `collapsed` · `data` |
| `tts` | `label` |
| `verticaldivider` | `orientation` |
| `vibrate` | `label` |
| `video` | `src` |
| `videocapture` | `label` |
| `volume` | — |
| `when` | — |
| `wifi` | `label` |
| `wrap` | `runSpacing` · `spacing` |
