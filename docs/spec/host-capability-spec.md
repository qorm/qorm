# QORM Host Capability Specification

QORM 是 UI 层，不实现所有底层能力。底层能力通过 Host Capability 暴露。

## 目标

- 隔离 UI Runtime 与平台 API。
- 为桌面、移动、Web、游戏、插件提供统一调用方式。
- 让 Agent 可以检查能力是否可用。
- 让权限模型可声明、可校验。

## Capability 命名

```text
network.request
storage.read
storage.write
filesystem.openFile
filesystem.saveFile
window.resize
clipboard.read
clipboard.write
notification.show
navigation.go
camera.capture
audio.play
video.play
window.fullscreen
ime.status
accessibility.announce
game.surface
```

## 调用格式

```json
{
  "type": "host.call",
  "capability": "clipboard.write",
  "input": {
    "text": "{{ shareLink }}"
  }
}
```

## Capability Manifest

```json
{
  "platform": "desktop",
  "capabilities": {
    "network.request": {
      "supported": true,
      "permission": "network.request",
      "domains": ["api.example.com"]
    },
    "clipboard.write": {
      "supported": true,
      "permission": "clipboard.write"
    },
    "filesystem.saveFile": {
      "supported": true,
      "permission": "filesystem.write",
      "requiresApproval": true
    }
  }
}
```

布尔值 `true` 可视为 `{"supported": true}` 的简写，但正式实现应使用对象形式。

## 平台实现

```text
Desktop: Go Host Adapter（cmd/qorm window_desktop.go desktopHardware*）+ WebView/OS API
Mobile: Go 运行时（Go→WASM）+ qormToNative + Swift/Kotlin thin bridge
        （iOS: package_native.go iosBridgeBody() / Android: androidMainActivity()）
Web: Web Host Adapter + Web API / fetch
Game: Game Host Adapter + game state / external surface
```

硬件能力统一路径：widget（`internal/render/render.go`）→ JS（`internal/server/server.go`）→ `qormToNative` op → 平台原生桥。

## 硬件能力矩阵

QORM 当前提供约 **26 个硬件能力**（camera/photo、microphone/record、location、bluetooth、nfc、biometric、haptic、vibrate、torch/flashlight、brightness、volume、battery、motion 传感器、network/wifi status、clipboard、storage、share、notification/badge、keepAwake、screenshot、screen record、device info、orientation 等）。

| 平台 | 通用硬件（Web API 可达） | Web API 缺失能力（走原生桥） |
|---|---|---|
| iOS（dev 客户端） | Web API + 全量原生桥 | 蓝牙 / NFC 等由内置原生桥补齐 |
| iOS / Android（离线 WASM 包） | Web API（收敛中：原生桥正合并进离线 VC） | 合并前蓝牙/NFC 暂缺，合并后全量 |
| Android | Web API + 原生桥 | 蓝牙 / NFC 等由原生桥补齐 |
| 桌面（macOS/Linux/Windows） | 摄像头/麦克风/定位等 Web API（localhost 安全上下文） | 由 `desktopHardware*` + WebView/OS API 提供 |
| 浏览器 | Web API | 无原生桥，自动回退 |

`qormHasNative()`（有原生桥）/ `qormHasMobileNative()`（iOS/Android 全量桥）用于运行时探测；浏览器/桌面自动回退到 Web API。详见 [用户中间层](../platforms/native-middlelayer.md)。

## 权限判定

每次 `host.call` 必须经过：

```text
capability exists
platform supports capability
bundle declares capability
app/system policy allows capability
agent/plugin policy allows capability
user approval if required
```

规则：
- 默认 deny。
- 任一环节拒绝即拒绝。
- 用户 approval 不能覆盖 `platform unsupported` 或系统 hard deny。
- 结构化限制项（如 `domains`、`methods`）只能进一步收窄，不能放宽。

## requiresApproval

`requiresApproval: true` 表示即使能力已被允许，该次调用仍需要有效审批凭证。

最小语义：
- 审批 scope 绑定 capability、目标资源范围和调用方身份。
- 审批可为一次性或会话级，但必须由策略显式决定。
- 策略、Bundle、Platform Pack、Agent Pack 或目标资源变化后，旧审批必须失效。

## network.request 能力契约

### 输入

```json
{
  "method": "POST",
  "url": "/api/login",
  "headers": {
    "content-type": "application/json"
  },
  "query": {
    "lang": "zh-CN"
  },
  "body": {
    "username": "ada",
    "password": "secret"
  },
  "timeoutMs": 10000,
  "responseType": "json",
  "credentials": "same-origin"
}
```

字段：
- `method`: 必填，HTTP method。
- `url`: 必填，相对路径或被策略允许的绝对 URL。
- `headers`: 可选，字符串字典；header name 在策略匹配与传输请求中按 trim + lowercase 归一化，匹配时大小写无关。
- `query`: 可选，键值对。
- `body`: 可选，JSON 值。
- `timeoutMs`: 可选，请求超时。
- `responseType`: `json` / `text` / `bytes`。
- `credentials`: `omit` / `same-origin` / `include`，仅在平台支持时有效。

### 约束

- 权限键使用 `network.request`。
- 最终允许域名由平台 manifest、app policy、bundle declaration 三者求交集决定。
- header 约束采用 deny 优先；当 header allowlist 非空时，未列入 allowlist 的请求 header 必须拒绝。
- 自定义 HttpClient 不能绕过域名、方法、header、凭证和审批限制；所有策略约束都必须在 dispatch 到 HttpClient 前执行。
- 非法 URL、超出域名范围或不支持的凭证策略必须在 dispatch 前拒绝。

### 输出

成功结果的规范化结构：

```json
{
  "ok": true,
  "status": 200,
  "headers": {
    "content-type": "application/json"
  },
  "body": {
    "token": "..."
  }
}
```

失败结果的规范化结构：

```json
{
  "ok": false,
  "error": {
    "code": "timeout",
    "message": "request timed out",
    "capability": "network.request"
  }
}
```

### 标准错误码

```text
permission_denied
approval_required
approval_expired
platform_unsupported
timeout
offline
dns_error
tls_error
invalid_request
http_error
```

- `http_error` 用于平台选择把非 2xx 视为失败的情形。
- 如果实现选择“非 2xx 仍返回 ok: true + status”，必须在平台文档中明确，并保持一致。

## 安全边界

Bundle 可以声明需要能力，但不能绕过平台权限。

禁止：

- 未声明能力直接调用底层 API。
- 从 Bundle 动态新增 Native API。
- Agent 默认调用危险 Host Capability。
- UI Action 直接执行 shell。
- 自定义 client 充当权限裁决者。

## Host Error

Host 调用错误必须结构化：

```json
{
  "error": {
    "code": "permission_denied",
    "message": "clipboard.write requires permission",
    "capability": "clipboard.write"
  }
}
```

## 审计事件

每次 host 决策至少应记录：

```text
capability
caller type
caller id
policy result
approval id
input summary
result code
timestamp
```
