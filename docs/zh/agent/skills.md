<!-- data-lang-nav --> [English](../../agent/skills.md) · 中文

# QORM 技能（Skills）

QORM 技能是面向 AI 智能体的实战指令集：它编码了可运行的应用格式、MCP 工具面，
以及智能体可靠完成 QORM 任务所需的验证闭环。技能随仓库发布于
[`integrations/skill/`](https://github.com/qorm/qorm/tree/main/integrations/skill)，
由智能体在会话开始时加载。

## 已发布的技能

QORM 发布 **一份综合技能**：[`integrations/skill/SKILL.md`](https://github.com/qorm/qorm/blob/main/integrations/skill/SKILL.md)。
它是单个 markdown 文件，带有兼容 Claude Code 的 frontmatter
（`name: qorm` + 触发描述），正文按以下章节组织。加载方式：让智能体直接读取该文件
（或经由仓库的 [`llms.txt`](https://github.com/qorm/qorm/blob/main/llms.txt)，其中已链接该技能）。

| 章节 | 教会智能体什么 |
|---|---|
| 编写可运行格式 | 运行时今天接受的精确 JSON：清单、可选的 `qorm.config.json` 宿主窗口块（尺寸/标题/resizable/chromeless/transparent → 异形窗口）、`text` + `{{ state.x }}` 绑定、`richtext` spans、`video`、`path`、`onPress`、动作步骤、组件 —— 以及哪些"规格格式"不能用。 |
| 标准动作模式 | 加载态、错误处理、乐观更新、表单校验、分页 —— 均为可直接加载的形态，配方见 `docs/tutorials/first-action.md`。 |
| 通过 MCP 驱动应用 | 全部 **25 个 MCP 工具**按用途分组（理解 / 操作 / 窗口 / 设计 / 验证 / 模拟），与 [mcp-tools.md](mcp-tools.md) 一致。 |
| 永远自我验证 | 何时用 `qorm_measure` / `qorm_check_layout`（实时宿主）、何时用 CLI `qorm measure` / `qorm check`（无头）；像素证据用 `qorm_capture_canvas` / `qorm shot`。 |
| 交付 | `qorm run` / `render` / `build` / `verify` / `package` 与签名流程。 |
| 保持 QORM 更新 | 开工前先执行 `qorm update` 的纪律。 |
| Canvas 引擎 | macOS 默认 canvas 窗口 vs `-tags desktop` WebView；布局、交互、共享活动流、全部 146 种组件。 |
| 声明式交互与 canvas 特效 | 任意节点可用的样式键全表：pressed/hover、过渡、FLIP、变换、滤镜、遮罩、fx/timeline、stagger。 |
| 横版卷轴与瓦片世界 | `board` 相机属性 + `tilemap` 烘焙；游戏运动规则。 |
| 样式系统 | QSS 层叠、主题变量、渐变、108 个已知样式键。 |
| 脚本动作（qscript） | `let`/`if`/`for`/`while`/`fn`、状态写入、音频内置函数。 |
| 文本输入编辑 | canvas 的选择/剪贴板/撤销/IME 行为。 |
| 图标字体 | 内置 53 个图标名称。 |
| 禁止事项 | 硬性规则：不用"理想规格"格式、无 preview token 不得 apply、不用 emoji、不得跳过验证。 |

## 技能编码的工作流

这一份技能文件编码了以下可复用工作流；每个工作流都点名所用 MCP 工具及调用顺序。

### 场景创作闭环

脚手架 → 接线 → 验证。

1. 读取 `qorm_inspect` 输出（状态模式、场景、动作、设计令牌）。
2. 使用[组件目录](/api/widgets.md)中的规范组件名编写场景 JSON；用 `{{ state.x }}` 绑定状态。
3. 通过 `onPress` 动作名或 `actions/*.json` 步骤接入行为。
4. 用 `qorm_validate` 校验；每次编辑后运行 `qorm_check_layout`。

### 布局调试闭环

1. `qorm_measure` —— 每个节点的渲染 x、y、w、h 与计算样式。
2. 定位溢出（`x-overflow`）、裁剪（盒子越出其 `scroll` 视口）、overlay 的 z-index 叠放。
3. `qorm_preview_patch` → `qorm_diff` → `qorm_apply_patch` 修复。
4. 重新测量确认。

### 评审绑定的补丁闭环

`qorm_query`（定位）→ `qorm_preview_patch`（安全副本 + `previewToken`）→
`qorm_diff`（结构评审）→ `qorm_apply_patch`（提交，与 token 绑定）→
出错时 `qorm_undo`。每次提交都绑定于一次先行评审。

### 共享会话协作闭环

`qorm_activity`（人类刚做了什么；`inflight: 0` = 已静默）→
`qorm_dispatch` / `qorm_set_state`（操作）→ `qorm_render_html`
（读取守卫解析后的结果）。

## 技能文件的结构（格式）

一份 QORM 技能文件遵循如下构造：

```text
frontmatter        — 名称 + 描述（智能体触发条件）
编写格式            — 可运行的 JSON，含反模式
工具分组            — 使用哪些 MCP 工具、以何种顺序
工作流              — 上述编号闭环
禁止行为            — 绝不做的事
验证                — 如何用渲染现实证明一次编辑
```

## 技能如何保持同步

技能只引用自动生成的、以代码为准的参考面 —— 组件目录（`api/widgets.md`）、
MCP 工具列表（`docs/agent/mcp-tools.md`，由 `internal/mcp/tools.go` 生成）、
能力矩阵（`docs/platforms/capabilities.md`）。QORM 变化时，更新后
（`qorm update`）重新阅读 `AGENTS.md` 与技能文件；技能本身与其所描述的特性
在同一提交中更新。
