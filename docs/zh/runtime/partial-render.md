# 局部渲染

每次状态变更时,服务端都会重渲染当前场景并向已连接客户端推送 HTML。对于简单的标量更新(例如计数器 +1),整场景重渲染并不必要 —— QORM 可以只 patch 绑定了变更 state 路径的节点。

## 机制

1. **Dirty paths** —— action 中每个 `state.*` 步骤(`state.set`、`state.increment` 等)在 runtime 上记录被修改的路径(`DirtyPaths`)。
2. **依赖索引** —— 全量渲染时,服务端扫描场景树中的 `state.*` 引用,建立 state 路径 → 节点 id 的映射。
3. **Patch** —— 下一次 `bump()` 时,若所有受影响节点均为*可 patch*类型(`text`、`badge`、`progress`、`icon`),则仅重渲染这些子树并按 `id` 拼入缓存 HTML;handler 表复用。
4. **回退** —— 视口变化、导航、场景 patch、list 的 `data` 变更,或任意不可 patch 的组件,触发全量渲染并重建索引。

客户端仍收到完整的 `#qorm-root` HTML 片段并 morph —— 无额外客户端 patch 协议。

## 对应用作者的含义

- 频繁变化的值尽量绑在简单组件上(如用 `text` 显示数字/标签),收益最大。
- 列表、`when` 分支、按钮、输入框在数据结构变化时仍需全量渲染。
- 局部渲染对作者透明:相同 JSON 产生相同 UI;仅为服务端优化。

## Agent / inspect

局部渲染不改变 MCP 或 HTTP API。仍可按以往方式使用 `qorm_render_html` 或 SSE 观察 —— HTML 与全量渲染一致,只是在 `examples/counter` 等热路径上更快。
