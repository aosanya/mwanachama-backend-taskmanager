package routes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// wk15Manager mirrors wk14Manager but pins the sqlite pool to one
// connection so concurrent goroutines share the same in-memory database
// instead of each getting its own anonymous one (a harness-only quirk, not
// a product bug — see mwanachama-backend-git's CLAUDE.md note on the
// identical fixture issue with pooled sqlite `:memory:` connections).
func wk15Manager(t *testing.T, prefix string) mwanachamataskmanager.TaskManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	tables := mwanachamataskmanager.DefaultTableNames(prefix)
	if err := mwanachamataskmanager.Migrate(db, tables); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, tables, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr
}

// TestIncrementFailureBudget_PinsConcurrentDistinctChildIDsLoseUpdates pins
// board row W15: workflow_run_failure_budget.go's IncrementFailureBudget is
// documented as atomic but is a plain read-modify-write with no CAS guard.
// Five concurrent increments with five DISTINCT child_run_ids (a legitimate
// case, not a repeat of the same id) should all land — the final counter
// should read 5. It does not: this test asserts the CURRENT broken
// behavior (fewer than 5 survive, consistently 1 in local runs). Once W15's
// fix (a CAS-guarded UPDATE or a wrapping transaction) lands, this
// assertion must be changed to require finalUsed == n and the len(counted)
// == n check strengthened to require every childRunID present, not just
// the count.
func TestIncrementFailureBudget_PinsConcurrentDistinctChildIDsLoseUpdates(t *testing.T) {
	tm := wk15Manager(t, "wk15")
	mux := wk14Mux(tm)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	rootResp := wk14Do(t, mux, "POST", "/workflow-runs", wk14JSON(t, map[string]any{"name": "root"}))
	var root map[string]any
	if err := json.Unmarshal(rootResp.Body.Bytes(), &root); err != nil {
		t.Fatalf("unmarshal root: %v", err)
	}
	rootID, _ := root["id"].(string)
	if rootID == "" {
		t.Fatalf("no root id in response: %s", rootResp.Body.String())
	}

	setResp := wk14Do(t, mux, "PUT", "/workflow-runs/"+rootID+"/failure-budget", wk14JSON(t, map[string]any{"budget": 10}))
	if setResp.Code != 200 {
		t.Fatalf("SetFailureBudget: got %d, body %s", setResp.Code, setResp.Body.String())
	}

	const n = 5
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body, _ := json.Marshal(map[string]any{"child_run_id": fmt.Sprintf("child-%d", i)})
			resp, err := srv.Client().Post(srv.URL+"/workflow-runs/"+rootID+"/failure-budget/increment", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Errorf("increment %d: %v", i, err)
				return
			}
			resp.Body.Close()
		}(i)
	}
	wg.Wait()

	getResp := wk14Do(t, mux, "GET", "/workflow-runs/"+rootID, nil)
	var final map[string]any
	if err := json.Unmarshal(getResp.Body.Bytes(), &final); err != nil {
		t.Fatalf("unmarshal final: %v", err)
	}
	usedF, _ := final["failure_pipelines_used"].(float64)
	counted, _ := final["counted_child_run_ids"].([]any)

	// PINS THE BUG: with a correct CAS/transactional guard this would be
	// n (5). Today it is consistently 1 — every increment past the first
	// to commit clobbers the prior write instead of building on it.
	if int(usedF) >= n {
		t.Fatalf("W15 appears fixed: failure_pipelines_used=%d, want < %d to pin the known lost-update race — update this test's assertion to require == %d and remove this pin", int(usedF), n, n)
	}
	if len(counted) >= n {
		t.Fatalf("W15 appears fixed: counted_child_run_ids has %d entries, want < %d to pin the known lost-update race — update this test's assertion to require == %d and remove this pin", len(counted), n, n)
	}
	t.Logf("pinned lost-update race: failure_pipelines_used=%d counted_child_run_ids=%v (want %d once W15 is fixed)", int(usedF), counted, n)
}
