<!-- data-lang-nav --> [English](../expressions.md) · 中文

# 表达式

`{{ … }}` 里面是一个表达式。同一套小语言在各处通用:节点的 `text`、`style` 的取值、
`if` 条件、列表的 `data`、动作步骤的 `value`、invoke 的 `args`,以及 `http` 的 URL。

```json
{ "type": "text", "text": "{{ state.user.name }} has {{ count(state.cart) }} items" }
```

**恰好只有一个绑定**的字符串会保留原值类型——布尔仍是布尔,数字仍是数字,列表仍是列表。
文本与绑定混排的字符串则插值为字符串。当你把列表喂给 `data`、把布尔喂给 `if`、
把数字喂给组件 prop 时,这个区别很关键。

```json
{ "if": "{{ state.open }}",              "text": "open" }
{ "data": "{{ state.rows }}" }
{ "text": "Rows: {{ len(state.rows) }}" }
```

## 作用域

哪些名字能解析,取决于表达式所在的位置。

| 名字 | 可用位置 | 含义 |
|---|---|---|
| `state.*` | 任意位置 | `qorm.json` 中声明的全局状态 |
| *裸参数名* | 动作的 steps 内 | invoke 传入的 `args`——`{{ id }}`、`{{ text }}` |
| `prop.*` | 组件模板内 | 实例传入的属性 |
| `item`、`index`、`first`、`last` | 列表/网格的 `renderItem` 模板内 | 当前行及其位置(见[第一个场景](tutorials/first-scene.md)) |
| `route.*` | 任意位置 | 当前深链的路由参数 |
| `viewport.*` | 任意位置 | 当前视口,用于响应式绑定 |
| `t` | 任意位置 | i18n 查找表 |
| `response` | `http.*` 步骤的 `onSuccess` 分支内 | 解码后的响应体 |
| `error` | `http.*` 步骤的 `onError` 分支内 | 失败信息 |
| `it` | `map` / `filter` / `count` 的子表达式内 | 正在访问的元素 |

两条能省下调试时间的说明:

- 不存在的名字求值为 `null`,不是报错。`{{ state.nope }}` 渲染为空。
- 在 `map`/`filter`/`count` 的子表达式内,**`it` 是唯一可见的名字**——外层作用域被
  刻意隔离,所以 `map(state.nums, "state.x")` 得到的是一串 null,而不是一串 `state.x`。

## 字面量与运算符

字面量:数字、`'单引号'` 或 `"双引号"` 字符串、`true`、`false`、`null`。

| 类别 | 运算符 |
|---|---|
| 算术 | `+` `-` `*` `/` `%`(以及一元 `-`) |
| 比较 | `==` `!=` `<` `<=` `>` `>=` |
| 逻辑 | `&&` `\|\|` `!` |
| 条件 | `cond ? a : b` |
| 分组 | `( … )` |

**任意一侧**是字符串时 `+` 做拼接,否则做加法——`{{ 1 + 2 }}` 是 `3`,
`{{ "n=" + 2 }}` 是 `n=2`。

**真值判定**(`if`、`!`、`&&`、`||`、三元与 `filter` 都用它):
`null`、`false`、`0`、`""`、空数组、空对象为假,其余为真。

## 下标访问

后缀 `[ … ]` 读取数组元素或对象键,并可与 `.` 成员访问自由串接。

```json
{ "text": "{{ state.items[0].name }}" }
{ "text": "{{ state.grid[1][0] }}" }
{ "text": "{{ state.users[state.idx].name }}" }
{ "text": "{{ state.user['name'] }}" }
{ "text": "{{ state.user[state.key] }}" }
{ "text": "{{ split(state.csv, ',')[1] }}" }
```

下标可以是任意表达式。需要知道的规则:

- 越界、键不存在、对非集合取下标,结果都是 `null`——绝不报错。
- **负下标是 `null`**。只有 `at()` 从末尾倒数。
- 小数下标截断取整;数字字符串会被转换(`items['1']` 就是第 1 个元素)。
- 对象键会被字符串化,所以 `obj[0]` 查的是键 `"0"`。
- 字符串不可下标——`'abc'[0]` 是 `null`。请用 `split(s, '')`。

## 内置函数

### 集合

| 调用 | 结果 |
|---|---|
| `len(x)` | 列表/对象的元素个数,字符串的字符数 |
| `at(list, i)` | 第 `i` 个元素;**负数从末尾倒数**;越界为 `null` |
| `first(list)` / `last(list)` | 首/末元素,空列表为 `null` |
| `sum(list)` | 数值求和;非数值元素按 0 计;空列表为 `0` |
| `avg(list)` | 平均值;空列表为 `0`(绝不 `NaN`) |
| `count(list)` | 元素个数;`count(list, "谓词")` 统计满足条件的元素 |
| `keys(obj)` / `values(obj)` | 键按字典序排序,值按同一顺序排列 |
| `map(list, "表达式")` | 每个元素经子表达式映射 |
| `filter(list, "表达式")` | 子表达式为真的元素 |
| `slice(list, lo, hi)` | 子区间,边界自动钳制;区间倒置则为空 |

`map`、`filter` 与双参数的 `count` 接受一个**写成字符串的子表达式**,元素绑定在 `it` 上:

```json
{ "text": "Total {{ sum(map(state.cart, \"it.price * it.qty\")) }}" }
{ "data": "{{ filter(state.todos, \"!it.done\") }}" }
{ "text": "{{ count(state.todos, \"it.done\") }} of {{ len(state.todos) }} done" }
```

主体不是列表时返回空结果而非报错,所以当某个状态键尚未加载时绑定也不会炸。

### 字符串

| 调用 | 结果 |
|---|---|
| `str(x)` | 把任意值字符串化 |
| `trim(s)` · `upper(s)` · `lower(s)` | 去空白 / 转大写 / 转小写 |
| `contains(s, sub)` · `startsWith(s, p)` · `endsWith(s, p)` | 布尔判断 |
| `replace(s, old, new)` | 替换全部出现 |
| `matches(s, regexp)` | 正则匹配;非法模式返回 `false` |
| `split(s, sep)` | 拆分为列表;`sep` 为空则按字符拆分;主体为空得 `[]` |
| `join(list, sep)` | 连接元素;`null` 元素连接为空串 |
| `format(pattern, …)` | `%s`、`%d`、`%f`、`%.Nf`、`%%`;未知的动词原样输出 |

```json
{ "text": "{{ format('%s scored %.1f%%', state.name, state.pct) }}" }
{ "text": "{{ join(map(state.tags, \"upper(it)\"), ' · ') }}" }
```

### 数值与逻辑

| 调用 | 结果 |
|---|---|
| `number(x)`(别名 `num`)· `int(x)` | 数值转换;`int` 截断取整 |
| `abs(x)` · `round(x)` · `floor(x)` · `ceil(x)` | 常规数学 |
| `min(a, b, …)` · `max(a, b, …)` | 在参数间取最小/最大 |
| `not(x)` · `empty(x)` | 对真值判定取反 |
| `default(x, fallback)`(别名 `coalesce`) | `x` 为真时取 `x`,否则取 `fallback` |

未知函数名在运行时求值为 `null`——但加载器会对集合类与 `format` 调用做静态的元数与
参数类型检查,所以 `qorm run` 会在你看到空白屏幕之前就报出错误。

## 下一步

- [第一个场景](tutorials/first-scene.md)——列表、条目作用域与样式。
- [第一个 action](tutorials/first-action.md)——`if`、`invoke` 与 `http` 分支。
- [节点与样式属性](/api/zh/props.md)——绑定可以写入的每一个属性。
