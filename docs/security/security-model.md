# QORM Security Model

> **Target model vs. current implementation.** This document describes QORM's **target** security model. What the current Go runtime **already enforces**:
> ed25519 "verify the artifact before running it" Bundle signing + content integrity (integrity takes precedence over signing); OTA updates require a trusted public key
> (`--trust`, otherwise rejected); key revocation bound to the actual verification key; the local server blocks cross-origin (CSRF/DNS-rebind) access to dangerous endpoints (`/window` eval,
> `/update`, `/mcp`); mobile native capabilities are gated by system permission prompts, and generated projects derive `Info.plist` / `AndroidManifest` declarations from the widgets actually used.
> **Not yet implemented** (the targets described here; do not treat them as guarantees already in effect): a standalone runtime "capability approval layer" — that is, per-capability
> allowlist adjudication, an approval lifecycle (revocation/expiration), and "desktop native ops gated beyond the transport layer". Desktop native
> ops are currently not independently approved; the "security invariants" below are design intent, not currently enforced requirements.

QORM supports dynamic Bundles, Agent Patches, Host Capabilities, Plugins, and the Native Bridge, so the security model must be built in.

## Security Goals

- Bundles must not bypass platform permissions.
- Agents must not perform dangerous operations by default.
- Host Capabilities must be allowlisted.
- Plugins must declare their permissions and capabilities.
- Mobile dynamic updates must be verifiable and reversible.

## Trust Boundaries

```text
Source JSON
Bundle
Runtime
Host Capability
Native Bridge
Plugin
Agent
Platform
```

Every boundary must be verifiable.

## Decision Boundaries

- A Bundle is declarative input; it does not grant permissions.
- The Runtime is responsible for executing verification, resolution, patching, and permission dispatch.
- The Host / Platform is the final arbiter of low-level capabilities.
- An Agent cannot approve its own dangerous operations.
- A custom Web client, Plugin, or Bridge can only implement transport / adapter logic; it cannot replace the permission adjudication layer.

## Security Invariants

- No path exists that can bypass Host Capabilities to directly access low-level APIs.
- No approval can turn an unsupported capability into a supported one.
- A signed Bundle still cannot bypass app/system policy.
- A revoked or expired approval must not continue to be used for future calls.
- MCP tools must not enter the rendering hot path.

## Bundle Security

A Bundle must contain:

```text
bundleVersion
minRuntimeVersion
hash
signature
keyId
capability requirements
source manifest
```

The rules for signing, canonicalization, trust root, rotation, and revocation are governed by `bundle-signing.md`.

## Agent Security

By default, an Agent can only inspect, validate, and preview. Write-class capabilities such as apply, host.call, and filesystem.saveFile, and deploy, must go through policy and any required approval.

The relationship between Agent permissions and system permissions is governed by `agent/permissions.md` and `permission-model.md`.

## Host Capability Security

Every Host Capability must declare:

```text
name
input schema
output schema
permissions
platform support
requires approval
```

Runtime permission priority, approval lifecycle, and audit rules are governed by `permission-model.md` and `host-capability-spec.md`.

## Plugin Security

A Plugin requires:

```text
manifest
permission declaration
sandbox strategy
version
signature
```

A Plugin can only narrow its own capabilities; it must not widen the scope allowed by host policy.

## Web / Custom Client Security Boundaries

- The browser sandbox is the outermost capability boundary on the Web side.
- The Web Host Adapter is QORM's permission enforcement point inside the browser.
- An injected/custom HttpClient is only a transport adapter; it cannot skip domain, method, credential, and approval checks.
- A browser native permission prompt is not equivalent to a QORM approval; both must be satisfied before access is granted.

## OTA Trust for Packaged Apps

A package built with `--update-url` MUST also carry `--trust <key.pub>` —
the flags are enforced as a pair. The trust split is deliberate:

- **The bundled payload** (bundle.json shipped inside the .ipa / .apk / PWA)
  is trusted via its installation channel — store signing, TLS origin — the
  same channel that delivered the WASM runtime itself. It is not re-verified
  at boot.
- **Every OTA-origin bundle** (fetched from the update server, or restored
  from local storage where an earlier update persisted it) is verified with
  ed25519 against the embedded trust key before activation; a bundle that
  fails verification is discarded and the app falls back one tier (previous
  update → bundled payload). A failed update never touches the running app.

## CLI Self-Update Trust

`qorm update` downloads a release binary only after verifying the release's
`SHA256SUMS` manifest against an ed25519 signature from the keys embedded in
the running binary, then checking the binary's own sha256 against that
manifest. Missing or unverifiable signatures fail closed
(`--insecure-skip-verify` is the explicit escape hatch); the embedded key
list supports rotation by shipping old + new keys in a transition release.

A valid signature is necessary but not sufficient: the release's tag must
also parse as a semver (`X.Y.Z` with an optional leading `v`) and be
STRICTLY NEWER than the running version before anything is replaced. An
equal or older tag — including a prerelease of the running `X.Y.Z`, which
sorts older than the release — is reported as already up to date (or
refused as a downgrade) and the binary is left untouched, so a compromised
or misconfigured release endpoint cannot roll the CLI back to a stale,
potentially vulnerable build by serving an old signed release. An
unparseable remote tag fails closed the same way.

## Prohibited Behaviors

- A Bundle dynamically adding an unreviewed Native API.
- An Agent writing to the filesystem without confirmation.
- A UI Action directly executing a shell.
- A plugin bypassing Host Capabilities.
- A custom client acting as the permission arbiter.