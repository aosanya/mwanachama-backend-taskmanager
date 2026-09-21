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

// TestWorkflowRun_TouchLastEventAtRejectsGarbageString covers board row W12:
// a non-RFC 3339 timestamp is refused (400) and leaves the run visible to the
// stale-run watchdog, whose last_event_at comparison is a string ordering that
// only holds for canonical timestamps.
func TestWorkflowRun_TouchLastEventAtRejectsGarbageString(t *testing.T) {
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
	if touchRec.Code != http.StatusBadRequest {
		t.Fatalf("touch last-event-at with garbage string: got %d, body %s, want 400", touchRec.Code, touchRec.Body.String())
	}

	// The refused touch must leave the run visible to the watchdog.
	staleAfterTouch := httptest.NewRecorder()
	m.ServeHTTP(staleAfterTouch, httptest.NewRequest("GET", "/workflow-runs/stale?cutoff=9999-01-01T00:00:00Z", nil))
	if staleAfterTouch.Code != http.StatusOK {
		t.Fatalf("list stale (after touch): got %d, body %s", staleAfterTouch.Code, staleAfterTouch.Body.String())
	}
	var afterTouchRuns []map[string]any
	if err := json.Unmarshal(staleAfterTouch.Body.Bytes(), &afterTouchRuns); err != nil {
		t.Fatalf("decode after-touch stale list: %v", err)
	}
	if len(afterTouchRuns) != 2 {
		t.Fatalf("want both runs reported stale after the refused touch, got %s", staleAfterTouch.Body.String())
	}

	// A valid non-UTC timestamp is accepted and stored canonically.
	ok := httptest.NewRecorder()
	m.ServeHTTP(ok, httptest.NewRequest("PUT", "/workflow-runs/"+evasive.ID+"/last-event-at", wk12JSON(t, map[string]any{"timestamp": "2026-01-01T10:00:00+02:00"})))
	if ok.Code != http.StatusNoContent {
		t.Fatalf("touch with a valid RFC 3339 timestamp: got %d, body %s", ok.Code, ok.Body.String())
	}
}
