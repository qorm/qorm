# QORM Codex Pack

Codex Pack 用于代码型 Agent 操作 QORM 仓库和 JSON 文件。

## 目标

- 生成 JSON 源文件。
- 修改 Go 代码（`cmd/`、`internal/`）。
- 运行测试。
- 通过 MCP 检查 QORM Bundle。
- 生成 Patch 并预览。

## 默认允许

```text
read files
edit JSON docs/code
run qorm check
run go test ./...
preview_patch
```

## 默认禁止

```text
直接 deploy
执行危险 shell
绕过 preview_patch
新增未授权 Host Capability
```

## 权限边界

- Codex Pack 只能进一步限制自身行为，不能放宽 platform / app / host policy。
- `preview_patch` 默认允许，但必须保持无副作用。
- `apply_patch` 必须服从统一 approval 规则。

## 工作流

```text
1. 读取 README 和相关 spec
2. 读取目标 JSON 文件
3. 生成 patch
4. qorm.validate_bundle
5. qorm.preview_patch
6. 用户确认后 apply
7. 运行测试
```

## 输出要求

Codex 修改 QORM 文件时，应输出：

```text
改动摘要
修改文件
验证结果
潜在风险
是否需要用户确认
```