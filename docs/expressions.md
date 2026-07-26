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
| *bare argument names* | an action's steps | the `args` the invoke passed in — `{{ id }}`, `{{ text }}` |
| `prop.*` | a component template | the properties the instance passed in |
| `item`, `index`, `first`, `last` | a list/gridview `renderItem` template | the current row and its position (see [First Scene](tutorials/first-scene.md)) |
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

An unknown function name evaluates to `null` at runtime — but the loader
statically checks arity and argument types for the collection and `format`
calls, so `qorm run` reports the mistake before you see a blank screen.

## Where to go next

- [First scene](tutorials/first-scene.md) — lists, item scope, and styling.
- [First action](tutorials/first-action.md) — `if`, `invoke`, and `http` branches.
- [Node & style props](/api/props.md) — every prop a binding can be written into.
