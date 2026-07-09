<!-- data-lang-nav --> [English](../build-with-ai.md) · 中文

# 与你的 AI 助手一起构建 QORM 应用

QORM 面向智能体:让你的 AI 编码助手(Claude Code、Claude Desktop、Cursor、Windsurf……)
指向它,由 AI 来**搭建、编辑、运行并验证** QORM 应用——然后与你实时协作一个运行中的应用。
这是工作流中属于人的一侧。

## 先看效果

60 秒版本:[`scripts/demo.sh`](https://github.com/qorm/qorm/blob/main/scripts/demo.sh) 会开一个共享会话并按脚本自动做一串 AI 编辑——打开打印出的 URL、按下录制,就能看到应用实时变化 + 「AI edited」提示:

```sh
./scripts/demo.sh                 # examples/counter
./scripts/demo.sh examples/dashboard
```

## 1. 安装 QORM

```sh
go install github.com/qorm/qorm/cmd/qorm@latest   # puts `qorm` on your PATH
# or use the container: ghcr.io/qorm/qorm
```

## 2. 把 QORM 的工具 + 技能交给你的 AI

QORM 附带一个即插即用的 MCP 服务器(让 AI 能读取、编辑并验证一个运行中的应用)
和一个技能(让它写出运行时真正接受的格式)。各智能体的具体配置见
[`integrations/`](https://github.com/qorm/qorm/tree/main/integrations)。简而言之:

- **Claude Code:** `claude mcp add qorm -- qorm mcp .`
- **Claude Desktop / Cursor / Windsurf:** 把
  [`integrations/mcp.json`](https://github.com/qorm/qorm/blob/main/integrations/mcp.json) 里的代码块合并进你智能体的 MCP 配置。
- 把 AI 指向技能
  [`integrations/skill/SKILL.md`](https://github.com/qorm/qorm/blob/main/integrations/skill/SKILL.md)(或本仓库的
  [`llms.txt`](https://github.com/qorm/qorm/blob/main/llms.txt) / [`AGENTS.md`](https://github.com/qorm/qorm/blob/main/AGENTS.md)),让它使用可运行的
  格式而不是靠猜。

## 3. 让它构建点东西

工具挂载好后,用大白话开口,例如:

> "在 ./habits 里搭一个 QORM 习惯追踪器——一个习惯列表,带每日打卡和连续天数计数。"

AI 会写出 `qorm.json` + `scenes/` + `actions/`,并能运行 `qorm run ./habits`
和 `qorm check ./habits` 来查看并验证它所构建的东西。

## 4. 在运行中的应用上协作

启动一个共享会话,与 AI 并肩工作:

```sh
qorm run ./habits          # opens in your browser; agent endpoint at /mcp
```

- 你在浏览器里点击;AI 通过 `qorm_activity` 看到你的操作。
- AI 经由 MCP 编辑;改动即时出现在你的浏览器中,并带一个
  **"AI edited"** 提示,让你眼看着它发生。
- AI 的设计改动是评审绑定的(预览 → 应用),它会用 `qorm measure` / `qorm check`
  自我验证其编辑。

完整闭环见[人机协作](collaboration.md)。

## 设计令牌（让 AI 守住配色）

你可以在 `qorm.json` 中声明一套**设计令牌系统**，让 AI 的样式编辑守在你的设计
系统之内，而不是漂移到任意颜色。加一个可选的 `designTokens` 映射——每一项都是一个
有名称、有类型的值：

```json
"designTokens": {
  "color.primary": { "type": "color", "value": "#0a84ff", "enforce": true },
  "color.bg":      { "type": "color", "value": "#f2f2f7", "enforce": true },
  "spacing.md":    { "type": "number", "value": 16,        "enforce": false }
}
```

- **`type`** —— `color`、`number`……（值以字符串形式存储；`16` → `"16"`）。
- **`enforce`** —— 在*硬约束*与*建议*之间的开关。

**它如何约束智能体。** 当你把一个 `color` 令牌标为 `enforce: true` 时，
`qorm_apply_patch`（以及无副作用的 `qorm_preview_patch`）会**拒绝**任何将颜色样式
——`color`、`background`、`backgroundColor`、`borderColor`——设为非你所声明的
enforce 颜色令牌值的 `setProp` style 操作。拒绝是一条清晰的、列出允许值的错误，例如：

```
design token violation: color "#ff0000" is not an allowed token (allowed: #0a84ff, #f2f2f7)
```

比较时会对十六进制大小写与前导 `#` 做归一化，因此 `#0A84FF`、`0a84ff` 和
`#0a84ff` 都匹配同一个令牌。

- `enforce: false` 的令牌是**建议性**的——会暴露给智能体，但绝不拦截。
- 一个**未**声明 `designTokens`（或没有 enforce 颜色令牌）的应用行为与以往完全一致
  ——不受任何约束。

智能体通过 `qorm_inspect` 发现你的令牌——它现在会返回一个 `designTokens` 字段，
从而在编辑前就知道自己被允许使用哪些值。可运行的声明示例见
[gallery 示例](https://github.com/qorm/qorm/blob/main/examples/gallery/qorm.json)。

## 好用的提示词

- "在设置场景里加一个深色主题开关,并验证布局。"
- "这个按钮在移动端溢出了——量一下并修正宽度。"
- "把任务行改造成一个可复用组件。"
- "把它打包成一个可安装的 web 应用。"

AI 手头握有整个表面:[组件目录](/api/widgets.md)、
[能力清单](../platforms/capabilities.md),以及
[MCP 工具](../agent/mcp-tools.md)。
