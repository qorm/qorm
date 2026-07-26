<!-- data-lang-nav --> [English](../../tutorials/first-scene.md) · 中文

# 第一个场景

场景是 QORM 的 UI 入口。一个场景有一个 `id` 和一个根节点 `root`;节点树描述 UI。文本内容放在 `text` 字段里,`{{ state.x }}` 会插值全局状态。

## 最小场景

```json
{
  "type": "scene",
  "id": "main",
  "root": { "type": "text", "id": "hello", "text": "Hello QORM" }
}
```

## 带布局的场景

容器节点(`column` / `row`)使用 `style`(内边距、间距、背景)和 `layout`(尺寸、对齐)来排布它们的子节点。

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

- 文本使用 `text`(而非 `value`);模板插值(如 `"Welcome, {{ state.user }}"`)同样放在 `text` 里。
- 按钮通过 `onPress` 触发动作(参见 [第一个动作](first-action.md))。
- 所有可用的节点类型,参见 [组件目录](/api/zh/widgets.md)。

## 列表与条目作用域

`list` 把 `data` 绑定到一个数组,并重复渲染一份 `renderItem` 模板。在该模板内部,
当前行是 `item`,它的位置是 `index`(从 0 开始),外加 `first` / `last` 两个标志。

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

`index` / `first` / `last` 是独立的作用域名字,因此同名的数据字段不会被遮蔽:
`{{ item.index }}` 读的是你的数据,`{{ index }}` 读的是循环位置。

**嵌套列表**会在内层重新绑定 `item`。给外层列表一个 `as` 别名即可让它保持可达——
`"as": "section"` 会绑定 `section` 以及 `sectionIndex` / `sectionFirst` / `sectionLast`:

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

非法或保留的别名会退回 `item`,并在加载期给出警告。`gridview` 接受同样的 `as`
和同样的作用域名字。

### 值得知道的列表属性

| 属性 | 作用 |
|---|---|
| `separator` | 在条目之间画一条细线——`true`,或 `{ "height": 1, "inset": 16, "color": "…" }`。绝不画在末项之后 |
| `pageSize` + `page` | 内建分页:只渲染 `data` 的一个窗口。`page` 从 1 开始并会钳制到有效范围,页码越界时显示最后一页而不是空白 |
| `groupBy` + `sectionHeader` | 把**连续相等**的键切分为分节并加上分节标题。渲染器绝不重排你的数据——请先排序 |
| `sticky` + `stickyTop` | 滚动时分节标题吸顶(`sticky` 默认为 `true`);`stickyTop` 让它停在 appbar 之下 |
| `onRefresh` | 下拉刷新;派发指定的动作 |
| `reorderable` + `onReorder` | 长按拖拽重排 |

```json
{
  "type": "list", "id": "contacts", "data": "{{ state.contacts }}",
  "groupBy": "letter", "sectionHeader": "{{ item.letter }}", "stickyTop": 44,
  "separator": true, "pageSize": 50, "page": "{{ state.page }}",
  "onRefresh": "reloadContacts",
  "renderItem": { "type": "listtile", "text": "{{ item.name }}" }
}
```

`index` 始终相对于完整的 `data`,而不是当前页,所以序号会跨页连续。`groupBy` 与
`reorderable` 不会同时生效(分节标题会让拖拽索引错位),而 `reorderable` 优先于
`onRefresh`(两者会争抢同一个拖拽手势)。

注意 `virtualize` 只是一个渲染提示(`content-visibility`),不是真正的窗口化——
完整的条目标记仍会输出。当你需要 DOM 本身变小时,请用 `pageSize`。

## 标签页与面板

`tabs` 的标签文字来自一个 `tabs` 数组,面板来自 `children`,两者**按下标配对**:

```json
{
  "type": "tabs", "id": "detail",
  "tabs": ["Overview", "Specs", "Reviews"],
  "active": "{{ state.tab }}",
  "scrollable": true, "swipe": true, "lazy": true,
  "indicator": "pill", "indicatorColor": "var(--accent)",
  "onChange": "tabChanged",
  "children": [
    { "type": "text", "id": "t1", "text": "Overview…" },
    { "type": "text", "id": "t2", "text": "Specs…" },
    { "type": "text", "id": "t3", "text": "Reviews…" }
  ]
}
```

| 属性 | 作用 |
|---|---|
| `active` | 当前打开的是第几个标签页,从 0 开始。越界会钳制回范围内 |
| `scrollable` | 横向可滚动的标签条;无论标签页是怎么切换的,当前标签都会自动滚入视野 |
| `swipe` | 在面板上横向拖动即切到相邻标签页。它触发的是那个标签自己的控件,所以三种模式下行为一致;两端不循环 |
| `lazy` | 只渲染当前面板 |
| `indicator` · `indicatorColor` | `"pill"` 或 `"none"`(其它值都是默认的下划线),颜色由 `indicatorColor` 决定 |

把 `active` 绑定到一个**纯**  `{{ state.x }}` 路径,标签页就变成**受控**的:
标签条自己写入那个状态键,因此切换标签完全不需要 action 文件,任何绑定同一个键的
节点也会随之联动。像 `{{ state.i + 1 }}` 这样的表达式不是纯路径,只会渲染成一个
只读的值。

`onChange` 在每次切换时触发,并额外合入两个参数:`index`(0 起的位置,字符串)
与 `tab`(标签文字)。`lazy` 要求标签页是受控的、或者带有 `onChange`——两者都没有
时切换纯粹发生在浏览器里,只能显示已经在 DOM 里的面板,因此 `lazy` 会被忽略。
`indicatorColor` 要求节点有一个普通标识符形式的 `id`,并且它的值会经过一张严格的
字符白名单:任何可能提前结束 CSS 声明的内容都会被整体丢弃,而不是原样输出。

## 单元格里放组件的表格

`table` 与 `datatable` 按 `columns` 渲染 `data`。一个列可以是一个键字符串,也可以是
一个带 `value`(数据键)、`label`(表头文字)、`width`、`sticky` 的对象。

默认单元格就是 `obj[key]` 的纯文本。给表格一个**带 `column` 的子节点**,该列就改为
渲染你的组件:

```json
{
  "type": "table", "id": "people",
  "data": "{{ state.people }}",
  "columns": [
    { "value": "name", "label": "Name", "width": 180, "sticky": true },
    { "value": "role", "label": "Role" }
  ],
  "stickyHeader": true, "stickyTop": 44,
  "scrollX": true, "maxHeight": 420, "minWidth": 720,
  "children": [
    { "type": "tag", "id": "role", "column": "role", "label": "{{ row.role }}" },
    { "type": "text", "id": "bio", "detail": true, "label": "Details", "text": "{{ row.bio }}" }
  ]
}
```

在单元格模板内部,当前行是 `row`,它的位置是 `rowIndex` / `rowFirst` / `rowLast`,
单元格本身是 `cell`——`{{ cell.value }}`、`{{ cell.column }}`、`{{ cell.index }}`。
这与列表用的是同一套别名机制,所以 `"as": "person"` 会把这组名字改成 `person` /
`personIndex` / `personFirst` / `personLast`,外层列表的 `item` 仍然可以同时读到。

带 `detail: true` 的子节点不是列:它在每一行下面渲染一个可展开行,用的是浏览器
自己的折叠控件。它的 `label`(或 `text`)是摘要行,在行作用域里求值。

除 `sticky` 属于列对象外,其余外观键都写在表格节点上:

| 键 | 效果 |
|---|---|
| `stickyHeader` + `stickyTop` | 表头在表体滚动时固定;`stickyTop` 把它停在 appbar 下面 |
| `scrollX` | 表格在自己的盒子里横向滚动 |
| `maxHeight` | 表体在该高度内纵向滚动 |
| `minWidth` | 开启 `scrollX` 时避免列被压扁 |
| 列的 `sticky` | 横向滚动时冻结该前置列 |

`datatable` 在自己的选择、排序属性之外同样支持以上全部。两者都没有内建分页——
参见[第一个动作](first-action.md)。

## 手风琴、树、时间线、轮播

| 组件 | 值得知道的属性 |
|---|---|
| `accordion` | `active` 是当前展开面板的下标(可绑定、会钳制、默认第一个)。`single: true` 让面板互斥——展开一个就收起其余;默认是各自独立开合 |
| `tree` | `collapsed: true` 让所有分支初始收起。数据项自己的 `expanded` 字段仍会覆盖它自己那一个节点 |
| `timeline` | `items` 的每一项可以带 `time`(渲染在标题上方)、`color`(圆点颜色)与 `icon`(内置 SVG 图标集里的名字,画在圆点里;未知名字退回普通圆点) |
| `carousel` | `autoplay: 4000` 按时钟推进(下限 250 毫秒,指针悬停或浏览器标签页隐藏时暂停,到末尾回卷)。`indicators: true` 渲染一行可点的圆点,当前点由实时滚动位置推导 |

## 会折叠的大标题

`largetitle`(别名 `sliverappbar`、`cupertinolargetitle`)绘制 iOS 大标题头,并且
**默认会折叠**:紧凑条吸顶,大标题在它后面向上滚走并交叉淡入紧凑标题。

```json
{ "type": "largetitle", "id": "hdr", "label": "Places", "subtitle": "8 saved nearby",
  "children": [ { "type": "icon", "id": "hdr-plus", "name": "plus", "size": 20 } ] }
```

- 标题文字是节点的 `label`(或 `text`);`subtitle` 可选。
- `children` 是紧凑条右侧的操作区。
- `"collapsible": false` 恢复旧的静态头——只有一个标题、没有吸顶条,与折叠能力
  出现之前逐字节一致。
- `background` 决定条的填充(默认 `var(--bg)`)。
- `appbar` 是另一个组件,不会折叠。

折叠本身只是普通的 sticky 定位,因此到处都能用;交叉淡入在支持滚动驱动动画的
浏览器上走 CSS,不支持的浏览器则由一小段脚本驱动同一批声明。

## 底部弹层(sheet)

`sheet`(别名 `bottomsheet`、`draggablesheet`、`draggablescrollablesheet`、
`modalbottomsheet`)是一个可以在若干吸附档位之间拖动的底部面板。

```json
{ "type": "sheet", "id": "detail", "open": "{{ state.detail }}",
  "title": "Blue Bottle Coffee",
  "snapPoints": [0.3, 0.6, 0.92], "initialSnap": 1,
  "onSnap": { "name": "setSnap" },
  "children": [ { "type": "text", "id": "d1", "text": "Ferry Building · 240m" } ] }
```

| 属性 | 作用 |
|---|---|
| `open` | 取值为假时**什么都不渲染**——没有隐藏的标记。不写这个键则弹层始终存在 |
| `snapPoints` | 档位阶梯,取值是所在盒子高度的比例,升序排列。大于 1 的值按百分比读(`90` 即 `0.9`)。它本身也可以是一个绑定到数组的表达式。默认 `[0.25, 0.5, 0.9]` |
| `initialSnap` | 阶梯的下标;越界回退到最低档 |
| `onSnap` | 一个动作,每次拖拽停稳时派发。它会为每个档位各注册一次,并各自带上参数 `snap`——该档位的下标,因此动作里用 `{{ snap }}` 就知道停在哪 |
| `onClose` | 弹层被关闭时派发 |
| `title` · `handle` · `backdrop` | 标题行、拖拽把手(`false` 去掉)与背景遮罩(`false` 去掉) |

如果 `open` 绑定的是一个纯状态路径,且 `dismissable` 不为 `false`,点击遮罩就会
经由内建的 dismiss 关闭弹层,常见场景因此不需要任何 action 文件。把弹层甩到最低
档位的 60% 以下,也会以同样的方式关闭。没有 Escape 键关闭。

只有把手那一行会占用拖拽手势——内容照常滚动——所以弹层里可以放列表、可滑动行或
可重排列表,两种手势不会互相打架。

`examples/places` 把这三样放在了一起:会折叠的大标题、一块毛玻璃面板,以及一个在
三个档位之间拖动的弹层。

```sh
qorm run examples/places
```

## 交互态样式

伪状态样式键描述节点如何响应交互,不需要 JavaScript,也不需要额外节点:

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

八个键是 `hoverBackground`、`hoverColor`、`hoverOpacity`、`pressedScale`、
`pressedOpacity`、`focusBorderColor`、`disabled` 和 `disabledOpacity`。悬停样式
只在真正支持悬停的设备上生效,因此一次点按不会把节点卡在悬停态;`disabled`
同时会输出 `aria-disabled`。

`width` 与 `height` 接受像素数字、`"fill"`,或带单位的字符串——`"50%"`、`"30vw"`、
`"40vh"`、`"120px"`。层叠与弹性行为则由 `zIndex`、`alignSelf`、`flexShrink` 提供。

## 图片

`src` 与 `alt` 会被插值,因此图片可以放在 `renderItem` 里。三个属性覆盖了一张
远程图片会经历的各个状态:

```json
{ "type": "image", "id": "hero",
  "src": "{{ item.photo }}", "alt": "{{ item.caption }}",
  "placeholder": "#e5e5ea",
  "fallback": "/assets/missing.png",
  "fit": "cover" }
```

- `placeholder` 是加载期间显示的内容——一个 CSS 颜色,或一张低清 URL 作为模糊占位背景。
- `fallback` 在图片加载失败时顶替它。替换只发生一次,所以坏掉的 fallback 不会死循环。
- 懒加载**默认开启**;首屏图片可以设 `"lazy": false` 关掉它。

`placeholder` 与 `fallback` 是静态值,不是绑定。

## 在运行时选择组件

节点的 `type` 本身可以是一个绑定,在每次渲染时按当前作用域解析。一份列表模板
因此能按行渲染不同的组件,而不必堆叠 `if` 分支:

```json
{
  "type": "list", "id": "feed", "data": "{{ state.messages }}",
  "renderItem": { "type": "{{ item.kind }}", "text": "{{ item.text }}", "src": "{{ item.src }}" }
}
```

当 `kind` 为 `"text"` 或 `"image"` 时,同一行会渲染成文本节点或图片。如果该绑定
解析为空,或解析出的名字没有对应组件,节点会退化为渲染器的未知节点占位,并且
诊断信息会点名该表达式——所以拼写错误是看得见的,而不是悄无声息。

表达式语言本身,参见[表达式](../expressions.md)。
