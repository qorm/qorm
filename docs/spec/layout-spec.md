# QORM Layout Specification

QORM Layout 是平台无关的布局模型，不直接等同于 CSS。内部可以适配布局库，但对外暴露的是 QORM LayoutSpec。

## 布局模式

```text
row       横向排列
column    纵向排列
stack     层叠排列
grid      网格
absolute  绝对定位
overlay   浮层、HUD、弹窗
scroll    滚动容器
```

## SizeValue

```text
auto
fill
fit
px number
percent string, e.g. "50%"
fr number
minContent
maxContent
```

示例：

```json
{
  "layout": {
    "width": "fill",
    "height": "fit",
    "minWidth": 240,
    "maxWidth": 960
  }
}
```

## 基础布局字段

| 字段 | 类型 | 说明 |
|---|---|---|
| `width` | size | 宽度 |
| `height` | size | 高度 |
| `minWidth` | size | 最小宽度 |
| `maxWidth` | size | 最大宽度 |
| `minHeight` | size | 最小高度 |
| `maxHeight` | size | 最大高度 |
| `padding` | number/object | 内边距 |
| `margin` | number/object | 外边距 |
| `gap` | number | 子节点间距 |
| `align` | string | 交叉轴对齐 |
| `justify` | string | 主轴对齐 |
| `overflow` | string | visible/hidden/scroll |
| `position` | object/string | 定位 |

## Safe Area

移动端必须支持 Safe Area：

```json
{
  "layout": {
    "padding": {
      "top": "safeArea.top",
      "bottom": "safeArea.bottom"
    }
  }
}
```

`safeArea.top` / `safeArea.bottom` 属于环境值引用，不是通用 `{{ ... }}` 表达式。Runtime 在 `safe area change` 生命周期事件后必须重新求值相关布局字段。

## Absolute / Overlay

游戏 HUD 和浮层常用：

```json
{
  "type": "overlay",
  "children": [
    {
      "type": "text",
      "layout": {
        "position": "absolute",
        "left": 24,
        "top": 24
      }
    }
  ]
}
```

## Scroll

滚动容器字段：

```json
{
  "type": "scroll",
  "layout": {
    "direction": "vertical",
    "width": "fill",
    "height": "fill"
  }
}
```

## Virtual List

虚拟列表用于大数据场景：

```json
{
  "type": "virtualList",
  "data": "{{ messages }}",
  "estimatedItemHeight": 64,
  "item": {
    "type": "component",
    "ref": "message_item"
  }
}
```

- `data` 必须求值为数组。
- `estimatedItemHeight` 用于初始视口估算，不等于最终真实高度。
- `item` 模板内部可见 `item` 上下文。

## Layout Dirty

会触发布局的变化：

```text
text content size changes
width/height changes
padding/margin/gap changes
children changes
visibility changes affecting flow
font size changes
```

不会触发布局的变化：

```text
opacity
transform
color
shadow
clip
progress
```

除非显式声明 `affectsLayout: true`。
