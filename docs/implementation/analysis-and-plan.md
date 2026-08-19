# QORM 整体分析与目标实施规划

> 主控文档 · 2026-08-19 **第五轮（testrunner 组件物化 + 诊断联动）** · 第四轮 G1–G6 已提交
> 第三轮（2026-08-13）G0–G4 已随 v0.9.1–v0.9.2 交付；v0.9.3 仓库改名，
> v0.9.4 落地 agent.policy / capability 门 / breakpoints / partial render。
> 本轮关闭「目标对照实现」审计里排过序的缺口，不扩 widget、不加 MCP 工具。

## 1. 目标（已拍板）

主要矛盾：**给 agent 的小表面** vs **已膨胀的双宿主运行时**（非对抗性）。
规定其它矛盾的是：JSON 要人机都能写对，但类型对等 ≠ 保真度对等，
`qorm test` 也看不到 list 行。本轮集中兵力做可验证性，不扩表面。

| 目标 | 内容 | 验收 |
|------|------|------|
| G1 双宿主质量面 | canvas measure 暴露 `hostLimits`；MCP 文案与 canvas 已有 contrast 对齐 | 测试：fontFamily / webview 出现 hostLimits；`qorm_check_layout` 描述不再声称 canvas contrast 不可用 |
| G2 `qorm test` 走出 MVP | 物化 list/gridview `renderItem`；examples/todo + derived 各有测试文档 | `go test ./internal/testrunner`；`qorm test examples/todo` 与 `examples/derived` exit 0 |
| G3 文档对齐 | README 架构图、security-model（能力门已落地）、SKILL 冻结 widget、实施文档 | 漂移句删除或改成现状 |
| G4 桌面 beta 诚实 | 支持矩阵注明 Linux chromeless 解析但不剥 GTK 装饰 | `QORM_UPDATE_DOCS=1 go test ./internal/support` |
| G5 LAN 观察窗 | `--lan` 下 `/logwindow` `/console` `/dev/tree` `/dev/canvas` `/dev/highlight`：loopback 免鉴权，非回环要 admin token | token_gate 测试：127.0.0.1 200，10.x 401，admin 200，page token 401 |
| G6 package --revoked | OTA 包装注入吊销快照 | 缺 `--update-url` 时拒绝；有配对时 `__QORM_UPDATE__` 含 `revoked` |
| G7 JSON 组件物化 | `qorm test` 展开组件模板 + slot；`simulate_event` 用 list item / prop 作用域求 handler 参数 | `TestMaterializeExpandsJSONComponents`；`TestSimulateEventUsesListItemScope`；`qorm test examples/uikit` |
| G8 诊断联动 | error 级 loader 诊断让 `qorm test` / `qorm check` 失败（computed 动态 key、悬空 onEnter） | `TestRunRefusesErrorDiagnostics`；`TestCmdCheckRefusesErrorDiagnostics` |
| G9 对比表诚实 | README 小程序标静态 WXML；MCP 工具数 25 | README en+zh |
| G10 示例测试加深 | todo 行内 Done（item scope + computed 断言）；counter 纳入 CI 回归 | `examples/todo/tests/toggle.json`；`TestRunExampleAppsWithTests` |
| G11 derived 别名 + 守卫 + tetris | list `as:"line"` 行内 Wrap；checkout 守卫；tetris onEnter + qscript moveLeft | `gift_line.json` / `checkout_*.json` / `examples/tetris/tests/move_left.json` |

完成门槛：相关包测试绿；文档生成器同步（mcp / support）；不 push。

## 2. 实施决策

1. **G1 不实现任意 `fontFamily`。** canvas 字体是 bitmap + CJK subset；把「忽略了什么」写进 measure，agent 才能证明差距。webview 一律标 placeholder（`-tags canvaswebview` 除外的语义：纯 Go 宿主没有引擎）。
2. **G2 第四轮只物化 list。第五轮补上 JSON 组件与 list item scope。** 行内按钮的 press 按物化节点的 ctx 求 `{{item.id}}` / `{{prop.x}}`，与 HTML Handler.Scope / canvas itemInstance 对齐。
3. **G4 不接线 GTK。** Linux 原生层刻意 cgo-free DBus；接线等于推翻该约束。矩阵改注释，状态保持 beta。
4. **G5 产品决策：观察窗绑在 loopback。** 满足 AGENTS.md「必须开人类观察窗」，同时堵住 LAN 对等端裸读 `/dev/tree`。不把 token 嵌进 logwindow 页（第三轮已禁止 admin token 出现在任何响应页）。
5. **非目标：** 新 widget、新 MCP 工具、小程序交互运行时、Platform Pack、Error Boundary / http cancel、VS Code LSP、Registry、马里奥 v2 未移植回归。

## 3. 文件域

| 目标 | 文件 |
|------|------|
| G1 | `internal/render/canvas/measure_report.go` · `internal/mcp/tools.go` · `internal/mcp/doc.go` · `internal/measure/measure.go` |
| G2 | `internal/testrunner/query.go` · `internal/testrunner/doc.go` · `examples/todo/tests` · `examples/derived/tests` |
| G7 | `internal/testrunner/components.go` · `internal/testrunner/query.go` · `examples/uikit/tests` |
| G8 | `internal/testrunner/testrunner.go` · `cmd/qorm/commands.go` |
| G3 | `README.md` · `docs/security/security-model.md` · `docs/zh/security/security-model.md` · `integrations/skill/SKILL.md` · 本目录 |
| G4 | `internal/support/support.go` |
| G5 | `internal/server/server.go` · `internal/server/token_gate_test.go` |
| G6 | `cmd/qorm/package.go` · `cmd/qorm/package_ota_test.go` · `internal/server/offline_fetch_test.go` · `api/cli.md` |

## 4. 需监控的次要矛盾

游戏（canvas 60fps）若继续当门面，而应用（HTML/MCP）才是 agent 协作产品，默认宿主会继续暴露 canvas 降级。G1 的 `hostLimits` 是监测点，不是解决点。
