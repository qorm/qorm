# First Platform Pack

A Platform Pack describes how QORM runs on a given platform.

## Directory

```text
platform-packs/desktop/
├─ manifest.json
├─ capabilities.json
├─ renderer.json
├─ host-adapter.json
├─ event-adapter.json
└─ skill.md
```

## manifest.json

```json
{
  "qorm": "0.1",
  "type": "platform-pack",
  "id": "desktop",
  "version": "0.1.0"
}
```

## capabilities.json

```json
{
  "network.request": {
    "supported": true,
    "permission": "network.request"
  },
  "clipboard.write": {
    "supported": true,
    "permission": "clipboard.write"
  },
  "filesystem.saveFile": {
    "supported": true,
    "permission": "filesystem.write",
    "requiresApproval": true
  }
}
```

Notes:
- The boolean `true` should only be used as shorthand for the object form.
- A production Platform Pack should prefer the object form, which makes it easy to add constraints such as `permission`, `domains`, and `requiresApproval`.

## Usage

```bash
qorm check qorm.json --target desktop
qorm build qorm.json --target desktop
```