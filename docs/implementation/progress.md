# 实施进度

> 主控同步 · 更新：2026-08-13 **v0.9.1 已发布 · 发布后维护轮完成**

## v0.9.1 发布后维护轮（2026-08-13）

CI 变红触发排查，两个失败都是真实缺陷，按根因修复而非移除 job：

| 提交 | 修复 |
|------|------|
| `b6460db` | cross-compile 失败根因：darwin 视频解码器是 cgo 文件，`CGO_ENABLED=0` 交叉编译时被排除，而纯 Go fallback 的 tag 又排除了 darwin → `startVideoDecoder` 无定义。同一 bug 已导致 release.yml 失败（**v0.9.1 GitHub Release 无二进制产物**）。重排两个文件的 build tag；另 TestRaidenPerf 在 -race 下仅超性能预算（0 处数据竞争），加 `race` build tag 跳过预算、保留逻辑断言 |
| `ae6e98f` | genicons 生成器静默失败：写盘用仓库相对路径但 go:generate 在包目录执行，错误被吞 → 6 个马里奥图标（brick/coin/flag/goomba/ground/mario）从未进入 canvas 位图字体。加 `-o` flag + 写失败即报错；重新生成 47 → 53 字形，与 `widgets.IconSet()` 完全一致 |
| `39a2995` | **SKILL/MCP/文档/API 完全同步**：skills.md 记载了 7 个从未存在的技能文件 → 重写为真实的单一 SKILL.md（含 4 条工作流）；图标数 66 → 53、组件数 80+ → 146、样式键 ~90 → 108（SKILL.md / claude-pack.md / AGENTS.md 三处）；doc.go 中文版补 qorm_validate 缺失译文、修复 qorm_query 半角病句、补齐 activity/capabilities 漏句；生成的 mcp-tools.md（en+zh）新增「工具分类」+「示例」两节，由 TestMCPDocInSync 永久守护。审计：四个面（mcp-tools en+zh、SKILL.md、integrations/README.md）均为 25/25 工具，零多余零遗漏 |

遗留待决：v0.9.1 tag 的 release workflow 在修复前已失败，tag 不可移动 —— GitHub Release 的二进制产物需等下一个 tag（如 v0.9.2）或接受缺失。

## 当前状态

| 项 | 状态 | 证据 |
|----|------|------|
| **v0.9.1 发布** | 🟢 | tag `v0.9.1` → `347e07a` · main push 对齐 · 官网 deploy（v0.9.1 戳记 + 首页 200） |
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
| 上一版 | **v0.9.0**（2026-08-13 早段 tag，canvas-ultimate） |
| 本版 | **v0.9.1**（2026-08-13）：changelog 归档 `9b0e643` · version bump `347e07a` · annotated tag · main+tag push · 官网 deploy（wasm 同源重建防漂移 · 109 页 · sitemap 104） |

## 链接

- 仓库：https://github.com/qorm/qorm
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
