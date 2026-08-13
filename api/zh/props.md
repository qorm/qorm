# 节点与组件属性

> 由源码自动生成(`TestAPIRef`),请勿手工编辑。下方的属性表从代码抽取,不会与实现漂移。

每个 QORM 应用据以编写的声明式契约。界面是一棵**节点**树,每个节点是一个 JSON 对象。

## 节点结构

每个节点对象可携带这些顶层键:

| 键 | 类型 | 含义 |
|---|---|---|
| `type` | string | 组件名——见[组件目录](widgets.md);可为 `{{ 绑定 }}`(如 `{{ item.kind }}`),渲染期按当前作用域求值决定节点类型,因此一个模板可按数据渲染成不同组件 |
| `id` | string | 稳定的节点 id(用于状态绑定、补丁、`data-state`) |
| `text` | string | 文本内容(text/heading/paragraph 节点) |
| `label` | string | 按钮 / 控件标签 |
| `placeholder` | string | 输入框占位符 |
| `value` | string | 输入值或绑定值;可含 `{{ binding }}` |
| `style` | object | 视觉样式——见[通用样式属性](#通用样式属性) |
| `layout` | object | 布局提示:`width` `height` `align` `justify` |
| `onPress` | action / string | 按下处理器——动作 id 或内联 steps |
| `onChange` | action / string | 变化处理器(输入、开关、滑块、下拉) |
| `onKeyDown` | action / string | 按键按下处理器——先发给焦点节点,未处理再冒泡到场景根;按键名作为 `key` 参数注入,以 `{{ key }}` 读取(原生 canvas 后端) |
| `onKeyUp` | action / string | 按键抬起处理器(分发同 `onKeyDown`) |
| `renderItem` | node | 绑定 `list` 的条目模板 |
| `data` | string | 列表数据绑定表达式(如 `state.todos`) |
| `children` | node[] | 子节点 |
| `condition` | string | 仅 `when` 节点:基于 `viewport.width` / `viewport.height` / `viewport.orientation` 的 `{{ … }}` 表达式,真值渲染 `then`,否则渲染 `else`;视口未知(服务端首帧)按假值处理 |
| `then` | node | 仅 `when` 节点:`condition` 为真时渲染的子树 |
| `else` | node | 仅 `when` 节点:否则渲染的子树(与隐藏单个节点的 `if` 属性不同,`when` 在两棵备选子树间切换) |
| `d` | string | 矢量路径的 SVG 路径数据字符串(`path` 组件) |
| `…` | any | 其余任何键都是组件专有**属性**(见下表) |

## 通用样式属性

由共享渲染器读取,任何绘制盒子的节点都可用:

- **Box (`style`)** — `width` `height` `minWidth` `maxWidth` `minHeight` `maxHeight` `padding` `margin` `gap` `background` `gradient` (linear / radial / **conic**, canvas) `radialGradient` `borderRadius` `borderWidth` `borderColor` `shadow` `opacity` `aspectRatio` `flexGrow` `flexShrink` `alignSelf` `zIndex` (canvas: sibling paint + hit order; `0` = auto) `position` `top` `right` `bottom` `left` `x` `y` (canvas absolute aliases for left/top) `cursor` `overflow` (`hidden` clips children; rounded when `borderRadius` set, canvas) `transition` `transitionEasing` `transitionYoyo` `transitionLoop` `transitionRepeat` (DOTween-style SetLoops on property tweens, canvas)
- **Text (`style`)** — `color` `fontSize` `fontWeight` `fontFamily` `lineHeight` `letterSpacing` `fontStyle` `textDecoration` (`underline` / `line-through` / `overline`) `textTransform` (`uppercase` / `lowercase` / `capitalize`) `textAlign` `textOverflow` (`ellipsis`) `lineClamp` `textStrokeColor` `textStrokeWidth` `textShadowColor` `textShadowBlur` `textShadowX` `textShadowY` (stroke/shadow: canvas)
- **Chrome / shadow (`style`, canvas)** — `strokeColor` `strokeWidth` `strokeDasharray` `strokeDashoffset` `outline` `outlineColor` `outlineWidth` `outlineOffset` `boxShadowColor` `boxShadowBlur` `boxShadowX` `boxShadowY` `boxShadowInset` (CSS inset box-shadow)
- **Filter / mask / clip (`style`, canvas)** — `filter` (`blur()` `brightness()` `contrast()` `saturate()` `grayscale()` `hue-rotate()` `opacity()` `drop-shadow()` `invert()` `sepia()`) `blur` `filterBlur` `contrast` `hue-rotate` (shortcuts) `dropShadowX` `dropShadowY` `dropShadowBlur` `dropShadowColor` (drop-shadow properties) `tint` (RGB modulate, canvas) `imageRendering` (`pixelated` nearest-neighbour, canvas) `mixBlendMode` (`multiply` / `screen` / `overlay` / `darken` / `lighten` / `difference` / `exclusion` / `color-dodge` / `color-burn` / `hard-light` / `plus-lighter` / `lighter`) `maskFade` (`top` / `bottom` / `left` / `right`) `maskFadeSize` `maskImage` `clipPath` (`circle()` / `ellipse()` / `inset(… round …)` / `polygon(...)`) `layerCache` (reuse offscreen bitmap when content fingerprint is unchanged)
- **Scroll snap (`style`, canvas)** — `scrollSnapType` (`x|y|both` + `mandatory|proximity`, on scroll viewports) `scrollSnapAlign` (`start` / `center` / `end`, on children)
- **Layout motion (`style`, canvas)** — `layoutMotion` (FLIP ease of absolute position/size when set with `transition` + stable `id`) — pair with `transition: "0.3s"` or `"0.3s spring"` / `transitionEasing: "spring"`
- **Transform (`style`, canvas)** — `rotate` (degrees) `scale` `scaleX` `scaleY` (0 = unset → 1) `flipX` `flipY` `skewX` `skewY` (degrees; graph shear) `transformOrigin` (CSS pivot: `center`, `left top`, `50% 0`, `12px 8px`; default center) — layout box unchanged; composes with entrance/`fx`/press scale
- **Game motion (node props, canvas)** — `fx` + `fxToken` (shake/punch/flash/hit/float/wobble/knockback/burst) · `timeline` + `timelineToken` (Sequence: Append/`parallel` Join, `path` follow, yoyo/loop) · `timelineOnComplete`/`onComplete` · `stagger` (list delay ms×index) · entrance `animation` + `curve` — see [animation](animation.md)
- **Pseudo-state (`style`)** — `hoverBackground` `hoverColor` `hoverOpacity` `hoverScale` `pressedBackground` `pressedScale` `pressedOpacity` `focusBorderColor` `disabled` `disabledOpacity`
- **Backdrop (`style`)** — `backdropBlur` (frosted-glass radius in px, capped at 120; `0` turns the frost off on `appbar` / `largetitle`, which are frosted by default) `backdropTint` (the translucent fill the blur shows through; a browser without `backdrop-filter` falls back to a solid panel)
- **Layout (`layout`)** — `width` `height` `align` `justify` (`wrap` on containers, `columns` on `grid`, `orientation` on `scroll`)
- **Accessibility (top-level)** — `role` `ariaLabel` `title` `tooltip`

Canvas 几何细节:`aspectRatio` 是宽/高,且仅在恰好一个正数 `width`/`height`
显式设置时推导另一轴。`position: "absolute"` 会脱离布局流;未设置 `left`/`x`
或 `top`/`y` 时,`right`/`bottom` 将盒子锚定到父内容边。`cursor` 经同一套
QSS/内联级联接受 `pointer`、`text`、`not-allowed`、`default`(以及 `hand`、
`ibeam`、`forbidden`、`arrow` 别名)。

## 各组件专有属性

在上述通用样式属性之外,每个渲染器额外读取的专有键。由 `internal/render` 的 `node()` 分发自动抽取——`—` 表示该组件只接受通用属性。

| 组件 | 属性 |
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
| `board` | `cameraCell` · `cameraCenter` · `cameraDeadZone` · `cameraLockLeft` · `cameraMax` · `cameraResetToken` · `cameraTarget` · `cameraViewport` · `disablePan` |
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
| `path` | `d` |
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
| `tilemap` | `atlas` · `bumpT` · `bumpX` · `bumpY` · `cell` · `rows` · `tileSize` |
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
| `webview` | `html` · `src` · `url` |
| `when` | — |
| `wifi` | `label` |
| `wrap` | `runSpacing` · `spacing` |
