# QORM Game UI Render Profile

`game-ui` 是一种 Render Profile / Runtime Mode，用于游戏 UI、HUD、菜单、Overlay 和外部游戏 Surface 集成，但它不是 Platform。

它可以运行在 Desktop、Mobile、Web 等平台之上；平台负责宿主能力与系统集成，`game-ui` 负责渲染策略、运行节奏和组件偏好。

## 支持范围

`game-ui` profile 适用于：

```text
主菜单
暂停菜单
HUD
血条
技能栏
背包
对话框
战斗结算
排行榜
小地图外壳
游戏 Overlay
```

QORM 不支持或不负责：

```text
物理
地图渲染
3D 场景
角色控制
骨骼动画
AI
粒子系统
光照和材质
```

## Profile 定义

```json
{
  "render": {
    "profile": "game-ui",
    "frameDriven": true,
    "targetFps": 60,
    "textMode": "fast"
  }
}
```

## State Snapshot

游戏状态以快照形式传入：

```json
{
  "player": {
    "hp": 72,
    "maxHp": 100
  },
  "combat": {
    "targetHp": 0.42
  }
}
```

QORM 只更新依赖这些字段的 HUD 节点。

## 与平台的关系

- Platform 决定运行宿主，例如 desktop / mobile / web。
- `game-ui` 决定 UI 的渲染与更新模式，例如 frame-driven、fast text、HUD 组件偏好。
- `game.surface` 是一种 host capability，不等于 `game-ui` 本身。

## External Surface

```json
{
  "type": "externalSurface",
  "id": "game_view",
  "capability": "game.surface",
  "layout": {
    "width": "fill",
    "height": "fill"
  }
}
```

## Game UI 组件

建议提供：

```text
progressBar
radialProgress
cooldown
skillSlot
buffIcon
inventoryGrid
sprite
spriteText
fastText
damageText
hudPanel
crosshair
```

## Runtime Mode 约束

- 每帧不解析 JSON。
- HUD 更新不触发全量 layout。
- 高频动画只改 transform、opacity、progress、color。
- 使用 texture atlas。
- 使用 fast text。
- 使用 frame-driven runtime。
- 不引入新的平台权限；所需能力仍通过 Host Capability 和 Platform Pack 提供。 
