# QORM 标准化格式方案

## 决策

QORM V1 统一选择 **JSON** 作为源格式、Bundle 格式、Patch 格式和 IR 序列化格式。

不采用 YAML 或 TOML。原因：

- JSON 对 Agent 友好，便于生成、解析和 Patch。
- JSON 易于跨语言 SDK 支持。
- JSON Schema 可直接用于校验。
- MCP、LSP、SDK、Bundle、Patch 天然适配 JSON。
- 桌面端、移动端可直接内置 Go 运行时（移动端为 Go→WASM）动态解析。

## 文件命名

根文件固定为：

```text
qorm.json
```

其它文件不使用自定义后缀，统一用 `.json`：

```text
scenes/main.json
components/button.json
styles/theme.json
motions/basic.json
actions/user.json
resources/zh-CN.json
platforms/mobile.json
agents/codex.json
patches/demo.json
tests/counter.json
```

文件语义通过 `type` 字段区分：

```json
{
  "qorm": "0.1",
  "type": "scene",
  "id": "main"
}
```

## 文件类型

| type | 用途 |
|---|---|
| `app` | 应用根配置，通常是 `qorm.json` |
| `scene` | 场景文件 |
| `component` | 组件定义 |
| `style` | 样式、token、variant |
| `motion` | 动效定义 |
| `action` | Action 定义 |
| `resource` | 多语言和资源表 |
| `platform` | 平台能力声明 |
| `plugin` | 插件声明 |
| `agent` | Agent Pack 配置 |
| `patch` | Patch 文件 |
| `test` | 测试场景 |
| `bundle` | 构建后的运行包 |

## 引用规则

逻辑引用使用 URI-like 字符串：

```json
{
  "entry": "scene://main",
  "styles": ["style://theme"],
  "actions": ["action://user"],
  "resources": ["resource://zh-CN"]
}
```

本地文件引入使用相对路径：

```json
{
  "imports": ["./scenes/main.json"],
  "components": ["./components/button.json"]
}
```

### 规则

- 文件路径用于加载源文件。
- 逻辑引用用于运行时与 IR 中的稳定 ID。
- 同一命名空间内重复 ID 必须报错。
- Resolver 必须在进入 Typed IR 前完成规范化。

## 嵌入规则

QORM 支持三类组合方式：

```text
import     引入定义，不展开内容
include    构建时合并内容
embed      将资源嵌入 Bundle
```

示例：

```json
{
  "imports": ["./components/button.json"],
  "includes": ["./styles/theme.json"],
  "embeds": ["./assets/icons.json"]
}
```

## Patch 与 Source Map

- Patch 使用逻辑路径，例如 `/scenes/main/nodes/submit_button/text`。
- Source Map 使用源文件 JSON Pointer，仅用于诊断、回源和编辑器定位。
- 两者不能混用。

## Bundle

开发时使用多个 JSON 源文件。生产发布时生成：

```text
qorm.bundle.json
```

Bundle 包含：

```text
manifest
resolved scenes
resolved components
resolved styles
resolved motions
resolved actions
resource table
capability requirements
compiled execution plans
signature/hash/version/keyId
```

## 不做的事

V1 不做自定义 DSL。后续如果增加 `.qorm` DSL，也必须编译成同一套 JSON IR。