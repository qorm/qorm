# QORM OpenClaw Pack

The OpenClaw Pack is for long-running, cross-channel, or automated agents connecting to QORM.

## Positioning

OpenClaw can call the QORM MCP Server, but QORM does not depend on OpenClaw.

```text
OpenClaw Agent
  ↓
QORM MCP Server
  ↓
QORM Agent Protocol
  ↓
QORM Core
```

## Default Policy

The OpenClaw Pack uses a stricter permission policy by default:

```text
inspect: allow
validate: allow
preview_patch: allow
explain: allow
apply_patch: deny by default
host.call: deny by default
shell: deny
filesystem.write: deny unless approved
```

## Permission Boundaries

- The OpenClaw Pack can only narrow permissions; it cannot loosen platform or system policy.
- `preview_patch` must still be free of side effects.
- Any elevation must be bound to an explicit approval lifecycle.

## Applicable Tasks

```text
Remote UI inspection
Generating change suggestions
Previewing Patches
Platform compatibility checks
Layout diagnostics
Documentation generation
```

## Discouraged Tasks

```text
Automatic patch apply
Automatic deployment
Automatic invocation of low-level system capabilities
Automatic modification of permission configuration
```

## Permission Elevation

Any permission elevation must include:

```text
Reason
Scope
Target files
Risk
User confirmation
Expiration time
```
