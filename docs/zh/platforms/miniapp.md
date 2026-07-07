<!-- data-lang-nav --> [English](../../platforms/miniapp.md) · 中文

# QORM 小程序平台

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

## 接下来(基础层尚未包含)

- **完整交互** —— 一个 JS 解释器,用于求值 QORM 的绑定/动作和 `setData`,使状态变更在设备上重新渲染(如今 `onTap` 只是一个占位实现)。
- **厂商配置档** —— 各厂商的能力差异(微信 / 支付宝 / 字节跳动 ……)、审核/调试约束,以及降级能力声明。

## 约束(厂商沙箱)

小程序宿主会限制任何框架能做的事 —— 复杂的 GPU 渲染、动态脚本、文件系统、后台任务、跨域套接字以及完整的剪贴板访问通常都受到限制。QORM 的权限策略位于该沙箱**内部**,且从不将其放宽:不受支持的能力会被拒绝,而不是被静默降级。
