---
title: Expressions
description: The {{ … }} expression language QORM binds with — scopes, operators, index access, and the full builtin function set.
---

<!-- data-lang-nav --><p align="right"><b>English</b> · <a href="zh/expressions.md">中文</a></p>

# Expressions

Anything inside `{{ … }}` is an expression. The same little language is used
everywhere: a node's `text`, a `style` value, an `if` condition, a list's
`data`, an action step's `value`, an invoke's `args`, and an `http` URL.

```json
{ "type": "text", "text": "{{ state.user.name }} has {{ count(state.cart) }} items" }
```

A string that is **exactly one** binding keeps the value's type — a boolean
stays a boolean, a number stays a number, a list stays a list. A string that
mixes text and bindings interpolates to a string. That distinction matters
when you feed a list into `data`, a boolean into `if`, or a number into a
component prop.

```json
{ "if": "{{ state.open }}",              "text": "open" }
{ "data": "{{ state.rows }}" }
{ "text": "Rows: {{ len(state.rows) }}" }
```

## Scopes

Which names resolve depends on where the expression sits.

| Name | Available in | What it is |
|---|---|---|
| `state.*` | everywhere | the global state store declared in `qorm.json` |
| `state.computed.*` | everywhere | the manifest's derived values (see below) |
| `computed.*` | scene bindings, action steps, other derived values | the same values, without the `state.` prefix. Inside an action it is a dispatch-entry snapshot (see below). **Not** available inside a scene `guard` |
| *bare argument names* | an action's steps | the `args` the invoke passed in — `{{ id }}`, `{{ text }}` |
| `prop.*` | a component template | the properties the instance passed in |
| `item`, `index`, `first`, `last` | a list/gridview `renderItem` template, and a `forEach` step's body | the current element and its position (see [First Scene](tutorials/first-scene.md)). An `as` alias renames the whole set — `"as":"row"` gives `row` / `rowIndex` / `rowFirst` / `rowLast` |
| `row`, `rowIndex`, `cell` | a `table` / `datatable` cell template | the current row, its position, and `{{ cell.value }}` / `{{ cell.column }}` / `{{ cell.index }}` |
| `route.*` | everywhere | route parameters of the current deep link |
| `viewport.*` | everywhere | the current viewport, for responsive bindings |
| `t` | everywhere | the i18n lookup table |
| `response` | an `http.*` step's `onSuccess` branch | the decoded response body |
| `error` | an `http.*` step's `onError` branch | the failure message |
| `it` | a `map` / `filter` / `count` sub-expression | the element being visited |

Two notes that save debugging time:

- A missing name is `null`, not an error. `{{ state.nope }}` renders empty.
- Inside a `map`/`filter`/`count` sub-expression, **`it` is the only visible
  name** — the surrounding scope is deliberately hidden, so
  `map(state.nums, "state.x")` yields a list of nulls, not a list of `state.x`.

## Literals and operators

Literals: numbers, `'single'` or `"double"` quoted strings, `true`, `false`,
`null`.

| Group | Operators |
|---|---|
| arithmetic | `+` `-` `*` `/` `%` (and unary `-`) |
| comparison | `==` `!=` `<` `<=` `>` `>=` |
| logical | `&&` `\|\|` `!` |
| conditional | `cond ? a : b` |
| grouping | `( … )` |

`+` concatenates when **either** side is a string, and adds otherwise —
`{{ 1 + 2 }}` is `3`, `{{ "n=" + 2 }}` is `n=2`.

**Truthiness** (used by `if`, `!`, `&&`, `||`, the ternary, and `filter`):
`null`, `false`, `0`, `""`, an empty array and an empty object are falsy.
Everything else is truthy.

## Index access

Postfix `[ … ]` reads an array element or an object key, and chains freely
with `.` member access.

```json
{ "text": "{{ state.items[0].name }}" }
{ "text": "{{ state.grid[1][0] }}" }
{ "text": "{{ state.users[state.idx].name }}" }
{ "text": "{{ state.user['name'] }}" }
{ "text": "{{ state.user[state.key] }}" }
{ "text": "{{ split(state.csv, ',')[1] }}" }
```

The index may be any expression. Rules worth knowing:

- Out of range, a missing key, or indexing a non-collection is `null` — never
  an error.
- A **negative index is `null`**. Only `at()` counts from the end.
- A fractional index truncates; a numeric string coerces (`items['1']` is
  element 1).
- An object key is stringified, so `obj[0]` looks up the key `"0"`.
- Strings are not indexable — `'abc'[0]` is `null`. Use `split(s, '')`.

## Builtin functions

### Collections

| Call | Result |
|---|---|
| `len(x)` | element count of a list/object, rune count of a string |
| `at(list, i)` | element `i`; **negative counts from the end**; out of range is `null` |
| `first(list)` / `last(list)` | first / last element, or `null` when empty |
| `sum(list)` | numeric sum; non-numeric elements count as 0; empty is `0` |
| `avg(list)` | mean; empty is `0` (never `NaN`) |
| `count(list)` | element count; `count(list, "predicate")` counts matching elements |
| `keys(obj)` / `values(obj)` | keys sorted lexically, values in that same key order |
| `map(list, "expr")` | each element mapped through the sub-expression |
| `filter(list, "expr")` | elements whose sub-expression is truthy |
| `slice(list, lo, hi)` | sub-range, bounds-clamped; an inverted range is empty |

`map`, `filter` and the two-argument `count` take a **sub-expression written
as a string**, with the element bound to `it`:

```json
{ "text": "Total {{ sum(map(state.cart, \"it.price * it.qty\")) }}" }
{ "data": "{{ filter(state.todos, \"!it.done\") }}" }
{ "text": "{{ count(state.todos, \"it.done\") }} of {{ len(state.todos) }} done" }
```

A non-list subject yields the empty result rather than an error, so a binding
never blows up on a state key that has not loaded yet.

### Strings

| Call | Result |
|---|---|
| `str(x)` | stringify any value |
| `trim(s)` · `upper(s)` · `lower(s)` | whitespace-trimmed / upper / lower |
| `contains(s, sub)` · `startsWith(s, p)` · `endsWith(s, p)` | boolean tests |
| `replace(s, old, new)` | replace every occurrence |
| `matches(s, regexp)` | regular-expression test; an invalid pattern is `false` |
| `split(s, sep)` | split into a list; an empty `sep` splits into runes; an empty subject is `[]` |
| `join(list, sep)` | join elements; a `null` element joins as empty |
| `format(pattern, …)` | `%s`, `%d`, `%f`, `%.Nf`, `%%`; an unknown verb passes through literally |

```json
{ "text": "{{ format('%s scored %.1f%%', state.name, state.pct) }}" }
{ "text": "{{ join(map(state.tags, \"upper(it)\"), ' · ') }}" }
```

### Numbers and logic

| Call | Result |
|---|---|
| `number(x)` (alias `num`) · `int(x)` | numeric coercion; `int` truncates |
| `abs(x)` · `round(x)` · `floor(x)` · `ceil(x)` | the usual |
| `min(a, b, …)` · `max(a, b, …)` | over the arguments |
| `not(x)` · `empty(x)` | logical negation of truthiness |
| `default(x, fallback)` (alias `coalesce`) | `x` when truthy, otherwise `fallback` |
| `now()` | Unix time in milliseconds (the one non-deterministic builtin) |

An unknown function name evaluates to `null` at runtime — but the loader
statically checks arity and argument types for the collection and `format`
calls, so `qorm run` reports the mistake before you see a blank screen.

## Derived values (`computed`)

When the same expression appears in a dozen bindings, declare it once in the
manifest instead. `computed` is a map of name → expression, written beside
`globalState` (or nested inside it, which normalises to the top level on a
round trip):

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

Read them as `{{ state.computed.total }}` in a scene binding and
`{{ computed.total }}` inside an action. Both spellings resolve in both places
(`examples/derived` uses the `state.`-rooted form in scenes), but they are not
quite the same expression inside an action — see *Which spelling to use in an
action* below. A derived value may read other derived values in any declaration
order, as `shipping` and `total` do above.

What a declaration is worth knowing about:

- **Evaluated once per frame, not once per read.** A value bound by twelve
  nodes is computed once, and the view is stable for the whole of a dispatch:
  the published values refresh at frame boundaries (the end of a top-level
  dispatch, a `render` step, a scene entry), not after each step. So a step
  that writes `state.items` and a *later step in the same action* that reads
  `{{ computed.subtotal }}` still sees the pre-dispatch value.
- **Read-only.** An action step whose `path` / `result` / `error` targets the
  namespace is a load-time error and is dropped at dispatch — the whole step,
  so a gated `http.get` never even issues its request. Both spellings are
  refused: `"path": "computed.total"` and `"path": "state.computed.total"`
  alike. (A step path is *already* relative to the state root, so any
  `state.`-prefixed path is a mistake — writing `"path": "state.count"` would
  create a top-level state key literally named `state`. The loader warns.)
- **A cycle is inert, not fatal.** Values on a dependency cycle (and anything
  downstream of one) are reported at load time and simply evaluate to nothing;
  the rest of the app still works.
- Names must be plain identifiers. A derived expression sees `state.*`, `t` and
  `viewport.*` — but **not** `route.*`, so route parameters cannot feed one.
- An app that declares no `computed` keeps `computed` as an ordinary, writable
  state key; none of the above applies to it.

### Which spelling to use in an action

Inside an action the two spellings read the same value *until a frame lands
mid-action*, and then they diverge:

| Spelling | What it reads |
|---|---|
| `{{ state.computed.total }}` | the **live** namespace — refreshed by a `render` step, a `delay`, or an async reply |
| `{{ computed.total }}` | the value the namespace held when the action was **dispatched** |

That is not special to `computed`. An action's context is built once, at
dispatch entry: `state` is the live store, while every top-level state key
*also* offered under its bare name (`{{ count }}` for `{{ state.count }}`) is a
copy taken at that moment. `{{ computed.total }}` is that bare spelling of the
`computed` key, so it behaves exactly like `{{ count }}` does.

```json
{ "type": "state.set", "path": "items", "value": "{{ append(state.items, 1) }}" },
{ "type": "render" },
{ "type": "state.set", "path": "shown", "value": "{{ state.computed.subtotal }}" }
```

Reading `{{ computed.subtotal }}` on that last line would show the subtotal from
before the append. **Use the `state.`-rooted spelling in an action that renders
mid-flight**; the bare one is fine (and shorter) in a straight-line action, and
in scene bindings both are always current.

## Where to go next

- [First scene](tutorials/first-scene.md) — lists, item scope, and styling.
- [First action](tutorials/first-action.md) — `if`, `invoke`, and `http` branches.
- [Node & style props](/api/props.md) — every prop a binding can be written into.
