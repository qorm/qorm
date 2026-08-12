# QORM 整体分析与目标实施规划

> 主控文档 · 2026-08-12 第二轮 · 以代码与 `examples/` 为准

## 1. 项目定位（事实）

QORM = **Agent-native 跨平台应用运行时**：JSON 描述 UI + 纯 Go 运行时 + MCP 协作 + ed25519 OTA。

| 维度 | 结论 |
|------|------|
| 核心路径 | loader → model → runtime (expr/qscript) → **HTML 或 canvas 渲染** → server/MCP/bundle |
| 规模 | ~13 万行 Go · 38+ examples · MCP 24 tools · 双（三）渲染宿主 |
| 差异化 | 人机同 live runtime · 签名 OTA · measure/check 几何自验证 |
| 脚本引擎 | **规格内完整**（expr+qscript；游戏 examples 已证） |
| 渲染引擎 | **主路径可用，双路径未对等**（QSS/swipes/相机/音频偏 canvas） |

## 2. 目标分层（本周期「完成」的定义）

### G0 — 交付闭环（本周必须）

使 **Web Canvas Games（Raiden/Mario）** 与 **仓库主线质量信号** 可复现、可合入、可部署。

成功标准：

1. `cmd/qorm-wasm` asseturl 修复在跟踪源中且 `go test ./cmd/qorm-wasm/` 绿  
2. 本地/预发 games 页 Raiden+Mario 有图、可玩（`preloaded_count>0`，`graph_imgs>0`）  
3. 实施进度与审计文档与仓库状态一致  

### G1 — 质量可信（本周力争）

1. `TestGoldenFrames` counter_* 有结论：刷新或修复，**不留无解释红灯**  
2. 关键文档纠错：canvas ≠ `-tags desktop`（AGENTS / styles 至少一处权威表述）  
3. 产品一句话：**游戏默认 canvas 宿主**；`package -p web` 是 HTML 路径  

### G2 — 引擎对等（下一周期，不阻塞 G0）

按优先级：HTML QSS → 产品打包档 → board 相机 → swipes → WebAudio → widget harness  

### G3 — 安全硬化（下一周期）

SEC-01 WASM 吊销 · SEC-04 LAN 鉴权 · SEC-02/03 审批模型（产品拍板后）  

## 3. 架构快照

```
App JSON / .qs / .qss
    → loader → model.App
    → runtime (state · expr · qscript · steps)
    → ┌─ HTML/CSS (browser / WebView / package -p web)
       └─ Canvas software (macOS 默认窗 · games WASM + qorm_canvas)
    → server SSE/MCP/OTA · bundle sign
```

| 子系统 | 成熟度 | 备注 |
|--------|--------|------|
| Loader/Runtime/Expr/QScript | 高 | 脚本规格闭合、有治理与测试 |
| MCP | 高 | 24 tools；preview→apply 门闩 |
| HTML 渲染 | 高 | 业务主路径；无 QSS cascade |
| Canvas 渲染 | 高（有债） | 游戏主路径；golden 当前红 |
| WASM games 资源加载 | 修复在途 | key 规范化；待合入部署 |
| Desktop Linux/Windows | beta | — |

## 4. 本轮 Agent 分工（执行 + 审计）

| Agent | 目标 | 模式 | 产出 |
|-------|------|------|------|
| A-impl-golden | G1：处理 canvas golden 失败 | 读+写 | 刷新或修根因 + 测试绿 |
| A-impl-docs | G1：纠错 canvas/desktop 与 games 路径 | 读+写 | AGENTS/styles 等最小 diff |
| A-gate-wasm | G0：再验证 wasm 修复与合入清单 | 执行测试 | 审计条目 + 确认 commit 集 |
| A-score-engines | G0/G1：脚本/渲染完整度记分卡 | 只读 | 写入审计的正式评分 |
| A-audit-regress | G1：全量关键包回归快照 | 执行测试 | 新失败列表 |

主控：合并 → 更新 `progress.md` / `audit-log.md` → 标出阻塞与决策。

## 5. 审计门槛

- 声称完成必须附命令与退出码摘要  
- 区分 **事实 / 假设 / 产品决策**  
- 不扩大范围（禁止无关重构）  
- 密钥与 exploit 禁区保持  
- 每波结束同步进度文档，不静默丢项  

## 6. 非目标（本轮）

- HTML 全量 QSS 实现（G2）  
- VS Code/LSP  
- GPU 渲染  
- 未授权 commit 到 origin（合入由用户确认后主控执行）  

## 7. 与首轮规划的关系

首轮 Wave0–3 与安全/文档表仍有效。本轮将目标收束为 **G0/G1 可验证交付**，避免无限审计空转。  
