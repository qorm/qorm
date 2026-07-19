# QORM Bundle Signing

A QORM app ships as a single JSON **bundle**: content-addressed, optionally
ed25519-signed. Signing is the trust primitive behind over-the-air UI delivery —
the runtime verifies the bundle before running it instead of trusting the server
it came from. The implementation lives in `internal/bundle` (format + verify),
`internal/keys` (key generation/storage), and `internal/ota` (fetch + verify).

This page describes the format and trust model as implemented. Where older
docs described fields the code does not have (`bundleVersion`,
`minRuntimeVersion`, a top-level `hash`), the code below is authoritative.

## Bundle Format

A real bundle, produced by `qorm build examples/counter --key qorm_key --version 1.0.0`
(content abbreviated):

```json
{
  "format": "qorm-bundle/1",
  "content": {
    "app": {
      "id": "qorm_counter",
      "type": "app",
      "name": "QORM Premium Counter",
      "version": "1.0.0",
      "entry": "main"
    },
    "scenes": {
      "main": { "id": "main", "type": "scene", "root": { "...": "..." } }
    },
    "actions": {
      "increment": { "id": "increment", "type": "action", "...": "..." }
    }
  },
  "contentHash": "sha256:FoElQYNuc1V8SWyz8F3g53INEnriY9wR7w1PM1jfzck=",
  "signature": {
    "algorithm": "ed25519",
    "keyId": "fod5SMmyqtJJ",
    "value": "ziVsnGtOX6oiB/sNBq2D18mRJKYgoGx2y6y6+YGlGpn8rao1DPbk7NsL/4CLZiYfe1IRjonB0lZ7v0IckMEADA=="
  }
}
```

Top-level fields:

- `format` — always `"qorm-bundle/1"`. A bundle with any other value is
  rejected at decode time.
- `content` — the canonical payload the hash and signature cover:
  - `app` — the manifest document (from `qorm.json`). An app `version`
    stamped with `qorm build --version` lives here, inside the signed content.
  - `scenes` — map of scene id to scene document.
  - `actions` — map of action id to action document.
  - `locales` — optional i18n catalogs (`locale -> key -> string`).
  - `requiredCapabilities` — optional list of capability names (e.g.
    `"camera"`) the app needs; the runtime refuses to activate the bundle on a
    platform missing any of them. Stamped with `qorm build --require-capability`.
- `contentHash` — `"sha256:" + base64(sha256(canonical JSON of content))`.
- `signature` — optional; omitted for unsigned bundles.

There is no separate multi-file form: building a bundle collects every source
document (`qorm.json`, `scenes/*.json`, `actions/*.json`) into `content`
first, so the single hash covers the manifest, all scenes, all actions, and
locales. Signing the root file alone is exactly what the format prevents —
there is no root file, only `content`.

## Canonicalization and Content Hash

The hash is computed over Go's `encoding/json` serialization of `content`,
which is deterministic:

- object keys sorted in lexicographic order,
- array order preserved,
- no insignificant whitespace.

Re-serializing the same `content` value always yields the same bytes, so the
hash is stable across machines. `signature` and `contentHash` itself are
outside the hashed region; everything inside `content` — including the stamped
`version` and `requiredCapabilities` — is covered by both the hash and any
signature made over it.

## Signature

The signature is a detached **ed25519** signature over the `contentHash`
string (the ASCII bytes of e.g. `"sha256:FoEl..."`, not the raw digest):

```text
hash:      SHA-256 over canonical content JSON, base64-encoded, "sha256:"-prefixed
signature: ed25519 over the contentHash string, base64-encoded in signature.value
```

`signature.algorithm` is `"ed25519"`; any other value fails verification.
`signature.keyId` identifies the signing key for display and diagnostics. The
key id is derived from the public key — the first 12 characters of the base64
encoding of the raw key bytes — and `qorm build --key` / `qorm sign` fill it in
automatically. Because the signature covers only `contentHash`, `keyId` is
**not** signed; treat it as a hint, never as a trust decision input (see
Revocation below).

## Verification Flow

`bundle.VerifyWithRevocation` runs, in order:

```text
1. Decode JSON; reject unless format == "qorm-bundle/1"
2. Recompute the content hash; reject on mismatch (tampered content)
3. If no trusted public key was supplied: stop — integrity verified,
   authenticity NOT verified
4. Require signature present
5. Require signature.algorithm == "ed25519"
6. Base64-decode signature.value
7. ed25519-verify the signature against the trusted key over contentHash
8. If a revocation list was supplied: reject if the verifying key is revoked
```

Every failure is a hard error — verification never degrades into "run with a
warning" inside `Verify`. (The `qorm run` CLI prints its own warning when you
load a bundle *without* `--trust`, because that run mode is integrity-only;
pass `--trust` to require a signature.)

### Integrity vs. authenticity

- **Without `--trust <key.pub>`** verification is tamper detection only: it
  proves the bundle was not corrupted after its hash was computed, nothing
  about who made it. `qorm verify` reports `OK ... (integrity)`.
- **With `--trust`** it additionally proves the bundle was signed by the
  holder of that private key: `OK ... (integrity + signature (key fod5SMmyqtJJ))`.
- **`--revoked <list.json>`** adds the revocation check.

## Threat Model

Bundle signing defends against:

- **A compromised or malicious update server / CDN / man-in-the-middle** —
  a modified bundle fails the hash check; a bundle re-signed by anyone but the
  trusted key fails the signature check. The server is a transport, not a
  trust root.
- **Accidental corruption** — hash mismatch.
- **A leaked signing key** — the key id goes on a local revocation list; even
  a perfectly valid signature from that key is then refused.

It does not defend against a leak of an un-revoked private key, and it does
not replace platform permissions: a signed bundle still only gets the
capabilities the platform and policy grant (see `permission-model.md`).

## Keys

`qorm keygen [--out-dir .]` generates an ed25519 keypair and writes two files
(mode `0600`):

```text
qorm_key       QORM-ED25519-PRIVATE-KEY\n<base64 private key>
qorm_key.pub   QORM-ED25519-PUBLIC-KEY\n<base64 public key>
```

Key handling guidance:

- Keep the private key out of the repo and off the build artifacts; treat it
  like a release signing key (CI secret, hardware token, or at minimum an
  access-controlled file). `qorm sign` and `qorm build --key` only need the
  private key file at signing time.
- Distribute the **public** key with whatever consumes bundles — it is the
  trust root (`--trust qorm_key.pub`, or embedded via
  `qorm package --update-url ... --trust qorm_key.pub`).
- Rotate by generating a new keypair and shipping the new public key to
  clients *before* publishing bundles signed with the new key; during the
  transition a client can simply trust the new key once updated, and the old
  key can be added to revocation lists after the cutover.

## Revocation

A revocation list is a local JSON file — either a bare array or an object:

```json
["fod5SMmyqtJJ"]
```

```json
{ "revoked": ["fod5SMmyqtJJ"] }
```

The check deliberately runs against the **actual verifying key's** derived id,
not the bundle's self-declared `signature.keyId`: the signature covers only
`contentHash`, so a revoked-key holder could otherwise rewrite `keyId` to any
un-revoked string and evade the list while the signature still verifies.

There is no remote revocation refresh in the current implementation; the list
is whatever local file you pass via `--revoked` (or embed at package time).
Distribute updated lists through the same channel as the public key.

## OTA Updates

`internal/ota` is the transport half: fetch bytes, then verify **before**
activating — never the other way around.

- `Fetch` reads from an `http(s)` URL (30s timeout, 32 MiB cap) or a local
  file path.
- `FetchVerified` fetches, decodes, and runs `VerifyWithRevocation`. Any error
  returns no bundle at all, so the caller simply keeps running the current
  one. This is the rollback strategy: **rollback by inaction** — a failed
  update never touches the running app, so the last known-good bundle is
  whatever is already running.

### `qorm run` on a bundle

Running from a bundle file (rather than a source directory) yields an
OTA-capable server with two extra endpoints, both blocked to cross-origin
callers:

- `POST /update {"source": "https://example.com/app.qorm.bundle"}` — fetch,
  verify, check `requiredCapabilities` against this platform, then activate.
  Requires the server to have been started with `--trust`; without a trust key
  the endpoint refuses (403), because authenticity cannot be verified. On any
  failure it answers 409 and the live app keeps the previous bundle.
- `POST /rollback` — re-activates the previous bundle held in memory (one
  level: the bundle the last successful `/update` replaced).

### Packaged apps

`qorm package --update-url <url> --trust <key.pub>` bakes OTA into the
packaged web/mobile app; the flags are enforced as a pair. The trust split is
deliberate:

- the **bundled payload** (`bundle.json` inside the package) is trusted via
  the install channel — store signing / TLS origin — the same channel that
  delivered the runtime itself, so it is not re-verified at boot;
- every **OTA-origin bundle** (fetched from the update server, or restored
  from local storage where an earlier update persisted it) is verified with
  ed25519 against the embedded trust key before activation. A bundle that
  fails verification is discarded and the app falls back one tier
  (current update → previous update → bundled payload).

## CLI Reference

```text
qorm keygen [--out-dir .]                                     generate an ed25519 signing keypair
qorm build <app-dir> [-o out] [--key priv] [--version v] [--require-capability a,b]
                                                              compile a bundle; sign when --key is given
qorm sign <bundle> --key priv [-o out]                        sign an existing (e.g. agent-exported) bundle
qorm verify <bundle> [--trust pub] [--revoked list.json]      verify integrity (+ signature, + revocation)
qorm run <bundle> [--trust pub] [--revoked list.json]         serve a bundle; verifies before running, enables /update + /rollback
```

`qorm sign` recomputes the content hash before signing, so re-signing a
tampered bundle does not launder it — the signature then covers the tampered
hash, which is exactly what a verifier with the right public key will accept
*only if the tamperer holds the private key*. The guarantee chain is always:
the trusted public key decides whose content runs.

## What the Format Does Not Do

Stated plainly, so nothing here is mistaken for a guarantee:

- No `minRuntimeVersion` / runtime-version gating — a `qorm-bundle/1` runtime
  accepts any bundle with that format tag; version information
  (`content.app.version`) is informational (used in update/rollback status
  lines), not an activation gate.
- No expiry or timestamp in the signature — a validly signed bundle stays
  valid until its key is revoked.
- No remote revocation or trust-metadata refresh — trust roots and revocation
  lists are local files you distribute.
- No per-field signing — the signature covers all of `content` at once; you
  cannot sign scenes individually.
