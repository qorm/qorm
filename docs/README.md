# QORM - 扩梦

**QORM** = **Queryable Object Rendering Model**  
中文名：**扩梦**

QORM 是一个 Go 原生、Agent 可查询的 UI Runtime，用于构建可检查、可修改、可跨平台渲染的交互界面系统。运行时以纯 Go 实现：默认在服务端渲染为 HTML/CSS 交给浏览器/WebView 布局，离线场景则把同一份 Go 通过 `cmd/qorm-wasm` 编译为 WebAssembly 在客户端运行。当前权威架构见 [Go 架构](overview/go-architecture.md)。

QORM 的目标不是成为简化版 HTML/CSS/JavaScript，也不是成为完整游戏引擎，而是提供一套统一的 UI 描述、布局、渲染、事件、Action、Motion、Host Capability、Agent Patch 和多平台适配体系。

## 核心原则

1. **统一源格式：JSON**  
   除根配置 `qorm.json` 外，scene、component、style、motion、action、resource、platform、agent 等文件均使用 `.json` 后缀，通过 `type` 字段区分语义。

2. **核心使用 Go**  
   Loader、Model、Runtime、State、Action、表达式、i18n、Render、Host Capability、Bundle 等核心模块以纯 Go 实现（module `github.com/qorm/qorm`，代码位于 `cmd/`、`internal/`）。

3. **离线优先靠 Go→WASM**  
   服务端渲染是默认路径；离线（PWA/APK/IPA）时把运行时和应用一起通过 `cmd/qorm-wasm` 编译为 WebAssembly，在 WebView 内本地运行 `qorm.bundle.json`，便于更新、灰度和回滚。

4. **不考虑 JIT**  
   执行层使用 Typed IR、Execution Plan 和解释器；性能重点放在前台渲染效率上。

5. **GPU-first 渲染**  
   渲染层使用 Display List、Render Graph、Dirty Tree、缓存、批处理、纹理图集和增量更新。

6. **QORM 是 UI 层**  
   底层能力通过 Host Capability、Platform Pack、Native Bridge、Plugin、SDK、MCP 和 Skill 接入。

7. **Agent 原生**  
   QORM 支持 inspect、validate、preview patch、apply patch、simulate、explain、platform check 等 Agent 操作。

## 仓库结构建议

运行时是根目录下的单个 Go module（`go.mod` module 为 `github.com/qorm/qorm`）：

```text
qorm/
├─ cmd/
│  ├─ qorm/            # CLI：run / build / package / inspect …（含平台桥、打包器）
│  └─ qorm-wasm/       # 离线客户端：运行时+应用编译为 WASM 在 WebView 内运行
├─ internal/
│  ├─ model/           # App / Node / Action / GlobalState 数据模型
│  ├─ loader/          # 解析目录 / bundle -> model.App
│  ├─ runtime/         # state store + 表达式求值 + i18n + action 分发
│  ├─ render/          # 渲染为 HTML/CSS（widget -> DOM）
│  ├─ server/          # 服务端渲染 + SSE 实时会话 + qormToNative 派发
│  ├─ bundle/, keys/   # 纯 Go ed25519 签名 bundle
│  └─ mcp/             # MCP server 工具暴露
├─ pkg/qormext/        # 用户中间层：app 用 Go 注册自己的 native op
├─ extensions/         # vscode 等编辑器扩展
├─ examples/
├─ schemas/
├─ docs/
└─ tools/
```

> 历史说明：早期参考实现曾计划为 Rust（`crates/`），已整体移除；现仓库为纯 Go。以 [Go 架构](overview/go-architecture.md) 为准。

## 最小 JSON 示例

```json
{
  "qorm": "0.1",
  "type": "scene",
  "id": "counter_scene",
  "state": {
    "count": 0
  },
  "root": {
    "type": "column",
    "id": "root",
    "layout": {
      "width": "fill",
      "height": "fill",
      "gap": 16,
      "padding": 24,
      "align": "center",
      "justify": "center"
    },
    "children": [
      {
        "type": "text",
        "id": "count_text",
        "value": "当前数值：{{ count }}"
      },
      {
        "type": "button",
        "id": "inc_button",
        "text": "+",
        "on": {
          "press": [
            {
              "type": "state.set",
              "path": "count",
              "value": "{{ count + 1 }}"
            }
          ]
        }
      }
    ]
  }
}
```

关键约束：
- `"{{ expr }}"` 作为整字段值时返回 typed value。
- `"当前数值：{{ count }}"` 属于模板插值字符串。
- `state path` 与 `patch path` 不是同一种路径。
- `network.request` 是 `host.call` 的 capability，不是独立脚本入口。

## 建议阅读顺序

- [JSON 格式规格](docs/spec/json-format-spec.md)
- [IR 规格](docs/spec/ir-spec.md)
- [Runtime 规格](docs/spec/runtime-spec.md)
- [Action 规格](docs/spec/action-spec.md)
- [Motion 规格](docs/spec/motion-spec.md)
- [Host Capability 规格](docs/spec/host-capability-spec.md)
- [Agent Protocol 规格](docs/spec/agent-protocol-spec.md)
- [Concept Boundaries](docs/spec/concept-boundaries-spec.md)
- [Global State 规格](docs/spec/global-state-spec.md)
- [Error Boundary 规格](docs/spec/error-boundary-spec.md)
- [Dependency Resolution 规格](docs/spec/dependency-resolution-spec.md)
- [Test Runner 规格](docs/spec/test-runner-spec.md)
- [Query Selector 规格](docs/spec/query-selector-spec.md)
- [Asset Package 规格](docs/spec/asset-package-spec.md)
- [DevServer / HMR 规格](docs/spec/devserver-hmr-spec.md)
- [Miniapp Vendor Profiles 规格](docs/spec/miniapp-vendor-profiles-spec.md)
- [Permission Model](docs/security/permission-model.md)
- [Bundle Signing](docs/security/bundle-signing.md)

## 文档入口

- [项目愿景](docs/overview/vision.md)
- [架构总览](docs/overview/architecture-overview.md)
- [路线图](docs/overview/roadmap.md)
- [JSON 格式规格](docs/spec/json-format-spec.md)
- [IR 规格](docs/spec/ir-spec.md)
- [Runtime 规格](docs/spec/runtime-spec.md)
- [Rendering 规格](docs/spec/rendering-spec.md)
- [Host Capability 规格](docs/spec/host-capability-spec.md)
- [Agent Protocol 规格](docs/spec/agent-protocol-spec.md)
- [研发实施规划](docs/planning/implementation-plan.md)
- [包发布规划](docs/planning/package-release-plan.md)
- [官方站点与开发者门户规划](docs/planning/official-website-plan.md)
- [生态 Registry 与分发服务规划](docs/planning/ecosystem-registry-plan.md)
- [正式规格补完计划](docs/planning/formal-spec-expansion-plan.md)
- [V1 实施切面规划](docs/planning/v1-implementation-cut-plan.md)
- [V1 Crate / Service 实施拆解](docs/planning/v1-crate-implementation-plan.md)
- [QORM 主控研发进度规划](docs/planning/qorm-development-progress-plan.md)

## 当前阶段建议

第一阶段优先完成：

```text
JSON Schema → Parser → Resolver → Typed IR → Runtime Interpreter → Layout → Display List → Desktop Preview → Agent Patch
```

不要一开始扩展过多平台。先证明：格式稳定、运行时可执行、渲染管线高效、Agent 可安全修改。
