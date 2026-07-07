<!-- data-lang-nav --> [English](../../platforms/web.md) · 中文

# QORM Web 平台

Web 平台通过 WASM Runtime 或 TypeScript Adapter 接入 QORM。

## 打包

```sh
qorm package examples/dashboard -p web -o dashboard-web   # an installable, offline PWA
```

托管输出目录并「添加到主屏幕」。任何示例都可以打包为 web。参见[支持矩阵](../../platforms/support-matrix.md)。

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
