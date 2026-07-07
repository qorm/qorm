# Contributing to QORM

欢迎参与 QORM。QORM 是一套 **Go 原生**、Agent 可查询的声明式 UI Runtime 与渲染模型——它的产出物同时交付给 **AI 与人类开发者**:人类可安装/可读/好写,AI 可发现/可推导/可自证。贡献时请优先遵守模块边界、命名一致性和安全模型。

## 开发环境

只需要 Go(纯 Go,不使用 cgo 的部分可从任意平台交叉编译):

```bash
go version   # 1.22+
```

WASM 客户端运行时需要 Go 的 wasm 支持(随 Go 自带);桌面构建(`-tags desktop`)在各自平台需要该平台的 WebView 依赖(macOS 自带 WebKit、Linux 需 WebKitGTK、Windows 需 WebView2)。

## 仓库结构

```text
cmd/qorm            qorm 命令行 + 服务器 + 打包器 + 桌面宿主
cmd/qorm-wasm       客户端运行时(Go→WASM,离线包在 WebView 里跑)
internal/model      Scene IR、Node、App、Window
internal/loader     目录 → App(manifest + scenes + actions)
internal/render     Node → HTML/CSS,以及内置 SVG 图标集(icons.go)
internal/runtime    Event、Action、State、Motion 运行时
internal/server     HTTP 服务、Page/OfflineHTML、SSE 实时同步、MCP over HTTP
internal/mcp        MCP Server(Agent 工具:inspect/measure/check/dispatch/patch…)
internal/measure    自检:render → measure → check(AI 不靠浏览器自证布局)
internal/bundle     Bundle 编译、签名、版本
internal/ota        OTA 更新与回滚
pkg/qormext         用户中间层注册表(app 用 Go 加自己的能力)
examples/           示例 app(hardware、native-ext…)
docs/               规格、平台、ADR
```

## 依赖方向

保持单向依赖:`model` 在最底层,`render/runtime` 依赖它,`server/mcp` 依赖 `runtime`,`cmd` 在最上层。禁止低层依赖高层(`model` 不依赖 `runtime`,`runtime` 不依赖 `server`)。

## 提交流程

1. 新建 issue 描述问题或功能。
2. 小步提交,保持 PR 可 review。
3. 新功能必须包含测试或可复现的自检(见下)。
4. 修改 JSON 格式、IR、Runtime、Host Capability、Agent Protocol 时必须更新对应规格文档(`docs/spec/*`)。
5. 重大架构决策必须新增 ADR(`docs/adr/`)。

## 测试与自检

提交前至少运行:

```bash
gofmt -l .                 # 无输出 = 格式干净
go vet ./...
go build ./...             # 默认构建
go build -tags desktop ./cmd/qorm   # 桌面构建(在支持的平台)
GOOS=js GOARCH=wasm go build -o /dev/null ./cmd/qorm-wasm   # WASM 客户端
go test ./...
```

QORM 的核心是**无需浏览器的自检**——用框架自身的 harness 验证渲染与布局:

```bash
qorm render <app>                 # 产出 HTML,肉眼/程序可查
qorm measure <app>                # 自报每个元素的 rect + 关键样式
qorm check <app> <checks.json>    # 断言布局/交互(AI 的回归护栏)
```

涉及能力(硬件/原生 op)的改动:优先做到**可 headless 验证**(mock 桥录 `qormToNative` 调用、模拟 `qormOn*` 回调),真机行为无法在 CI 验证时明确标注"待真机"。

## 命名规范(重要)

命令、函数、op、widget 的用词**不能有歧义**——AI 与人类开发者都要能不查文档地推导:

- 每个能力一个**规范词根**,各层机械派生:widget 类型 = 词根,触发 `qorm<Stem>`,读 `<stem>Get`,写 `<stem>Set`,增减 `<stem>Up`/`<stem>Down`,生命周期 `<stem>Start`/`<stem>Stop`,布尔 `<stem>Toggle`,回调 `qormOn<Stem>`。
- 破坏性重命名必须保留旧名做**别名**,不破坏现有 app / op 字符串 / Agent 调用。
- 不用 emoji——用内置 SVG 图标集(`internal/render/icons.go` 的 `icon` widget)。

## 代码风格

- 所有导出 API 写文档注释;错误信息可诊断、可定位。
- 渲染热路径避免不必要分配;不在渲染中解析 JSON。
- 不在 UI Bundle 中引入未授权的底层能力。

## 文档同步规则

修改以下模块必须同步文档:

| 模块 | 文档 |
|---|---|
| JSON 字段 | `docs/spec/json-format-spec.md` |
| IR 类型 | `docs/spec/ir-spec.md` |
| Runtime 行为 | `docs/spec/runtime-spec.md` |
| Host Capability | `docs/spec/host-capability-spec.md` |
| Agent 工具 | `docs/spec/agent-protocol-spec.md` |
| 平台/能力 | `docs/platforms/*` |

## 安全原则

Agent、Bundle、Host Capability、Native Bridge、用户中间层都遵守最小权限。任何影响文件、网络、系统权限、原生能力、部署或执行外部命令的操作,都必须有显式权限策略。
