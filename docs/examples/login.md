# Example: Login

A styled login form — text inputs, bound state, and a submit button. Source:
[`examples/login`](https://github.com/qorm/qorm/tree/main/examples/login).

```sh
qorm run examples/login
```

## The pieces

Global state holds the form fields and status (in `qorm.json`):

```json
"globalState": {
  "schema": { "email": "string", "password": "string", "isLoggingIn": "boolean", "errorMessage": "string" },
  "initial": { "email": "", "password": "", "isLoggingIn": false, "errorMessage": "" }
}
```

Inputs bind two-way to the fields, and the submit button invokes an action with
the entered values:

```json
{ "type": "input", "id": "email", "binding": "email", "placeholder": "Email Address" }
{ "type": "button", "id": "submit", "label": "Sign In",
  "onPress": { "type": "invoke", "name": "performLogin", "args": { "email": "{{state.email}}", "password": "{{state.password}}" } } }
```

Fields also carry the browser's native input attributes — they cost nothing and
give the user immediate feedback and the right on-screen keyboard:

```json
{ "type": "input", "id": "email", "binding": "email", "placeholder": "Email Address",
  "inputMode": "email", "required": true, "autocomplete": "email", "autofocus": true }
{ "type": "input", "id": "password", "binding": "password", "placeholder": "Password",
  "required": true, "maxLength": 64, "pattern": ".{8,}", "autocomplete": "current-password" }
```

These are native constraints, not a validation engine — they cannot express
"passwords must match". But they *can* stop a submission: put the fields in a
`form` and mark the submit button `"submit": true`, and the browser's own check
gates the dispatch, showing its message bubble on the offending field instead
of running the action. A `"submit": false` button (Cancel) is never gated, and
`"novalidate": true` on the form turns the whole check off. See
[First action](../tutorials/first-action.md).

Without a `form`, or on a button that carries neither prop, nothing is gated:
the button's `onPress` dispatches from its own click handler. Bind the button's
`disabled` to a validity expression for that case, and keep the message in
state.

An error line binds to state so a failed attempt shows a message:

```json
{ "type": "text", "id": "err", "text": "{{state.errorMessage}}" }
```

This flow can be exercised against the running app with `qorm check` (layout
audit) or the agent-side `qorm_assert` / `qorm_dispatch` MCP tools — see
[verification](../verification.md).

## Format notes

- Inputs bind with `binding` (two-way); the button's `onPress` names an action
  and passes state values as args.
- Native input attributes available on `input` / `textarea` / `textformfield`:
  `required`, `maxLength`, `pattern` (input only), `inputMode`, `autofocus`,
  `readonly`, `autocomplete`.
