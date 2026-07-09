# HTTP 与 SSE

> 由源码自动生成(`TestAPIRef`),请勿手工编辑。下方的路由表从代码抽取,不会与实现漂移。

`qorm run` 提供应用服务并暴露一小组 HTTP 接口:浏览器与之通信,AI 智能体在 `/mcp` 使用 MCP 工具,OTA 更新经 `/update` 进入。改变状态的端点要求同源请求。

| 路由 | 方法 | 用途 |
|---|---|---|
| `/` | GET | 应用外壳——服务端渲染的 HTML + 轻量客户端运行时 |
| `/event` | POST | 派发一个 UI 事件(动作 / 输入变化)并重新渲染 |
| `/events` | GET (SSE) | SSE 事件流:服务端推送最新 HTML + 日志行 |
| `/poll` | GET | SSE 不可用时的长轮询兜底——返回当前修订号,若有更新则附带 HTML |
| `/log` | GET / POST | GET 拉取 `?since=` 之后的活动条目;POST 转发一条客户端控制台日志 |
| `/presence` | GET / POST | 协作在场——谁(人 / 智能体)正聚焦或输入在何处 |
| `/viewport` | GET / POST | 浏览器回报窗口尺寸(缩放去抖)以便响应式 `when` 节点在服务端重渲染;GET 读取当前值 |
| `/console` | GET | 日志窗口的控制台信息流页面 |
| `/logwindow` | GET | 伴随桌面应用的独立日志窗口 |
| `/window` | POST | 桌面窗口控制(移动 / 缩放 / 打开 / 关闭 / 聚焦) |
| `/measure` | POST | 浏览器回报每个节点的实测布局(x/y/w/h、计算样式) |
| `/mcp` | POST | HTTP 上的 MCP JSON-RPC——与 `qorm mcp` 相同的工具,共享同一活动运行时 |
| `/update` | POST | OTA:向运行中的应用应用一个新的**已签名**捆绑包 |
| `/rollback` | POST | 回滚到上一个运行的捆绑包 |
| `/dev/state` | GET / POST | DevTools 状态检查器：读取或修改运行中的应用状态 |
| `/dev/tree` | GET | DevTools 组件树：读取当前场景的节点树 JSON |
| `/dev/highlight` | POST | DevTools 高亮事件：向所有客户端广播节点高亮检查信号 |

## `/events` 事件流

客户端打开 `GET /events` 并保持连接。服务端每次变化写入一条 SSE 消息:

```
: connected

data: <变化区域的 html>

```

每个 `data:` 帧携带客户端替换用的重渲染 HTML。日志与在场更新走同一条流。当代理缓冲 SSE 时,客户端回退到 `GET /poll?rev=<n>`。
