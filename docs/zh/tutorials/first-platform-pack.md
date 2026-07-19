<!-- data-lang-nav --> [English](../../tutorials/first-platform-pack.md) · 中文

# 第一个平台包

> **设计意图——尚未实现。** 平台包是规划中的打包格式;运行时目前不会加载它,
> CLI 也没有 `--target` 标志。当前实际的打包能力见[移动端](../platforms/mobile.md)、
> [桌面端](../platforms/desktop.md)与 [Web](../platforms/web.md)。

平台包(Platform Pack)描述 QORM 如何在某个特定平台上运行。

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

说明:
- 布尔值 `true` 只应作为对象形式的简写来使用。
- 生产环境的平台包应优先采用对象形式,这样便于添加诸如 `permission`、`domains` 和 `requiresApproval` 之类的约束。

## 用法

```bash
qorm check qorm.json --target desktop
qorm build qorm.json --target desktop
```
