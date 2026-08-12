# 实施进度

> 主控同步 · 更新：2026-08-12 **实施波次 2 完成**  
> 规划见 [analysis-and-plan.md](./analysis-and-plan.md) · 审计见 [audit-log.md](./audit-log.md)

## 目标状态

| 目标 | 状态 | 说明 |
|------|------|------|
| **G0** WASM games 修复 | 🟢 | 待 commit + 部署 |
| **G1** 质量/文档/golden | 🟢 | — |
| **G2** 引擎对等 | 🟢 **A–F 本周期关闭** | 见下表 |
| **G3** 安全硬化 | ⬜ | 下一周期 |

## G2 票板

| 票 | 状态 | 证据 |
|----|------|------|
| W2-A HTML QSS | 🟢 | render_style + style_test |
| W2-B games 路径产品句 | 🟢 | AGENTS / web.md |
| W2-C board 相机 HTML | 🟢 | board_camera.go + tests |
| W2-D HTML swipes | 🟢 | app.js + swipes_test |
| W2-E WASM WebAudio | 🟢 | web_js.go + runtime adapter |
| W2-F widget 对等 harness | 🟢 | TestHTMLCanvasWidgetParity PASS |
| QSS 残留直读 | 🟢 | a11y/spacer/chart via effectiveStyle |

## 波次 2 Agent

| Agent | 状态 | 交付 |
|-------|------|------|
| A-impl-camera | 🟢 | HTML cameraTarget 等 pan 数学 |
| A-impl-parity | 🟢 | integration parity harness；30 HTML-only allowlist |
| A-impl-qss-residual | 🟢 | disabled aria、spacer、chart 走 effectiveStyle |
| 主控回归 | 🟢 | 见下 |

## 主控回归（事实）

```text
go test ./internal/render/ ./internal/integration/ ./internal/server/ \
  ./internal/runtime/ ./cmd/qorm-wasm/ ./internal/audio/ -count=1
# 全部 ok（integration ~7.7s）
```

## 待 commit（工作区汇总）

**新增：** asseturl* · audio wasm · board_camera* · swipes_test · widget_parity_test · resolve_web* · web_js · docs/implementation  

**修改：** render_* · server · runtime · canvas golden · CHANGELOG · AGENTS · docs · api/gestures · cmd/qorm-wasm  

**不进 git：** `web_server/**` → `build-wasm.sh && deploy-site.sh`

## 残余（已知，不阻塞合入）

| 项 | 说明 |
|----|------|
| HTML dead-zone 粘滞 pan | 无跨帧 CurPan，纯 paint 用 0 |
| 多场景 morph 不刷新 keys/swipes | 与既有 keys 同 |
| autoplay 需手势 | 浏览器策略 |
| 30 个 HTML-only core 类型 | allowlist 有意登记，非 bug |
| G3 SEC-* | 未开工 |

## 主控下一步

1. 你确认 **「提交」** → 一笔或分笔 commit  
2. **部署** wasm/games 做 Raiden/Mario 有图+有声 spot-check  
3. 可选 G3：SEC-01 WASM 吊销 / LAN 文档  

## 变更日志

| 时间 | 事件 |
|------|------|
| 2026-08-12 | 波次1：QSS / swipes / audio / G0G1 |
| 2026-08-12 | 波次2：camera / parity / QSS residual；G2 关闭；主控回归绿 |
