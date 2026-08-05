# QORM Codex Pack

The Codex Pack lets code-oriented agents operate on QORM repositories — JSON
scenes, Go source, QSS stylesheets, qscript actions, and the canvas engine.

## Goals

- Author and modify QORM scene JSON (`scenes/*.json`).
- Write and edit QSS stylesheets (`styles/*.qss`) with type/class/id selectors.
- Write qscript action files (`actions/*.qs`) with `let`/`if`/`for`/`while`/`fn`.
- Modify Go source (`cmd/`, `internal/`) — including canvas engine code.
- Run tests: `go test ./...` and `go test -race ./...`.
- Build: `go build ./...` and `go build -tags desktop ./...`.
- Generate icon fonts: `go generate ./internal/render/canvas/`.
- Inspect and preview patches via MCP.

## Allowed by Default

```text
read files
edit JSON / QSS / qscript / Go files
run qorm check / qorm measure
run go test ./...
run go build ./...
run go generate ./...
preview_patch
```

## Denied by Default

```text
Direct deploy (qorm deploy)
Running dangerous shell commands
Bypassing preview_patch → apply_patch flow
Adding unauthorized Host Capabilities
Modifying auto-generated files (icon_font_auto.go, api/widgets.md)
```

## Canvas Engine Development

When working on the native canvas engine (`internal/render/canvas/`):

- **Build**: `go build ./internal/render/canvas/` (standalone) or
  `go build -tags desktop ./...` (full desktop build).
- **Test**: `go test ./internal/render/canvas/...` (229 tests) and
  `go test ./internal/widgets/...` (114 tests).
- **Race detector**: `go test -race ./internal/render/canvas/...`.
- **Architecture**: single-threaded, input + rendering on main thread.
  External mutations enqueued via `EnqueueMutation`, drained at frame boundary.
- **Key files**: `engine.go` (frame loop, dispatch), `measure.go` (layout),
  `style.go` (theme cascade, interaction effects), `scroll.go` (viewports,
  momentum), `input.go` (text editing), `animation.go` (tweens).
- **Icon font**: edit `icon_font_data.go` for hand-crafted glyphs, then
  `go generate ./internal/render/canvas/` to regenerate auto-generated entries.

## Workflow

```text
1. Read AGENTS.md and the relevant spec
2. Read the target files (JSON / QSS / qscript / Go)
3. Check existing tests: go test ./...
4. Make changes
5. Run tests: go test ./... && go test -race ./internal/render/canvas/...
6. Build: go build -tags desktop ./...
7. If modifying MCP tools, regenerate docs: QORM_UPDATE_DOCS=1 go test ./internal/mcp/
8. Preview changes via qorm_preview_patch (for scene edits)
9. Verify with qorm_check_layout
10. Apply after user confirmation
```

## Output Requirements

When Codex modifies QORM files, it should output:

```text
Summary of changes
Files modified
Test results (pass/fail count)
Build status (all tags)
Validation results (qorm_validate, qorm_check_layout)
Potential risks or regressions
Whether user confirmation is required
```
