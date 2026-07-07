<!-- data-lang-nav --> [English](../collaboration.md) · 中文

# 在运行中的应用上进行人机协作

QORM 的前提:一个人和一个 AI 智能体**同时**在**同一个运行中的应用**上工作,
且彼此都看得见对方。`qorm run` 通过三条通道提供同一个实时运行时——面向人的浏览器、
面向智能体的 MCP,以及让每个观察者保持同步的服务器推送事件(SSE)。

## 启动一个共享会话

```sh
qorm run examples/counter          # browser UI + agent endpoint at /mcp
```

- **人**——打开打印出的 URL。点击会 POST `/event`;UI 实时更新。
- **AI**——经由 MCP 连接:`qorm mcp examples/counter`(stdio),或向
  `http://127.0.0.1:PORT/mcp` POST JSON-RPC。它共享浏览器所渲染的*同一个*运行时。

## 循环——彼此都看得见对方

- **人看得见 AI。** 当智能体编辑应用(`qorm_apply_patch`、
  `qorm_dispatch`、`qorm_set_state`)时,改动会经 SSE **即时**出现在每个已连接的
  浏览器中,并有一个实时的 **"AI edited · &lt;what&gt;"** 提示显示是谁做的——
  你实时看着 AI 工作。
- **AI 看得见人。** `qorm_activity` 返回共享的活动日志——谁(人 / 智能体)
  按顺序做了什么——因此智能体能响应人的点击,而不是从状态里瞎猜。人的操作也会
  反映在智能体的下一次 `qorm_inspect` 中。

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
[`integrations/`](../../integrations)。
