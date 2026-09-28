package routes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/spec"

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
	workSpec, _ := specForInstance(t, "../spec/examples/work.taskmanager.json", prefix)
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr
}

// TestIncrementFailureBudget_ConcurrentDistinctChildIDsAllLand guards board
// row W15, fixed 2026-09-28. IncrementFailureBudget was documented as atomic
// but was a plain read-modify-write: five concurrent charges with five
// DISTINCT child_run_ids (a legitimate case, not a repeat of one id) left the
// counter reading 1, because every increment past the first to commit
// clobbered the prior write. It now writes under a compare-and-swap on the
// counter it read and goes round again when it loses, so all five land.
//
// This asserted the broken behaviour until the fix; it now asserts the fixed
// one, and every child id has to be present rather than just the count.
func TestIncrementFailureBudget_ConcurrentDistinctChildIDsAllLand(t *testing.T) {
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

	if int(usedF) != n {
		t.Errorf("failure_pipelines_used = %d, want %d — a concurrent charge was lost", int(usedF), n)
	}
	if len(counted) != n {
		t.Errorf("counted_child_run_ids has %d entries, want %d: %v", len(counted), n, counted)
	}
	seen := map[string]bool{}
	for _, c := range counted {
		if id, ok := c.(string); ok {
			seen[id] = true
		}
	}
	for i := 0; i < n; i++ {
		if id := fmt.Sprintf("child-%d", i); !seen[id] {
			t.Errorf("%s was charged but is not in counted_child_run_ids: %v", id, counted)
		}
	}
}

// Charging the same child twice stays a no-op — the idempotency the counter
// already had, which the compare-and-swap must not have cost it.
func TestIncrementFailureBudget_ChargingOneChildTwiceCountsItOnce(t *testing.T) {
	tm := wk15Manager(t, "wk15idem")
	mux := wk14Mux(tm)

	rootResp := wk14Do(t, mux, "POST", "/workflow-runs", wk14JSON(t, map[string]any{"name": "root"}))
	var root map[string]any
	if err := json.Unmarshal(rootResp.Body.Bytes(), &root); err != nil {
		t.Fatalf("unmarshal root: %v", err)
	}
	rootID, _ := root["id"].(string)
	if setResp := wk14Do(t, mux, "PUT", "/workflow-runs/"+rootID+"/failure-budget",
		wk14JSON(t, map[string]any{"budget": 10})); setResp.Code != 200 {
		t.Fatalf("SetFailureBudget: got %d, body %s", setResp.Code, setResp.Body.String())
	}

	for i := 0; i < 3; i++ {
		resp := wk14Do(t, mux, "POST", "/workflow-runs/"+rootID+"/failure-budget/increment",
			wk14JSON(t, map[string]any{"child_run_id": "the-same-child"}))
		if resp.Code != 200 {
			t.Fatalf("charge %d: got %d, body %s", i, resp.Code, resp.Body.String())
		}
	}

	getResp := wk14Do(t, mux, "GET", "/workflow-runs/"+rootID, nil)
	var final map[string]any
	if err := json.Unmarshal(getResp.Body.Bytes(), &final); err != nil {
		t.Fatalf("unmarshal final: %v", err)
	}
	if usedF, _ := final["failure_pipelines_used"].(float64); int(usedF) != 1 {
		t.Errorf("failure_pipelines_used = %d, want 1 after charging one child three times", int(usedF))
	}
}
