# QORM Codex Pack

The Codex Pack lets code-oriented agents operate on QORM repositories and JSON files.

## Goals

- Generate JSON source files.
- Modify Go code (`cmd/`, `internal/`).
- Run tests.
- Inspect QORM Bundles via MCP.
- Generate and preview Patches.

## Allowed by Default

```text
read files
edit JSON docs/code
run qorm check
run go test ./...
preview_patch
```

## Denied by Default

```text
Direct deploy
Running dangerous shell commands
Bypassing preview_patch
Adding unauthorized Host Capabilities
```

## Permission Boundaries

- The Codex Pack can only further restrict its own behavior; it cannot loosen the platform / app / host policy.
- `preview_patch` is allowed by default, but it must remain free of side effects.
- `apply_patch` must obey the unified approval rules.

## Workflow

```text
1. Read the README and relevant spec
2. Read the target JSON file
3. Generate a patch
4. qorm.validate_bundle
5. qorm.preview_patch
6. apply after user confirmation
7. Run tests
```

## Output Requirements

When Codex modifies QORM files, it should output:

```text
Summary of changes
Files modified
Validation results
Potential risks
Whether user confirmation is required
```
