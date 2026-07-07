# QORM IR Specification

QORM IR 是 JSON 源文件解析、引用解析和语义校验后的内部中间表示。Runtime、Layout、Render、Host、Agent、MCP、SDK 都围绕 IR 工作。

> **实现说明**：本文中的 `struct` 代码块是**语言中立的 IR schema 记法**（沿用早期 Rust 风格书写），不代表运行时语言。QORM 运行时为**纯 Go**（`github.com/qorm/qorm`），实际类型见 `internal/model`（`model.App` / `model.Node` 等）与 [Go 架构](../overview/go-architecture.md)。

## IR 设计目标

- 平台无关。
- Agent 可查询。
- 可序列化。
- 可 Patch。
- 可诊断。
- 可生成 Execution Plan。
- 可映射回源文件位置。

## 顶层结构

```text
pub struct AppIr {
    pub id: AppId,
    pub version: QormVersion,
    pub entry: SceneId,
    pub scenes: SceneStore,
    pub components: ComponentStore,
    pub styles: StyleStore,
    pub actions: ActionStore,
    pub motions: MotionStore,
    pub resources: ResourceStore,
    pub platforms: PlatformStore,
    pub capabilities: CapabilityRequirements,
    pub source_map: SourceMap,
}
```

## Scene IR

```text
pub struct SceneIr {
    pub id: SceneId,
    pub root: NodeId,
    pub nodes: NodeStore,
    pub state_schema: StateSchema,
    pub initial_state: ValueMap,
    pub event_graph: EventGraph,
}
```

## Node IR

```text
pub struct NodeIr {
    pub id: NodeId,
    pub kind: NodeKind,
    pub semantic: Option<SemanticRole>,
    pub props: Props,
    pub layout: LayoutIr,
    pub style: StyleRefIr,
    pub bindings: Vec<BindingIr>,
    pub events: EventMapIr,
    pub children: Vec<NodeId>,
    pub source_ref: SourceRef,
}
```

## Layout IR

Layout IR 不直接等于 CSS。它表示 QORM 自己的跨平台布局模型。

```text
pub struct LayoutIr {
    pub mode: LayoutMode,
    pub width: SizeValue,
    pub height: SizeValue,
    pub min_width: Option<SizeValue>,
    pub max_width: Option<SizeValue>,
    pub padding: EdgeValues,
    pub margin: EdgeValues,
    pub gap: GapValue,
    pub align: Align,
    pub justify: Justify,
    pub position: PositionIr,
    pub overflow: Overflow,
}
```

## Style IR

```text
pub struct StyleIr {
    pub tokens: TokenTable,
    pub variants: VariantTable,
    pub state_styles: StateStyleTable,
    pub resolved_styles: ResolvedStyleCache,
}
```

## State Schema IR

```text
pub struct StateSchema {
    pub fields: BTreeMap<String, StateType>,
    pub additional_properties: bool,
}

pub enum StateType {
    Any,
    String,
    Number,
    Boolean,
    Null,
    Object(BTreeMap<String, StateType>),
    Array(Box<StateType>),
}
```

- `initial_state` 必须满足 `state_schema`。
- `additional_properties: false` 时，不允许在运行时写入未声明顶层字段。
- V1 中可允许 `Any` 作为过渡类型，但应尽量显式声明。

## Expression IR

```text
pub type ExprId = u32;

pub enum ExprIr {
    Literal(Value),
    Path(StatePath),
    Unary {
        op: UnaryOp,
        expr: ExprId,
    },
    Binary {
        left: ExprId,
        op: BinaryOp,
        right: ExprId,
    },
    NullCoalesce {
        left: ExprId,
        right: ExprId,
    },
    Template {
        parts: Vec<TemplatePartIr>,
    },
}

pub enum TemplatePartIr {
    Text(String),
    Expr(ExprId),
}
```

`ExprIr` 只表示纯表达式，不允许任意函数调用或副作用节点。

## Binding IR

```text
pub struct BindingIr {
    pub field: BindingField,
    pub expr: ExprId,
    pub dependencies: Vec<StatePath>,
    pub mode: BindingMode,
}

pub enum BindingMode {
    TypedValue,
    TemplateString,
    BooleanGuard,
}
```

- `TypedValue` 对应整字段表达式。
- `TemplateString` 对应字符串模板插值。
- `BooleanGuard` 对应 `visibleWhen`、`disabledWhen`、`condition.when` 等布尔上下文。

## Path Types

```text
pub struct StatePath {
    pub segments: Vec<PathSegment>,
}

pub enum PathSegment {
    Key(String),
    Index(usize),
}

pub enum NodeSelector {
    Node(NodeId),
    Semantic(SemanticRole),
    SelfNode,
}
```

- `StatePath` 对应运行时状态访问路径。
- `NodeSelector` 用于 `motion.play` 等运行时目标选择。

## ValueExpr

```text
pub enum ValueExpr {
    Literal(Value),
    Expr(ExprId),
    Object(BTreeMap<String, ValueExpr>),
    Array(Vec<ValueExpr>),
}
```

`ValueExpr` 用于 `host.call.input`、`state.append.value` 等可能嵌套表达式的结构化值。

## Event IR

```text
pub struct EventIr {
    pub name: EventName,
    pub handlers: Vec<ActionStepIr>,
    pub intent: Option<IntentName>,
}
```

## Action IR

```text
pub enum ActionStepIr {
    StateSet {
        path: StatePath,
        value: ValueExpr,
    },
    StateToggle {
        path: StatePath,
    },
    StateAppend {
        path: StatePath,
        value: ValueExpr,
    },
    StateRemove {
        path: StatePath,
        at: Option<ExprId>,
    },
    StateUpdate {
        path: StatePath,
        value: ValueExpr,
    },
    ActionCall {
        action: ActionId,
    },
    MotionPlay {
        target: NodeSelector,
        motion: MotionId,
    },
    HostCall {
        capability: CapabilityId,
        input: ValueExpr,
        output: Option<StatePath>,
    },
    PluginCall {
        plugin: PluginId,
        operation: String,
        input: ValueExpr,
    },
    NativeCall {
        operation: String,
        input: ValueExpr,
    },
    NavigationGo {
        target: ValueExpr,
    },
    ToastShow {
        message: ValueExpr,
    },
    ModalOpen {
        target: ValueExpr,
    },
    ModalClose {
        target: Option<ValueExpr>,
    },
    EventEmit {
        name: String,
        payload: Option<ValueExpr>,
    },
    Condition {
        when: ExprId,
        then_steps: Vec<ActionStepIr>,
        else_steps: Vec<ActionStepIr>,
    },
    Delay {
        ms: u32,
    },
    Batch {
        steps: Vec<ActionStepIr>,
    },
}
```

`ActionCall` 在解析后仍保留逻辑 action ID；Execution Plan 可以选择在运行前将其内联展开。

## Motion IR

```text
pub struct MotionIr {
    pub id: MotionId,
    pub from: Option<MotionProps>,
    pub to: Option<MotionProps>,
    pub timeline: Vec<MotionFrame>,
    pub duration_ms: Option<u32>,
    pub easing: Option<Easing>,
    pub affects_layout: bool,
    pub fallback: Option<Box<MotionIr>>,
}

pub struct MotionFrame {
    pub at_ms: u32,
    pub props: MotionProps,
    pub easing: Option<Easing>,
}

pub struct MotionProps {
    pub values: BTreeMap<MotionProperty, Value>,
}
```

- `from` / `to` 可用于简单 tween。
- `timeline` 用于关键帧动效。
- `fallback` 用于平台不支持特定属性时降级。

## Render IR

Render IR 不等于 Display List。Render IR 是节点渲染意图；Display List 是最终绘制命令。

```text
pub struct RenderIr {
    pub profile: RenderProfile,
    pub display_nodes: Vec<DisplayNodeIr>,
    pub external_surfaces: Vec<ExternalSurfaceIr>,
}
```

## Reference Resolution Output

Resolver 输出必须满足：

- 文件路径引用已解析并去重。
- 逻辑引用已规范化为稳定 ID。
- 未解析引用以诊断形式保留，不能进入可执行 IR。
- 组件实例、样式引用、动作引用、动效引用都在 IR 中以逻辑 ID 表示。

## Source Map

每个 IR 节点必须可以映射回源 JSON：

```text
pub struct SourceRef {
    pub file: SourceFileId,
    pub json_pointer: String,
    pub range: Option<TextRange>,
}
```

Source Map 用于：

- 编辑器诊断。
- Agent patch 定位。
- 错误解释。
- 格式化。
- 回滚。

注意：`json_pointer` 用于回源定位，不等于运行时 Patch Path。

## IR 与 Bundle

生产 Bundle 可直接包含 IR 或 IR 的序列化形式。App 内部解释器加载 Bundle 后不需要重新解析所有源文件，只需要校验和构建运行态对象。