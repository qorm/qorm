# 参与 QORM

感谢关注。QORM 是一个 Go 原生、面向 agent 的声明式 UI 运行时——它的产出物是**双消费者**的:人可安装/可读/好写,AI 可发现/可推导/可自证。欢迎在遵守模块边界、命名一致性和安全模型的前提下贡献。

> [English](CONTRIBUTING.md)

## 5 分钟上手

```sh
go install github.com/qorm/qorm/cmd/qorm@latest   # 或:go run ./cmd/qorm
qorm run examples/counter                          # 在浏览器里打开
qorm mcp examples/counter                          # 或让你的 AI 经 MCP 驱动它
```

也可以把 AI 助手接上——见 [docs/zh/build-with-ai.md](docs/zh/build-with-ai.md)。

## 开发环境

默认构建是纯 Go,可从任意机器交叉编译:

```sh
go version               # 1.22+
go build ./... && go test ./...
```

WASM 客户端运行时用 Go 自带的 wasm 支持;原生窗口构建(`-tags desktop`)在各平台需要该平台的 WebView(macOS 自带 WebKit、Linux 需 WebKitGTK、Windows 需 WebView2),使用 cgo。

## 约定

- **任何地方都不用 emoji / 图形状态符号**(代码、UI、文档、示例、日志、提交信息)。用内置 SVG 图标集(`internal/render/icons.go`);排版箭头/制表符可以。
- 自动生成的文档(widgets/capabilities/support-matrix/mcp-tools)由代码生成、测试守护——改生成器后跑 `QORM_UPDATE_DOCS=1 go test ./...`,不要手改文件。
- 保持 `go build ./... && go test ./...` 全绿。

## 反馈

试用时发现 bug 或不顺手的地方,请[开 issue](https://github.com/qorm/qorm/issues/new/choose):告诉我们你在做什么、哪里坏了、DX 卡在哪。问题和想法发到 [Discussions](https://github.com/qorm/qorm/discussions)。早期反馈会影响路线图。
