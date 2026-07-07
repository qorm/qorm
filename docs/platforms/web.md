# QORM Web Platform

Web 端通过 WASM Runtime 或 TypeScript Adapter 接入 QORM。

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

## Host Adapter

Web 端底层能力受浏览器限制。应通过 Web Host Adapter 封装：

```text
network.request
storage.read/write
clipboard.read/write
navigation.go
file.open
notification.show
```

## Web 安全边界

- Browser sandbox 是外层能力边界。
- QORM Web Runtime 不能超过浏览器已授予的能力。
- Web Host Adapter 是 QORM 在浏览器内的权限与策略执行点。
- 浏览器原生权限提示不等于 QORM approval；若两者都要求，必须都通过。

## 网络请求

Web 端使用 HttpClient 抽象：

```text
默认 fetch
可插拔 custom HttpClient adapter
```

### Custom HttpClient 边界

- custom client 只负责传输实现，不负责权限裁决。
- 域名、方法、header、credentials、approval 检查必须在 Host Adapter 侧完成。
- custom client 不能绕过 QORM 的 `network.request` 能力约束。
- CORS、cookie、same-origin 限制仍受浏览器控制。

Action 示例：

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

## 渲染路线

可选路线：

```text
DOM renderer
Canvas renderer
WebGPU renderer
WASM + GPU renderer
```

V1 可先使用最容易验证的路线，后续再强化高性能渲染。

## 限制

Web 端不能假设：

```text
任意文件系统访问
系统级剪贴板访问
后台长期运行
任意网络跨域
本地 Native Capability
```

## 审计可见性

Web 端的审计记录受浏览器隐私与存储约束影响。即便如此，权限决策、approval id 和 capability 结果仍应尽量记录到可用的本地或宿主日志中。