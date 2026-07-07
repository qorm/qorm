# QORM 官方站点与开发者门户规划

## 目标

QORM 需要一个统一的官方站点与开发者门户，用于承载文档、示例、交互式 playground、版本导航和生态入口。

该站点不是营销站点优先项目，而是开发者基础设施的一部分，目标是降低学习成本、提升试用效率，并为后续生态分发和社区资产引流提供稳定入口。

## 主要受众

- 初次了解 QORM 的开发者
- 需要查阅规格与架构文档的实现者
- 需要试跑示例、验证行为的产品与设计协作者
- 需要发布或引入第三方组件/样式/插件的生态贡献者
- 需要查看兼容性、变更日志和迁移指南的集成方

## 非目标

- 不在第一阶段建设复杂 CMS
- 不把 playground 做成完整在线 IDE
- 不在早期承担账号体系和团队协作编辑
- 不将站点作为 Runtime 正式执行环境的唯一入口

## 门户构成

### 1. 官方首页

承载内容：
- QORM 定位
- 核心能力概览
- 架构图入口
- 文档入口
- Playground 入口
- 生态入口
- 版本与发布信息入口

### 2. 文档站 (Docs Portal)

承载内容：
- 规格文档
- 规划文档
- 教程
- 示例
- ADR
- 迁移指南
- 版本切换
- 全文搜索

文档站应直接消费仓库中的文档源，避免维护第二套文档内容。

### 3. Playground

承载内容：
- 最小 scene 编辑与预览
- 示例一键加载
- Patch 预览
- 平台兼容性检查结果展示
- 运行日志 / diagnostics 面板

约束：
- Playground 是受限沙盒，不直接开放危险 Host Capability
- 默认只允许 mock / preview 级能力
- 若需要演示 `network.request` 等能力，应使用受控 mock adapter

### 4. 示例与组件画廊

承载内容：
- 官方示例
- Render Profile 示例
- 平台差异示例
- 第三方组件 / 样式展示入口

### 5. 生态入口 (Ecosystem Portal)

承载内容：
- 第三方组件、样式、插件目录
- 包详情页
- 版本、依赖、兼容性信息
- 信任状态 / 审核状态 / 签名状态
- 发布文档与接入说明

## 信息架构

建议一级导航：

```text
Overview
Docs
Specs
Examples
Playground
Ecosystem
Releases
```

建议站内关键横向入口：
- 按 Platform 浏览
- 按 Render Profile 浏览
- 按 Integration Mode 浏览
- 按 Host Capability 浏览

## 技术架构

### 内容来源

- Docs 直接来自仓库 `docs/`
- 示例来自仓库 `docs/examples/` 或未来 `examples/`
- 规格元数据来自 schema / spec 索引
- 生态信息来自 Registry API

### 服务组成

```text
site frontend
docs content pipeline
search indexer
playground backend or static sandbox host
registry metadata client
release metadata feed
```

### Playground 运行模型

建议分阶段：

#### Phase A
- 静态示例浏览
- 只读 JSON / 只读 Patch 预览
- 本地或 WASM 预览

#### Phase B
- 在线编辑 scene/style/action
- 受控 mock host
- diagnostics / explain / layout debug

#### Phase C
- 生态包导入演示
- 多版本兼容检查
- shareable playground links

## 与现有仓库的关系

官方站点不应发明新的文档源或新的示例格式。

站点应复用：
- 现有 planning/spec/tutorial/example 文档
- 概念边界规范
- 发布与兼容信息
- 生态 Registry 元数据

## 关键能力需求

- 版本化文档
- 全文搜索
- 深链接到 spec section
- 示例实时预览
- Release Notes 聚合
- 包详情与依赖可视化
- 平台 / profile / capability 过滤浏览

## 运营与发布流程

- 文档发布应绑定仓库版本或主分支快照
- Playground 示例应与相同版本的 schema / runtime 保持一致
- 生态入口展示的数据应来自 Registry，而不是手写目录
- Release 发布后应自动同步站点版本入口与迁移提示

## 分阶段实施

### Phase 1
- 官方首页
- 文档站
- 版本导航
- 搜索
- 示例浏览

### Phase 2
- Playground MVP
- diagnostics 面板
- 规范示例一键加载

### Phase 3
- Ecosystem Portal 接入
- 包详情页
- 兼容性与信任状态展示

### Phase 4
- 分享链接
- 版本对比
- 迁移助手入口

## 验收标准

```text
开发者能从首页快速进入 docs / examples / playground
规格、教程、规划文档有统一搜索入口
示例可在 playground 中加载并预览
生态包可在门户中按类型、版本、信任状态浏览
发布新版本后文档与 release 信息可同步更新
```

## 风险与注意点

- 文档站与仓库文档若分离，会迅速产生双写漂移
- Playground 若过早支持真实外部能力，会扩大安全面
- Ecosystem Portal 若先于 Registry 规范完成，数据模型会返工
- 站点搜索、版本、示例和生态页必须共享统一元数据来源
