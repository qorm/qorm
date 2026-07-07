# QORM Permission Model

The permission model controls the available scope of Host Capabilities, Agents, Plugins, Bundles, and Platform Packs.

## Permission Sources

```text
platform capabilities
bundle requirements
app policy
agent policy
plugin manifest
user approval
```

## Permission Priority

A single call is adjudicated in the following order:

```text
1. capability exists
2. platform supports capability
3. bundle declares capability
4. app/system policy allows capability
5. agent/plugin policy allows capability
6. user approved if required
```

Rules:
- Deny by default.
- A deny at any stage results in a deny.
- User approval cannot override platform unsupported or app/system hard deny.
- `bundle declares capability` only expresses a requirement declaration; it does not grant permission.

## Conflict Resolution

- deny overrides allow.
- A missing declaration is treated as deny.
- Structured restrictions can only narrow, never widen.
- For example, when multiple sources declare `domains`, the final allowed scope is their intersection.
- If one source does not declare `methods` while another declares `methods`, the final result only allows the declared method set.

## Permission Example

```json
{
  "permissions": {
    "network.request": {
      "allowed": true,
      "domains": ["api.example.com"],
      "methods": ["GET", "POST"]
    },
    "filesystem.write": {
      "allowed": true,
      "requiresApproval": true
    },
    "shell": {
      "allowed": false
    }
  }
}
```

## requiresApproval Semantics

`requiresApproval` means that even if the capability or operation is already allowed, a valid approval credential is still required.

Minimum rules:
- The scope must at least bind the `capability`, the caller, and the target resource scope.
- Approval may be one-time or session-level, but must be explicitly defined by policy.
- After the target resource changes, the original approval must not automatically extend to a broader scope.
- After a change to policy, Bundle, Platform Pack, Agent Pack, user identity, or runtime environment, approval must be re-evaluated.

## Approval Lifecycle

An approval record contains at least:

```text
approval id
issuer
scope
issuedAt
expiresAt
reuse policy
revokedAt
revocation reason
```

Invalidation conditions include at least:
- Timeout expiration.
- Explicit revocation by the user.
- Bundle or Pack version switch.
- Permission policy change.
- Change in the scope of target files, target domains, or target capability.

## Agent Permissions

Agent-side permissions should be stricter:

```text
read-only
preview-only
edit-json
apply-patch
host-call
admin
```

`apply-patch` and `host-call` are not granted by default.

Agent allow cannot relax app/system policy; Agent deny can further restrict that Agent.

## Auditing

All permission-related operations must be recorded in an audit log.

Minimum fields:

```text
decision id
subject type
subject id
capability or operation
input summary
policy result
approval id
reason
timestamp
```

Audit logs should avoid recording raw sensitive payloads; if context must be retained, record summaries or redacted fields.