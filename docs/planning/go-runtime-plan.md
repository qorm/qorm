# QORM Go 运行时规划（可交付、跨平台）

更新时间：2026-07-03
文档性质：架构决策 + 实施规划（历史记录）

> **现状（后续更新）**：Rust 参考实现已整体移除，Go 运行时已提升为仓库根（`cmd/`、`internal/`、`go.mod` 位于根目录，module 为 `github.com/qorm/qorm`）。本文保留为当初引入 Go 的决策依据；文中 `runtime-go/` 前缀现即仓库根。

## 一、为什么引入 Go 运行时

- QORM 的**真正资产是 JSON 格式与语义**，与实现语言无关。Go 运行时可以直接运行**同一批 example 应用**，无需改一行 JSON。
- 纯 Go（不使用 cgo）可从 Mac **一条命令交叉编译**出 macOS / Linux / Windows × amd64 / arm64 的单文件可执行程序。这是 Rust 工具链做不到的开箱即用体验。
- 现有 Rust 树 `cargo build --workspace` / `clippy` 当前**失败**（wasm-bridge 不编译、大量死代码），把"可运行的产品层"押在 Go 上能立刻得到一个**真实跑通、随处可发**的成果。

Rust 侧保留为 IR/spec 的参考实现；Go 侧是**面向交付的运行时**。两者以 JSON 格式为契约。

## 二、架构

```text
QORM app (JSON: manifest + scenes + actions)
        │  loader
        ▼
  model.App  (Node 树 / Action / GlobalState)
        │  runtime (state store + expr eval + action dispatch)
        ▼
  render → HTML + CSS(flexbox)  ← 用浏览器做布局，不自研布局引擎
        │  server (HTTP + 事件回传 + 局部刷新)
        ▼
  浏览器 / 系统 WebView       单文件二进制，跨平台
```

关键决策：**渲染直接产出 HTML/CSS flexbox，交给浏览器布局**。这比移植 Rust 绝对定位布局引擎更简单、更健壮、视觉更好，并天然解决之前发现的 button 0×0 尺寸问题。

## 三、包结构（`runtime-go/`）

| 包 | 职责 |
|---|---|
| `internal/model` | App / Node / Action / GlobalState 数据模型 |
| `internal/loader` | 读目录（跳过 `type:test`），解析 manifest/scene/action |
| `internal/expr` | 表达式求值器（`{{ count + 1 }}` / `{{state.x}}`）：算术、比较、逻辑、三元、成员访问、字符串 |
| `internal/runtime` | 状态存储、绑定插值、action 分发（state.set/append/toggle） |
| `internal/render` | Node → 语义化 HTML + CSS；**完整控件集** |
| `internal/server` | HTTP 服务 + `/event` 回传 + 极简内联 JS 局部刷新 |
| `cmd/qorm` | CLI：`run` / `render` |
| `scripts/build-all.sh` | 交叉编译全平台 |

## 四、UI 完整性（控件集）

Go 渲染器一次性覆盖之前 Rust 侧缺失/部分的控件：

| 控件 | HTML 映射 | 状态 |
|---|---|---|
| row/column/stack/absolute | `div` + flexbox | [done] |
| text | `div` | [done] |
| button | `<button>` + onPress | [done] 交互 |
| input | `<input>` + 双向绑定 state | [done] |
| checkbox / switch | `<input type=checkbox>`（switch 加样式） | [done] |
| slider | `<input type=range>` | [done] |
| image | `<img>` | [done] |
| progress | `<progress>` / 条 | [done] |

对账原则：**渲染器支持的控件集 = 对外承诺的控件集**，新增控件必须三段齐（parse/render/交互）。

## 五、实现进度

- [done] 全链路：加载 → 状态 → 求值 → 分发 → 渲染 → 服务 → 浏览器交互。
- [done] **counter 完全可交互**（补齐缺失的 `decrement` action）：点击 +/- 实时改变计数。
- [done] **todo 完全可交互**：`list` 数据绑定重复 + 输入回传 + 补齐 `addTodo`/`toggleTodo`。
- [done] 交叉编译脚本 + 实际产出 6 平台二进制（纯 Go，无 cgo）。
- [done] **签名 bundle**（`internal/bundle`+`internal/keys`，纯 Go ed25519）：`keygen`/`build --key`/`verify --trust`，篡改与非信任 key 均拒绝。
- [done] **OTA + 回滚**（`internal/ota`）：`POST /update` 拉取→验签→热切换；`POST /rollback`；坏更新拒绝且不影响在运行的 app。
- [done] **MCP server**（`internal/mcp`，stdio JSON-RPC）：inspect/render/get_node/list_actions/simulate（simulate 无副作用）。
- [done] **原生 WebView（Wails 式）**：`-tags desktop` 用平台原生 WebView（macOS WKWebView 已实测编译+运行）；默认纯 Go 走浏览器 app 模式。两条构建路径并存。
- ⬜ 后续：HTTPS OTA server + key 吊销；Agent `apply_patch`（绑定 preview 的写操作）；Linux/Windows 原生构建 CI 矩阵。

## 六、构建两路架构（参考 Wails）

| 路径 | 构建 | WebView | 跨平台 | 场景 |
|---|---|---|---|---|
| 默认（纯 Go） | `go build` / `build-all.sh` | 系统浏览器（`--app` 无边框窗口） | [done] 单机交叉编译全平台 | 分发、CI、服务器 |
| `-tags desktop` | `build-desktop.sh` | 原生 WKWebView/WebView2/WebKitGTK（cgo） | [warn] 按平台构建（同 Wails） | 桌面原生窗口 |

两路共用同一 QORM 内核（loader→runtime→render→server），仅窗口宿主不同。

## 七、验收（全部通过）

1. `go build ./...` + `go vet ./...` + `gofmt` 干净；`go test ./...` 全绿（bundle/server/mcp/integration）。
2. `qorm run examples/counter --app`：点击 +/- 实时改变计数。
3. `build-all.sh` 产出 6 平台纯 Go 二进制；`build-desktop.sh` 产出本机原生窗口二进制（已编译+运行验证）。
4. 签名/OTA/MCP 端到端实测通过（篡改拒绝、坏更新不影响运行、simulate 无副作用）。
