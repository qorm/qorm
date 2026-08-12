# 实施进度

> 主控同步 · 更新：2026-08-12 **v0.8.8 已发布**

## 发布

| 项 | 状态 | 证据 |
|----|------|------|
| 功能 commit | 🟢 | `f6133ea` feat: HTML QSS/swipes/camera/audio parity… |
| gofmt | 🟢 | `c1a0382` |
| changelog archive | 🟢 | `e19220b` docs: changelog v0.8.8 |
| version bump | 🟢 | `8a80a64` · `var version = "0.8.8"` |
| CI 修复 | 🟢 | `d7cdb02` CanonicalAssetURL → playcore |
| tag | 🟢 | `v0.8.8` → `d7cdb02`（含 CI 修复 force 更新） |
| main push | 🟢 | origin/main 对齐 |
| 官网 deploy | 🟢 | 两次 `deploy-site.sh` OK · wasm+docs+v0.8.8 戳记 |

## G0–G2

| 目标 | 状态 |
|------|------|
| G0 WASM preload | 🟢 已入 0.8.8 |
| G1 文档/golden | 🟢 已入 0.8.8 |
| G2 A–F 对等 | 🟢 已入 0.8.8 |
| G3 安全 | ⬜ 未做 |

## 链接

- 仓库：https://github.com/qorm/qorm  
- Tag：https://github.com/qorm/qorm/releases/tag/v0.8.8  
- 官网：https://qorm.com/  
- Docs：https://qorm.com/docs/  
- Games：https://qorm.com/games/  

## Canvas 完善波次

| 项 | 状态 |
|----|------|
| style: gradient / flexGrow / boxShadowX / disabledOpacity | 🟢 |
| widgets: aspectratio, ignorepointer, skeleton, circularprogress | 🟢 |
| aliases: activityindicator, tag, animatedcontainer | 🟢 |
| fab / switchlisttile / letterSpacing·lineHeight·italic | 🟢 |
| searchbar / checkboxlisttile / radiolisttile | 🟢 |
| field / textformfield / richtext(wrap) / carousel | 🟢 |
| multi-stop CSS-angle gradient + backdropBlur | 🟢 |
| rangeslider / pageview / tree / autocomplete | 🟢 |
| remaining: actionsheet/alertdialog/descriptions/dropdownbutton/materialstepper/monthview/motion/picker/rating/refreshindicator/selectabletext/transform | 🟢 |
| parity allowlist 收缩 | 🟢 核心 example 类型 allowlist 已清空 |
| 测试 widgets/canvas/integration | 🟢 |

## 变更日志

| 时间 | 事件 |
|------|------|
| 2026-08-12 | 实施波次 1–2 完成 |
| 2026-08-12 | **v0.8.8** tag + main + 官网部署 |
| 2026-08-12 | Canvas 完善：样式键 + 7 个 widget/别名 |
| 2026-08-12 | Canvas：rangeslider/pageview/tree/autocomplete + richtext wrap + true angle gradient |
