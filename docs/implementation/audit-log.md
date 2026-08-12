# 审计日志

> 主控汇总 · 更新：2026-08-12 **实施波次**  
> 标准：可复现 · 可分类 · 可行动 · 不越权

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
