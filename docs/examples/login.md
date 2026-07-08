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

An error line binds to state so a failed attempt shows a message:

```json
{ "type": "text", "id": "err", "text": "{{state.errorMessage}}" }
```

The login flow is exercised by [`login.test.json`](https://github.com/qorm/qorm/blob/main/examples/login/login.test.json)
(a `type: test` fixture the loader skips at run time but the harness runs).

## Format notes

- Inputs bind with `binding` (two-way); the button's `onPress` names an action
  and passes state values as args.
