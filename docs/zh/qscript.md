# QScript 脚本语言参考

QScript (`actions/*.qs`) 是 QORM 的脚本语言 — 一个小型、确定性、类 JavaScript
的解释型语言，用于应用和游戏逻辑。场景 JSON 声明结构和数据，而脚本负责那些本该由
Go 组件或冗长步骤列表完成的计算。

## 语句

```
let x = expr                     # 声明局部变量
x = expr                         # 为已声明的局部变量 / 参数赋值
state.a = expr                   # 写入状态路径（setPath 语义：
state.obj.k = expr               #   中间 map 会自动创建）
state.arr[i] = expr              # 写入数组元素 / map 条目
if (expr) { ... } else { ... }   # else 可选；支持 `else if` 链
for name in expr { ... }         # 遍历数组元素
while expr { ... }               # 条件括号可选
break / continue                 # 跳出 / 继续最内层循环
return expr?                     # 在 fn 内返回；在顶层结束脚本。
                                 # 裸 `return` 在 '}' 前或脚本结尾被识别；
                                 # 其他地方 `return` 之后的文本解析为返回值。
expr                             # 表达式语句（如函数调用）
```

## 函数

```
fn name(a, b, ...) { ... }
```

调用语法与内建函数一致：`name(1, state.x)`。**没有**闭包：函数只能看到
自己的参数、`let` 局部变量、`state` 和 `args` 句柄以及全局函数表 —
无法访问任何调用者的作用域。

### 共享库：`actions/lib.qs`

将函数定义（只能有函数定义 — 不能有顶层语句）放在 `actions/lib.qs` 中。
加载器将其作为 `type:"scriptlib"` 收集，运行时在每次分发时将它前置到
**每个** action 的脚本体之前。游戏的共享核心 — 物理引擎、碰撞逻辑、
敌人生成、视口渲染 — 都在这里，每个 `.qs` 文件都可以调用。

```
# actions/lib.qs
fn clamp(v, lo, hi) {
  if (v < lo) { return lo }
  if (v > hi) { return hi }
  return v
}

# actions/tick.qs
if (state.status == "playing") {
  state.player.x = clamp(state.player.x + 1, 0, state.viewW)
}
```

## 表达式

表达式与 `{{ }}` 模板语法完全一致：数字/字符串/布尔值/null 字面量、点号标识符
（`state.piece.x`）、后缀索引（`a[i]`、`users[0].name`）、一元 `!` 和 `-`、
二元 `* / % + - < <= > >= == != && ||`、三元 `?:`，以及完整的
[内建函数集](expressions.md)。脚本额外支持**数组字面量** `[e1, e2, ...]`
（模板语言中没有）。

## 注释

注释从 `#` 到行尾。没有分号；语句自定界。

## 保留字

`let if else for in while return fn break continue state args true false null nil`

## 确定性

没有任何 I/O 或外部调用面：脚本是 `(state, args)` 的纯函数。唯一的时钟是
`now()` 内建函数（Unix 毫秒时间戳）。随机性只能通过状态传入
（例如将 LCG 种子存入 `state.rng` — 参见
[`examples/tetris`](https://github.com/qorm/qorm/tree/main/examples/tetris)）。

## 管控（反失控保护）

失控脚本会降级为错误，绝不卡死：

| 限制 | 上限 | 说明 |
|---|---|---|
| 源码长度 | 64 KB | 每个 action 的总文本量 |
| 解析器深度 | 256 | 嵌套表达式深度 |
| 最大操作数 | 200 000 | 每次顶层分发，跨所有 call() 链 |
| 最大循环迭代 | 100 000 | 每次循环执行 |
| 最大调用深度 | 64 | fn 嵌套调用 |

每次违规都会返回带有脚本行号的错误 — 解释器无论输入什么都不会 panic。

## `call()` — 编排多个 action

脚本可以触发同级 action：

```
call("otherAction")
call("someAction", { key: 42 })   # 带参数
```

调用通过宿主安装的 dispatch 钩子桥接；运行时将其接到自己的 `Dispatch`，
因此 `call()` 链会重新进入正常的调用深度管控。未安装钩子时 `call()` 为空操作。

## 完整示例：物理帧

完整游戏脚本请参见 [Raiden](https://github.com/qorm/qorm/tree/main/examples/raiden)、
[Mario](https://github.com/qorm/qorm/tree/main/examples/mario) 或
[Tetris](https://github.com/qorm/qorm/tree/main/examples/tetris) 示例。
以下是 Raiden 的 `tick.qs` 节选：

```
# tick.qs — 一个物理帧。由场景的 timer 驱动。

if (state.status == "playing") {
  state.tick = state.tick + 1
  scrollStars()
  movePlayer()
  fireBullet()
  moveBullets()
  moveEnemies()
  checkHits()
  tickInvuln()

  state.spawnTimer = state.spawnTimer - 1
  if (state.spawnTimer <= 0) {
    spawnEnemy()
    state.spawnTimer = 25
  }
}
```

## 内建函数

完整的内建函数参考见 [表达式文档](expressions.md)。在模板中可用的所有函数
在 qscript 中同样可用，外加脚本专属的 `call()`。
