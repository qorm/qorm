<!-- data-lang-nav --> [English](../../agent/codex-pack.md) · 中文

# QORM Codex Pack

Codex Pack 让面向代码的 agent 能够操作 QORM 仓库和 JSON 文件。

## 目标

- 生成 JSON 源文件。
- 修改 Go 代码(`cmd/`、`internal/`)。
- 运行测试。
- 通过 MCP 检查 QORM Bundle。
- 生成并预览 Patch。

## 默认允许

```text
read files
edit JSON docs/code
run qorm check
run go test ./...
preview_patch
```

## 默认拒绝

```text
Direct deploy
Running dangerous shell commands
Bypassing preview_patch
Adding unauthorized Host Capabilities
```

## 权限边界

- Codex Pack 只能进一步限制其自身行为;它不能放松平台 / 应用 / 宿主策略。
- `preview_patch` 默认允许,但它必须保持无副作用。
- `apply_patch` 必须遵守统一的批准规则。

## 工作流

```text
1. Read the README and relevant spec
2. Read the target JSON file
3. Generate a patch
4. qorm.validate_bundle
5. qorm.preview_patch
6. apply after user confirmation
7. Run tests
```

## 输出要求

当 Codex 修改 QORM 文件时,它应输出:

```text
Summary of changes
Files modified
Validation results
Potential risks
Whether user confirmation is required
```
