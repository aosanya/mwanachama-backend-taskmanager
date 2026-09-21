package routes_test

import (
	"strings"
	"testing"
)

// TestImportProject_RejectsDuplicateShortKey covers board row W17: two tasks
// whose names collide once task_prefix is trimmed refuse the import (400,
// naming the reference) instead of letting the second silently take over the
// first's dependency edges.
func TestImportProject_RejectsDuplicateShortKey(t *testing.T) {
	tm := wk14Manager(t, "w17imp")
	mux := wk14Mux(tm)

	doc := `{
		"project": "Dup Key Probe",
		"task_prefix": "PP-",
		"tasks": [
			{"name": "PP-1", "title": "Task One (wanted target)"},
			{"name": "PP-1", "title": "Task Two (should be separate)"},
			{"name": "PP-2", "title": "Depends on PP-1", "depends_on": ["1"]}
		]
	}`
	resp := wk14Do(t, mux, "POST", "/projects/import", wk14JSON(t, map[string]any{"document": doc}))
	if resp.Code != 400 {
		t.Fatalf("want 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "both resolve to reference") {
		t.Fatalf("error should name the colliding reference, got %s", resp.Body.String())
	}
}
