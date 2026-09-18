package routes_test

import (
	"encoding/json"
	"testing"
)

// TestImportProject_PinsDuplicateShortKeySilentlyMisattributesDependency
// pins board row W17: import.go's runImport builds idMap[shortKey] = t.ID
// with no duplicate check, so a second task in the same import document
// whose name (after trimming task_prefix) collides with an earlier task's
// silently overwrites that earlier task's entry in idMap. Any depends_on
// reference naming that shortKey then resolves to whichever task was
// created LAST, not the one actually intended, and the import still
// reports full success with no error and no distinguishing signal.
//
// Driven through the real routes.Routes(tm) mux's POST /projects/import:
// a document with two tasks both named "PP-1" ("Task One"/"Task Two")
// plus a third task depending on "1". The response is 201 with
// tasks_created=3, deps_created=1 (the dependency edge really was
// created) — but the edge points at Task Two's id, not Task One's, with
// nothing in the response revealing the collision. A duplicated task name
// (a copy-paste mistake in a hand-written or generated import document,
// not a contrived input) silently rewires which task a dependency
// actually targets.
//
// Once W17 is fixed (most likely: fail the import with ErrInvalidImport
// naming the duplicate task name, matching validateImportDoc's existing
// up-front structural checks), this test's assertion must invert — either
// the request should be refused, or the two tasks should be
// distinguishable and the dependency should resolve unambiguously.
func TestImportProject_PinsDuplicateShortKeySilentlyMisattributesDependency(t *testing.T) {
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
	if resp.Code != 201 {
		t.Fatalf("want 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var parsed struct {
		TasksCreated int `json:"tasks_created"`
		DepsCreated  int `json:"deps_created"`
		Tasks        []struct {
			ID       string `json:"id"`
			TaskName string `json:"task_name"`
			Title    string `json:"title"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.TasksCreated != 3 {
		t.Fatalf("want tasks_created=3, got %d", parsed.TasksCreated)
	}
	if parsed.DepsCreated != 1 {
		t.Fatalf("want deps_created=1, got %d", parsed.DepsCreated)
	}

	var taskTwoID string
	for _, task := range parsed.Tasks {
		if task.Title == "Task Two (should be separate)" {
			taskTwoID = task.ID
		}
	}
	if taskTwoID == "" {
		t.Fatalf("could not find Task Two in response tasks: %+v", parsed.Tasks)
	}

	traverseResp := wk14Do(t, mux, "GET", "/relationships/traverse?vertex_id="+taskTwoID+"&label=depends_on&direction=inbound", nil)
	if traverseResp.Code != 200 {
		t.Fatalf("traverse status = %d, body %s", traverseResp.Code, traverseResp.Body.String())
	}
	var rels []map[string]any
	if err := json.Unmarshal(traverseResp.Body.Bytes(), &rels); err != nil {
		t.Fatalf("unmarshal traverse: %v", err)
	}

	// PINS THE BUG: the dependency edge resolved to Task Two (created
	// last), not Task One (the shortKey's first, presumably intended,
	// occupant) — if this now fails, W17 is fixed.
	if len(rels) != 1 {
		t.Fatalf("W17 appears fixed: expected the dependency edge to land on Task Two (last writer wins), got %d edges pointing at it — update this test to match the fixed behavior", len(rels))
	}
	t.Logf("pinned duplicate-shortkey misattribution: dependency edge silently resolved to Task Two (%s), not Task One, with zero errors reported", taskTwoID)
}
