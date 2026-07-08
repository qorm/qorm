<!-- data-lang-nav --><p align="right"><b>English</b> · <a href="zh/README.md">中文</a></p>

# QORM documentation

QORM (Queryable Object Rendering Model) is a pure-Go, agent-native declarative-UI
runtime: write a UI as JSON, run it live, sign it, and package it for
web / iOS / Android / desktop — readable and writable by both people and AI agents.

The name is also what you do with a live app: **Query** the node tree and state
over HTTP/MCP, **Observe** it in real time over SSE, **Render** it on every
platform, **Mutate** it through actions and the write API.

New here? Read the [top-level README](https://github.com/qorm/qorm/blob/main/README.md) for the big picture and the
CLI, then dive in below. The [`examples/`](https://github.com/qorm/qorm/tree/main/examples) apps are the canonical,
runnable reference — when a doc and a running example disagree, trust the example.

## Learn

- [Project structure](project-structure.md) — the layout of a QORM app folder, file by file
- [Getting started](tutorials/getting-started.md) — install, your first app, the run loop
- [First scene](tutorials/first-scene.md) · [First action](tutorials/first-action.md) · [First component](tutorials/first-component.md) · [First platform pack](tutorials/first-platform-pack.md)

## Reference

The full, auto-generated contract lives on the separate **[API reference](/api/)** —
node & widget props, the widget catalog, actions & state, gestures, animation,
navigation, the HTTP/SSE surface, and the public Go package. It is extracted from
the runtime source, so it never drifts.

App-facing capability docs stay here with the platform guides:

- [Capabilities](platforms/capabilities.md) — built-in hardware/OS ops, callbacks, and platforms
- [MCP tools](agent/mcp-tools.md) — the tools an AI agent drives the live app with

## Platforms & packaging

- [Platform support matrix](platforms/support-matrix.md) — what works where, at a glance
- [Mobile](platforms/mobile.md) · [Desktop](platforms/desktop.md) · [Web](platforms/web.md) · [Mini-app](platforms/miniapp.md)
- [User middle-layer](platforms/native-middlelayer.md) — add your own native ops in one Go file that compiles into desktop *and* mobile/web WASM

## Examples (walkthroughs)

- [Counter](examples/counter.md) · [Todo](examples/todo.md) · [Login](examples/login.md) · [Dashboard](examples/dashboard.md)
- The full set of runnable apps lives in [`examples/`](https://github.com/qorm/qorm/tree/main/examples).

## Human-AI collaboration

- [Build with your AI assistant](build-with-ai.md) — point your AI at QORM to scaffold, edit, run, and verify apps
- [Collaborating on a live app](collaboration.md) — a human and an AI agent on the same running app, each seeing the other (the QORM premise)

## For AI agents

- [Agent integrations](https://github.com/qorm/qorm/tree/main/integrations) — drop-in MCP config + a QORM skill for Claude / Cursor / Windsurf
- [MCP tools](agent/mcp-tools.md) — the Model Context Protocol surface to read, edit, and verify a live app
- [Verifying an app](verification.md) — self-verify edits with `qorm measure` / `qorm check`
- [Skills](agent/skills.md) · [Permissions](agent/permissions.md)

## Trust & security

- [Bundle signing](security/bundle-signing.md) — ed25519 verify-the-bundle delivery
- [Permission model](security/permission-model.md) · [Security model](security/security-model.md)

## Commercial use

- [Terms](https://github.com/qorm/qorm/blob/main/ops/TERMS.md) — the source is MIT; a Patreon membership covers commercial white-labeling
