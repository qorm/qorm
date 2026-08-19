// Package testrunner implements the headless `qorm test` runner (Phase 9 of
// planning/spec/test-runner-spec.md, MVP). It loads a QORM app from a
// directory, executes the app's declarative `type:"test"` JSON documents
// against a fresh runtime each, and reports the spec's minimal JSON report.
// Discovery takes ANY document with type:"test" anywhere under the app
// directory (the loader's walk, with its usual skip rules); the canonical
// location is tests/*.json. Exit semantics are delegated to the CLI: exit 0
// when every test passed, exit 1 when any test failed or errored.
//
// # Test document shape (MVP surface of the spec)
//
//	{
//	  "qorm": "0.1",          // format version, optional ("0.1" accepted)
//	  "type": "test",         // required — anything else is rejected
//	  "id": "counter_increment",
//	  "scene": "main",        // scene under test ("scene://main" accepted);
//	                          // defaults to the manifest entry scene
//	  "steps": [...],         // executed in order against one fresh runtime
//	  "assert": [...]         // run after the steps; "asserts" is an alias
//	}
//
// Steps (MVP): mount_scene, simulate_event, set_state.
//
//	mount_scene   {"type":"mount_scene","scene":"other"}  — navigates the
//	              runtime to the named scene and fires its onEnter; the
//	              document-level "scene" field is mounted implicitly first.
//	simulate_event — two spellings:
//	              {"type":"simulate_event","action":"increment","args":{...}}
//	                dispatches the named action directly (args evaluated in
//	                scene context, like onPress handlers).
//	              {"type":"simulate_event","target":{"id":"btn_x"},"event":"press"}
//	                finds the node by selector in the materialized scene tree
//	                and dispatches the handler bound to that event
//	                (press/submit→onPress, change/input→onChange,
//	                keydown/keyup, hoverin/hoverout, touchstart/touchmove/
//	                touchend), with args re-evaluated against current state.
//	set_state     {"type":"set_state","path":"count","value":5} — writes the
//	              runtime state store (refusing the computed namespace, like
//	              every other write path).
//
// Asserts (MVP): state_equals, node_exists, node_not_exists, text_equals,
// prop_equals.
//
//	state_equals   {"type":"state_equals","path":"count","value":2}
//	node_exists    {"type":"node_exists","target":{"id":"btn_increment"}}
//	node_not_exists same shape, inverse
//	text_equals    {"type":"text_equals","target":{"id":"number"},"value":"2"}
//	prop_equals    {"type":"prop_equals","target":{"id":"btn_theme"},
//	                "prop":"label","value":"Toggle Theme (apple-light)"}
//
// Target selectors follow the spec's Query Selector rules: only the id/type/
// text fields are matched (ANDed when several are given), against the
// materialized tree of the current scene — the tree the renderer shows, with
// bindings evaluated and conditional visibility (if/visible/show, `when`)
// applied. A selector carrying `path`, `within`, `match` or any other key is
// refused with query_invalid_selector (path selects on state, so the spec
// points those reads at state_equals). `semantic` is deferred: the model has
// no semantic-tag slot yet. List and gridview renderItem templates are
// expanded once per data item; JSON-component instance expansion is still
// deferred.
//
// # Error codes
//
// Codes named in the spec: test_scene_not_found, test_assertion_failed,
// query_invalid_selector, test_query_ambiguous, test_runtime_error. MVP
// additions (documented here; the spec's code list is updated on its next
// revision):
//
//	test_load_error        the app directory could not be loaded
//	test_none_found        no test documents found under the app directory
//	test_doc_invalid       a test document is malformed or not type:"test"
//	test_step_unknown      unknown step type
//	test_assert_unknown    unknown assert type
//	test_target_not_found  simulate_event target matched no node
//	test_event_unknown     simulate_event event name is not a supported event
//	test_event_not_handled the target node declares no handler for the event
//	test_action_not_found  simulate_event named an action the app lacks
//
// # Deferred (MVP boundary — see also the spec status header)
//
// advance_time / flush_async (delay and async http.* steps run synchronously
// in the MVP — the runtime degrades every pending step to the sync path when
// no Async sink is installed, so chains settle without a clock); apply_patch;
// global_equals / diagnostic_contains / host_called / host_not_called, plus
// the per-failure diagnostics snapshot (each failure already names the
// failed assertion, actual vs expected, and the target selector or path); the
// host-mock registry and its spec error codes test_mock_missing /
// test_host_call_unmocked / test_host_call_unmatched (a test document's
// "mocks" field is accepted, warned about, and ignored — host capabilities
// execute UNMOCKED in the MVP); the spec's --target / --report CLI flags (the
// CLI rejects any "-"-prefixed argument as an unsupported flag).
package testrunner
