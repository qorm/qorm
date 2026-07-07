# QORM 竞品对标与规划修正

更新时间：2026-07-03
文档性质：战略分析 / 路线图修正建议
依据：对 `crates/**` 真实代码、依赖、`docs/planning/*` 进度文档的核对（非文档宣称）

---

## 一、核心结论

QORM 的**协议设计与安全边界达到顶尖水准**，但**产品化的"最后一公里"目前几乎全部为 skeleton / in-memory / mock**。

当前最大风险不是能力不足，而是**规划方向失衡**：约 50 个批次（Batch 7A → 7AZ）投入在对一个内存态内核做"诊断冻结 / 证据同步"式的**横向加固**，其边际价值已趋近于零；却始终没有产出**一条端到端真实可跑、可演示、可被采用**的垂直链路。

修正方向一句话：**停止横向加固，改为垂直穿刺（tracer bullet）——选一条链路做到 100% 真实、可演示、可发布，再谈广度。**

---

## 二、真实状态核对（对照文档宣称）

| 维度 | 文档宣称 | 代码实际 | 结论 |
|---|---|---|---|
| 代码规模 | 28 crates | 63.8k 行 Rust，分层干净 | [done] 属实 |
| 渲染 | "GPU-first"（ADR-0004） | 无 `wgpu`/`vello`/`skia`；`render` 产出 Display List；`preview-desktop` 仅 `egui/eframe` skeleton | [no] 无真实 GPU 后端 |
| Web 渲染 | Web Platform Pack | `qorm-web`（DOM + Canvas2D 渲染器）+ `qorm-wasm-bridge`（`render_dom` FFI）**基本完整** | [done] 被低估的资产 |
| 签名 | Bundle 签名 / 信任 / 吊销 | `PlaceholderMetadataAccepted`，无 `ed25519`/`ring` | [no] 占位实现 |
| 网络 / Host | `network.request` 能力 | 无 `reqwest`/真实 transport，全为 Capturing mock | [no] mock |
| DevServer / HMR | 热更新 | CLI 有 `axum`+`tokio`，但真实 watcher/channel 未实现，为 in-process foundation | [warn] 半成品 |
| Mobile OTA | 签名 / 回滚 / 灰度 | FFI stub（测试多，无真机运行时） | [warn] stub |
| MCP / Agent | 13 工具 | catalog 齐全；`explain_node` 仅 snapshot，无真实因果图 | [warn] 部分 |
| VS Code 扩展 | Phase 5 交付 | `extensions/` 仅 `.gitkeep` | [no] 未启动 |
| 官网 / Playground / Registry | Phase 12 | 未启动 | [no] 未启动 |

**判断**：规格与内核扎实，但没有任何一端真实落地。V2 标称 12/12 完成，实为"协议层定义完成"。

---

## 三、竞品坐标系

QORM 横跨四个赛道，每个赛道都有已量产的顶尖产品：

| 赛道 | 顶尖产品 | 它们已量产、QORM 尚缺 |
|---|---|---|
| **JSON 服务端驱动 UI**（最像 QORM） | DivKit（Yandex 开源）、Adaptive Cards（微软）、Airbnb SDUI、Beagle | 三端真实 renderer 已上线、模板系统、schema fallback、生产规模验证 |
| **跨平台声明式 UI** | Flutter（Impeller + Hot Reload + DevTools Inspector）、React Native、SwiftUI / Jetpack Compose | 世界级渲染性能、热重载 + 可视化 Inspector、a11y/i18n、海量组件库 |
| **Rust GPU UI** | Slint（`.slint` + LSP + 实时预览 + interpreter 动态加载）、Xilem/Vello、Makepad、GPUI(Zed) | 真 GPU 渲染（Vello 为当前最强 2D 计算渲染器）、VS Code 实时预览、LSP —— 正是 QORM Phase 5/10 想要却未做的 |
| **AI 生成式 UI** | v0(Vercel)、thesys C1（LLM 直接返回 UI spec，思路等同 QORM）、Builder Visual Copilot、Vercel AI SDK Generative UI | 从"意图 → UI"的**生成**能力，QORM 完全没有 |

---

## 四、QORM 的护城河（应死守）

没有任何一个竞品同时拥有以下三样，这是唯一值得押注的差异化：

1. **可验证的 UI IR + 密码学交付**：canonicalize → hash → signature → rollback。DivKit / Adaptive Cards 都是"信任服务端"，QORM 是"验证 Bundle"。
2. **Agent 安全操作协议**：`preview_patch`（无副作用）→ `apply_patch`（必须绑定 preview 结果）→ approval / resource-scope / rollback。这套权限模型比任何竞品都严谨，是真正的原创。
3. **概念边界纪律**：platform / render-profile / integration-mode 的正交拆分。

更锐利的一句话定位（替代 "Queryable Object Rendering Model"）：

> **一个 Agent 能安全生成与改写、可被密码学验证、可热更新到原生端的 UI 中间表示。**

---

## 五、对照顶尖产品的规格缺口

除"未真实实现"外，规格本身也缺了每个真实 UI 系统都有的东西：

- **无障碍（a11y）树**：spec 完全没有。而 QORM 是"对象化 UI 模型"，a11y 树本应是天然强项。Flutter / SwiftUI / Compose 均为标配。**低成本、高差异化。**
- **i18n / l10n**、**列表虚拟化 / 长列表性能**、**表单 / 输入 / 焦点管理 / 校验**、**图片 / 媒体加载**：真实 App 必需，spec 均未覆盖。
- **生成路径缺失**：QORM 全部围绕"改已有 UI"，但当前 Agent-UI 主战场是"从意图**生成** UI"（thesys C1 / v0）。要么补一条 generation 通道，要么明确放弃并说清边界。
- **命名风险**：QORM ↔ ORM 强冲突，愿景文档需专门澄清"不是数据库 ORM"，构成每次介绍的"定位税"。建议评估副标题或对外传播名。

---

## 六、路线图修正

### 6.1 原则

- **广度冻结**：Batch 7 系列的诊断加固、evidence sync 暂停，只保留阻断性缺陷修复。
- **垂直优先**：以"可演示的端到端链路"为唯一验收单位，而非"某 crate 的测试覆盖"。
- **复用优先于自研**：渲染先走已有 `web`+`wasm-bridge` 出像素；GPU（Vello）留给后续 app/game 性能档。

### 6.2 三条垂直穿刺（按顺序）

**垂直一 · Agent + 实时预览闭环**（证明核心命题）
- 目标画面："Agent 改 QORM 应用 → 秒级看到真实渲染 → 权限拦截危险操作"。
- 复用 `qorm-web` + `qorm-wasm-bridge` 编 wasm，挂到 VS Code webview 出像素；真实 stdio MCP server；真实文件 watcher 驱动重渲染。
- 一条链路同时证明**渲染、Agent、DevServer、安全**四件事。
- 详见 `docs/planning/vertical-slice-1-agent-live-preview-plan.md`。

**垂直二 · 原生端签名 OTA 动态 UI**（商业护城河）
- 市场真空：微软 CodePush 已于 2025 年停服；EAS Update 仅限 RN/Expo；**没有厂商中立、可验证签名、可回滚的"原生 UI OTA"**。这正是 QORM mobile bundle 的天然战场。
- 把 placeholder 签名换成真 `ed25519`；Swift/Kotlin bridge 接真机 runtime；跑通"远程 Bundle → 验签 → 切换 → 失败回滚"。

**垂直三 · a11y 规格 + 生成路径边界**
- 补 a11y 树 spec（低成本高差异化）。
- 明确 QORM 对"生成式 UI"的立场：作为 LLM 的**输出目标格式**（类 thesys C1），而非自己做生成模型。

### 6.3 每条垂直的硬性交付

- 一个**可公开的 Demo**（录屏或在线）。
- 一组**基准数据**（渲染帧时间 / patch 延迟 / bundle 体积）——竞品都有，QORM 一个都没有。
- 一页**对外可读的文档**（非内部批次证据）。

---

## 七、给主控 agent 的执行建议

1. 将"批次成功"的定义从"fmt/test/clippy + QA PASS"升级为"**端到端 Demo 可跑 + 基准达标**"。
2. 暂停 evidence-sync 类文档批次，改为维护**一份对外 Roadmap** + **一份 Demo 清单**。
3. 每个垂直派发时，先派"**穿刺 spike**"（打通即可、允许粗糙），验证可行后再派硬化批次——避免再次陷入"先硬化后落地"的顺序错误。
