# QORM Performance Budget

QORM 不考虑 JIT，性能重点在渲染管线、缓存和增量更新。

## 通用原则

- JSON 只在加载或更新时解析。
- 运行时使用 Typed IR 和 Execution Plan。
- 状态变化只更新依赖节点。
- 动效默认不触发布局。
- 文本测量必须缓存。
- Display List 必须支持局部更新。
- preview_patch 不进入 live 渲染热路径。

## 普通应用目标

```text
首次中型 scene 渲染 < 100ms
常规事件响应 < 50ms
常规 Action 执行 < 1ms
60 FPS 动效
无全量重建 Display List
```

## 移动端目标

```text
Bundle 校验和预解析可后台进行
切换 Bundle 支持回滚
Keyboard/IME 不阻塞主线程
滑动列表使用 virtual list
文本和图片使用缓存
```

## Game UI 目标

```text
HUD 更新 < 1ms - 3ms
每帧不解析 JSON
每帧不全量 layout
每帧不全量 text shaping
动画不触发布局
draw call 可控
资源提前上传 GPU
```

## 规范约束下的性能不变量

- 完整表达式字段和模板插值字段都必须预编译为可复用表达式 IR。
- State Path 更新只触发相关 binding 和 dirty subtree。
- `opacity`、`transform`、`progress` 等纯渲染属性变化不得触发布局。
- `preview_patch` 必须在隔离副本上运行，不得污染 live runtime。
- Agent / MCP 工具不得进入渲染热路径。

## 监控指标

```text
bundle_parse_ms
ir_build_ms
layout_ms
render_build_ms
gpu_submit_ms
text_measure_count
text_cache_hit_rate
display_list_rebuild_count
host_call_latency_ms
frame_time_ms
```

## 性能失败标准

以下情况视为性能失败：

- 一次按钮点击导致全 Scene layout。
- 一个 opacity motion 导致 layout。
- Game HUD 每帧重建全部 Display List。
- 高频文本没有 cache。
- MCP 进入渲染热路径。
- preview_patch 直接操作 live scene。