<!-- data-lang-nav --> [English](../../platforms/web.md) · 中文

# QORM Web 平台

Web 平台通过 WASM Runtime 或 TypeScript Adapter 接入 QORM。

## 打包

```sh
qorm package examples/dashboard -p web -o dashboard-web   # an installable, offline PWA
```

托管输出目录并「添加到主屏幕」。任何示例都可以打包为 web。参见[支持矩阵](../../platforms/support-matrix.md)。

Raiden/Mario 级别的游戏需要 canvas 宿主（`qorm_canvas` / games 页）；默认的 `qorm package -p web` 是 HTML 形态，而非完整的 canvas 游戏保真。

## 这里的 `http.*` 步骤跑在后台

打包后的应用把 QORM 运行时作为 WASM 携带,而 js/wasm 是单线程的:执行你 action 的
那个 goroutine,正是给浏览器事件循环提供服务的那一个。在那里做阻塞请求不是让应用
变慢,而是把它死锁。因此这个宿主——以及独立 WASM 运行时和在线 playground,它们是
同一个二进制——会把**每一个** `http.*` 步骤都放到后台工作者上执行,无论它的 JSON
有没有写 `"async": true`。

这是一条用户可见的语义,而不是实现细节:

> `http.*` 步骤之后的同级步骤会在**请求仍在进行时**执行。任何依赖响应的步骤都应当
> 写进 `onSuccess` / `onError`。

按这种方式写的 action 在 `qorm run` 与打包产物里行为一致。而从同级步骤读取响应的
action 在开发服务器上能工作(那里的请求除非你主动选择,否则是阻塞的),同一份代码
一旦打包就会悄悄读到过期的值——开发期显式写上 `"async": true`,就能复现打包后的
行为。完整写法见[第一个动作](../tutorials/first-action.md)。

把答案送回来的推送通道,与中间帧用的是同一条:页面安装 `window.qormApplyFrame`,
因此 `render` 步骤的加载帧能真正到达屏幕,而请求完成时会作为它自己的一帧稍后到达。
每一条会替换运行时的路径——应用了 OTA 更新、回滚——都会重新安装它;而属于一个已被
替换掉的运行时的响应会被丢弃,不会写进它的后继者。

## 架构

```text
qorm.bundle.json
  ↓
QORM WASM Runtime / Web Runtime
  ↓
Web Host Adapter
  ↓
Renderer
  ↓
Browser
```

## 宿主适配器(Host Adapter)

在 web 上,底层能力受浏览器约束。它们应通过 Web Host Adapter 封装:

```text
network.request
storage.read/write
clipboard.read/write
navigation.go
file.open
notification.show
```

## Web 安全边界

- 浏览器沙箱是最外层的能力边界。
- QORM Web Runtime 无法超越浏览器所授予的能力。
- Web Host Adapter 是 QORM 在浏览器内部的权限与策略强制执行点。
- 浏览器原生的权限提示不等同于 QORM 的授权;如果两者都需要,则两者都必须通过。

## 网络请求

Web 平台使用 HttpClient 抽象:

```text
default fetch
pluggable custom HttpClient adapter
```

### 自定义 HttpClient 边界

- 自定义客户端只负责传输实现,不负责权限决策。
- 域名、方法、请求头、凭据和授权检查必须在 Host Adapter 一侧执行。
- 自定义客户端无法绕过 QORM 的 `network.request` 能力约束。
- CORS、cookie 和同源限制仍由浏览器控制。

Action 示例:

```json
{
  "type": "host.call",
  "capability": "network.request",
  "input": {
    "method": "GET",
    "url": "/api/tasks",
    "responseType": "json"
  },
  "output": {
    "path": "tasksResponse"
  }
}
```

## 渲染路径

可用路径:

```text
DOM renderer
Canvas renderer
WebGPU renderer
WASM + GPU renderer
```

V1 可以从最容易验证的路径起步,之后再增强高性能渲染。

## 限制

Web 平台不能假定拥有:

```text
Arbitrary filesystem access
System-level clipboard access
Long-running background execution
Arbitrary cross-origin networking
Local Native Capability
```

## 审计可见性

Web 审计记录会受到浏览器隐私和存储约束的影响。即便如此,权限决策、授权 id 和能力结果仍应尽可能记录到可用的本地或宿主日志中。
