# QORM API reference

The canonical, machine-generated contract for QORM apps — extracted straight
from the runtime source, so it never drifts from what the code actually does.
This is the reference you (or your AI agent) code against; for tutorials and
guides see the [docs](/docs/).

## The declarative UI contract

- [Node & widget props](props.md) — the node schema, common style props, and every widget's specific props
- [Widget catalog](widgets.md) — every node `type` the renderer accepts, with aliases
- [Actions & state](actions.md) — every action step `type` and its fields
- [Gestures](gestures.md) — tap / long-press / swipe / drag, as widget props
- [Animation](animation.md) — entrance effects and value-driven transitions
- [Navigation](navigation.md) — scenes, the navigate step, and page transitions

## The runtime surface

- [CLI: qorm](cli.md) — every command, flag, and exit code of the `qorm` binary
- [HTTP & SSE](http-api.md) — the endpoints `qorm run` serves (browser, MCP, OTA)
- [Go package: qormext](go-api.md) — the one public Go package, for app-owned native ops
- [MCP tools](/docs/agent/mcp-tools.html) — the tools an AI agent drives the live app with
- [Capabilities](/docs/platforms/capabilities.html) — built-in hardware / OS ops and callbacks

> Every page here except [cli.md](cli.md) is regenerated from source by
> `QORM_UPDATE_DOCS=1 go test ./...` — do not hand-edit those. `cli.md` is
> maintained by hand against `cmd/qorm/`; update it when the CLI changes.
