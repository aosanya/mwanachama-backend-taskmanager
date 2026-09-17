package routes_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

// wk12Manager/wk12Mux/wk12JSON mirror w11_caller_supplied_id_test.go's own
// local real-mux harness, duplicated so this file stands alone as the
// reproduction for board row W12.
func wk12Manager(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	tables := mwanachamataskmanager.DefaultTableNames("wk12")
	if err := mwanachamataskmanager.Migrate(db, tables); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, tables, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr
}

func wk12Mux(tm mwanachamataskmanager.TaskManager) *http.ServeMux {
	m := http.NewServeMux()
	for _, rt := range routes.Routes(tm) {
		m.HandleFunc(rt.Pattern(""), rt.Handler)
	}
	return m
}

func wk12JSON(t *testing.T, v map[string]any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(b)
}

// Pins board row W12: PUT /workflow-runs/{runID}/last-event-at writes the
// caller-supplied "timestamp" string straight into the last_event_at column
// with no parsing or validation (workflow_run.go's TouchWorkflowRunLastEventAt
// calls UpdateColumn("last_event_at", ts) directly). The watchdog's
// ListWorkflowRunsStaleSince compares this column against a cutoff via a
// plain lexicographic string comparison (last_event_at < cutoffStr) that
// only produces a chronologically-correct result when the stored value is
// itself a valid, zero-padded RFC3339 string. A caller who touches a run
// with any string that sorts lexicographically greater than every future
// RFC3339 cutoff (e.g. one starting with a letter) makes that run
// permanently invisible to the stale-run watchdog, regardless of how long
// it has actually gone quiet — a real evasion of the AI-failure-recovery
// mechanism this column exists to drive (see this repo's CLAUDE.md on
// WorkflowRun's watchdog).
func TestWorkflowRun_TouchLastEventAtWithGarbageStringEvadesStaleWatchdog(t *testing.T) {
	tm := wk12Manager(t)
	m := wk12Mux(tm)

	// Baseline: an untouched run with a real RFC3339 last_event_at IS
	// reported stale against a far-future cutoff.
	createBaseline := httptest.NewRecorder()
	m.ServeHTTP(createBaseline, httptest.NewRequest("POST", "/workflow-runs", wk12JSON(t, map[string]any{"name": "w12-baseline"})))
	if createBaseline.Code != http.StatusCreated {
		t.Fatalf("create baseline run: got %d, body %s", createBaseline.Code, createBaseline.Body.String())
	}

	staleBaseline := httptest.NewRecorder()
	m.ServeHTTP(staleBaseline, httptest.NewRequest("GET", "/workflow-runs/stale?cutoff=9999-01-01T00:00:00Z", nil))
	if staleBaseline.Code != http.StatusOK {
		t.Fatalf("list stale (baseline): got %d, body %s", staleBaseline.Code, staleBaseline.Body.String())
	}
	var baselineRuns []map[string]any
	if err := json.Unmarshal(staleBaseline.Body.Bytes(), &baselineRuns); err != nil {
		t.Fatalf("decode baseline stale list: %v", err)
	}
	if len(baselineRuns) != 1 {
		t.Fatalf("baseline: got %d stale runs, want 1 (the untouched run should be flagged stale): %s", len(baselineRuns), staleBaseline.Body.String())
	}

	// Now create a second run and "touch" its last-event-at with a
	// non-timestamp string.
	createEvasive := httptest.NewRecorder()
	m.ServeHTTP(createEvasive, httptest.NewRequest("POST", "/workflow-runs", wk12JSON(t, map[string]any{"name": "w12-evasive"})))
	if createEvasive.Code != http.StatusCreated {
		t.Fatalf("create evasive run: got %d, body %s", createEvasive.Code, createEvasive.Body.String())
	}
	var evasive struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createEvasive.Body.Bytes(), &evasive); err != nil {
		t.Fatalf("decode evasive run: %v", err)
	}

	touchRec := httptest.NewRecorder()
	m.ServeHTTP(touchRec, httptest.NewRequest("PUT", "/workflow-runs/"+evasive.ID+"/last-event-at", wk12JSON(t, map[string]any{"timestamp": "zzzz-not-a-real-timestamp"})))
	if touchRec.Code != http.StatusNoContent {
		t.Fatalf("touch last-event-at with garbage string: got %d, body %s — if this now rejects invalid input, W12 may be fixed; update this test", touchRec.Code, touchRec.Body.String())
	}

	// BUG (W12): the far-future cutoff query no longer reports the evasive
	// run as stale, even though its real last event was seconds ago and
	// every plausible reading of "has this run gone quiet" says yes.
	staleAfterTouch := httptest.NewRecorder()
	m.ServeHTTP(staleAfterTouch, httptest.NewRequest("GET", "/workflow-runs/stale?cutoff=9999-01-01T00:00:00Z", nil))
	if staleAfterTouch.Code != http.StatusOK {
		t.Fatalf("list stale (after touch): got %d, body %s", staleAfterTouch.Code, staleAfterTouch.Body.String())
	}
	var afterTouchRuns []map[string]any
	if err := json.Unmarshal(staleAfterTouch.Body.Bytes(), &afterTouchRuns); err != nil {
		t.Fatalf("decode after-touch stale list: %v", err)
	}
	// W12 pin: the evasive run is INVISIBLE to the watchdog query despite
	// being the more obviously-stale of the two runs. This assertion is
	// expected to start FAILING once W12 is fixed (TouchWorkflowRunLastEventAt
	// should reject a non-RFC3339 timestamp, or the watchdog query should
	// stop relying on lexicographic string comparison) — that failure is
	// the signal to update this test to assert the evasive run IS found,
	// or that the touch itself was refused.
	for _, r := range afterTouchRuns {
		if r["id"] == evasive.ID {
			t.Fatalf("W12 pin violated: evasive run unexpectedly appeared in the stale list: %s", staleAfterTouch.Body.String())
		}
	}
	if len(afterTouchRuns) != 1 || afterTouchRuns[0]["name"] != "w12-baseline" {
		t.Fatalf("W12 pin: want only the untouched baseline run reported stale, got %s", staleAfterTouch.Body.String())
	}
}
