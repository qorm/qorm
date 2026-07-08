<!-- data-lang-nav --><p align="right"><b>English</b> · <a href="zh/README.md">中文</a></p>

# QORM documentation

QORM (Queryable Object Rendering Model) is a pure-Go, agent-native declarative-UI
runtime: write a UI as JSON, run it live, sign it, and package it for
web / iOS / Android / desktop — readable and writable by both people and AI agents.

The name is also what you do with a live app: **Query** the node tree and state
over HTTP/MCP, **Observe** it in real time over SSE, **Render** it on every
platform, **Mutate** it through actions and the write API.

New here? Read the [top-level README](../README.md) for the big picture and the
CLI, then dive in below. The [`examples/`](../examples) apps are the canonical,
runnable reference — when a doc and a running example disagree, trust the example.

## Learn

- [Project structure](project-structure.md) — the layout of a QORM app folder, file by file
- [Getting started](tutorials/getting-started.md) — install, your first app, the run loop
- [First scene](tutorials/first-scene.md) · [First action](tutorials/first-action.md) · [First component](tutorials/first-component.md) · [First platform pack](tutorials/first-platform-pack.md)

## Reference (auto-generated from the code — canonical)

- [Node & widget props](reference/props.md) — the node schema, common style props, and every widget's specific props
- [Actions & state](reference/actions.md) — every action step `type` and its fields
- [Widget catalog](reference/widgets.md) — every node `type` the renderer accepts
- [Animation](reference/animation.md) — entrance effects, value-driven transitions, and press feedback
- [Gestures](reference/gestures.md) — tap / long-press / swipe-to-dismiss / drag-to-reorder, as widget props
- [Navigation](reference/navigation.md) — multiple scenes + the navigate action + page transitions
- [Capabilities](platforms/capabilities.md) — built-in hardware/OS ops, callbacks, and platforms

### API surface

- [HTTP & SSE](reference/http-api.md) — the endpoints `qorm run` serves (browser, MCP, OTA)
- [MCP tools](agent/mcp-tools.md) — the tools an AI agent drives the live app with
- [Go package: qormext](reference/go-api.md) — the one public Go package, for app-owned native ops

## Platforms & packaging

- [Platform support matrix](platforms/support-matrix.md) — what works where, at a glance
- [Mobile](platforms/mobile.md) · [Desktop](platforms/desktop.md) · [Web](platforms/web.md) · [Mini-app](platforms/miniapp.md)
- [User middle-layer](platforms/native-middlelayer.md) — add your own native ops in one Go file that compiles into desktop *and* mobile/web WASM

## Examples (walkthroughs)

- [Counter](examples/counter.md) · [Todo](examples/todo.md) · [Login](examples/login.md) · [Dashboard](examples/dashboard.md)
- The full set of runnable apps lives in [`examples/`](../examples).

## Human-AI collaboration

- [Build with your AI assistant](build-with-ai.md) — point your AI at QORM to scaffold, edit, run, and verify apps
- [Collaborating on a live app](collaboration.md) — a human and an AI agent on the same running app, each seeing the other (the QORM premise)

## For AI agents

- [Agent integrations](../integrations) — drop-in MCP config + a QORM skill for Claude / Cursor / Windsurf
- [MCP tools](agent/mcp-tools.md) — the Model Context Protocol surface to read, edit, and verify a live app
- [Verifying an app](verification.md) — self-verify edits with `qorm measure` / `qorm check`
- [Skills](agent/skills.md) · [Permissions](agent/permissions.md)

## Trust & security

- [Bundle signing](security/bundle-signing.md) — ed25519 verify-the-bundle delivery
- [Permission model](security/permission-model.md) · [Security model](security/security-model.md)

## Commercial use

- [Terms](../ops/TERMS.md) — the source is MIT; a Patreon membership covers commercial white-labeling
