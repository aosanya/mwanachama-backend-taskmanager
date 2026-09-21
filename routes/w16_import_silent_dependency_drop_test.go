package routes_test

import (
	"strings"
	"testing"
)

// TestImportProject_RejectsUnresolvedDependency covers board row W16: a
// depends_on that names no task in the import document refuses the whole
// import (400, naming the reference) up front, instead of being silently
// dropped from an otherwise "successful" import.
func TestImportProject_RejectsUnresolvedDependency(t *testing.T) {
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
	if resp.Code != 400 {
		t.Fatalf("want 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "does-not-exist") {
		t.Fatalf("error should name the unresolved reference, got %s", resp.Body.String())
	}
}
