# 手势

QORM 把触摸/指针手势作为组件属性提供——无需编写 JavaScript。

| 手势 | 怎么用 |
|---|---|
| 点按 / 双击 / 长按 | 任意节点上的 `onPress` / `onDoubleTap` / `onLongPress` |
| 场景滑动(游戏操控) | 场景级 `swipes` 映射——`{"left": "slideLeft", …}`——是场景 `keys` 的触摸对应物;按下后朝某个主导方向拖动再抬起即派发绑定的动作,同一款游戏在桌面用方向键、在手机上用滑动即可畅玩 |
| 滑动删除 | 带 `onDismissed` 的 `dismissible` 组件 |
| 滑动露出操作 | 带 `actions` 列表的 `swipeactions` 组件 |
| 可滑动分页 | 水平方向的 `scroll`(滚动吸附) |
| 下拉刷新 | 带 `onRefresh` 的 `scroll` |
| 拖拽重排 | 带 `reorderable: true` + `onReorder` 的 `list` |
| 上下文菜单 | 节点上的 `contextMenu` |
| 跨组件拖放 | `draggable` → `dragtarget` 跨面板拖拽 |
| 悬停反馈 | 任意节点的 `hoverBackground`、`hoverOpacity`、`hoverScale` |
| 按压反馈 | 任意节点的 `pressedBackground`、`pressedOpacity`、`pressedScale` |
| 动画过渡 | 任意节点的 `transition: "0.2s"` 动画化交互效果变化 |
| 滚动动量 | 滚动视口在滚轮/触控板输入后有惯性 |
| 画板平移动量 | 无限画布在拖动松手后有惯性滑行 |

## 文本输入编辑(原生 canvas 后端)

原生 canvas 窗口实现了完整的文本编辑系统:

- **选择**: Shift+方向键扩展选择,Cmd+A 全选,点击定位光标,双击选词,三击选行
  (textarea 中)或全选(单行 input 中),拖拽选择文本。
- **剪贴板**: Cmd+C 复制选中内容,Cmd+X 剪切,Cmd+V 粘贴(多行内容粘贴到单行 input
  时换行符转为空格)。系统剪贴板在 macOS 上通过 `pbcopy`/`pbpaste` 访问。
- **撤销/重做**: Cmd+Z 撤销上次编辑,Cmd+Shift+Z / Cmd+Y 重做。撤销栈上限 50 条。
- **导航**: Left/Right 逐字符,Cmd+Left/Right 逐词,Home/End 到行首/尾(textarea)
  或缓冲区首/尾(input),Up/Down 逐行(仅 textarea)。
- **删除**: Backspace 删除光标前字符,DeleteForward(Fn+Delete)删除后字符;有选中时
  均替换选中内容。
- **光标闪烁**: 字段聚焦时光标以 500ms 周期闪烁。
- **密码输入**: `"secure": true` 在编辑时将值遮罩为圆点;真实缓冲区用于复制和状态
  回写。
- **数字输入**: `"inputType": "number"` 限制只能输入数字、一个 `-` 和一个 `.`;
  `min`、`max`、`step` 属性约束提交值。
- **只读**: `"readonly": true` 字段仍可聚焦且键盘可见,但禁止编辑。
- **IME 组合输入**: 未提交的正在组合的文本以带下划线形式绘制在缓冲区后。

## 滚动动量

原生 canvas 引擎在滚轮/触控板输入停止后对滚动视口施加惯性:

- **逐视口速度跟踪**: 通过已消耗增量的指数移动平均(α=0.15)跟踪,连续触控板输入
  累积出平滑速度,离散滚轮事件产生微弱的后续滑行。
- **帧率无关摩擦**: 每帧速度以 `0.88^frames` 衰减——16ms 帧走一步摩擦,32ms 帧
  走两步。
- **防重复计数**: 滚动事件到达后 2ms 内跳过动量(事件处理器已施加自身增量)。
- **边界处理**: 偏移触底(0)时速度清零;layout 阶段的完整 clamp(`scrollOffsetPos`)
  修复顶部的任何 overshoot。
- 引擎在任何视口仍有活动动量时持续动画,所有速度降至 0.3 px/frame 以下后停止。

## 画板平移动量

无限画布 `board` 根节点在拖动松手后滑行:

- **速度跟踪**: 空白区域拖动期间,引擎跟踪连续移动间的位移并用 EMA(α=0.2)平滑。
- **滑行阶段**: 松手后,跟踪的速度每帧施加并以 `0.92^frames` 衰减——比滚动动量更
  长的滑行(约 50 帧 / 833ms),画布感更强。
- **取消**: 新的空白区域按压立即取消任何进行中的滑行,下次拖动从干净状态开始。

## 动画交互过渡

任意节点可声明 `transition` 时长(`"0.2s"`、`"200ms"`)来动画化交互效果变化:

- **缩放**: `"pressedScale": 0.95` + `"transition": "0.2s"` 在按压和释放时平滑
  缩放节点,通过 `Float64Tween` 使用主题缓动曲线。
- **背景色**: 主题 `pressedBackgroundColor` / 节点级 `"pressedBackground"` 随缩放
  一起动画。
- **透明度**: `"pressedOpacity"` / `"hoverOpacity"` 加入同一补间动画。

## 禁用状态视觉淡化

禁用节点(`"disabled": true`在 style 中)在原生 canvas 后端自动以 50% 透明度渲染:

- **非组件节点**(text、box、button、column、row 等)使用通用淡化。
- **交互组件**(switch、checkbox、slider、select 等)自行处理禁用视觉效果,被排除在
  通用淡化之外以防双重淡化。
- **自定义透明度**: `"disabledOpacity": 0.3` 覆盖默认值 0.5。
- 禁用节点同时:阻止指针激活、拒绝焦点、显示 `not-allowed` 光标、从 Tab 序中排除
  ——全部在引擎层处理,无需逐组件重复逻辑。
