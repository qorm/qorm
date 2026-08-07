# QScript language reference

QScript (`actions/*.qs`) is the scripting language of QORM — a small,
deterministic, JS-lite for app and game logic. Scene JSON declares structure
and data; a script action carries the logic that would otherwise need a Go
component or long step lists.

## Statements

```
let x = expr                     # declare a local
x = expr                         # assign a declared local / parameter
state.a = expr                   # write a state path (setPath semantics:
state.obj.k = expr               #   intermediate maps are created)
state.arr[i] = expr              # write one array element / map entry
if (expr) { ... } else { ... }   # else is optional; `else if` chains
for name in expr { ... }         # iterate the elements of an array
while expr { ... }               # (parens around the condition optional)
break / continue                 # leave / re-iterate the innermost loop
return expr?                     # inside fn; at top level ends the script.
                                 # A bare `return` is recognised before '}'
                                 # or end of script; anywhere else the text
                                 # after it is parsed as the return value.
expr                             # expression statement (e.g. a call)
```

## Functions

```
fn name(a, b, ...) { ... }
```

Calls look like builtins: `name(1, state.x)`. There are **no closures**: a
function sees its parameters, its own `let` locals, the `state` and `args`
handles, and the global function table — nothing from any caller's scope.

### The shared library: `actions/lib.qs`

Place fn definitions (and only fn definitions — no top-level statements) in
`actions/lib.qs`. The loader collects it as `type:"scriptlib"` and the
runtime prepends it to **every** action body at dispatch. This is where your
game's shared core lives — physics, collision, spawn logic, viewport
rendering — callable from any `.qs` file.

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

## Expressions

Expressions mirror the `{{ }}` binding language exactly: number/string/bool/null
literals, dotted identifiers (`state.piece.x`), postfix indexing (`a[i]`,
`users[0].name`), unary `!` and `-`, binary `* / % + - < <= > >= == != && ||`,
the ternary `?:`, and the full [builtin function set](expressions.md).
Scripts add **array literals** `[e1, e2, ...]` (which the binding language
does not have).

## Comments

Comments run from `#` to end of line. There are no semicolons; statements
are self-delimiting.

## Reserved words

`let if else for in while return fn break continue state args true false null nil`

## Determinism

There is no I/O and no external call surface: a script is a pure function
of `(state, args)`. The only clock is the explicit `now()` builtin (Unix ms).
Randomness enters only through state (e.g. an LCG kept in `state.rng` — see
[`examples/tetris`](https://github.com/qorm/qorm/tree/main/examples/tetris)).

## Governance

A runaway script degrades to an error, never a hang:

| limit | cap | description |
|---|---|---|
| source length | 64 KB | total source text per action |
| parser depth | 256 | nested expression depth |
| max ops | 200 000 | per top-level dispatch, across all call()-chains |
| max loop iters | 100 000 | per single loop execution |
| max call depth | 64 | fn → fn nesting |

Every violation returns an error carrying the script line number — the
interpreter never panics, whatever the input.

## `call()` — composing actions

A script may fire a sibling action:

```
call("otherAction")
call("someAction", { key: 42 })   # with args
```

The call is bridged through a host dispatch hook; the runtime wires it to
its own `Dispatch`, so `call()` chains re-enter the normal invoke-depth
governance. Without a hook installed, `call()` is a no-op.

## Full example: a physics tick

See the [Raiden](https://github.com/qorm/qorm/tree/main/examples/raiden),
[Mario](https://github.com/qorm/qorm/tree/main/examples/mario), or
[Tetris](https://github.com/qorm/qorm/tree/main/examples/tetris) examples
for complete game scripts. Below is an excerpt from Raiden's `tick.qs`:

```
# tick.qs — one physics frame. The scene's timer drives it.

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

## Builtin functions

See [Expressions](expressions.md) for the complete builtin reference. Every
function usable inside `{{ }}` bindings is also callable from qscript, plus
the script-exclusive `call()`.
