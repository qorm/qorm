# First Platform Pack

Platform Pack 描述 QORM 如何在某个平台运行。

## 目录

```text
platform-packs/desktop/
├─ manifest.json
├─ capabilities.json
├─ renderer.json
├─ host-adapter.json
├─ event-adapter.json
└─ skill.md
```

## manifest.json

```json
{
  "qorm": "0.1",
  "type": "platform-pack",
  "id": "desktop",
  "version": "0.1.0"
}
```

## capabilities.json

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

说明：
- 布尔值 `true` 只应作为对象形式的简写。
- 正式 Platform Pack 应优先使用对象形式，便于补充 `permission`、`domains`、`requiresApproval` 等约束。

## 使用

```bash
qorm check qorm.json --target desktop
qorm build qorm.json --target desktop
```