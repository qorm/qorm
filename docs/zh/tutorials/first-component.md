<!-- data-lang-nav --> [English](../../tutorials/first-component.md) · 中文

# 第一个组件

组件让你可以复用 UI 结构。组件是一个模板,**声明在 `qorm.json` 的 `components` 中**;在模板内部,`{{ prop.x }}` 读取实例传入的属性。用一个 `type` 等于组件名的节点来实例化它。

## 声明一个组件(qorm.json)

```json
{
  "type": "app",
  "id": "my_app",
  "entry": "main",
  "components": {
    "user_card": {
      "type": "card",
      "style": { "padding": 16, "gap": 4 },
      "children": [
        { "type": "text", "text": "{{ prop.name }}",  "style": { "fontWeight": 700 } },
        { "type": "text", "text": "{{ prop.email }}", "style": { "color": "#8e8e93" } }
      ]
    }
  }
}
```

## 使用一个组件(场景)

节点的 `type` 就是组件名;属性作为普通字段直接写在节点上。

```json
{ "type": "user_card", "id": "u1", "name": "Ada", "email": "ada@example.com" }
```

## 插槽(填充子内容)

在模板中放置一个 `{ "type": "slot" }` 占位符;实例的 `children` 会被填入其中。

```json
"components": {
  "panel": {
    "type": "card",
    "style": { "padding": 16, "gap": 6 },
    "children": [
      { "type": "text", "text": "{{ prop.title }}", "style": { "fontWeight": 800 } },
      { "type": "slot" }
    ]
  }
}
```

实例传入 `children` 来填充插槽:

```json
{ "type": "panel", "id": "acct", "title": "Account", "children": [
  { "type": "text", "text": "Plan: Pro" },
  { "type": "text", "text": "Seats: 12" }
] }
```

## 把实时数据作为 prop 传入

prop 的值可以是一个绑定。它在**实例所在的**作用域中求值一次——所以
`{{ state.x }}`、`{{ item.x }}` 与路由参数都会被解析——并且结果保留原有类型:
布尔仍是布尔,数字仍是数字,列表仍是列表。

```json
{ "type": "stat_card", "id": "cpu",
  "label": "CPU",
  "value":  "{{ state.metrics.cpu }}",
  "warn":   "{{ state.metrics.cpu > 80 }}",
  "series": "{{ state.metrics.history }}" }
```

在模板内部,这些 prop 是真正的值,而不是文本:

```json
{
  "type": "column",
  "children": [
    { "type": "text", "text": "{{ prop.label }}: {{ prop.value * 100 }}%" },
    { "type": "text", "if": "{{ prop.warn }}", "text": "High" },
    { "type": "list", "data": "{{ prop.series }}",
      "renderItem": { "type": "text", "text": "{{ item }}" } }
  ]
}
```

由于组件实例可以放在 `renderItem` 里,这正是组件能当作列表行模板使用的原因——
直接把 `{{ item.… }}` 传进去即可。

如果你希望把 prop 与节点自身的字段分开,可以写在嵌套的 `props` 对象里;
其中的键会覆盖顶层的同名键。

```json
{ "type": "stat_card", "id": "cpu", "props": { "label": "CPU", "value": "{{ state.cpu }}" } }
```

## 回调 prop

invoke 的 `name` 本身也可以是绑定,因此组件可以把"要执行哪个动作"作为 prop 接收。
名字在处理器注册时被解析,所以按钮派发的是真正的那个动作:

```json
"components": {
  "confirm_bar": {
    "type": "row",
    "children": [
      { "type": "button", "id": "ok",     "text": "{{ prop.okLabel }}", "onPress": { "name": "{{ prop.onConfirm }}" } },
      { "type": "button", "id": "cancel", "text": "Cancel",             "onPress": { "name": "{{ prop.onCancel }}" } }
    ]
  }
}
```

```json
{ "type": "confirm_bar", "id": "del", "okLabel": "Delete",
  "onConfirm": "deleteItem", "onCancel": "closeDialog" }
```

## 具名插槽

模板可以通过给每个插槽一个 `name` 来声明多个插槽,实例则用 `slot` 字段把每个子节点
归属到其中之一。没有 `slot` 的子节点填入匿名插槽。

```json
"components": {
  "frame": {
    "type": "column",
    "children": [
      { "type": "slot", "name": "header" },
      { "type": "slot" },
      { "type": "slot", "name": "footer", "children": [
        { "type": "text", "text": "No actions" }
      ] }
    ]
  }
}
```

```json
{ "type": "frame", "id": "f1", "children": [
  { "type": "text",   "text": "Account",  "slot": "header" },
  { "type": "text",   "text": "Body copy" },
  { "type": "button", "text": "Save", "onPress": "save", "slot": "footer" }
] }
```

插槽自身的 `children` 是它的**默认内容**:只有在没有任何节点填充该插槽时才渲染,
所以上面的 `frame` 在实例没有提供 footer 子节点时会显示 "No actions"。单个匿名插槽
的行为与以前完全一致,因此已有组件不受影响。

- `{{ prop.* }}` 只在组件模板内部可见;实例上同名的字段就是传入的值。
- 组件可以嵌套组件(最深 32 层);模板内部的 id 会按实例加后缀,因此两个实例绝不会冲突。
- 组件没有自己的局部状态或生命周期——它们通过你传入的 prop 读取全局状态。
- 完整的可运行示例,参见 [`examples/uikit`](https://github.com/qorm/qorm/tree/main/examples/uikit)(metric / kv / panel)。
