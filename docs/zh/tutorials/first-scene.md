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
