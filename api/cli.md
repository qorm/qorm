# CLI: qorm

> Hand-written reference for the `qorm` binary (implemented in `cmd/qorm/`). Unlike the other pages here it is not generated — update it when the CLI changes.

One binary is the whole toolchain: scaffold, run, render, measure, verify, sign, package, publish, and serve agents. Pure Go by default (cross-compiles everywhere); a `-tags desktop` build adds the native WebView that `shot` / `measure` / `check` / `preview` and `run --app` build on.

| Command | What it does |
|---|---|
| [`new`](#new) | scaffold a runnable app |
| [`run`](#run) | serve an app live (browser + agent share the runtime) |
| [`render`](#render) | write a static HTML snapshot |
| [`shot`](#shot) | rasterize an app / page / window to PNG (macOS, `-tags desktop`) |
| [`measure`](#measure) | render + self-measure layout and styles (`-tags desktop`) |
| [`check`](#check) | verify the render against expectations (`-tags desktop`) |
| [`build`](#build) | compile (+ optionally sign) a bundle |
| [`keygen`](#keygen) | generate an ed25519 signing keypair |
| [`sign`](#sign) | sign an existing bundle |
| [`verify`](#verify) | verify a bundle's integrity / signature / revocation |
| [`mcp`](#mcp) | serve an app to agents over MCP (stdio) |
| [`preview`](#preview) | render a packaged app and report its layout (`-tags desktop`) |
| [`package`](#package) | package as an installable app (web / iOS / Android / mac / miniapp) |
| [`docs`](#docs) | render a markdown docs tree to a static HTML site |
| [`audit`](#audit) | verify a hash-chained activity audit log |
| [`updates`](#updates) | OTA publish server with staged (canary) rollout |
| [`update`](#update) | self-update the CLI (verifies signed checksums) |
| [`version`](#version) | print the version |

## Conventions

- **Inputs.** `<app-dir>` is an app source directory (`qorm.json` + `scenes/` + `actions/`). `run`, `render`, and `mcp` also accept a compiled `.qorm.bundle` — verified before use (integrity always; authenticity with `--trust`; revocation with `--revoked`). `render` additionally accepts a single scene JSON file.
- **Exit codes.** `0` success, `1` runtime or verification failure, `2` usage error (also for an unknown command).
- **Flags.** `-o` and `--out` are synonyms everywhere, as are `-p` and `--platform`. Flags are parsed by hand: an unrecognized token is taken as the positional input by most commands; `qorm update` is the exception and rejects unknown flags.
- **Environment variables.**

| Variable | Used by |
|---|---|
| `QORM_KEYSTORE_PASS` / `QORM_KEY_PASS` | `package -p android --release` — keystore / key passwords (else prompted) |
| `ANDROID_HOME` / `ANDROID_SDK_ROOT` | `package -p android` — locating the Android SDK |

- **No arguments.** Inside a packaged macOS `.app`, a bare `qorm` runs the embedded app. Anywhere else it prints usage and exits `2`.
- **Internal commands.** `__release-sign`, `__logwin`, and `__tray` are internal sub-process helpers (CI release signing, the desktop log window, the system tray) — not user commands; `__release-sign` reads the release key from `QORM_RELEASE_KEY`.

## new

```
qorm new <dir> [--name "App Name"]
```

Scaffolds a minimal runnable app: `qorm.json` (manifest; the app `id` is sanitized from the directory name), `scenes/main.json` (a counter screen), and `actions/inc.json`. Refuses a non-empty directory (exit `1`). `--name` sets the display name (default: the directory base name).

## run

```
qorm run <app-dir|bundle> [flags]
```

Serves the app live: the browser UI, the SSE push channel, and the agent endpoint at `/mcp` all share one runtime. See [HTTP & SSE](http-api.md) for the endpoint contract.

| Flag | Effect |
|---|---|
| `--port N` | listen port (default `10383`; if taken, falls back to a random free port and prints it) |
| `--no-open` | do not open a browser window |
| `--app` | standalone window: a native WebView in a `-tags desktop` build, otherwise a chromeless Chromium-family window (`--app=<url>`), falling back to a normal tab |
| `--console` | open the collaboration console (`/console`) instead of the app page |
| `--lan` | bind `0.0.0.0` so physical devices join the same session; prints the Wi-Fi URL and runs `adb reverse` for attached Android devices; implies `--no-open` |
| `--tls` | HTTPS with a self-signed certificate covering localhost + the LAN IPs (a secure context, required by camera/mic/location Web APIs on a device); implies `--lan` |
| `--mcp-read-only` | the shared MCP session rejects mutating tools (`qorm_dispatch`, `qorm_set_state`, `qorm_apply_patch`, `qorm_undo`); inspection and preview tools keep working |
| `--no-watch` | disable hot-reload |
| `--trust pub.key` | require a bundle to carry a valid signature from this key |
| `--revoked list.json` | reject bundles signed by a revoked key |
| `--audit-log file` | append every activity entry to a hash-chained JSONL log (verify with [`qorm audit`](#audit)); the chain resumes across restarts |

Behavior notes:

- A **directory** input hot-reloads: a 400 ms mtime poll re-parses the app on change and pushes the re-render to every client, preserving state/scene/viewport. A half-written file is reported and the current app kept.
- A **bundle** input is verified before serving and enables the OTA endpoints (`/update`, `/rollback`). Without `--trust` only integrity is checked — a warning on stderr says authenticity is not verified.
- The loader's static diagnostics (deprecated attributes, expression type mismatches) print to stderr and never block the run.

## render

```
qorm render <app-dir|scene.json|bundle> [-o out.html]
```

Writes a static HTML snapshot (the server-side first frame; no live client). Default output: `<input base name>.html`. A bundle input is integrity-checked only — this command has no `--trust`, so a warning notes authenticity is not verified.

## shot

```
qorm shot <app-dir> -o out.png [--width W --height H]
qorm shot --html page.html -o out.png
qorm shot --url URL -o out.png
qorm shot --live "window title" -o out.png
```

Rasterizes to PNG via an offscreen WebKit WebView. **macOS + `-tags desktop` only** — any other build prints an error and exits `2`. Requires a GUI session and Screen Recording permission for the terminal (capture goes through the system `screencapture` tool). CSS animations are frozen at their final state before capture.

- App directory: rendered, then captured (default `--width 440 --height 720`, default output `<dir>.png`).
- `--html`: capture a static HTML file (default output `<name>.png`).
- `--url`: capture a live URL — useful for server-backed pages like `/console` (default output `shot.png`).
- `--live`: capture an already-running window whose title matches (exact title match preferred, then substring), without spinning up a new WebView (default output `shot.png`).

## measure

```
qorm measure <app-dir> [--width N] [-o report.json]
```

Renders the app in a native WebView, lets it self-measure, and prints one JSON row per node joining the **intent** (type / text / binding) with the **rendered result** (rect + computed styles) — to stdout, or to `-o`. Viewport width defaults to 400 (height is fixed at 820). **Needs a `-tags desktop` build**; other builds exit `1`. Report fields: see [Interpreting & verifying a QORM app](/docs/verification.html).

## check

```
qorm check <app-dir> (--checks checks.json | --audit) [--width N] [-o report.json]
```

Measures the app like [`measure`](#measure), then evaluates expectations against the real render. **Needs a `-tags desktop` build.**

- `--checks checks.json` — either an array of `{ "id": …, <assertion>… }` static checks (visible, text, geometry, contrast, ARIA, …), or a `{"steps":[…]}` flow object: each step applies `do.dispatch "<action>"` or `do.setState`, waits for the re-render + re-measure, then runs that step's checks — interaction testing.
- `--audit` — no authored checks: generic invariants over every visible component (non-zero size, no horizontal overflow, within the window).

**Exit status:** `0` even when checks fail — pass/fail lives in the report's `ok` field. Non-zero only on runtime errors (bad checks JSON, load failure). Assertion schema and report format: [verification](/docs/verification.html).

## build

```
qorm build <app-dir> [-o app.qorm.bundle] [--key priv.key] [--version V] [--require-capability camera,location]
```

Compiles the app into a single bundle (`qorm-bundle/1` JSON): the content (manifest + scenes + actions + locales), a `contentHash` over its canonical encoding, and — with `--key` — a detached ed25519 signature. Default output: `<dir base name>.qorm.bundle`.

- `--version V` — stamp the manifest version (covered by the hash) before signing.
- `--require-capability a,b` — record required capabilities in the signed content; the runtime refuses to start the bundle on a platform missing any of them.
- Static diagnostics print to stderr; they do not fail the build.

## keygen

```
qorm keygen [--out-dir .]
```

Generates an ed25519 keypair: `qorm_key` (private, mode `0600`) and `qorm_key.pub` in `--out-dir`, and prints the 12-character key id. Key files are two lines of text — a header line plus base64 key bytes — easy to inspect and move.

## sign

```
qorm sign <bundle> --key priv.key [-o out]
```

Signs an existing bundle — e.g. one exported from a live design session via the MCP `qorm_export_bundle` tool. **Without `-o` the input file is overwritten in place.**

## verify

```
qorm verify <bundle> [--trust pub.key] [--revoked list.json]
```

Verifies a bundle in layers and prints the scope it proved: integrity (content hash recomputed) always, `+ signature` with `--trust`, `+ revocation` with `--revoked`. Also prints any required capabilities the bundle declares.

- The revocation list is a JSON array `["keyid", …]` or an object `{"revoked": […]}`; it is checked against the actual verifying key, not the bundle's self-declared `keyId`.
- Failure prints `VERIFY FAILED: <reason>` on stderr and exits `1`.

## mcp

```
qorm mcp <app-dir|bundle> [--trust pub.key] [--revoked list.json]
```

Serves the app to an AI agent over MCP (stdio JSON-RPC) — the same tool set a running [`qorm run`](#run) exposes at `/mcp`, but on its own private runtime (no shared browser session). Tool reference: [MCP tools](/docs/agent/mcp-tools.html).

## preview

```
qorm preview <package-dir> [--width N] [--eval JS] [-o report.json]
```

Verifies a **packaged** app, not the source: serves the static output of `qorm package -p web`, lets its client-side WASM runtime boot and render with no app server, and captures the app's self-measurement (stdout, or `-o`). `--eval JS` runs JavaScript in the page after the first measurement — e.g. `qorm(0)` to press the first action button — then re-measures, so the packaged build's interactivity is exercised too. **Needs a `-tags desktop` build.**

Not to be confused with the MCP `qorm_preview_patch` tool, which previews a design patch on a live session.

## package

```
qorm package <app-dir> [-p web|ios|android|mac|miniapp] [-o out-dir] [flags]
```

Compiles the app into an installable, fully offline package. Default platform `web`; default output `<app dir name>-<platform>`. Before building, a capability matrix prints to stderr warning about features the target platform does not support (and about a native middle layer missing for the target).

| Platform | Output |
|---|---|
| `web` | installable, offline-capable PWA: `index.html` + `bundle.json` + `qorm.wasm` + `wasm_exec.js` + manifest + icons + `sw.js` |
| `ios` | an Xcode project; with `xcodegen` + `xcodebuild` installed it also builds (unsigned simulator build, or a signed device build with `--team`) |
| `android` | a Gradle project; with `gradle` + an Android SDK (`ANDROID_HOME` / `ANDROID_SDK_ROOT`) it also builds (wrapper pinned to Gradle 8.9) |
| `mac` | a macOS `.app` with the desktop binary compiled in (needs macOS + cgo); ad-hoc code-signed for development |
| `miniapp` | a WeChat-style mini-program project (WXML/WXSS static export of the initial UI; open in WeChat DevTools) |

The `web`/`ios`/`android` payloads compile the client runtime with `go build` (`GOOS=js GOARCH=wasm`), so the Go toolchain and the QORM module must be reachable — run it from the QORM repo, or a directory whose `go.mod` requires `github.com/qorm/qorm`. The app's own Go middle layer (`native/desktop.go`) is injected into that build, and into the desktop binary for `mac`.

General flags:

| Flag | Effect |
|---|---|
| `--dev URL` | (ios/android only) build the thin **QORM Dev** client that connects to a live `qorm run --lan` server — install once, reuse for every app, changes hot-reload. Mutually exclusive with `--release` |
| `--team ID` | Apple development team for iOS signing |
| `--no-branding` | drop the "Made with QORM" note |
| `--subscribed` | confirm a QORM membership non-interactively (see below) |
| `--update-url URL` + `--trust pub.key` | wire the package to an OTA update server. The two flags **must be given together** (fail-closed: updates are only applied when signed by the trusted key); the URL must be http(s) |

**Commercial gate (honour system).** A custom `icon.png` in the app dir, or `--no-branding`, is commercial white-labeling: the packager prints a note and asks you to confirm a QORM Patreon membership — interactively, or via `--subscribed`. Non-interactive runs without `--subscribed` fail (exit `1`). Personal / educational / open-source use (default icon, branding on) never triggers it.

Release flags — `qorm package … --release` produces a distributable, signed artifact:

| Flag | Platform | Effect |
|---|---|---|
| `--app-version V` | all | marketing version (default `1.0`) |
| `--build N` | all | build number (default `1`) |
| `--export-method M` | ios | export method (default `app-store-connect`) |
| `--upload` | ios | upload to App Store Connect (TestFlight) after export |
| `--api-key F --api-key-id ID --api-issuer UUID` | ios | App Store Connect API credentials for unattended upload |
| `--keystore F --key-alias A` | android | sign with an existing keystore (default alias `qorm`; passwords from `QORM_KEYSTORE_PASS` / `QORM_KEY_PASS` or a prompt) |
| `--apk` | android | also produce a signed APK next to the AAB |
| `--identity "Developer ID Application: …"` | mac | signing identity (else auto-discovered) |
| `--notarize [--keychain-profile P]` | mac | notarize with `notarytool` |
| `--no-dmg` | mac | skip the DMG image |

Android release signing without `--keystore` uses a managed keystore at `<app-dir>/.qorm/release.keystore`: generated with `keytool` on the first release (a JDK is required), passwords stored in `keystore.properties` (mode `0600`), the whole `.qorm` directory git-ignored, and reused so every update carries the same signature. A macOS `--release` build never falls back to ad-hoc signing — it fails instead.

## docs

```
qorm docs [--docs docs] [-o site] [--name Name]
```

Renders a markdown tree into a static HTML site (this documentation site is built with it). Defaults: source `docs/`, output `site/`, header label = the source directory's base name. The header is stamped with the version of the `qorm` binary doing the render.

## audit

```
qorm audit <audit-log.jsonl>
```

Verifies a hash-chained activity log written by `qorm run --audit-log`: each entry's hash covers the previous entry's hash plus its own sequence, timestamp, source, and detail, so any edited, dropped, reordered, or re-attributed entry breaks the chain. Success prints `AUDIT OK: N entries, hash chain intact`; failure prints `AUDIT FAIL after N verified entries: <reason>` (locating the first bad entry) and exits `1`. The chain is self-anchoring from the first entry — keep an out-of-band copy of the final hash to also detect truncation.

## updates

```
qorm updates [bundles-dir] [--port N]
```

The publish side of OTA: an HTTP server that hands each client the bundle it should run, honoring a staged (canary) rollout. Defaults: directory `.`, port `0` (a random free port, printed on startup).

- `bundles-dir/rollout.json` maps app id to `{"stable": "app-v1.qorm.bundle", "canary": "app-v2.qorm.bundle", "canaryPercent": 10}` (`canary` / `canaryPercent` optional). The file is read once at startup.
- `GET /resolve?app=<id>&client=<id>` returns the bundle JSON this client should run: the canary when `FNV-1a(client id) % 100 < canaryPercent` — deterministic per client — else the stable bundle. Unknown app ids and missing files get `404`; a file that is not a well-formed bundle is never served.
- `GET /bundles/<file>` serves bundle files directly. All routes answer with open CORS (`*`) — packaged shells call cross-origin; trust comes from the client-side ed25519 verification (see [`qorm package --update-url`](#package)), not from who may read the bundles.

## update

```
qorm update [--insecure-skip-verify]
```

Self-updates the CLI from the latest GitHub release. Already current (release tag equals the running version): prints so and exits `0`. Otherwise, with a Go toolchain present it runs `go install github.com/qorm/qorm/cmd/qorm@latest`; without one it downloads the `qorm-<os>-<arch>[.exe]` asset, verifies it against the release's `SHA256SUMS` + `SHA256SUMS.sig` using the release public keys embedded in the build (a build without embedded keys cannot verify and refuses), then swaps the executable — the old binary is renamed to `<exe>.old` first and restored if the swap fails. `--insecure-skip-verify` installs the download without verification (prints a warning; not recommended).

## version

```
qorm version        # aliases: --version, -v
```

Prints `qorm <version> (<go version> <os>/<arch>)`. The version is stamped at build time via `-ldflags -X main.version=<tag>`; un-stamped builds report the in-source dev default.
