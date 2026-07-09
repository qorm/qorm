# Node & Widget Props

> Auto-generated from the source (`TestAPIRef`) — do not edit by hand. The props table below is extracted from the code, so it can never drift.

The declarative contract every QORM app is written against. A UI is a tree of **nodes**; each node is one JSON object.

## Node schema

Every node object may carry these top-level keys:

| Key | Type | Meaning |
|---|---|---|
| `type` | string | widget name — see the [widget catalog](widgets.md) |
| `id` | string | stable node id (for state binding, patching, `data-state`) |
| `text` | string | text content (text/heading/paragraph nodes) |
| `label` | string | button / control label |
| `placeholder` | string | input placeholder |
| `value` | string | input or bound value; may contain `{{ binding }}` |
| `style` | object | visual style — see [common style props](#common-style-props) |
| `layout` | object | layout hints: `width` `height` `align` `justify` |
| `onPress` | action / string | press handler — an action id, or inline steps |
| `onChange` | action / string | change handler (inputs, toggles, sliders, selects) |
| `renderItem` | node | item template for a bound `list` |
| `data` | string | list data-binding expression (e.g. `state.todos`) |
| `children` | node[] | child nodes |
| `condition` | string | `when` nodes only: `{{ … }}` expression over `viewport.width` / `viewport.height` / `viewport.orientation` selecting `then` (truthy) or `else`; an unknown viewport (server first frame) is falsy |
| `then` | node | `when` nodes only: subtree rendered when `condition` is truthy |
| `else` | node | `when` nodes only: subtree rendered otherwise (unlike the `if` prop, which hides one node, `when` swaps two alternative subtrees) |
| `…` | any | any other key is a widget-specific **prop** (table below) |

## Common style props

Read by the shared renderer, so they work on any node that draws a box:

- **Box (`style`)** — `width` `height` `minWidth` `maxWidth` `minHeight` `maxHeight` `padding` `margin` `gap` `background` `gradient` `borderRadius` `borderWidth` `borderColor` `shadow` `opacity` `aspectRatio` `flexGrow` `position` `top` `right` `bottom` `left` `cursor` `transition`
- **Text (`style`)** — `color` `fontSize` `fontWeight` `fontFamily` `lineHeight` `letterSpacing` `fontStyle` `textDecoration` `textTransform` `textAlign` `lineClamp`
- **Layout (`layout`)** — `width` `height` `align` `justify` (`wrap` on containers, `columns` on `grid`, `orientation` on `scroll`)
- **Accessibility (top-level)** — `role` `ariaLabel` `title` `tooltip`

## Per-widget props

The widget-specific keys each renderer reads, on top of the common style props above. Auto-extracted from the `node()` switch in `internal/render` — a `—` means the widget takes only common props.

| Widget | Props |
|---|---|
| `accordion` | `title` |
| `actionsheet` | `open` · `title` |
| `activityindicator` | `size` |
| `alert` | `title` · `variant` |
| `alertdialog` | `message` · `open` · `title` |
| `animatedcontainer` | `curve` · `duration` |
| `animatedopacity` | `duration` · `opacity` |
| `appbar` | `background` · `leading` |
| `aspectratio` | `ratio` |
| `autocomplete` | `options` |
| `avatar` | `initials` · `name` · `size` · `src` |
| `backbutton` | — |
| `badge` | `color` · `showZero` · `smallSize` |
| `barometer` | `label` |
| `battery` | `label` |
| `biometric` | `label` |
| `bluetooth` | `label` |
| `bottomappbar` | — |
| `bottomnav` | — |
| `breadcrumb` | `items` · `separator` |
| `brightness` | — |
| `button` | `variant` |
| `calendar` | `label` |
| `camera` | `label` |
| `carousel` | — |
| `chart` | `chartType` · `color` · `data` |
| `checkbox` | `checked` |
| `chip` | `avatar` · `selected` · `showCheck` |
| `circularprogress` | `color` · `size` · `stroke` · `value` |
| `clipboard` | `label` |
| `closebutton` | — |
| `compass` | `label` |
| `contacts` | `label` |
| `contextmenu` | `items` · `menuStyle` |
| `datatable` | `columns` · `rowKey` · `selectable` · `sortDir` · `sortField` |
| `datepicker` | `maxYear` · `minYear` |
| `descriptions` | — |
| `deviceinfo` | `label` |
| `dismissible` | `icon` · `onDismissed` |
| `divider` | `orientation` |
| `dockbadge` | — |
| `drawer` | `open` · `side` |
| `dropdownbutton` | `hint` · `options` |
| `empty` | `icon` · `title` |
| `expansiontile` | `initiallyExpanded` · `leading` |
| `fab` | `extended` |
| `field` | `error` · `help` · `label` · `required` |
| `filepicker` | `label` |
| `form` | — |
| `gesturedetector` | `onDoubleTap` · `onLongPress` |
| `gridview` | `crossAxisCount` · `minItemWidth` · `spacing` |
| `haptics` | `label` |
| `icon` | `glyph` · `icon` · `size` |
| `image` | `alt` · `fit` · `src` |
| `indexedstack` | `index` |
| `input` | `inputType` |
| `insets` | `label` |
| `keepawake` | `label` |
| `largetitle` | `background` · `subtitle` |
| `limitedbox` | `maxHeight` · `maxWidth` |
| `link` | `href` |
| `list` | `itemHeight` · `onReorder` · `reorderable` · `virtualize` |
| `listsection` | `footer` · `header` |
| `listtile` | `chevron` · `leading` · `subtitle` · `trailing` |
| `location` | `label` |
| `loginitem` | — |
| `materialstepper` | `active` |
| `menu` | — |
| `modal` | `open` · `title` |
| `navigationdrawer` | — |
| `navigationrail` | — |
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
| `radio` | `options` |
| `rangeslider` | `high` · `low` · `max` · `min` · `step` |
| `rating` | `max` · `size` · `value` |
| `recorder` | `label` |
| `refreshindicator` | `onRefresh` |
| `richtext` | `spans` |
| `scaffold` | — |
| `screenrecord` | `label` |
| `screens` | — |
| `screenshot` | `label` |
| `securestorage` | `label` |
| `segmented` | `options` |
| `select` | `options` |
| `selectabletext` | — |
| `sensors` | `label` |
| `share` | `label` |
| `skeleton` | — |
| `slider` | `max` · `min` · `step` |
| `slot` | — |
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
| `table` | `columns` |
| `tabs` | `tabs` |
| `tag` | — |
| `text` | — |
| `textarea` | `rows` |
| `textformfield` | `error` · `helper` · `inputType` · `label` · `maxLength` · `prefix` · `suffix` |
| `timeline` | — |
| `torch` | `label` |
| `transform` | `rotate` · `scale` · `scaleX` · `scaleY` · `skew` · `translateX` · `translateY` |
| `tree` | — |
| `tts` | `label` |
| `verticaldivider` | `orientation` |
| `vibrate` | `label` |
| `video` | `src` |
| `videocapture` | `label` |
| `volume` | — |
| `when` | — |
| `wifi` | `label` |
| `wrap` | `runSpacing` · `spacing` |
