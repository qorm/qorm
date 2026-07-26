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

## URL routing (implemented)

The in-memory scene stack is mirrored into the browser address bar, so a
deep-linked URL and the browser Back/Forward buttons both fall out of the same
model. A URL encodes the current scene and its route params as a query string:

```
/?scene=profile&userId=u-101&name=Ada
   │       │        └────┬─────┘
   │       │             └── route params  → route.userId, route.name
   │       └── scene id                     → the "profile" scene
   └── the entry scene is just "/"
```

The rules:

- **`RoutePath`** — the runtime renders the current scene + route params as this
  path (the entry scene with no params is `/`; keys are sorted for stability). It
  is the single source of truth for what the address bar should show.
- **Deep-link entry** — loading `/?scene=<id>&k=v` navigates the runtime to that
  scene *before* the first render, so the page opens straight into it with the
  query parameters bound to `route.*`. An unknown scene id is ignored and the app
  falls back to the entry scene.
- **Address-bar sync** — every navigation ships its `RoutePath` to the client
  (the `X-Qorm-Route` header on the `/event` response and a `route` field on the
  SSE/poll payload). The client `history.pushState`s it when it changes, so the
  URL follows navigation without a reload.
- **Back / Forward** — a browser history move reports the target URL's
  scene + params to the server (`POST /navigate`, human-token-gated like
  `/event`), which drives the runtime to match. Returning to the previous frame
  pops the stack (restoring its params); going forward pushes.

Route param values that arrive from a URL are strings (a query string is
untyped), so a scene reached by deep link sees `route.userId` as `"u-101"`.
Because a route parameter is the QORM analogue of a function argument, apps that
pass identifiers as route params rather than stashing them in global state get
shareable, reloadable deep links for free.

## Route guards

Once a URL can address any scene, "only signed-in users see this screen" stops
being something a navigating action can enforce — a guard has to live on the
destination. A scene declares its own precondition next to `root`:

```json
{
  "type": "scene",
  "id": "checkout",
  "guard": {
    "condition": "{{ state.computed.signedIn }}",
    "redirect": "signin",
    "params": { "next": "{{ 'checkout' }}" }
  },
  "root": { "type": "column", "id": "root", "children": [] }
}
```

- **`condition`** is required — a `{{ … }}` expression over the scene scope
  (`state.*`, `state.computed.*`, `t.*`, `viewport.*`, `route.*`). Truthy means
  enter. A guard with no `condition` is dropped with a load-time error rather
  than silently letting everyone through.
- **`redirect`** is the scene id a failing guard sends you to instead. It is a
  literal id, not a binding. Without one the navigation is simply **refused** —
  you stay where you are. A refusal is fail-CLOSED everywhere, including at the
  entry scene: see *When there is nowhere to go* below.
- **`params`** are evaluated when the guard fires, in the context of the scene
  you are leaving, and become the target's `{{ route.* }}` — the usual way to
  carry a "come back here afterwards" hint. The refused navigation's own params
  do not travel with the redirect.

### Where it applies

The guard runs on **every** entry path: an action's `navigate` step, a browser
Back/Forward move, a deep link straight into the scene, `navigate` with
`back: true`, and the initial entry scene at first render. So a protected route
cannot be reached by spelling a URL, and the check lives in one place instead of
being repeated in every action that navigates.

It runs **before** the scene's `onEnter`, so a hook that fetches private data
never fires for a visitor who was refused the page.

Two details about the back stack:

- A redirect out of the **entry scene** *replaces* rather than pushes, so Back
  cannot return to a scene you were denied.
- A refusal on a *navigation* never touches the back stack at all.

### When there is nowhere to go

A guard can refuse **outright** rather than redirect: it names no `redirect`,
its redirects form a cycle, or the chain runs past the hop cap. On a navigation
that means "stay where you are", which is safe — where you are is a scene you
were already allowed.

At **first render** it cannot mean that, because where you are IS the refused
scene. So the runtime leaves, in this order:

1. the nearest frame on the back stack the guards still admit (frames it walks
   past are dropped, so one Back cannot walk into the refusal again);
2. failing that, the entry scene;
3. failing that — the entry scene is itself the refused one and there is no
   history — the session is **blocked**: nothing renders, and the URL stays `/`.

The refused scene's `onEnter` never runs in any of the three, and the scene
actually entered runs its own.

### Back is an entry, not an undo

`navigate` with `back: true` is guarded like everything else. The frame was
pushed when you were allowed there; by the time you return the permission may be
gone (a token expired, a sign-out earlier in the same action), and Back must not
be the one door nobody checks.

A frame whose guard now redirects is followed to its redirect target. A frame
refused outright is skipped and the unwind continues into the frames below it —
"go back" is your intent to leave the current scene, and the answer to "you may
not enter that" is the next place you may enter. If no frame at all may be
entered, the stack is left untouched and you stay put.

Guards chain: the redirect target is itself guarded. A chain is capped at 8
hops, and a chain that revisits a scene refuses the navigation instead of
looping — the loader reports a redirect that points at its own scene or at an
unknown scene as an error, and flags a redirect cycle.

### Guards and derived values

A guard evaluates derived values freshly, for itself only. That matters for the
most common shape there is — an action that signs the user in and then
navigates:

```json
{ "type": "action", "id": "signIn", "steps": [
  { "type": "state.set", "path": "user", "value": "{{ draftName }}" },
  { "type": "navigate",  "to": "checkout" }
] }
```

Derived values are otherwise frozen for the whole dispatch (see
[Expressions](../expressions.md)), so a guard reading `{{ state.computed.signedIn }}`
would decide on the pre-login view and bounce the user straight back. The
private refresh is what makes the two steps above work. It is not published:
steps *after* the `navigate` still read the frame-stable view, as they do
everywhere else.

`examples/derived` runs this end to end — a checkout scene guarded on a derived
`signedIn`, redirecting to a sign-in scene that reads `{{ route.next }}`.
