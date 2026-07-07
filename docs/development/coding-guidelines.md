# QORM Coding Guidelines

## Go 规则

- 导出类型/函数必须有 doc 注释。
- 错误必须结构化（`error` 包裹 + 诊断上下文），不吞错。
- 热路径避免频繁分配。
- 使用明确的 ID 类型，不滥用裸 string。
- SourceRef 必须保留到诊断可定位。
- 默认路径保持**纯 Go 无 cgo**（可交叉编译）；cgo 只在 `-tags desktop` WebView 窗口等场景使用。

## 错误类型

错误应包含：

```text
code
message
source file
json pointer
suggestion
severity
```

## 命名

运行时是根目录下的单个 Go module（`github.com/qorm/qorm`）；核心包在 `internal/` 下。

```text
internal/model    → 数据模型
internal/runtime  → 运行时（state / action / 表达式 / i18n）
internal/render   → 渲染为 HTML/CSS
```

## JSON 字段

- 使用 camelCase。
- `type` 表示对象类型。
- `id` 在作用域内唯一。
- 引用使用 URI-like 或相对路径。

## Runtime 热路径

禁止：

```text
每帧解析 JSON
每帧全量 layout
每帧全量重建 Display List
每帧重新 shape 所有文本
事件热路径调用 MCP
```

## 安全

任何 Host Capability、Plugin、Native Bridge、Agent Tool 都必须声明权限。
