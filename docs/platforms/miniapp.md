# QORM Miniapp Platform

Miniapp 平台用于适配小程序类运行环境。该平台能力通常受限，因此需要独立能力声明和降级策略。

更细的厂商差异与审核/调试约束以 `../spec/miniapp-vendor-profiles-spec.md` 为准。

## 目标

- 支持基础 scene/component/action/motion。
- 降级复杂渲染能力。
- 限制 Host Capability。
- 保留 Agent 可检查和平台兼容性校验。

## 限制

Miniapp 可能限制：

```text
复杂 GPU 渲染
动态脚本
文件系统
后台任务
WebSocket 或跨域网络
复杂自定义组件
完整剪贴板能力
```

## 宿主沙箱边界

- Miniapp 平台本身的宿主权限模型高于 QORM policy。
- QORM approval 不能绕过小程序平台的 host sandbox。
- 不支持的 capability 必须明确拒绝，不能静默降级为更宽权限路径。

## Capability Manifest

Miniapp Pack 必须明确声明支持能力：

```json
{
  "platform": "miniapp",
  "capabilities": {
    "network.request": {
      "supported": true,
      "permission": "network.request"
    },
    "storage.read": {
      "supported": true,
      "permission": "storage.read"
    },
    "storage.write": {
      "supported": true,
      "permission": "storage.write"
    },
    "clipboard.write": {
      "supported": false
    },
    "filesystem.saveFile": {
      "supported": false
    }
  },
  "render": {
    "displayList": false,
    "nativeComponents": true
  }
}
```

## 降级策略

```text
复杂 motion → fade/slide
externalSurface → 不支持或平台组件
filesystem → storage API
custom painter → canvas 或不支持
```

降级必须保持安全边界，不得把受限能力替换成更宽权限路径。

## Agent 行为

Agent 修改目标为 Miniapp 时，必须先调用 platform_check，避免生成平台不支持的能力。