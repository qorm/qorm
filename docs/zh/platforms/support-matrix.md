# 平台支持矩阵

> 由支持矩阵注册表自动生成 —— 请勿手动修改。

QORM 在各运行目标上的支持情况一览。 **`ok`** = 已支持并测试； **`beta`** = 基础支持/部分或受限支持； **`—`** = 不适用。硬件/原生能力的详细支持情况请参见 [能力清单](capabilities.md)。


## 分发 (Distribution)

| 特征/能力 | Web | iOS | Android | macOS | Linux | Windows | 小程序 |
|---|---|---|---|---|---|---|---|
| 可安装包 (Installable package) | ok | ok | ok | ok | beta | beta | ok |
| 离线 / 自包含 (Offline / self-contained) | ok | ok | ok | ok | ok | ok | beta |
| PWA 安装 (PWA install) | ok | ok | beta | — | — | — | — |
| 签名包 (Signed bundle) | ok | ok | ok | ok | ok | ok | — |
| 热更新与回滚 (OTA) | ok | ok | ok | ok | ok | ok | — |

## 渲染 (Rendering)

| 特征/能力 | Web | iOS | Android | macOS | Linux | Windows | 小程序 |
|---|---|---|---|---|---|---|---|
| 声明式 HTML/CSS 渲染 | ok | ok | ok | ok | ok | ok | beta |
| 全量组件集 (Full widget set) | ok | ok | ok | ok | ok | ok | beta |
| 主题（Apple / Material / 深色） | ok | ok | ok | ok | ok | ok | beta |
| 自定义组件（JSON 定义） | ok | ok | ok | ok | ok | ok | beta |
| 多语言与 RTL 支持 (i18n / RTL) | ok | ok | ok | ok | ok | ok | beta |
| 原生窗口（无边框/透明） | — | — | — | ok | beta | beta | — |
| 系统菜单栏/托盘/右键菜单 | — | — | — | ok | beta | beta | — |

## 运行时 (Runtime)

| 特征/能力 | Web | iOS | Android | macOS | Linux | Windows | 小程序 |
|---|---|---|---|---|---|---|---|
| 实时状态/动作/绑定 | ok | ok | ok | ok | ok | ok | — |
| 表达式绑定 (Expression bindings) | ok | ok | ok | ok | ok | ok | — |
| 条件渲染与数据绑定列表 | ok | ok | ok | ok | ok | ok | — |
| Go 中间层（自定义原生操作） | ok | ok | ok | ok | ok | ok | — |
| 硬件与 OS 能力 | ok | ok | ok | ok | beta | beta | beta |

## 智能体 (Agent)

| 特征/能力 | Web | iOS | Android | macOS | Linux | Windows | 小程序 |
|---|---|---|---|---|---|---|---|
| MCP 服务端（读取/编辑/验证） | ok | ok | ok | ok | ok | ok | — |
| 人机共享实时会话 (SSE) | ok | ok | ok | ok | ok | ok | — |
| 审查限制编辑 (preview → apply) | ok | ok | ok | ok | ok | ok | — |
| 自我验证渲染 (qorm measure / check) | ok | ok | ok | ok | ok | ok | — |

## 备注说明

- **可安装包 (Installable package)** —— 桌面端为针对不同平台的 cgo 构建；小程序为微信小程序项目
- **离线 / 自包含 (Offline / self-contained)** —— Web/移动端通过 Go→WASM 离线运行；小程序渲染静态 UI
- **PWA 安装 (PWA install)** —— Web 清单 + Service Worker；iOS/Android 支持添加到主屏幕
- **签名包 (Signed bundle)** —— 纯 Go 自校验签名包；小程序由平台签名
- **热更新与回滚 (OTA)** —— 小程序更新受微信平台控制
- **声明式 HTML/CSS 渲染** —— 小程序渲染映射后的 WXML/WXSS
- **全量组件集 (Full widget set)** —— 布局、输入、媒体、结构 —— 参见 widgets.md
- **主题（Apple / Material / 深色）** —— 设计 Token；小程序携带 Token WXSS
- **自定义组件（JSON 定义）** —— 在 qorm.json 中声明，采用 {{prop.x}} 模板
- **多语言与 RTL 支持 (i18n / RTL)** —— ICU 消息、复数、货币、自右向左文本支持
- **原生窗口（无边框/透明）** —— 需使用 -tags desktop 编译；macOS 为参考实现
- **实时状态/动作/绑定** —— 小程序为仅静态导出（static export only），设备端无运行时
- **表达式绑定 (Expression bindings)** —— 算术、比较、三元、字符串操作、内置函数；小程序为仅静态导出（导出时求值一次）
- **条件渲染与数据绑定列表** —— if: 条件渲染，列表重复以及 {{item.*}} 作用域；小程序为仅静态导出（导出时求值一次）
- **Go 中间层（自定义原生操作）** —— 将单个 native/desktop.go 编译入桌面端以及移动端/Web 的 WASM 中
- **硬件与 OS 能力** —— 各能力的详细支持情况见 capabilities.md
- **MCP 服务端（读取/编辑/验证）** —— 对运行中的应用通过 stdio 或 /mcp 进行交互；小程序为仅静态导出（static export only），实时工具不适用
- **人机共享实时会话 (SSE)** —— AI 的编辑立即显示在人类浏览器中；人类的点击和输入焦点反馈在 qorm_activity 中
- **审查限制编辑 (preview → apply)** —— 应用补丁的 apply_patch 必须携带 preview token；小程序为仅静态导出（static export only）
- **自我验证渲染 (qorm measure / check)** —— 渲染应用并报告真实的几何空间布局
