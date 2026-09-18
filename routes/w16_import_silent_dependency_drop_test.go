package routes_test

import (
	"encoding/json"
	"testing"
)

// TestImportProject_PinsSilentlyDroppedUnresolvedDependency pins board row
// W16: import.go's runImport (the loop building depends_on edges, around
// "toID, ok := idMap[depShortID]; if !ok { continue }") silently skips any
// depends_on reference that doesn't resolve to a task in the same import
// document, instead of failing the import or surfacing a warning. Driven
// through the real routes.Routes(tm) mux's POST /projects/import: an
// import document with two tasks, where the second task's depends_on lists
// one real reference ("1") and one unresolvable one ("does-not-exist").
// The import reports 201 success with tasks_created=2 but deps_created=1 —
// the unresolved dependency is dropped with no error anywhere in the
// response or in ProgressSteps. A caller who typos a depends_on reference,
// or references a task under the wrong prefix, gets a project that looks
// like a complete, successful import while missing an edge their document
// explicitly asked for. Once W16 is fixed (most likely: fail the import
// with ErrInvalidImport naming the unresolved reference, matching
// validateImportDoc's up-front structural checks), this test's assertions
// must invert — either the request should be refused, or the response
// should surface the drop.
func TestImportProject_PinsSilentlyDroppedUnresolvedDependency(t *testing.T) {
	tm := wk14Manager(t, "w16imp")
	mux := wk14Mux(tm)

	doc := `{
		"project": "Probe Project",
		"task_prefix": "PP-",
		"tasks": [
			{"name": "PP-1", "title": "First task"},
			{"name": "PP-2", "title": "Second task", "depends_on": ["1", "does-not-exist"]}
		]
	}`
	resp := wk14Do(t, mux, "POST", "/projects/import", wk14JSON(t, map[string]any{"document": doc}))
	if resp.Code != 201 {
		t.Fatalf("want 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var parsed struct {
		TasksCreated int `json:"tasks_created"`
		DepsCreated  int `json:"deps_created"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.TasksCreated != 2 {
		t.Fatalf("want tasks_created=2, got %d", parsed.TasksCreated)
	}

	// PINS THE BUG: the document names 2 depends_on entries; only the
	// resolvable one is written, and the import still reports success.
	if parsed.DepsCreated != 1 {
		t.Fatalf("W16 appears fixed: deps_created=%d, want 1 to pin the known silent-drop bug — once fixed, update this test to assert the request is refused (or the drop is surfaced) instead", parsed.DepsCreated)
	}
	t.Logf("pinned silent dependency drop: tasks_created=%d deps_created=%d (document asked for 2 depends_on edges, only 1 resolvable)", parsed.TasksCreated, parsed.DepsCreated)
}
