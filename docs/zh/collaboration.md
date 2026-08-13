<!-- data-lang-nav --> [English](../collaboration.md) · 中文

# 在运行中的应用上进行人机协作

QORM 的前提:一个人和一个 AI 智能体**同时**在**同一个运行中的应用**上工作,
且彼此都看得见对方。`qorm run` 把同一实时运行时提供给人类 UI
(macOS 默认原生 canvas,或浏览器/WebView)、智能体的 HTTP MCP,
以及 DevTool/活动流。浏览器/WebView 观察者通过 SSE 保持同步。

## 启动一个共享会话

```sh
qorm run examples/counter          # native/browser UI + agent endpoint at /mcp
```

- **人**——使用 QORM 打开的窗口,或打开打印出的 URL。原生 canvas 直接分发;
  浏览器/WebView 点击 POST `/event`。
- **AI**——向 `http://127.0.0.1:PORT/mcp` POST JSON-RPC。该 HTTP 端点与人类 UI
  共享*同一个*运行时。`qorm mcp examples/counter` 是独立 stdio 运行时,
  适用于独立工具,而不是这个实时共享会话循环。

## 循环——彼此都看得见对方

- **人看得见 AI。** 当智能体编辑应用(`qorm_apply_patch`、
  `qorm_dispatch`、`qorm_set_state`)时,改动会经 SSE **即时**出现在每个已连接的
  浏览器中,并有一个实时的 **"AI edited · &lt;what&gt;"** 提示显示是谁做的——
  你实时看着 AI 工作。
- **AI 看得见人。** `qorm_activity` 返回共享的活动日志——谁(人 / 智能体)
  按顺序做了什么——因此智能体能响应人的点击,而不是从状态里瞎猜。人的操作也会
  反映在智能体的下一次 `qorm_inspect` 中。
  原生 canvas 指针/键盘动作与浏览器/WebView 动作进入同一份人类 `events`
  流和 DevTool。Canvas 也通过同一 presence 模型回报隐私安全的 focus、typing
  与隐藏字段标签。

## 安全编辑——评审绑定

智能体的设计改动受到管控,使运行中的应用无法在未经评审的情况下被改动:

- `qorm_simulate_action`、`qorm_preview_patch` 和 `qorm_diff` 都针对一份副本运行,
  从不触碰运行中的应用。
- `qorm_apply_patch` 只有在携带来自相同操作的匹配 `qorm_preview_patch` 的
  `previewToken` 时才会提交——每一次已提交的改动都经过了预览。
- `qorm_undo` 撤销上一次应用。

## 自我验证

智能体针对渲染出的真实情况来证明其编辑,而非基于假设:
`qorm_measure` / `qorm_check_layout`(或 CLI 的 `qorm measure` / `qorm check`)
会渲染应用并报告真实的几何。参见[验证一个应用](verification.md)。

## 工具一览

| role | tools |
|---|---|
| understand | `qorm_inspect`, `qorm_query`, `qorm_get_node`, `qorm_render_html`, `qorm_activity` |
| operate | `qorm_dispatch`, `qorm_set_state` |
| design (safe → commit) | `qorm_preview_patch` / `qorm_diff` → `qorm_apply_patch`, `qorm_undo` |
| verify | `qorm_measure`, `qorm_check_layout` |

完整参考:[MCP 工具](../agent/mcp-tools.md)。要把 QORM 添加到你的智能体,见
[`integrations/`](https://github.com/qorm/qorm/tree/main/integrations)。
