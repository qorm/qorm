# QORM API 参考

QORM 应用的权威、机器生成的契约——直接从运行时源码抽取,因此永不与代码实际行为漂移。
这是你(或你的 AI 智能体)据以编写的参考;教程与指南见[文档站](/docs/zh/)。

## 声明式 UI 契约

- [节点与组件属性](props.md)——节点结构、通用样式属性,以及每个组件的专有属性
- [组件目录](widgets.md)——渲染器接受的每一种节点 `type` 及别名
- [动作与状态](actions.md)——每种动作步骤 `type` 及其字段
- [手势](gestures.md)——点按 / 长按 / 滑动 / 拖拽,作为组件属性
- [动画](animation.md)——入场效果与值驱动的过渡
- [导航](navigation.md)——场景、navigate 步骤与页面转场

## 运行时接口面

- [HTTP 与 SSE](http-api.md)——`qorm run` 提供的端点(浏览器、MCP、OTA)
- [Go 包:qormext](go-api.md)——唯一的公开 Go 包,用于应用自有的原生操作
- [MCP 工具](/docs/agent/mcp-tools.html)——AI 智能体驱动活动应用所用的工具
- [能力清单](/docs/platforms/capabilities.html)——内置的硬件 / OS 操作与回调

> 本站每一页都由 `QORM_UPDATE_DOCS=1 go test ./...` 从源码重生成——请勿手工编辑。
