# 节点与组件属性

> 由源码自动生成(`TestAPIRef`),请勿手工编辑。下方的属性表从代码抽取,不会与实现漂移。

每个 QORM 应用据以编写的声明式契约。界面是一棵**节点**树,每个节点是一个 JSON 对象。

## 节点结构

每个节点对象可携带这些顶层键:

| 键 | 类型 | 含义 |
|---|---|---|
| `type` | string | 组件名——见[组件目录](widgets.md) |
| `id` | string | 稳定的节点 id(用于状态绑定、补丁、`data-state`) |
| `text` | string | 文本内容(text/heading/paragraph 节点) |
| `label` | string | 按钮 / 控件标签 |
| `placeholder` | string | 输入框占位符 |
| `value` | string | 输入值或绑定值;可含 `{{ binding }}` |
| `style` | object | 视觉样式——见[通用样式属性](#通用样式属性) |
| `layout` | object | 布局提示:`width` `height` `align` `justify` |
| `onPress` | action / string | 按下处理器——动作 id 或内联 steps |
| `onChange` | action / string | 变化处理器(输入、开关、滑块、下拉) |
| `renderItem` | node | 绑定 `list` 的条目模板 |
| `data` | string | 列表数据绑定表达式(如 `state.todos`) |
| `children` | node[] | 子节点 |
| `condition` | string | 仅 `when` 节点:基于 `viewport.width` / `viewport.height` / `viewport.orientation` 的 `{{ … }}` 表达式,真值渲染 `then`,否则渲染 `else`;视口未知(服务端首帧)按假值处理 |
| `then` | node | 仅 `when` 节点:`condition` 为真时渲染的子树 |
| `else` | node | 仅 `when` 节点:否则渲染的子树(与隐藏单个节点的 `if` 属性不同,`when` 在两棵备选子树间切换) |
| `…` | any | 其余任何键都是组件专有**属性**(见下表) |

## 通用样式属性

由共享渲染器读取,任何绘制盒子的节点都可用:

- **Box (`style`)** — `width` `height` `minWidth` `maxWidth` `minHeight` `maxHeight` `padding` `margin` `gap` `background` `gradient` `borderRadius` `borderWidth` `borderColor` `shadow` `opacity` `aspectRatio` `flexGrow` `position` `top` `right` `bottom` `left` `cursor` `transition`
- **Text (`style`)** — `color` `fontSize` `fontWeight` `fontFamily` `lineHeight` `letterSpacing` `fontStyle` `textDecoration` `textTransform` `textAlign` `lineClamp`
- **Layout (`layout`)** — `width` `height` `align` `justify` (`wrap` on containers, `columns` on `grid`, `orientation` on `scroll`)
- **Accessibility (top-level)** — `role` `ariaLabel` `title` `tooltip`

## 各组件专有属性

在上述通用样式属性之外,每个渲染器额外读取的专有键。由 `internal/render` 的 `node()` 分发自动抽取——`—` 表示该组件只接受通用属性。

| 组件 | 属性 |
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
| `link` | `href` |
| `list` | `itemHeight` · `onReorder` · `reorderable` · `virtualize` |
| `listsection` | `footer` · `header` |
| `listtile` | `chevron` · `leading` · `subtitle` · `trailing` |
| `location` | `label` |
| `loginitem` | — |
| `materialstepper` | `active` |
| `menu` | — |
| `modal` | `open` · `title` |
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
