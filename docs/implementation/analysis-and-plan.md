# QORM v0.9.5 发布规划

> 主控文档 · 2026-08-19 · **主题：agent 可验证性收口**
> 第四–八轮已在 main 落地（相对 origin 超前）；本文件是发版前的完整规划：
> 范围内做什么、明确不做什么、怎么验收、怎么线上发布。

## 1. 这一版要达成什么

v0.9.4 给了 agent.policy / capability 门 / breakpoints / partial render。
对照「目标 vs 实现」，agent 仍无法**证明**同一份 JSON 在双宿主上差在哪，
`qorm test` 也看不见 list 行、组件实例、路由守卫、游戏世界坐标。

v0.9.5 不扩表面（不加 widget、不加 MCP 工具）。只把验证闭环补齐，然后发版。

**完成定义：** `go test ./...` 绿；`qorm test` 覆盖 counter / todo / derived / uikit / tetris / mario；CHANGELOG `[Unreleased]` 可归档；tag `v0.9.5` 触发二进制 + GHCR；站点部署后首页 200。

## 2. 范围内（已落地 / 本轮补齐）

| ID | 内容 | 状态 | 验收 |
|----|------|------|------|
| G1 | canvas `hostLimits`；MCP contrast 文案对齐 | 🟢 | `TestCollectMeasureHostLimits` |
| G2 | list/gridview 物化 | 🟢 | `TestMaterializeExpandsListRenderItem` |
| G3 | README 架构图 / security-model / SKILL 冻结 widget | 🟢 | 与 runtime 一致 |
| G4 | Linux chromeless：矩阵诚实，不接线 GTK | 🟢 | support-matrix |
| G5 | LAN 观察窗 loopback/admin 门 | 🟢 | `TestLANObservationWindowLoopbackOnly` |
| G6 | `qorm package --revoked` | 🟢 | OTA 包装含 revoked |
| G7 | JSON 组件物化 + list item scope | 🟢 | uikit `qorm test`；`TestSimulateEventUsesListItemScope` |
| G8 | error 诊断失败 `qorm test` / `qorm check` | 🟢 | `TestRunRefusesErrorDiagnostics` |
| G9 | 对比表：小程序=静态 WXML；MCP=25 | 🟢 | README en+zh |
| G10 | todo 行内 Done + `computed.*` 断言 | 🟢 | `tests/toggle.json` |
| G11 | derived `as:line` / checkout 守卫 / tetris smoke | 🟢 | gift_line + checkout_* + move_left |
| G12 | `state_lt/gt` + `repeat` + mario 左走 | 🟢 | `walk_left.json` |
| R1 | mario 跳跃阈值；CHANGELOG；SKILL 游戏规则补 `layerCache` | 本轮 | `jump.json`；`## [Unreleased]` 非空 |

## 3. 明确不进 v0.9.5

这些不是「没写完」，是产品决策：**这一刀不砍它们**。

| 项 | 原因 |
|----|------|
| Linux GTK chromeless 接线 | Linux 层刻意 cgo-free DBus；接线等于推翻约束。矩阵保持 beta。 |
| Error Boundary 组件 | Phase 10；loader/runtime 大改，不是验证缺口。 |
| 独立 http cancel-token 步骤 | `"key"` 已 supersede 取消重叠请求；单独 cancel 步骤仍 planned。 |
| canvas list 视口虚拟化 | 性能项，改动面大，发版后做。 |
| 小程序交互运行时 | 仍是静态 WXML 快照。 |
| Platform Pack / VS Code LSP / Registry | 超规模。 |
| 马里奥全关卡回归 | canvas 引擎已有 `mario_v2_test.go`；`qorm test` 只保证动作+阈值，不替代引擎测试。 |

## 4. 本轮落地步骤

1. 写本规划（本文）。
2. `examples/mario/tests/jump.json`：`jump` + 若干 `tick`，`state_lt mario.y`。
3. SKILL 游戏规则加一句：滚动世界 / 大 list 用 `layerCache`。
4. 把 G1–G12 + R1 写入 `CHANGELOG.md` 的 `## [Unreleased]`（`release.sh` 归档前提）。
5. `go test ./...`。
6. 提交、**先 push main**（`release.sh` 要求 `main == origin/main`）。
7. 修正本地 `web_server/release.sh` 仓库名为 `qorm/platform`（rename 后旧名会看错 CI）。
8. `./web_server/release.sh 0.9.5`：changelog 归档 → bump `cmd/qorm/main.go` → tag → push → 等 Release/Docker → 改 Release notes → 部署站点 → 校验资产与首页。

## 5. 发布后

- GitHub Release `v0.9.5`：≥6 个平台二进制 + SHA256SUMS。
- `ghcr.io/qorm/platform:v0.9.5`。
- qorm.com 首页 200，文档带上本轮 verification 口径。
- 下一刀（不在本 tag）：canvas list 虚拟化，或 Error Boundary 的最小切片。
