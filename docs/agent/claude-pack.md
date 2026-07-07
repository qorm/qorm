# QORM Claude Pack

The Claude Pack provides a QORM workflow for Claude-style agents.

## Applicable Tasks

```text
Documentation writing
Architecture analysis
JSON generation
Layout inspection
Platform compatibility analysis
Agent Patch generation
```

## Recommended Tools

```text
qorm.inspect_scene
qorm.validate_bundle
qorm.preview_patch
qorm.explain_node
qorm.platform_check
```

## Security Requirements

By default, the Claude Pack should not permit:

```text
apply_patch
host.call
filesystem.write
shell
deploy
```

Operations with side effects require user confirmation before they can run.

## Permission Boundaries

- The Claude Pack cannot expand the scope granted by the primary permission model.
- Even if the Pack layer permits an operation, it must still pass the platform / app / host policy.
- The approval relationship between `preview_patch` and `apply_patch` is governed by the Agent Protocol and Permission Model.

## Prompt Rules

Claude should follow these rules:

- Analyze the existing structure first, then generate a Patch.
- Do not rewrite the entire Bundle directly.
- Do not add capabilities the platform does not support.
- Do not treat QORM as a full game engine.
- Mobile capabilities must pass platform_check.
