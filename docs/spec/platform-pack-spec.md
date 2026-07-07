# QORM Platform Pack Specification

Platform Pack 是 QORM 适配特定运行平台的发布单元。

## 包结构

```text
platform-packs/<platform>/
├─ manifest.json
├─ capabilities.json
├─ renderer.json
├─ host-adapter.json
├─ event-adapter.json
├─ build-target.json
├─ skill.md
├─ mcp-profile.json
└─ examples/
```

## manifest.json

```json
{
  "qorm": "0.1",
  "type": "platform-pack",
  "id": "mobile",
  "name": "QORM Mobile Platform Pack",
  "targets": ["ios", "android"],
  "version": "0.1.0"
}
```

## capabilities.json

声明平台能力：

```json
{
  "network.request": {
    "supported": true,
    "permission": "network.request"
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
```

规则：
- 布尔值 `true` 只应作为 `{"supported": true}` 的简写。
- 正式 Platform Pack 应优先使用对象形式。
- Platform Pack 只能声明平台支持和额外收窄约束，不能放宽 app/system policy。

## renderer.json

声明渲染能力：

```json
{
  "displayList": true,
  "renderGraph": true,
  "externalSurface": true,
  "fastText": true,
  "textureAtlas": true
}
```

## event-adapter.json

声明事件支持：

```json
{
  "pointer": true,
  "keyboard": true,
  "textInput": true,
  "ime": true,
  "gesture": true,
  "lifecycle": true
}
```

## skill.md

描述 Agent 在该平台上如何修改 QORM，哪些能力受限，哪些操作需要用户确认。

## mcp-profile.json

限制 MCP 工具权限：

```json
{
  "allow": ["inspect", "validate", "preview_patch"],
  "denyByDefault": ["apply_patch", "host.call", "deploy"]
}
```

规则：
- `mcp-profile.json` 只能进一步限制 Agent 能力，不能扩大主权限模型。
- `preview_patch` 必须保持无副作用。