# 响应式布局

QORM 在服务端渲染响应式 UI:浏览器通过 `/viewport` 上报视口,运行时重新求值绑定,SSE 推送更新后的 HTML。使用 `when` 节点在两棵**备选**子树之间切换(与 `if` / `visible` 不同,后者只是隐藏同一节点)。

可运行示例:[`examples/responsive`](https://github.com/qorm/platform/tree/main/examples/responsive)。

## 视口变量

场景绑定中可读:

| 变量 | 类型 | 含义 |
|---|---|---|
| `viewport.width` | number | 窗口宽度(px),未知时为 0 |
| `viewport.height` | number | 窗口高度(px) |
| `viewport.orientation` | string | `"portrait"` / `"landscape"`,未知时为 `""` |

服务端首帧在客户端上报尺寸之前渲染,因此 `viewport.width` 为 0、方向为空 —— `when` 条件为**假**,渲染 `else` 分支,直到客户端 POST 视口。

## 断点变量

在 `qorm.json` 中声明命名宽度阈值:

```json
{
  "breakpoints": { "sm": 640, "md": 768, "lg": 1024, "xl": 1280 }
}
```

省略 `breakpoints` 时使用相同默认值(`sm` 640,`md` 768,`lg` 1024,`xl` 1280)。

每个名称在表达式中为布尔值:

| 变量 | 为真当 |
|---|---|
| `breakpoint.sm` | `viewport.width >= 640`(且视口已知) |
| `breakpoint.md` | `viewport.width >= 768` |
| … | … |

示例 —— 优先使用断点而非裸宽度比较:

```json
{
  "type": "when",
  "id": "layout",
  "condition": "{{ breakpoint.md }}",
  "then": { "type": "row", "id": "wide", "children": [ … ] },
  "else": { "type": "column", "id": "narrow", "children": [ … ] }
}
```

仍可在同一表达式中组合 `viewport.*` 与 `breakpoint.*`,例如
`{{ breakpoint.lg && viewport.orientation == 'landscape' }}`。

`qorm_inspect` 会返回有效的 `breakpoints` 映射,供 Agent 读取。

## 客户端行为

窗口缩放时,内联脚本去抖 POST `/viewport` `{w,h}`。服务端全量重渲染(视口变更不走局部 patch)并通过 SSE 广播。客户端将新 HTML morph 进 `#qorm-root`,尽量保留焦点与滚动。

另见 HTTP API 参考中的 [`/viewport`](/api/http-api.md)。
