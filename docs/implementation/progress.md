# 实施进度

> 主控同步 · 更新：2026-08-19 **第八轮落地** · 第七轮已提交 `67d088e`

## 第八轮（2026-08-19）

让 `qorm test` 能测固定 dt 的 canvas 游戏，并用阈值断言世界坐标。

| 目标 | 状态 | 证据 |
|------|------|------|
| G12 `state_lt/gt/lte/gte` | 🟢 | `TestStateCompareAsserts` |
| G12 `repeat` 步骤 | 🟢 | `TestRepeatStep`；mario 60×tick |
| G12 mario 左走阈值 | 🟢 | `examples/mario/tests/walk_left.json`（x 低于 28） |
| G12 http `key` 取消文档 | 🟢 | first-action en+zh；SKILL |

## 第七轮（2026-08-19）

把 `qorm test` 覆盖到 list `as` 别名、路由守卫，以及 canvas 游戏的 qscript 动作。

| 目标 | 状态 | 证据 |
|------|------|------|
| G11 derived 行内 Wrap（`as: line`） | 🟢 | `examples/derived/tests/gift_line.json`；`TestSimulateEventUsesListAlias` |
| G11 checkout 守卫 | 🟢 | `checkout_guard.json` / `checkout_ok.json` |
| G11 tetris smoke | 🟢 | `examples/tetris/tests/move_left.json`（onEnter restart 后 moveLeft） |
| G11 步骤后排空 onEnter / 刷新 computed | 🟢 | `drainPendingEnter` 在每步之后 |

## 第六轮（2026-08-19）

把 `qorm test` 从「能跑 counter/todo add」推进到「能测 list 行内 handler + computed 断言」。

| 目标 | 状态 | 证据 |
|------|------|------|
| G10 todo 行内 Done + computed.openCount | 🟢 | `examples/todo/tests/toggle.json`；`state_equals` 支持 `computed.*` |
| G10 counter 纳入示例回归 | 🟢 | `TestRunExampleAppsWithTests` |

## 第五轮（2026-08-19）

第四轮故意留下的可验证性缺口：组件物化、list item scope、loader 错误被 test/check 丢掉。

| 目标 | 状态 | 证据 |
|------|------|------|
| G7 JSON 组件物化 + item scope | 🟢 | `TestMaterializeExpandsJSONComponents`；`TestSimulateEventUsesListItemScope`；`examples/uikit/tests` |
| G8 `qorm test` / `qorm check` 拒绝 error 诊断 | 🟢 | `TestRunRefusesErrorDiagnostics`；`TestCmdCheckRefusesErrorDiagnostics` |
| G9 对比表诚实 | 🟢 | README 小程序=静态 WXML；MCP=25 工具 |

未做：GTK 接线、Error Boundary、新 widget。

## 第四轮（2026-08-19）

对照 v0.9.4 的目标/实现缺口，收口 agent 可验证性，不扩 widget。

| 目标 | 状态 | 证据 |
|------|------|------|
| G1 canvas `hostLimits` + MCP contrast 文案 | 🟢 | `TestCollectMeasureHostLimits`；mcp-tools en+zh 再生 |
| G2 list 物化 + todo/derived `qorm test` | 🟢 | `TestMaterializeExpandsListRenderItem`；`TestRunExampleTodoAndDerived` |
| G3 文档对齐 | 🟢 | README 架构图、security-model、SKILL 冻结 widget |
| G4 Linux chromeless 矩阵注释 | 🟢 | support-matrix en+zh |
| G5 LAN 观察窗 loopback 门禁 | 🟢 | `TestLANObservationWindowLoopbackOnly` |
| G6 `qorm package --revoked` | 🟢 | `TestPackageUpdateFlagPairing` + `TestOfflineHTMLWithRevokedUpdateConfig` |

详见 [analysis-and-plan.md](analysis-and-plan.md)。未做：GTK 接线、Error Boundary、新 widget。

## 窗口建设完善轮（2026-08-13）

用户反馈两点：(1) `qorm.config.json` 在 MCP 与 SKILL 文档里完全没有说明；
(2) 窗口部分建设未完善 —— 异形窗口、窗口大小这类配置应统一收进 `qorm.config.json`。
本轮把窗口配置补齐并落到文档：

| 变更 | 内容 |
|------|------|
| `qorm.config.json` 成为窗口配置的家 | 新增 `window` 块（width/height/title/resizable/chromeless/transparent/hideLog/hideTray），`display` 块保留为兼容别名；优先级 config > platforms.desktop.window > 顶层 display；坏 JSON / 未知键改为出诊断而非静默吞掉 |
| `resizable` 真正生效 | 此前只解析不消费。macOS WebView style mask、非 macOS 走 webview `HintFixed`、canvas 窗口 style mask 均按 `resizable:false` 锁尺寸；新增 `Window.Fixed` 区分"显式 false"与"未声明" |
| `title` 真正生效 | 窗口标题此前从不消费；现在声明的 window title 优先于 app name 作为窗口标题 |
| 异形窗口（chromeless+transparent） | macOS WebView 已完整；Windows 补 chromeless（user32 去装饰）；Linux/BSD 解析但去装饰未接线（如实记录）；canvas 宿主不消费 chrome 标志（如实记录） |
| MCP 可见 | `qorm_inspect` 输出新增 `window`（解析后的最终窗口配置）；mcp-tools.md（en+zh）新增"宿主窗口"引言段 + qorm_inspect 描述更新，生成器测试守护 |
| 文档补齐 | SKILL.md 可运行格式节 + docs/agent/skills.md（en+zh）+ docs/project-structure.md（en+zh）新增 qorm.config.json 专节与优先级表 + AGENTS.md；`examples/mario` 转用新 `window` 块；纠正 project-structure 示例中从未存在的 `window.icon` 键；删除 model.DisplaySpec 死代码 |

### 对抗复审 + CI 修复波（同日，`4e7a103` 的后续）

对抗复审（3 项确认缺陷）+ CI 桌面矩阵（ubuntu/windows 本地无法 cgo 编译的盲区）共同暴露的问题，全部根因修复：

| 问题 | 修复 |
|------|------|
| Windows：chromeless 无条件剥掉 WS_THICKFRAME → `resizable` 被静默忽略 | `setUndecorated(hwnd, resizable)`：只剥 WS_CAPTION，resizable 时保留调整框（它正是 Windows 上用户可调整大小的位） |
| Windows：chromeless 内容区比声明尺寸大 ~16×39px（SetSize 先按带装饰风格算外框） | 去装饰挪到 SetSize **之前**，框架几何按无装饰风格计算 |
| macOS：陈旧 window.txt 恢复覆盖 fixed 窗口尺寸并锁死（持久化循环反复回写，永不自愈） | 仅 resizable 窗口恢复记忆框架；fixed 窗口以声明尺寸为准 |
| CI 编译错：`hint := webview.HintNone` 推断为 int（cgo 无类型常量）不匹配 `webview.Hint`；`uintptr(-16)` 常量溢出 | 显式 `webview.Hint(webview.HintNone)`；负数 GWL_STYLE 经运行时变量转换 |
| config `"width": 0` 无法把清单尺寸覆盖回流式（与"WINS + 0=fluid"文档矛盾） | `applyConfigWindow` 按键在场即生效（≥0），新增测试锁定；另加 per-key 合并语义测试 |
| CI build·vet·test 红：`TestStdoutSinkSFXDoesNotReplaceMusic` 竞态 | 根因不是测试而是 sink：loop 音乐靠"退出即重生"实现，无音频设备的 runner 上 aplay 秒退 → 重生风暴替换 `sink.music`（生产中即无限失败进程风暴）。加重生护栏：存活 <500ms 判为 sink 损坏不再重生；测试在音乐进程秒退时如实 skip |
| README 携带过期版本信息 | 按用户要求移除 README.md / README.zh.md 的 "What's new in 0.8.11" 整节 —— README 不再放版本信息 |
| 验证手段 | 本机 mingw-w64 交叉编译 `-tags desktop` Windows 构建通过（本地首次可验证该路径）；全门禁绿（34 包 + race + 三构建 + linux/windows 纯构建） |

## v0.9.1 发布后维护轮（2026-08-13）

CI 变红触发排查，两个失败都是真实缺陷，按根因修复而非移除 job：

| 提交 | 修复 |
|------|------|
| `b6460db` | cross-compile 失败根因：darwin 视频解码器是 cgo 文件，`CGO_ENABLED=0` 交叉编译时被排除，而纯 Go fallback 的 tag 又排除了 darwin → `startVideoDecoder` 无定义。同一 bug 已导致 release.yml 失败（**v0.9.1 GitHub Release 无二进制产物**）。重排两个文件的 build tag；另 TestRaidenPerf 在 -race 下仅超性能预算（0 处数据竞争），加 `race` build tag 跳过预算、保留逻辑断言 |
| `ae6e98f` | genicons 生成器静默失败：写盘用仓库相对路径但 go:generate 在包目录执行，错误被吞 → 6 个马里奥图标（brick/coin/flag/goomba/ground/mario）从未进入 canvas 位图字体。加 `-o` flag + 写失败即报错；重新生成 47 → 53 字形，与 `widgets.IconSet()` 完全一致 |
| `39a2995` | **SKILL/MCP/文档/API 完全同步**：skills.md 记载了 7 个从未存在的技能文件 → 重写为真实的单一 SKILL.md（含 4 条工作流）；图标数 66 → 53、组件数 80+ → 146、样式键 ~90 → 108（SKILL.md / claude-pack.md / AGENTS.md 三处）；doc.go 中文版补 qorm_validate 缺失译文、修复 qorm_query 半角病句、补齐 activity/capabilities 漏句；生成的 mcp-tools.md（en+zh）新增「工具分类」+「示例」两节，由 TestMCPDocInSync 永久守护。审计：四个面（mcp-tools en+zh、SKILL.md、integrations/README.md）均为 25/25 工具，零多余零遗漏 |

遗留已闭环：v0.9.1 tag 的 release workflow 在修复前失败、tag 不可移动 —— 由 **v0.9.2** 补齐（release 工作流绿，6 平台二进制 + SHA256SUMS 全部产出）。

## 当前状态

| 项 | 状态 | 证据 |
|----|------|------|
| **v0.9.2 发布** | 🟢 | tag `v0.9.2` → `7f938bb` · main push 对齐 · release 工作流绿（6 平台二进制 + SHA256SUMS）· 官网 deploy（v0.9.2 戳记 + 首页 200） |
| G0 基线修复 | 🟢 | `0fd604d` 删除 62df1ca 残留 11 文件；`go build ./...` 恢复 |
| G1 测试基线 | 🟢 | path widget 全栈落地 · video 测试修正 · 文档生成器手写段保护 |
| G2 安全残留 | 🟢 | 两 token LAN 门禁 · computed 动态 key 拒载 · WASM OTA 吊销 · bundle 版本提示 |
| G3 `qorm test` MVP | 🟢 | Phase 9 spec 落地；examples/counter 4/4；exit 0/1 语义实测 |
| 对抗复审 | 🟢 | 三轮闭环：首轮 9 缺陷 → 修复波 → 二轮 2 新 P1 → 主控直修 → 三轮双镜 pass（见 audit-log） |
| 全量回归 | 🟢 | `go test ./...` 全绿 · -race 关键包绿 · WASM 构建绿 · gofmt 净 |

## 第三轮交付（main，本地未推）

### 实施波（3 分支 worktree 并行，rebase 后合并）

| 分支 | 内容 |
|------|------|
| r3-testfix | path widget（canvas 光栅 + HTML `<svg><path>`，M/L/H/V/Q/T/S/Z 绝对+相对）；canvas-advanced qscript 尾随 `;`；video measure 期望修正；文档生成器手写段并入生成源 |
| r3-security | `--lan` token 门禁 · computed 动态 key 加载错误 · WASM OTA 吊销快照 · bundle 版本提示 |
| r3-qormtest | `internal/testrunner` + `qorm test` CLI + counter 4 份测试文档 |

### 修复波（对抗复审驱动，3 分支 worktree 并行）

| 分支 | 修复 |
|------|------|
| r3-fix-canvas | 解析器前进保证（弧/Z 后数字/1e309 不再死循环；弧按弦近似）；Measure 改原点锚定框（bbox max），偏移路径不再被裁剪 |
| r3-fix-gate | 门禁密钥与页面 token 解耦：admin token 从不出现在任何响应页面；诊断读面（/dev/state /log /presence GET）纳入门禁；启动提示措辞纠正 |
| r3-fix-testrunner | onEnter 运行时错误 → `test_runtime_error` 失败；`-` 前缀参数明确拒绝；发现规则/诊断快照/--target/--report 边界在 doc.go 与 spec 对齐 |

### 主控直修（二/三轮复审缺陷）

| 提交 | 修复 |
|------|------|
| `db3455d` | 读门禁键于"除 POST 外全部方法"（动词绕过 P1）；runtime `EnterScriptError` 链内首错累积器（enter 链吞错 P1） |
| `5e4e2c0` | `MarkPendingEnter`：mount_scene 步骤真正触发目标 onEnter（复审链镜发现的已交付功能洞） |

## 已知债（记录在案，不在本轮）

| 债 | 级别 | 说明 |
|----|------|------|
| `/logwindow` `/console` 页面在 --lan 无鉴权 | P2 | 纳入门禁会破坏 AGENTS.md 的人类观察窗约定，待产品决策 |
| POST /dev/state（DevTool 写）仅页面 token | P2 | 设计如此；持有页面 token 的 LAN 对端可写状态 |
| /poll /events 无 token 帧流 | P2 | EventSource 无法带 header；载荷即 GET / 已公开的 UI |
| /dev/tree /dev/canvas 无鉴权读 | P2 | DevTools 面，待 LAN 加固轮统一处理 |
| `qorm package --revoked` 缺失 | P2 | 吊销快照无 CLI 注入路径，需手工内置 |
| DevTool 页在 --lan 读面 401 降级 | P3 | 无 admin token 输入口，待产品决策 |
| `qorm check` 不诊断 computed 动态 key | P3 | 加载已拒，check 联动待补 |
| onEnter 悬空 action 引用静默 no-op | P3 | loader 不校验引用，测试作者笔误会绿跑 |
| 弧 → 贝塞尔展平 | P3 | 当前按弦近似，曲线呈折线 |
| token 比较非常数时间 | P3 | == 比较；dev 工具定位可接受，记录在案 |

## 发布

| 项 | 状态 |
|----|------|
| 上一版 | **v0.9.1**（2026-08-13）：qorm test MVP · path widget · LAN 双 token 门禁（**GitHub Release 无二进制产物**——release.yml 在 cross-compile 修复前失败，tag 不可移动） |
| 本版 | **v0.9.2**（2026-08-13）：changelog 归档 `014d5fc` · version bump `7f938bb` · annotated tag · main+tag push · **release 工作流绿，6 平台二进制 + SHA256SUMS 补齐** · 官网 deploy（wasm 同源重建防漂移 · 109 页 · sitemap 104 · 首页 v0.9.2 戳记 200） |

## 链接

- 仓库：https://github.com/qorm/platform
- 官网：https://qorm.com/
- Docs：https://qorm.com/docs/
- Games：https://qorm.com/games/

## 变更日志

| 时间 | 事件 |
|------|------|
| 2026-08-13 | v0.9.0 CHANGELOG 归档（video widget、canvas-advanced、measure/shot、live capture 等） |
| 2026-08-13 | 第三轮分析：4 路并行（回归/安全/文档/方向）→ 规划 analysis-and-plan.md |
| 2026-08-13 | G0 基线修复（62df1ca 残留清理）+ 文档同步（README/Roadmap/zh） |
| 2026-08-13 | 实施波：r3-testfix / r3-security / r3-qormtest 并行落地并合并 |
| 2026-08-13 | 对抗复审一轮：3 镜验证，9 项确认缺陷（正确性 2 / 安全 3 / 规格 4） |
| 2026-08-13 | 修复波：r3-fix-canvas / r3-fix-gate / r3-fix-testrunner 并行落地并合并 |
| 2026-08-13 | 对抗复审二轮：canvas pass（3.2M 模糊）；gate/spec 各 1 新 P1（动词绕过 / enter 链吞错） |
| 2026-08-13 | 主控直修两轮 P1 + 链镜发现的 mount_scene 空转洞（`MarkPendingEnter`） |
| 2026-08-13 | 对抗复审三轮：双镜 pass；残留债全部归档 audit-log |
| 2026-08-13 | 全量门禁绿：build · test · race · wasm · coverage gate · `qorm test` 4/4 |
| 2026-08-13 | **v0.9.1 发布**：changelog 归档 + version bump + annotated tag + push + 官网部署 |
| 2026-08-13 | 窗口建设完善轮：qorm.config.json `window` 块 + resizable/title 落地 + MCP inspect 可见 + SKILL/文档补齐 |
| 2026-08-13 | 对抗复审 + CI 修复波：Windows chromeless×resizable/尺寸几何、macOS fixed 窗口状态恢复、两处编译错、config 0=fluid 覆盖 |
| 2026-08-13 | CI build·vet·test 根因修复（audio sink 重生风暴护栏）+ README 去版本信息（en+zh） |
| 2026-08-13 | **v0.9.2 发布**：changelog 归档 + bump + annotated tag + push；release 工作流绿（6 平台二进制补齐 v0.9.1 缺失）；官网 deploy 并验证首页 v0.9.2 |
