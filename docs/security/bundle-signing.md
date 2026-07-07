# QORM Bundle Signing

Bundle signing ensures the security of dynamic updates.

## Bundle Metadata

```json
{
  "qorm": "0.1",
  "type": "bundle",
  "id": "demo_bundle",
  "bundleVersion": "1.0.0",
  "minRuntimeVersion": "0.1.0",
  "hash": "...",
  "signature": "...",
  "keyId": "release-2026-q2"
}
```

## Signature Algorithms

V1 must fix at least one signature algorithm to avoid implementation divergence. Recommended:

```text
hash: SHA-256
signature: Ed25519
```

If an implementation supports additional algorithms, they must be explicitly identified in the Bundle metadata and controlled by a Runtime allowlist.

## Canonicalization

Signatures must be based on a stable byte sequence, not arbitrary JSON text.

Minimum rules:
- UTF-8 encoding.
- Object keys sorted in lexicographic order.
- Arrays keep their original order.
- Must not depend on differences in whitespace, line breaks, or indentation.
- Multi-file Bundles must first be expanded into the canonical Bundle structure before computing hash / signature.

## Bundle Metadata and Signed Object

The signature should cover:

```text
manifest
resolved scenes
resolved components
resolved styles
resolved actions
resolved motions
resources
capability requirements
compiled execution plans
```

Signing the root file alone is not sufficient.

`signature` should cover the canonicalized Bundle content; `hash` should be the content hash of the same canonicalized content.

## Verification Flow

```text
1. Download Bundle
2. Verify size and basic JSON
3. Canonicalize Bundle
4. Verify hash
5. Verify signature
6. Verify keyId / trust root / revocation status
7. Verify bundleVersion / minRuntimeVersion
8. Verify capability requirements
9. Pre-resolution and semantic validation
10. Activate
11. Roll back on failure
```

## Trust Root and keyId

- `keyId` identifies the signing key.
- The Runtime must only trust signers allowed by the local or built-in trust store.
- Unknown `keyId`, untrusted signers, or revoked keys must be denied activation.

## Key Rotation

Minimum rules:
- Old and new keys may be trusted simultaneously within a limited window.
- During rotation, new trust metadata must be distributed first, before distributing Bundles signed only by the new key.
- After the trust store is updated, the old key may be marked as revoked or deprecated.

## Revocation

A key revocation or signer revocation mechanism must be supported.

Minimum semantics:
- Revocation information may come from local trust metadata or a remote refresh.
- In offline environments where revocation information cannot be refreshed, the most recent trusted revocation snapshot should be used.
- A cached Bundle whose signature has been revoked must not continue to serve as a new activation target.

## Rollback Strategy

Mobile and production environments should retain:

```text
current bundle
previous bundle
last known-good bundle
```

### known-good Definition

A `last known-good bundle` must at least satisfy:
- Signature and version verification pass.
- Pre-resolution and activation complete successfully.
- No fatal startup errors are triggered.

If a new Bundle fails to activate, automatically roll back to the `last known-good bundle`.

## Handling Verification Failures

- hash mismatch: deny activation.
- signature mismatch: deny activation.
- keyId unknown or revoked: deny activation.
- `minRuntimeVersion` not satisfied: deny activation.
- capability requirements not satisfied: deny activation.
- Pre-resolution or semantic validation fails: deny activation and roll back.

No failure may be degraded into "continue running with a warning".