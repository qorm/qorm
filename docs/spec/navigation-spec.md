# Navigation &amp; routing

How a QORM app moves between scenes, passes data across that move, and where
that data lives relative to the rest of the app's state.

## The scene stack

A running app shows exactly one scene at a time. Which scene is a property of
the live runtime, not of the app definition — the manifest only names the
`entry` scene the app opens on. Everything after that is driven by `navigate`
action steps.

The runtime keeps a **back stack** of the scenes you came from. Navigating
forward *pushes* the current scene onto the stack and shows the target;
navigating back *pops* the top of the stack and returns to it.

```
entry: home
  home                      stack: []
  → navigate to profile     stack: [home]        showing: profile
  → navigate to settings    stack: [home, profile] showing: settings
  → back                    stack: [home]        showing: profile
  → back                    stack: []            showing: home
  → back                    stack: []            showing: home   (no-op)
```

Popping an empty stack is a no-op, so a hardware/back button on the entry scene
never dead-ends the app. Navigating to the scene already shown, or to an unknown
scene id, is ignored.

### Navigating

A `navigate` step targets a scene by id (`to`) or pops the stack (`back: true`):

```json
{ "type": "action", "id": "openProfile",
  "steps": [ { "type": "navigate", "to": "profile" } ] }

{ "type": "action", "id": "back",
  "steps": [ { "type": "navigate", "back": true } ] }
```

`to` may itself be a binding — `"to": "{{ state.nextScene }}"` — so a single
action can route dynamically.

### Page transitions

Each navigation records a **direction** — `push` on a forward navigate, `pop` on
a back navigate. The client reads this once per frame (it is cleared after it
ships) to play the matching page transition: a forward push slides the new scene
in from the trailing edge, a pop slides back the other way. Direction is purely
presentational; it never affects state.

## Navigation parameters — `route.*`

A navigate step can carry **route parameters**: named values computed at dispatch
time and attached to the target scene. The target scene reads them through the
`route.*` namespace, alongside `state.*`, `viewport.*` and `t.*`.

Declare them under `params` (parameter name → value expression):

```json
{ "type": "navigate", "to": "profile",
  "params": { "userId": "{{ userId }}", "name": "{{ name }}" } }
```

Each expression is evaluated once, in the action's context, so it can read the
action's invocation args (as above), `state.*`, or anything else in scope. The
resulting typed values become the target scene's route.

The target scene binds them with `{{ route.<name> }}`:

```json
{ "type": "text", "text": "{{ route.name }}" }
{ "type": "text", "text": "User id: {{ route.userId }}" }
```

A missing key resolves to nil (and renders as empty text), so a scene reached
without a given parameter degrades cleanly rather than erroring.

### Parameters travel with the stack

Route parameters are **frame-local**: they belong to the specific stack frame
that showed the scene, not to the scene id. When you navigate forward, the
current scene *and its current route* are pushed together; when you navigate
back, both are restored. So returning from a detail screen puts the previous
screen back exactly as it was, parameters included.

```
home  (route: {})              → openProfile(userId=u-101)
profile  (route: {userId:u-101})   → openProfile(userId=u-102)   [drill-down]
profile  (route: {userId:u-102})   → back
profile  (route: {userId:u-101})   ← the earlier frame's route is restored
home  (route: {})              ← back again restores the empty entry route
```

The entry scene starts with an empty route (`{}`, never nil).

## Scene-local route vs. global state

QORM has two distinct places to keep data, and navigation is where the line
between them matters most:

| | `globalState` (`state.*`) | route params (`route.*`) |
|---|---|---|
| Scope | One store shared by **every** scene | The **current stack frame** only |
| Lifetime | The whole app session | While that frame is on the stack |
| Written by | `state.*` action steps, `http.*` results | A `navigate` step's `params` |
| Read as | `{{ state.x }}` | `{{ route.x }}` |
| Declared in | `qorm.json` `globalState.schema` | Ad hoc per navigation |

Use **global state** for data that outlives a single screen or is shared across
screens — the signed-in user, a cart, a cached list, the current theme/locale.
Use **route params** for the small identifiers that say *which* instance of a
screen this is — the `userId` a profile screen is showing, the order id a detail
screen opened for. A route parameter is the QORM analogue of a function argument:
it is how the caller tells the destination screen what to render, without
mutating shared state that other screens can see.

Rule of thumb: if navigating back should forget it, it is a route parameter; if
it should persist, it belongs in global state.

## URL routing (design convention — future track)

The model above maps directly onto conventional URL routing, and QORM's route
vocabulary is chosen so it can grow into it:

```
/profile/u-101?tab=activity
   │       │        └── query string  → route.tab
   │       └── path parameter          → route.userId
   └── scene id                        → navigate to "profile"
```

Under this convention a scene id is a path segment, path/query parameters are
route params, and the browser history stack mirrors the scene back stack — so
the browser Back button and a deep-linked URL both fall out of the same model.

**This is a documented direction, not the current behaviour.** Today the stack
and its parameters live entirely in memory on the runtime; navigation does not
read or write `window.location`, and reloading the page returns to the entry
scene. Wiring the in-memory stack to real URLs (path/query encoding, history
sync, deep-link entry) is a separate, later track. Authoring against `route.*`
now is forward-compatible: apps that pass identifiers as route params rather than
stashing them in global state will map onto URL routing unchanged when it lands.
