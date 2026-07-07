# Contributing to QORM

Thanks for your interest. QORM is an agent-native, declarative-UI runtime in Go —
its artifacts are dual-consumer: readable/installable/writable by a human, and
discoverable/derivable/self-verifiable by an AI. Contributions that respect the
module boundaries, naming consistency, and the security model are very welcome.

> [中文版](CONTRIBUTING.zh.md)

## Try it in 5 minutes

```sh
go install github.com/qorm/qorm/cmd/qorm@latest   # or: go run ./cmd/qorm
qorm run examples/counter                          # opens in your browser
qorm mcp examples/counter                          # or drive it from your AI over MCP
```

Point your AI assistant at it too — see [docs/build-with-ai.md](docs/build-with-ai.md).

## Dev setup

The default build is pure Go and cross-compiles from any machine:

```sh
go version               # 1.22+
go build ./... && go test ./...
```

The WASM client runtime uses Go's built-in wasm support. The native-window build
(`-tags desktop`) needs the platform WebView per OS (macOS ships WebKit; Linux
needs WebKitGTK; Windows needs WebView2) and uses cgo.

## Repo layout

- `cmd/qorm` — the CLI (`run` / `render` / `build` / `mcp` / `shot` / `package` / …).
- `internal/` — `loader`, `model`, `runtime`, `expr`, `render`, `server`, `mcp`,
  `bundle`, `capability`, `support`, `webview` (vendored), …
- `examples/` — the canonical runnable apps; **trust these over any spec**.
- `docs/` — user docs (English); `docs/zh/` — the Chinese mirror.
- `integrations/` — MCP config + a QORM skill for agents.

## House rules

- **No emoji or pictographic/status symbols** anywhere (code, UI, docs, examples,
  logs, commit messages). Use the built-in SVG icon set (`internal/render/icons.go`).
  Typographic arrows/box-drawing are fine.
- Auto-generated docs (`docs/reference/widgets.md`, `docs/platforms/capabilities.md`,
  `docs/platforms/support-matrix.md`, `docs/agent/mcp-tools.md`) are generated from
  code and guarded by tests — edit the generator, then run
  `QORM_UPDATE_DOCS=1 go test ./...`, not the file.
- Keep `go build ./... && go test ./...` green. Match the surrounding code's style.

## Sending a change

1. Fork + branch.
2. Make the change with a test; keep builds + tests green.
3. Open a PR describing what and why. Small, focused PRs merge fastest.

## Reporting / feedback

Found a bug or rough edge while trying it? Please
[open an issue](https://github.com/qorm/qorm/issues/new/choose) — tell us what you
built, what failed, and where the DX got in the way. Questions and ideas go in
[Discussions](https://github.com/qorm/qorm/discussions). Early feedback shapes the
roadmap.
