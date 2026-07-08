<!-- data-lang-nav --> [English](../../platforms/miniapp.md) · 中文

# QORM 小程序平台

> **定位:仅静态导出(static export only)。** `qorm package -p miniapp` 是把应用的**初始 UI** 一次性导出为 WXML/WXSS 工程。设备端没有 QORM 运行时:没有实时会话(live session)、没有状态/动作、没有 `qorm measure`/自我验证、没有动作分发(dispatch)。导出什么,渲染什么。

小程序(微信 / 其他)无法在其渲染路径中运行 Go 或 WASM —— 它们在厂商沙箱内渲染由 JS 页面模型驱动的 **WXML** 标记。因此 QORM 针对它们采取与 web/mobile/desktop 不同的方式:不再分发 Go→WASM 运行时,而是**把应用渲染出的 HTML/CSS 重新映射为 WXML/WXSS**,并生成一个开箱即用的微信项目。

有关这里究竟支持哪些功能,参见[平台支持矩阵](../../platforms/support-matrix.md)。

## 打包一个

```sh
qorm package examples/counter -p miniapp -o counter-weapp
# aliases: -p miniprogram | -p weapp
```

在**微信开发者工具**中打开输出。它是一个标准项目:

```
counter-weapp/
  app.json            pages list + window chrome
  app.js  app.wxss    app entry + global design tokens (QORM theme)
  project.config.json sitemap.json
  pages/index/
    index.wxml        the UI (QORM boxes → <view>, <img> → <image>)
    index.wxss  index.js  index.json
```

## 基础层做了什么

- **静态渲染** —— 应用的初始 UI,QORM 的布局和内联样式被整体复用(WXSS 接受内联 `style=` 和 CSS 变量)。
- **点击接线** —— `onclick="qorm(N)"` 变为 `bindtap="onTap"`,并在 `data-h` 中携带处理器索引,从而让事件到达页面模型。
- **图标与图表** —— 内联 SVG 被重新编码为 data-URI `<image>`(WXML 无法渲染 `<svg>`);图表颜色精确转换,图标颜色默认为中性色(完整的图标主题化是后续工作)。
- **主题** —— QORM 的设计令牌(`--accent`、`--label`、……)进入 `app.wxss`。

## Roadmap(v0.3,未承诺)

以下内容如今都不存在 —— 该目标当前为仅静态导出:

- **完整交互** —— 需要一个 JS 解释器,用于在设备上求值 QORM 的绑定/动作和 `setData`。**未实现**:如今 `onTap` 只会打印一条静态导出提示;动作在小程序中不会执行。
- **厂商配置档** —— 各厂商的能力差异(微信 / 支付宝 / 字节跳动 ……)、审核/调试约束,以及降级能力声明。**未实现。**

## 约束(厂商沙箱)

小程序宿主会限制任何框架能做的事 —— 复杂的 GPU 渲染、动态脚本、文件系统、后台任务、跨域套接字以及完整的剪贴板访问通常都受到限制。QORM 的权限策略位于该沙箱**内部**,且从不将其放宽:不受支持的能力会被拒绝,而不是被静默降级。
