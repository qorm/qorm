# QORM 整体分析与目标实施规划

> 主控文档 · 2026-08-13 **第三轮（v0.9.x）** · 以代码、测试与 `examples/` 为准
> 前两轮（2026-08-12）的 G0–G2 已随 v0.8.8–v0.9.0 交付；第二轮文档快照中
> "HTML 无 QSS cascade / 双路径未对等" 的表述已过时（QSS、swipes、相机、
> WebAudio、parity harness 均已落地），本轮以 git 实况为准。

## 1. 项目现状（第一阶段 4 路 agent 审计结论，HEAD 0fd604d）

| 维度 | 结论 |
|------|------|
| 版本 | v0.9.0（tag ffb5482）+ 2 个未入 changelog 的后续提交 |
| 规模 | ~13 万行 Go · 40+ examples · MCP 25 tools · HTML/Canvas 双渲染宿主 |
| 构建 | `go build ./...` / `go vet` / WASM 构建 **绿**（0fd604d 修复后） |
| 测试 | 30/33 包绿；**3 个确定性 FAIL 全部源自 62df1ca（canvas-ultimate）** |
| race | server/runtime/mcp/loader/bundle `-race` 无数据竞争 |
| 红队 | 2026-07-26 清单 P0/P1 全部关闭；4 项遗留经本轮逐条核验**全部属实** |

### 1.1 已处置（G0，本轮主控直接执行）

`62df1ca` 把调试会话残留扫进仓库：空 `scratch.go`、重复 `package main` 桩、
补丁/脚本杂物、两个未加构建标签的 cgo/ObjC 原型 cmd——main 上 `go build ./...`
直接失败（Linux CI 必红）。**已删除 11 个文件并提交 `0fd604d`**；视频功能本体
（`internal/widgets/video_decoder_darwin.go`，有 `//go:build darwin && !js`）不受影响。

### 1.2 待处置（本轮目标）

**测试基线（P1，3 个 FAIL 包）**
- T1 `examples/canvas-advanced` 损坏：场景用未注册的 `path` widget（scenes/main.json:29，
  SVG 风格 `d` 属性 + stroke 样式，意图是路径变形演示）；`actions/toggle_path.qs`
  行尾多余 `;` 被 qscript 拒绝（兄弟脚本均无分号）。牵连 internal/integration
  （examples/playground 编译检查）与 internal/server（TestSweepAllExamplesRenderClean）。
- T2 `TestVideoMeasure` 期望 4:3 640x480，实现返回 16:9 640x360（video.go:69-70 为准，测试过期）。
- T3 `api/props.md` / `api/widgets.md` 与目录失步；且 `QORM_UPDATE_DOCS=1` 是整文件覆写，
  会**静默删除 42 行手写 RichText/Video/A11y 章节**。

**安全残留（上轮 G3 遗留，本轮全部核验属实）**
- S1（P1）`--lan`（0.0.0.0）下 `/mcp`、`/update`、`/rollback`、`/window` 无鉴权，
  仅 Origin 头筛查（非浏览器客户端直接绕过）；`POST /measure` 同为未鉴权写。
- S2（P2）`computed[动态key]` 无依赖边且 loader 无诊断——隐藏环静默发布垃圾值
  （红队 :85/:122 遗留，从未设防）。
- S3（P2）WASM/offline/playground OTA **从不查吊销**（cmd/qorm-wasm/main.go:99/:436
  传 nil；UpdateConfig 无法携带列表）——README 的 `--revoked` 承诺只对 CLI/server 成立。
- S4（P3）旧 qorm 收到新 bundle 报 "tampered"（bundle.go:222），误导运维；
  红队建议的版本提示从未实现。

**DX / 路线图（方向评分 agent 结论）**
- D1 实施 Phase 9 **`qorm test` 无头测试运行器 MVP**。理由：spec 完全规定
  （test-runner-spec.md 258 行，零产品决策）；钩子最多（loader 已容忍 type:"test"
  文档、Runtime.Clone/Dispatch/RunPendingEnter 无副作用、无宿主 sink 时 delay 同步降级）；
  直接关闭 roadmap Phase 9 缺口（qorm_assert 只能跑在活渲染上，无 CI 级无头跑法）。
  Error Boundary / Global State 规格同样成熟但要改写核心求值/状态契约，风险更高，留后续轮。
  VS Code/LSP、Registry 超一轮规模，不做。

**文档漂移（审计 agent 清单）**
- DOC1 CHANGELOG 无 v0.9.0 段（[Unreleased] 空，release.sh 归档门被跳过）。
- DOC2 docs/implementation 三份文档停在 v0.8.8（本文件除外，随本轮重写）。
- DOC3 `docs/zh/platforms/web.md` 缺 games 需 canvas 宿主的镜像句（EN web.md:19-20 已有）
  ——第二轮 P2 文档债的唯一残留。
- DOC4 README Roadmap 仍把 docs portal / Playground 列为待做（roadmap.md Phase 11 标已落地）。
- DOC5 roadmap.md Phase 5 "15 个 MCP 工具" 过时（现 25）。

## 2. 本轮目标与验收

| 目标 | 内容 | 验收 |
|------|------|------|
| G1 测试基线全绿 | T1+T2+T3 | `go test ./...` 33/33 包 PASS（darwin） |
| G2 安全硬化 | S1–S4 | 对应回归测试 + 审计 agent 对抗复验 |
| G3 DX | D1 `qorm test` MVP | examples 上的测试文档跑通、exit code 正确、JSON 报告合 spec |
| G4 文档对齐 | DOC1–DOC5 + 本目录三份文档 | 漂移清单逐条关闭 |

完成门槛（固定约束）：全量代码审查（对抗复审）+ 全项目回归
（`go test ./...` + 关键包 `-race` + WASM 构建 + gofmt）全绿；
只在本地 main 提交，**不 push**。

## 3. 实施决策（主控拍板，agent 执行）

1. **T1 `path` widget：实现而非删改。** 场景意图明确（SVG 子集 `d` + stroke + 绑定切换），
   引擎已有贝塞尔基建（timeline path-follow cubic）。范围：SVG 路径子集解析
   （M/L/H/V/Q/T/C/S/Z 绝对+相对）、HTML 侧内联 `<svg><path>`（render.go 加 case）、
   canvas 侧栅格化（fill+stroke）并注册。**`d` 变形不做插值动画**（binding 切换即换形，
   文档如实说明）。若实施中遇结构性阻塞，降级方案：场景改用现有 widget 并记录原因。
2. **T2：改测试**（16:9 是当前 videoNode 的有意行为）。
3. **T3：修生成器**——把手写章节并入生成输出（永久保留），再重新生成；
   zh 镜像 api/zh/widgets.md 同样处理。禁止"只覆写不保留"。
4. **S1：非回环绑定即发 token**（复用 X-Qorm-Token 机制），启动时打印 + 大字警告；
   `/mcp`、`/update`、`/rollback`、`/window`、`POST /measure` 强制校验。
5. **S2：loader 加载期 error**（动态 bracket key 的 computed 直接拒绝，附原因）。
6. **S3：UpdateConfig/__QORM_UPDATE__ 增加可选 revocation 数组**，两个校验点接入，文档说明。
7. **S4：hash 失配且 bundle 声明版本时附版本提示**，不改 fail-closed 语义。
8. **D1 MVP 边界**：步骤 mount_scene / simulate_event（dispatch action）/ set_state；
   断言 state_equals / node_exists / node_not_exists / text_equals / prop_equals；
   JSON 报告 + exit code。advance_time / flush_async / host-mock 留下轮。

## 4. Agent 分工（worktree 隔离，文件域互斥）

| Agent | 目标 | 文件域 |
|-------|------|--------|
| impl-testfix | G1：T1+T2+T3 | examples/canvas-advanced · internal/render · internal/widgets · internal/integration · api/ |
| impl-security | G2：S1–S4 | internal/server · internal/loader · internal/bundle · cmd/qorm-wasm · internal/server/offline.go |
| impl-qormtest | G3：D1 | cmd/qorm · 新 internal/testrunner · examples/counter/tests · spec 状态注记 |

实施后主控合并 → **对抗复审 3 路**（回归正确性 / 安全实跑复验 / qorm test 对 spec 逐条）
→ 全量回归门 → G4 文档同步 → 收尾提交。

## 5. 审计门槛（沿用）

- 声称完成必须附命令与结果摘要；区分 事实 / 假设 / 产品决策
- 不扩大范围（禁止无关重构）；只读 agent 不得改动工作区
  （本轮第一阶段 audit-docs 曾遗留 api/*.md 试跑改动，主控已还原并记录）
- 每波结束同步 progress.md / audit-log.md，不静默丢项

## 6. 非目标（本轮）

- Error Boundary / Global State / 依赖解析（Phase 10，后续轮）
- VS Code/LSP、Ecosystem Registry、真机验收（需外部环境/决策）
- `d` 变形插值动画、advance_time/host-mock（D1 二期）
- push 到远程（完工前只本地提交）
