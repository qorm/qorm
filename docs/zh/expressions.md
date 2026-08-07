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
| `state.computed.*` | 任意位置 | 清单里声明的派生值(见下文) |
| `computed.*` | 场景绑定、动作步骤、其它派生值内 | 同样的值,只是不带 `state.` 前缀。在动作内它是派发入口处的快照(见下文)。场景 `guard` 内**不可用** |
| *裸参数名* | 动作的 steps 内 | invoke 传入的 `args`——`{{ id }}`、`{{ text }}` |
| `prop.*` | 组件模板内 | 实例传入的属性 |
| `item`、`index`、`first`、`last` | 列表/网格的 `renderItem` 模板内,以及 `forEach` 步骤的循环体内 | 当前元素及其位置(见[第一个场景](tutorials/first-scene.md))。`as` 别名会整组改名——`"as":"row"` 得到 `row` / `rowIndex` / `rowFirst` / `rowLast` |
| `row`、`rowIndex`、`cell` | `table` / `datatable` 的单元格模板内 | 当前行、它的位置,以及 `{{ cell.value }}` / `{{ cell.column }}` / `{{ cell.index }}` |
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
| `push(list, v, …)` · `unshift(list, v, …)` | 返回**新列表**,把值追加/前置到末尾(`null` 值被丢弃) |
| `pop(list)` · `shift(list)` | 返回**新列表**,去掉末/首元素(先用 `last()` / `first()` 读出) |
| `reverse(list)` · `sort(list)` | 返回**新列表**,倒序/排序——数值升序在前,其次字符串按字典序,再是布尔,`null` 最后;同类内稳定 |
| `indexOf(list, v)` · `includes(list, v)` | `v` 的首个下标,没有则 `-1` / `v` 是否在列表内 |
| `range(start, end, step)` | 从 `start` (含)到 `end` (不含)的数字列表,步长为 `step` (默认 1);上限 2\^20 |
| `fill(n, v)` | `n` 个 `v` 的新列表 (游戏棋盘 / 状态初始化) |
| `concat(a, b, …)` | 按顺序拼接多个列表;非列表值视为单元素列表 |
| `flatten(list)` | 扁平化一层 — `[1,[2,[3]]]` → `[1,2,[3]]` |

`map`、`filter` 与双参数的 `count` 接受一个**写成字符串的子表达式**,元素绑定在 `it` 上:

```json
{ "text": "Total {{ sum(map(state.cart, \"it.price * it.qty\")) }}" }
{ "data": "{{ filter(state.todos, \"!it.done\") }}" }
{ "text": "{{ count(state.todos, \"it.done\") }} of {{ len(state.todos) }} done" }
```

主体不是列表时返回空结果而非报错,所以当某个状态键尚未加载时绑定也不会炸。

整形列表的调用(`push`、`unshift`、`pop`、`shift`、`reverse`、`sort`)是**函数式**的:
各自返回一个**新列表**,绝不修改主体——想保留结果就把它赋回去:

```json
{ "type": "state.set", "path": "items", "value": "{{ push(state.items, state.draft) }}" }
```

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
| `charAt(s, i)` | 第 `i` 个字符(按 rune),越界为 `""` |
| `substring(s, start, end)` | 按 rune 钳制的子串;负数/倒置区间像 `slice` 一样收缩(`end` 可省:到末尾) |
| `repeat(s, n)` | 重复 `n` 次;`n <= 0` 为 `""` |
| `padStart(s, n, ch)` · `padEnd(s, n, ch)` | 用 `ch`(默认 `" "`)左/右补齐到 `n` 个字符 |
| `trimStart(s)` · `trimEnd(s)` | 只去前导/尾部空白 |
| `includes(s, sub)` | 主体是字符串时为子串包含判断 |

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
| `sin(x)` · `cos(x)` · `tan(x)` | 三角函数(弧度) — 用于游戏中曲线运动 / 瞄准弹道 |
| `atan2(y, x)` | 从正 x 轴到点 (x, y) 的弧度角,范围 (-π, π] — 朝向目标瞄准 |
| `sqrt(x)` | 平方根 |
| `now()` | Unix 毫秒时间戳(唯一的非确定性内建) |

### 类型检查

| 调用 | 结果 |
|---|---|
| `typeof(x)` | `"string"` / `"number"` / `"boolean"` / `"object"` / `"array"` / `"null"` |
| `isString(x)` · `isNumber(x)` · `isBool(x)` · `isObject(x)` · `isArray(x)` · `isNull(x)` | 布尔判断 |

### JSON

| 调用 | 结果 |
|---|---|
| `jsonEncode(v)` (别名 `JSON.stringify`) | JSON 字符串 |
| `jsonDecode(s)` (别名 `JSON.parse`) | 解析后的值;格式错误或空字符串均返回 `null` |

### 音频 (仅 canvas 引擎)

| 调用 | 结果 |
|---|---|
| `playSound(src)` | 播放一次 WAV — 路径相对于应用目录 |
| `playMusic(src)` | 循环播放 WAV — 调用 `stopMusic()` 停止 |
| `stopMusic()` | 停止当前循环播放 |

### 脚本分发

| 调用 | 结果 |
|---|---|
| `call(name)` | 在当前 runtime 上调用另一个脚本 — 遵循同样的递归保护 |

未知函数名在运行时求值为 `null`——但加载器会对集合类与 `format` 调用做静态的元数与
参数类型检查,所以 `qorm run` 会在你看到空白屏幕之前就报出错误。

## 派生值(`computed`)

当同一段表达式出现在十几个绑定里时,不如在清单里声明一次。`computed` 是一个
名字 → 表达式的映射,写在 `globalState` 旁边(或嵌在它里面,序列化回写时会归一到
顶层):

```json
{
  "type": "app", "id": "cart", "entry": "main",
  "globalState": { "schema": { "items": "array" }, "initial": { "items": [] } },
  "computed": {
    "itemCount": "{{ sum(map(state.items, \"it.qty\")) }}",
    "subtotal":  "{{ sum(map(state.items, \"it.price * it.qty\")) }}",
    "shipping":  "{{ computed.subtotal >= 50 ? 0 : 5 }}",
    "total":     "{{ computed.subtotal + computed.shipping }}",
    "isEmpty":   "{{ len(state.items) == 0 }}"
  }
}
```

在场景绑定里写 `{{ state.computed.total }}`,在动作里写 `{{ computed.total }}`。
两种写法在两处都能解析(`examples/derived` 在场景里统一使用带 `state.` 前缀的
形式),但在动作内它们并不完全是同一个表达式——见下文《动作里该用哪种写法》。
派生值之间可以互相引用,且不受声明顺序限制,上面的 `shipping` 与 `total`
就是这样。

声明之后值得知道的几点:

- **每帧求值一次,而不是每次读取都求值。** 被十二个节点绑定的值只算一次,并且在
  一次派发的全过程中视图是稳定的:发布出来的值在帧边界刷新(顶层派发结束、
  `render` 步骤、进入场景),而不是每步之后都刷新。因此同一个 action 里先写
  `state.items`、**后面的步骤**再读 `{{ computed.subtotal }}`,读到的仍是派发前的值。
- **只读。** 步骤的 `path` / `result` / `error` 指向该命名空间时,加载期报错,派发期
  整步被丢弃——是整步,所以被拦下的 `http.get` 连请求都不会发出。两种写法都会被
  拒绝:`"path": "computed.total"` 与 `"path": "state.computed.total"` 一样。
  (步骤路径**本来就**相对状态根,因此任何以 `state.` 开头的路径都是笔误——写
  `"path": "state.count"` 会真的创建一个名叫 `state` 的顶层状态键。加载期会告警。)
- **成环是失效而不是致命。** 处在依赖环上(以及位于环下游)的值会在加载期被报出,
  求值结果为空;应用其余部分照常工作。
- 名字必须是普通标识符。派生表达式能看到 `state.*`、`t` 与 `viewport.*`,但**看不到**
  `route.*`,所以路由参数无法喂给它。
- 没有声明 `computed` 的应用,`computed` 仍是一个普通的、可写的状态键,以上规则
  一概不适用。

### 动作里该用哪种写法

在动作内,两种写法读到的值一直相同——**直到动作中途落下一帧**,此后两者分道扬镳:

| 写法 | 读到什么 |
|---|---|
| `{{ state.computed.total }}` | **实时**的命名空间——`render` 步骤、`delay`、异步回包都会刷新它 |
| `{{ computed.total }}` | 动作**派发时刻**命名空间里的值 |

这不是 `computed` 独有的。动作上下文只在派发入口构建一次:`state` 是实时的状态
存储,而每个同时以裸名提供的顶层状态键(`{{ count }}` 即 `{{ state.count }}`)都是
那一刻的拷贝。`{{ computed.total }}` 就是 `computed` 这个键的裸写法,行为与
`{{ count }}` 完全一致。

```json
{ "type": "state.set", "path": "items", "value": "{{ push(state.items, 1) }}" },
{ "type": "render" },
{ "type": "state.set", "path": "shown", "value": "{{ state.computed.subtotal }}" }
```

最后一行若写成 `{{ computed.subtotal }}`,拿到的会是追加之前的小计。**在中途渲染
的动作里请使用带 `state.` 前缀的写法**;直线执行的动作里裸写法没问题(而且更短),
场景绑定中两种写法则始终是最新的。

## 下一步

- [第一个场景](tutorials/first-scene.md)——列表、条目作用域与样式。
- [第一个 action](tutorials/first-action.md)——`if`、`invoke` 与 `http` 分支。
- [节点与样式属性](/api/zh/props.md)——绑定可以写入的每一个属性。
