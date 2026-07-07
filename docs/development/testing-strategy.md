# QORM Testing Strategy

## 测试分类

```text
unit test
integration test
schema test
snapshot test
golden test
property test
benchmark
platform test
security test
conformance test
```

## 格式测试

- JSON Schema 校验。
- 合法样例测试。
- 非法样例测试。
- 错误 path 和 suggestion 测试。
- 表达式字段与模板插值字段的区分测试。
- State Path 与 Patch Path 语法测试。

## IR 测试

- IR snapshot。
- Source Map 测试。
- 引用解析测试。
- Bundle roundtrip。
- 未定义类型、未解析引用和重复 ID 诊断测试。

## Runtime 测试

- State change。
- Binding dependency。
- Event dispatch。
- Action execution。
- Motion tick。
- Patch preview/apply/rollback。
- preview 必须无副作用。
- 异步 `host.call` 顺序与错误传播测试。

## Layout 测试

- row / column / stack / absolute / overlay / scroll。
- safe area。
- text measurement。
- dirty layout subtree。
- virtual list。

## Render 测试

- Display List golden。
- Render Graph snapshot。
- Text cache。
- Texture atlas。
- Game HUD frame update。

## Host 测试

- Mock Host Capability。
- 权限拒绝。
- 平台不支持。
- 错误返回。
- `network.request` 域名、方法、timeout、responseType 测试。
- custom HttpClient 不能绕过权限约束的测试。

## Agent 测试

- MCP tool input/output。
- preview_patch。
- apply_patch 权限。
- simulate_event。
- explain_node。
- previewToken 失效与 approval 失效测试。

## Security 测试

- capability precedence。
- deny overrides allow。
- requiresApproval 生命周期。
- approval revocation。
- Bundle signature / keyId / revocation。

## Conformance 测试

- `json-format-spec.md`、`runtime-spec.md`、`action-spec.md`、`motion-spec.md` 的示例必须可相互验证。
- Action 支持集合在 JSON / IR / Runtime 三层必须一致。
- Patch 示例路径必须全部符合逻辑路径规范。
- 教程与示例不得使用超出 V1 支持范围的能力。

## 性能测试

- Bundle load。
- First render。
- Layout update。
- Display List diff。
- Game HUD update。
- Text cache hit。
- preview_patch 不进入渲染热路径。