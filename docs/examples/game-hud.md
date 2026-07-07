# Example: Game HUD

Game HUD 示例验证 QORM 的实时渲染能力。

## qorm.json 片段

```json
{
  "qorm": "0.1",
  "type": "app",
  "id": "game_hud_demo",
  "render": {
    "profile": "game-ui",
    "frameDriven": true,
    "targetFps": 60,
    "textMode": "fast"
  }
}
```

## HUD Scene

```json
{
  "qorm": "0.1",
  "type": "scene",
  "id": "hud",
  "root": {
    "type": "overlay",
    "id": "hud_root",
    "children": [
      {
        "type": "progressBar",
        "id": "hp_bar",
        "value": "{{ player.hp / player.maxHp }}",
        "layout": {
          "position": "absolute",
          "left": 24,
          "bottom": 24,
          "width": 240,
          "height": 18
        }
      },
      {
        "type": "text",
        "id": "hp_text",
        "textMode": "fast",
        "value": "{{ player.hp }} / {{ player.maxHp }}",
        "layout": {
          "position": "absolute",
          "left": 24,
          "bottom": 48
        }
      }
    ]
  }
}
```

## 验收

- 每帧不解析 JSON。
- HP 更新不触发全量 layout。
- progressBar 更新只改变绘制命令。
- 文本使用 fastText 缓存。