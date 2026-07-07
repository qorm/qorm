# QORM 正式规格补完计划

## 背景

QORM 已经在规划文档中引入了若干产品级能力：
- DevServer / HMR / `qorm test`
- 全局状态 (Global Store / Context)
- 错误边界 (Error Boundary)
- 第三方资产依赖与生态分发

这些方向是正确的，但目前大多仍停留在 planning 层，没有进入正式 spec。若直接进入实现，会导致不同实现者各自补语义，产生高返工成本。

## 目标

把已进入规划但尚未规范化的能力补成正式 spec，使其能够进入实现、测试和发布流程。

## 优先级一：Runtime 与数据模型

### 1. Global State Specification

需要定义：
- store 层级
- 作用域与可见性
- 读写路径语法
- 生命周期
- 与 scene local state 的关系
- Agent Patch 的隔离与权限边界
- 并发冲突与一致性模型

### 2. Error Boundary Specification

需要定义：
- 可捕获错误类型
- fallback 节点/组件契约
- 传播与拦截规则
- diagnostics 报告
- preview / apply / runtime 下的行为差异

## 优先级二：开发者体验与测试

### 3. Test Runner Specification

需要定义：
- `qorm test` 的运行模型
- headless runtime / renderer 边界
- 事件模拟
- host capability mock
- 时间控制与异步 step 测试
- 测试结果格式

已完成首批起草：`docs/spec/test-runner-spec.md`

### 4. Query Selector Specification

需要定义：
- 节点查询语法
- semantic role 查询
- text / state / component instance 查询能力
- 断言 API 的输入输出模型
- 与 Agent inspect / explain / patch 的关系

已完成首批起草：`docs/spec/query-selector-spec.md`

### 5. DevServer / HMR Specification

需要定义：
- 增量刷新单元
- state 保留规则
- schema 变化导致的 reset 规则
- HMR 与 Patch Preview 的关系
- diagnostics / logs / reload protocol

已完成首批起草：`docs/spec/devserver-hmr-spec.md`

## 优先级三：生态资产与依赖

### 6. Asset Package Specification

需要定义：
- declarative pack 与 executable plugin pack 的分类
- 包结构
- 元数据
- 兼容性声明
- 签名与 trust 字段
- Portal 展示所需字段

已完成首批起草：`docs/spec/asset-package-spec.md`

### 7. Dependency Resolution Specification

需要定义：
- `dependencies` 字段
- version range 语法
- lockfile
- transitive dependency
- conflict resolution
- source priority
- offline cache

## 优先级四：平台细化

### 8. Miniapp Vendor Capability Profiles

需要定义：
- 通用 miniapp abstraction
- vendor-specific capability profiles
- 动态更新与审核约束差异
- 测试与调试模型差异
- degraded mode 可观察性

已完成首批起草：`docs/spec/miniapp-vendor-profiles-spec.md`

## 推荐落地顺序

```text
1. global-state-spec
2. error-boundary-spec
3. test-runner-spec
4. query-selector-spec
5. devserver-hmr-spec
6. asset-package-spec
7. dependency-resolution-spec
8. miniapp-vendor-profiles
```

## 与现有文档的关系

以下规格已完成首批、第二批与第三批起草：
- `docs/spec/global-state-spec.md`
- `docs/spec/error-boundary-spec.md`
- `docs/spec/dependency-resolution-spec.md`
- `docs/spec/test-runner-spec.md`
- `docs/spec/query-selector-spec.md`
- `docs/spec/asset-package-spec.md`
- `docs/spec/devserver-hmr-spec.md`
- `docs/spec/miniapp-vendor-profiles-spec.md`

这些规格补完后至少需要同步：
- `docs/spec/runtime-spec.md`
- `docs/spec/json-format-spec.md`
- `docs/spec/action-spec.md`
- `docs/spec/sdk-spec.md`
- `docs/development/testing-strategy.md`
- `docs/planning/implementation-plan.md`
- `docs/planning/ecosystem-registry-plan.md`

## 验收标准

```text
所有已进入 implementation plan 的高级能力都有正式 spec 承接
Runtime / JSON / Action / SDK / Testing 文档不再依赖隐含语义
qorm test、global state、error boundary、dependencies 都有稳定协议定义
生态资产与依赖分发可进入实现，而不是继续停留在概念层
```