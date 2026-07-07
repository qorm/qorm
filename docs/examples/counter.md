# Example: Counter

Counter 示例用于验证状态、事件、Action 和绑定。

```json
{
  "qorm": "0.1",
  "type": "scene",
  "id": "counter",
  "state": {
    "count": 0
  },
  "root": {
    "type": "column",
    "id": "root",
    "layout": {
      "width": "fill",
      "height": "fill",
      "align": "center",
      "justify": "center",
      "gap": 16
    },
    "children": [
      {
        "type": "text",
        "id": "count_text",
        "value": "当前数值：{{ count }}"
      },
      {
        "type": "row",
        "id": "actions",
        "layout": { "gap": 12 },
        "children": [
          {
            "type": "button",
            "id": "dec",
            "text": "-",
            "on": { "press": [{ "type": "state.set", "path": "count", "value": "{{ count - 1 }}" }] }
          },
          {
            "type": "button",
            "id": "inc",
            "text": "+",
            "on": { "press": [{ "type": "state.set", "path": "count", "value": "{{ count + 1 }}" }] }
          }
        ]
      }
    ]
  }
}
```

验收：
- 点击按钮后仅依赖 `count` 的 binding 重新计算。
- `count_text` 被标记为相关更新目标。
- 不发生全 Scene layout。