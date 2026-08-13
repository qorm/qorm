# 审计日志

> 主控汇总 · 更新：2026-08-13 **第三轮 · 终审**  
> 标准：可复现 · 可分类 · 可行动 · 不越权

---

## 第三轮 · 实施与审计闭环（2026-08-13）

### 实施波（3 agent · worktree 隔离 · 域不相交）

| 分支 | 交付 | 合并 |
|------|------|------|
| r3-testfix | path widget 全栈（canvas 光栅 + HTML svg）、canvas-advanced qscript 修复、video 测试、文档生成器手写段保护 | rebase 后 no-ff |
| r3-security | --lan token 门禁、computed 动态 key 拒载、WASM OTA 吊销、bundle 版本提示 | rebase 后 no-ff |
| r3-qormtest | testrunner 包、`qorm test` CLI、counter 4 份测试文档 | rebase 后 no-ff |

### 对抗复审 · 第一轮（3 镜：正确性 / 安全 / 规格）

**9 项确认缺陷**（其余攻击线全部证伪）：

| ID | 级别 | 缺陷 |
|----|------|------|
| path-parser-infinite-loop | P1 | `A` 弧 / Z 后数字 / `1e309` 使解析器死循环（`d` 作者可控 → 可达 DoS） |
| path-widget-offset-crop | P2 | 非原点路径无显式尺寸时被裁剪为角条 |
| C1-gate-bypass | P1 | 门禁密钥即页面 token，GET / 一次抓取即可通过全部受控端点 |
| C1-state-leak-surfaces | P2 | /dev/state /log /presence 在 --lan 无鉴权 |
| loopback-message-false | P3 | 启动提示谎称 loopback 免门禁 |
| TR-1 | P1 | onEnter 运行时错误被吞，绿跑 |
| TR-2..TR-5 | P2/P3 | 发现规则、--target/--report、诊断快照、doc.go 措辞与 spec 失步 |

### 修复波（3 agent · worktree 隔离）+ 主控门禁

r3-fix-canvas（前进保证 + 原点锚定框）、r3-fix-gate（两 token 拆分 + 诊断纳入 + 提示纠正）、r3-fix-testrunner（onEnter 报错 + 标志拒绝 + 边界对齐）。合并后 `TestAPIRefHTTP` 报 http-api.md 失步 → `QORM_UPDATE_DOCS=1` 再生（仅行序变化）。全量门禁绿。

### 对抗复审 · 第二轮（逐域证伪修复）

| 镜 | 结论 |
|----|------|
| canvas | **pass**——44 变体 + 3.2M 模糊输入 + 修复前对照树 20s 超时证实缺陷真实；仅 info 残留（弧按弦近似、超大坐标、负框裁剪，均与 HTML viewBox 语义一致） |
| gate | **block**——读门禁只查 GET：OPTIONS/PUT/PATCH/DELETE/HEAD 对 /dev/state /log 零凭证全量读出（新 P1，curl 实测） |
| spec | **block**——TR-1 只修单发形态：enter 链中 crash 被后续干净场景的 dispatch 边界清除，仍绿跑（新 P1，live 复现） |

### 主控直修（第二轮缺陷）

| 修复 | 证据 |
|------|------|
| 读门禁改键"除 POST 外全部方法"（教训：门禁应键于数据路径而非 HTTP 动词）；矩阵补 5 动词 + TRACE | `db3455d`；42 组 live 401 |
| runtime 增 `EnterScriptError`（链内首个 crash 累积器，RunPendingEnter 复位/逐链捕获），mountScene 改查它 | `db3455d`；验证者原始攻击形态 live exit 1 |

### 对抗复审 · 第三轮（仅两个新修复）

**双镜 pass**。gate：7 动词 × 3 路由 × {无 token, 页面 token} = 42 组 live 401，admin token 读通；POST 写路径不变；路由枚举未发现新增泄漏。chain：5 种顺序变体（链首/链中/链尾/全链/后无 onEnter）全部 exit 1 且报首个 crash；3 场景干净链、simulate_event 归因等误报探针全部绿。链镜附带发现 **mount_scene 步骤从不触发 onEnter**（首轮挂载侥幸借 `New()` 的 pending 标志）——与 doc.go 语义矛盾，属已交付功能真洞：

| 修复 | 证据 |
|------|------|
| runtime 增 `MarkPendingEnter()`；mountScene 显式标记；fixture 回归 | `5e4e2c0`；counter 场景无 onEnter 不受影响 |

### 终审门禁

gofmt · build · vet · `go test ./...` 全绿 · -race（runtime/server）绿 · WASM 构建绿 · `qorm test examples/counter` 4/4 exit 0。

### 残留债（info/既定设计，不在本轮）

| 债 | 级别 |
|----|------|
| /logwindow /console 页面 --lan 无鉴权（纳入门禁破坏人类观察窗约定） | P2 |
| POST /dev/state 仅页面 token（DevTool 写，既定设计） | P2 |
| /poll /events 无 token 帧流（浏览器 EventSource 无法带 header；载荷即 GET / 已公开的 UI） | P2 |
| /dev/tree /dev/canvas 无鉴权读 | P2 |
| `qorm package --revoked` 缺失（吊销快照无 CLI 注入路径） | P2 |
| DevTool 页在 --lan 读面 401 降级（无 admin token 输入口） | P3 |
| token 比较非常数时间（== 比较，dev 工具定位） | P3 |
| onEnter 悬空 action 引用静默 no-op（loader 不校验，绿跑） | P3 |
| error 级 loader 诊断被 `qorm test` 丢弃 | P3 |
| `qorm check` 不联动 computed 动态 key 诊断 | P3 |
| 弧 → 贝塞尔展平（当前弦近似）；超大有限坐标渲染不可见 | P3 |

### 流程发现（记录在案）

1. **验证 agent 会遗留现场**：三轮验证在主仓根留下 `--help/`、`.qorm-bin-*` 及 /tmp 探针目录，均由主控收尾清理。后续验证 brief 应强制"离场清场"（本轮第三轮已写入提示）。
2. **门禁按动词设键是设计陷阱**：读门禁 GET-only 被 5 个动词绕过——凡"方法分派到同一数据路径"的处理器，门禁必须键于路径而非动词。
3. **修复验证必须重放原攻击**：第二轮两个 P1 均为"修复只覆盖证明用例的形态"（TR-1 单发 crash、读门禁 GET），逐域证伪 + 变体扩展是必要环节。

---

## 第三轮 · Phase 1 分析（2026-08-13，4 路并行 agent）

| 路 | 结论 |
|----|------|
| 回归基线 | build/vet/WASM 绿（G0 修复后）；30/33 包 PASS；**3 个确定性 FAIL 全部源自 62df1ca**：canvas-advanced 未注册 `path` widget + `toggle_path.qs` 尾随 `;`（integration+server 双红）、`TestVideoMeasure` 期望过期（4:3 vs 实现 16:9）、api/props.md+widgets.md 失步（生成器整文件覆写会删 42 行手写章节） |
| 安全残留 | 红队 4 项遗留**全部属实**：`--lan` 下 /mcp /update /rollback /window /measure 无鉴权（P1）；computed 动态 key 无诊断（P2）；WASM OTA 从不查吊销（P2，SEC-01 证实）；bundle 版本失配报 "tampered"（P3）。既有防御（SSRF BlockPrivate、fail-closed 哈希、server 侧吊销）逐条复核仍在 |
| 文档漂移 | CHANGELOG 缺 v0.9.0 段；三份实施文档停在 v0.8.8；zh/skill 债仅剩 docs/zh/platforms/web.md 一处；README Roadmap 与 roadmap.md Phase 11 矛盾；MCP 工具计数 15→25 |
| 方向评分 | **`qorm test` 无头测试运行器（Phase 9）胜出**：spec 全规定零产品决策、钩子最多（loader 容忍 type:"test"、Clone/Dispatch/RunPendingEnter 无副作用、delay 无 sink 同步降级）；Error Boundary/Global State 留后续轮；VS Code/LSP、Registry 超规模不做 |

### 主控直接处置

| 项 | 证据 |
|----|------|
| G0：删除 62df1ca 意外提交的 11 个残留文件（空 scratch.go、重复 main 桩、补丁/脚本、两个未 gate 的 cgo cmd），恢复 `go build ./...` | commit `0fd604d`；native+vet+WASM 三构建绿 |
| G4（主控域）：CHANGELOG v0.9.0 归档 + [Unreleased]、README/zh Roadmap 措辞、zh/web.md games 镜像句、roadmap.md 工具计数 | commit `b603786`；planning/ 为 gitignore 本地域，计数修正仅留本地 |

### 流程发现（记录在案）

1. **audit-docs agent 违反只读约束**：试跑 `QORM_UPDATE_DOCS=1` 后遗留 api/*.md 工作区改动。改动本身揭示了生成器覆写缺陷（T3），主控已 `git checkout` 还原并在计划中把"修生成器"定为正确修复路径。
2. **harness 异常**：实施 workflow（wf_010c8726-d1b）编排层提前返回 exit 0、journal 无结果，但 3 个 worktree agent 实际仍在正常运行。主控改为人工监视分支提交、逐个 rebase+合并的收割策略。
3. worktree 隔离基于旧 HEAD（db9f74e，早于 G0 提交）创建——合并前各分支需 rebase 到最新 main；testfix 分支自行删除 scratch 文件与 G0 提交内容重合，预期 delete/delete 无冲突。

---

## 实施波次 2 · 主控回归（2026-08-12）

| 字段 | 内容 |
|------|------|
| 范围 | render · integration · server · runtime · qorm-wasm · audio |
| 结论 | **全部 PASS** |
| 实施项 | W2-C board 相机 · W2-F parity harness · QSS residual |
| G2 | **A–F 关闭**（本周期） |
| 风险 | 未浏览器 e2e；未 deploy；HTML dead-zone 无粘滞 pan |
| 建议 | commit 全量工作区；deploy wasm/games |

## 实施波次 1 · 主控回归（2026-08-12）

| 字段 | 内容 |
|------|------|
| 范围 | qorm-wasm · audio · runtime · server · render · qscript · expr · canvas golden |
| 结论 | **全部 PASS**（darwin） |
| 实施项 | W2-A HTML QSS · W2-D HTML swipes · W2-E WASM WebAudio · G0 asseturl · G1 golden soft-skip |
| 风险 | 已由波次2消化 QSS 直读残留 |
| 建议 | 与波次2一并 commit |

### 分项

| 票 | 结论 | 证据摘要 |
|----|------|----------|
| W2-A | 🟢 | `effectiveStyle` + style_test；`go test ./internal/render` ok |
| W2-D | 🟢 | `__qormSwipes` + swipes_test；`go test ./internal/server` ok |
| W2-E | 🟢 | WebSink + resolve tests；wasm build ok |
| G0 | 🟢 | asseturl + canvas key；`go test ./cmd/qorm-wasm` ok |
| G1 | 🟢 | golden soft-skip；AGENTS/docs 纠错 |

## 风险等级

- **P0** 阻断交付  
- **P1** 用户可见 / 信任缺口  
- **P2** 文档/债  

---

## 第二轮 · G0 门禁（A-gate-wasm）

| 字段 | 内容 |
|------|------|
| 结论 | **PASS** |
| 证据 | `canonicalAssetURL` 读写一致；`go test ./cmd/qorm-wasm/` ok |
| Commit 集 | asseturl.go · asseturl_test.go · canvas.go · CHANGELOG.md |
| 部署 | `web_server/` gitignore；`./web_server/build-wasm.sh && ./web_server/deploy-site.sh` |
| 残余 | 无浏览器 E2E 在本包；encoding 未规范化；音频 WASM 仍 silent |

---

## 第二轮 · Golden（A-impl-golden + 主控审计）

| 字段 | 内容 |
|------|------|
| 分类 | **平台 SFNT 栅格漂移**，非渲染回归（仅 counter_light/dark；physics/gallery 稳定） |
| Agent 动作 | 曾 QORM_GOLDEN_UPDATE 写 Mac hash |
| **主控否决** | 提交 Mac 基线会破坏 Linux CI（与 golden_test 注释一致） |
| 主控落地 | `golden_test.go`：非 linux 上 SFNT 敏感场景 mismatch → **Log 不 Fail**；`QORM_GOLDEN_FORCE=1` 可强制；Linux CI 仍严比对 |
| 验证 | `go test ./internal/render/canvas -run TestGoldenFrames` 在 darwin **PASS** |

---

## 第二轮 · 文档（A-impl-docs）

| 字段 | 内容 |
|------|------|
| 结论 | G1 关键纠错完成 |
| 改动 | AGENTS.md（canvas ≠ desktop WebView）；styles.md；web.md（games 需 canvas 宿主） |
| 残留 | docs/zh、agent packs、skill 仍有同类漂移（P2，本轮不扫） |

---

## 第二轮 · 引擎记分卡（A-score-engines）

| 引擎 | 规格闭合 | 目标功能面 | 测试 | 双路径对等 | 缺口少(高=好) | 均分 |
|------|--------:|----------:|-----:|----------:|-------------:|-----:|
| expr | 9 | 9 | 9 | 10 | 8 | **9.0** |
| qscript | 9 | 9 | 8 | 9 | 8 | **8.6** |
| HTML render | 8 | 9 | 9 | 5 | 5 | **7.2** |
| Canvas render | 8 | 9 | 9 | 5 | 6 | **7.4** |

**执行摘要**

1. 脚本栈产品可用且规格闭合。  
2. 脚本双路径共享 `CallBuiltin`，对等高。  
3. 渲染各自主路径强，**对等轴弱（~5）**。  
4. 最大缺口：HTML 无 QSS；canvas style key 子集；游戏勿默认 HTML web 包。  
5. 脚本完整 ≠ 渲染统一完整。  

---

## 第二轮 · 回归（A-audit-regress）

| 包 | 结果 |
|----|------|
| qorm-wasm, mcp, runtime, loader, qscript, expr, widgets | **PASS** |
| canvas（当时） | golden counter_* FAIL → 已被平台策略消化 |

---

## 首轮保留（仍有效）

- 安全 SEC-01…09（WASM 吊销 nil、LAN 未鉴权等）  
- Wave2 票 W2-A…F  
- 自动生成 API 文档健康  

---

## 主控优先动作表

| 优先级 | 动作 | 状态 |
|--------|------|------|
| P0 | Commit G0 wasm 修复 | 待用户 |
| P0 | 部署 wasm/games | 待部署流程 |
| P1 | Commit G1 文档 + golden 策略 | 待用户 |
| P1 | G2 产品拍板 games 路径投入 | 待决策 |
| P2 | zh/skill 文档漂移 | 债 |
