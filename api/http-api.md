# HTTP & SSE

> Auto-generated from the source (`TestAPIRef`) — do not edit by hand. The route table below is extracted from the code, so it can never drift.

`qorm run` serves the app and exposes a small HTTP surface: the browser talks to it, an AI agent reaches the MCP tools at `/mcp`, and OTA updates come in over `/update`. Endpoints that change state require a same-origin request.

| Route | Method | Purpose |
|---|---|---|
| `/` | GET | the app shell — server-rendered HTML + the thin client runtime |
| `/event` | POST | dispatch a UI event (action / input change) and re-render |
| `/navigate` | POST | URL routing — the browser drives navigation from the address bar (`{scene,params}` or `{back:true}`) on Back/Forward |
| `/events` | GET (SSE) | Server-Sent Events stream: the server pushes fresh HTML + log lines |
| `/poll` | GET | long-poll fallback when SSE is unavailable — returns the current revision + HTML if it advanced |
| `/log` | GET / POST | GET activity entries after `?since=`; POST forwards a client console line |
| `/presence` | GET / POST | collaboration presence — who (human/agent) is focused/typing where |
| `/viewport` | GET / POST | the browser reports its window size (debounced on resize) so responsive `when` nodes re-render server-side; GET reads the current value |
| `/console` | GET | the log-window console feed page |
| `/logwindow` | GET | the standalone log window that accompanies the desktop app |
| `/window` | POST | desktop window control (move / resize / open / close / focus) |
| `/measure` | POST | the browser reports the measured layout (x/y/w/h, computed style) of every node |
| `/mcp` | POST | MCP JSON-RPC over HTTP — the same tools as `qorm mcp`, sharing the live runtime |
| `/update` | POST | OTA: apply a new **signed** bundle to the running app |
| `/rollback` | POST | revert to the previously running bundle |
| `/dev/state` | GET / POST | DevTools state inspector: read or write the live app state |
| `/dev/tree` | GET | DevTools component tree: read the current scene's node tree JSON |
| `/dev/highlight` | POST | DevTools highlight event: broadcast a node highlight inspect signal to all clients |

## The `/events` stream

The client opens `GET /events?rev=<n>` — `<n>` the revision of the page render that produced its HTML — and holds it open. The server writes one SSE message per change:

```
: connected

data: <html for the changed region>

```

Each `data:` frame carries the re-rendered HTML the client swaps in, and ships an `id: <rev>` line so a reconnecting browser replays it as `Last-Event-Id`. If a client connects (or reconnects) behind the live revision — a mutation landed between its page render and the stream opening — the server first pushes a current snapshot (`rev`+`html`+`theme`+`route`) to resync it, then streams; a client already at the tip gets no redundant snapshot. Log and presence updates arrive on the same stream. When a proxy buffers SSE, the client falls back to `GET /poll?rev=<n>`.
