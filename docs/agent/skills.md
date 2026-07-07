# QORM Skills

A Skill is a workflow description for agents to use. QORM should provide reusable Skills for different tasks.

## Skill Types

```text
scene-authoring
layout-debugging
agent-patch
platform-porting
motion-design
host-capability-check
mobile-adaptation
```

## Basic Skill Structure

```text
Goal
Applicable scope
Input files
Recommended tools
Steps
Prohibited actions
Output format
Permission requirements
```

## scene-authoring

Purpose: let the agent create or modify scene JSON.

Rules:

- The JSON must remain valid.
- The `type` field must be used to distinguish file semantics.
- Host Capabilities must not be added out of nowhere.
- After modification, validate is required.

## layout-debugging

Purpose: analyze layout anomalies.

Steps:

```text
inspect_scene
layout_debug
Inspect LayoutSpec
Inspect text measurement
Inspect safe area / scroll / absolute
preview_patch
```
