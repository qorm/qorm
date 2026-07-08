# QORM Agent Permissions

The QORM agent permission model controls what an agent can see, change, and execute.

## Permission Levels

```text
read-only
preview-only
edit-json
apply-patch
host-call
build
run
deploy
admin
```

## Default Permissions

The default agent permission is:

```text
read-only + preview-only
```

Allowed:

```text
inspect
validate
preview_patch
explain
platform_check
```

Denied:

```text
apply_patch
host.call
filesystem.saveFile
shell
deploy
```

Here, `filesystem.saveFile` denotes a file-write capability; its permission key is usually `filesystem.write`.

## Runtime Enforcement

The `read-only` level is enforced at runtime by `qorm run --mcp-read-only`: the shared MCP session then rejects mutating tools (`qorm_dispatch`, `qorm_set_state`, `qorm_apply_patch`, `qorm_undo`) with a JSON-RPC "read-only mode" error, while inspection and preview tools keep working.

## Permission Declaration

```json
{
  "agent": "codex",
  "permissions": {
    "inspect": true,
    "validate": true,
    "previewPatch": true,
    "applyPatch": "requiresApproval",
    "hostCall": false,
    "shell": false
  }
}
```

## Relationship Between Agent Permissions and System Permissions

- Agent policy is an additional constraint layer, not a source of authorization.
- An agent allow cannot loosen the platform / app / host policy.
- An agent deny can further restrict that agent.
- `preview_patch` is available by default, but must still follow the no-side-effect principle.

## Dangerous Operations

Dangerous operations include:

```text
filesystem.saveFile
network.request to external domain
shell
process.spawn
native.call
plugin.install
deploy
bundle.publish
```

Here the danger of file writes corresponds to the `filesystem.write` permission domain.

These must require user confirmation.

## Approval Semantics

For an agent, the following operations typically require approval:

```text
apply_patch
host.call
filesystem.write
shell
deploy
```

Minimum rules:
- An approval scope must at least bind the agent, operation, and target resource range.
- A passing `preview_patch` result cannot automatically be treated as `apply_patch` approval.
- If policy permits, a single approval may cover "the apply of the same previewToken."
- After the target file, patch content, Bundle version, or permission policy changes, the original approval must be invalidated.

## Approval Lifecycle and Revocation

An approval record contains at least:

```text
approval id
agent id
operation
scope
issuedAt
expiresAt
reuse policy
revokedAt
```

Revocation triggers include at least:
- The user revokes it manually.
- The Agent Pack changes.
- The Bundle or target document changes.
- The system policy changes.
- Timeout expiration.

## Audit Log

All agent operations should be recorded:

```text
agent id
tool name
input summary
output summary
files changed
permissions used
timestamp
approval id
audit event id
```

The audit log should avoid storing complete sensitive inputs where possible; if it involves tokens, passwords, or absolute file contents, it should record a summary or redacted fields.
