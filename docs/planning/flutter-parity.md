# QORM → Flutter Parity — Implementation Plan

**North star — human-AI collaboration.** Everything in QORM is judged by one
goal: a human and an AI operating the *same* live app session together. So every
feature ships on the **web path** (browser + SSE + MCP shared session — the only
stack that supports it), every interactive widget is verified through that live
session (agent dispatch/set_state → human sees it), and cross-cutting features
(theming, responsive PC/mobile, i18n) must be drivable by the agent at runtime.

**Goal (in service of the above).** Cover Flutter's widget catalog — components,
properties, actions — in QORM's language-neutral JSON on the web renderer.

**How each Flutter concept maps to QORM.**
- *Widget* → a node `type` in scene JSON, drawn to HTML/CSS by `internal/render`.
- *Property* → a node prop/style key (bindable via `{{ … }}` expressions).
- *Action / callback* (onPressed, onChanged, validator, …) → an `Invoke`
  dispatched to a declarative `state.*` action step, or a reactive expression.
- *State* → the global state store; two-way-bound inputs fold values back.


**Design language — Apple / iOS (Cupertino) is authoritative.** For any component that exists in both Material and Cupertino,
the Apple version is the standard: the default look uses the SF font, iOS
system colors (blue #007AFF, green #34C759, grouped bg #F2F2F7), iOS controls
(pill switch, segmented control, translucent nav bar), and iOS metrics. This
is a cross-cutting rule applied to every widget, plus a dedicated Cupertino
widget set (P6 below, promoted in priority).

**Scope note.** A few Flutter widgets are imperative/GPU concepts with no
declarative-server-render analog (CustomPaint, raw AnimationController,
Future/StreamBuilder, low-level slivers). These are marked **N/A** or
*approximated*; everything else is in scope.

Legend: [done] done · [partial] partial (works, missing props/variants) · [todo] todo · N/A.

---

## Progress snapshot (52 commits)

**~66 widget types, 10 action steps.** Layout, text, the full forms/controls
set, data-display, feedback, overlay, navigation, and the first gesture/motion
widgets are in. Every widget has a render/coverage test; interactive ones are
verified through the live MCP/SSE session. Baseline was ~50; this plan tracks the
climb to Flutter parity.

---

## Catalog & status (by Flutter category)

### 1. Layout
| Flutter | QORM | status |
|---|---|---|
| Container, Padding, SizedBox, Center, Align | row/column + style (padding/margin/size/align) | [done] |
| Row, Column, Stack, Positioned | row, column, stack, absolute | [done] |
| Wrap | wrap | [done] |
| Expanded, Flexible, Spacer | flexGrow / `width:fill`, spacer | [done] |
| Card, Divider, VerticalDivider | card, divider, verticaldivider | [done] |
| GridView (count/extent) | gridview | [done] |
| AspectRatio, FractionallySizedBox, ConstrainedBox | style aspectRatio/min/max | [partial] |
| FittedBox, IntrinsicHeight/Width, Baseline, Offstage, LimitedBox | — | [todo] |
| Table (column widths) | table | [partial] |
| IndexedStack, Flow, LayoutBuilder | — | [todo] / N/A |

### 2. Scrolling
| ListView (+builder, virtualized) | list | [done] |
| SingleChildScrollView | scroll (container) | [done] |
| PageView | pageview | [done] |
| Scrollbar | (native) | [partial] |
| RefreshIndicator | — | [todo] |
| ReorderableListView | — | [todo] |
| DraggableScrollableSheet, NestedScrollView, CustomScrollView/slivers | — | [todo] / N/A |

### 3. Text
| Text, DefaultTextStyle | text + textCSS | [done] |
| RichText / Text.rich (spans) | — | [todo] |
| SelectableText | — | [todo] |

### 4. Forms & input
| TextField, TextFormField (InputDecoration, validator) | input, textarea, textformfield | [done] |
| Form (aggregate validate/save) | — | [todo] |
| Checkbox, Radio, Switch, Slider | checkbox, radio, switch, slider | [done] |
| RangeSlider | rangeslider | [done] |
| Checkbox/Radio/SwitchListTile | *listtile variants | [done] |
| DropdownButton(FormField) | select, dropdownbutton | [done] |
| Autocomplete | autocomplete | [done] |
| Chip family (Input/Choice/Filter/Action) | chip/inputchip/choicechip/filterchip | [done] |
| SegmentedButton, ToggleButtons | segmented | [partial] |
| DatePicker, TimePicker, CalendarDatePicker | input[type=date/time] | [partial] (dialog picker [todo]) |

### 5. Buttons
| Elevated/Filled/Text/Outlined/Icon Button | button + `variant` | [done] |
| FloatingActionButton (+extended) | fab | [done] |
| PopupMenuButton | menu, dropdownbutton | [partial] |
| BackButton, CloseButton | — | [todo] (trivial) |

### 6. Material components
| Scaffold, AppBar | scaffold, appbar | [done] |
| BottomNavigationBar, NavigationBar | bottomnav | [done] |
| NavigationRail, NavigationDrawer, Drawer, BottomAppBar | drawer | [partial] (rail/rail [todo]) |
| TabBar, TabBarView | tabs | [done] |
| ListTile, ExpansionTile | listtile, expansiontile | [done] |
| ExpansionPanelList | accordion | [done] |
| DataTable (sort) | table (sortable) | [partial] (row-select [todo]) |
| PaginatedDataTable | table + pagination | [partial] |
| Stepper | materialstepper | [done] |
| Tooltip, Badge (on child) | tooltip, badge | [done] |
| SnackBar, MaterialBanner | snackbar, alert | [done] |
| BottomSheet, Dialog (Alert/Simple) | modal, alertdialog, actionsheet | [done] |
| Progress (Circular/Linear) | circularprogress, progress, spinner | [done] |
| Menu (MenuAnchor/MenuBar), SearchBar/SearchAnchor | menu, autocomplete | [partial] |

### 7. Media / assets / icons
| Image, Icon, CircleAvatar | image, icon, avatar | [done] |
| Placeholder, FlutterLogo | skeleton | [partial] |

### 8. Painting & effects
| Opacity, DecoratedBox, ClipRRect/Oval, Transform, RotatedBox | style (opacity/radius/…); transform [todo] | [partial] |
| BackdropFilter, ShaderMask, ColorFiltered, CustomPaint | — | [todo] / N/A |

### 9. Animation & motion
| AnimatedContainer/Opacity/Padding/Positioned/Align | — (CSS transition base) | [todo] |
| AnimatedSwitcher, AnimatedCrossFade, Hero | — | [todo] |
| Fade/Scale/Slide/Rotation/SizeTransition | — | [todo] |
| AnimatedList, Dismissible | dismissible | [partial] |

### 10. Interaction & gestures
| GestureDetector, InkWell (tap/double/long) | gesturedetector/inkwell | [done] |
| Draggable, DragTarget, LongPressDraggable | — | [todo] |
| Dismissible | — | [todo] |
| IgnorePointer, AbsorbPointer, MouseRegion, Focus | — | [todo] / N/A |

### 11. Async / builders
| FutureBuilder, StreamBuilder, ValueListenableBuilder | — | N/A (server-driven state instead) |

### 12. Cupertino (iOS) — Phase 6
| CupertinoButton/NavigationBar/TabBar/TextField/Switch/Slider | — | [todo] |
| CupertinoActionSheet/AlertDialog/Picker | actionsheet, alertdialog, picker | [done] (ContextMenu/DatePicker [todo]) |
| CupertinoListSection/ListTile/SlidingSegmentedControl/ActivityIndicator | listsection, listtile, segmented, activityindicator | [done] |

---

## Roadmap (priority order)

Each phase is a set of tasks; **one task = one widget** → implement (web) → add a
real case to `examples/components` → integration test asserting render markers
(+ live MCP/SSE check for interactive ones) → `go test ./...` green → commit.

- **P4 — finish data & scrolling** *(next)*: DataTable row-selection + select-all;
  Dismissible (swipe); ReorderableListView; RefreshIndicator; Badge-on-child;
  VerticalDivider.
- **P5 — motion**: AnimatedContainer & implicit animations (CSS transitions on
  style deltas); AnimatedSwitcher/CrossFade; Hero (shared-element via FLIP);
  Fade/Scale/Slide/Size transitions; AnimatedList.
- **P6 — layout completeness**: FittedBox, IntrinsicHeight/Width, AspectRatio
  node, FractionallySizedBox, LimitedBox, Offstage, VerticalDivider, Table
  column-widths; Transform/RotatedBox/Opacity as nodes.
- **P7 — forms completeness**: `form` (aggregate validate/save/reset);
  SegmentedButton/ToggleButtons multi; DatePicker/TimePicker dialog;
  DropdownButtonFormField decoration; SearchAnchor.
- **P8 — navigation completeness**: NavigationRail, NavigationDrawer, BottomSheet
  as a first-class widget, AlertDialog/SimpleDialog, PopupMenuButton with items,
  BackButton/CloseButton, PaginatedDataTable.
- **P9 — text & rich**: RichText/spans, SelectableText, DefaultTextStyle scope.
- **P10 — gestures & DnD**: Draggable/DragTarget, LongPressDraggable, reorder.
- **P11 — Cupertino set** (iOS look): full parallel widget family.
- **Cross-cutting, every phase**: fill [partial] property gaps (variants, decorations)
  for widgets already present; keep a `properties` matrix per widget.

## Done (P1–P3 + extras)

P1 Wrap/ListTile/AppBar/FAB/button-variants · P2 Scaffold/BottomNav/SnackBar/
ExpansionTile · P3 Switch/Checkbox/RadioListTile/RangeSlider · Chips ·
GridView/MaterialStepper · PageView/DropdownButton · GestureDetector/Autocomplete
· TextFormField(validation)/CircularProgress.

## Acceptance & quality gate (applies to every task)

- render/coverage test asserting the widget's HTML markers;
- interactive widgets: verified through the live MCP/SSE session (agent
  dispatch/set_state → human page updates);
- `go test ./...` (10 packages) green; `go vet` + `gofmt` clean;
- pure-Go 6-platform cross-compile intact;
- one commit per task, message naming the Flutter widget(s).

## Loop (autonomous)

analyze status → pick next task from the current phase → implement → run
acceptance → **pass:** commit + next task · **fail:** find the cause, try another
approach · **all approaches fail:** ask the human. Progress is measured by widgets
moved to [done] and the shrinking [partial]/[todo] set above.
