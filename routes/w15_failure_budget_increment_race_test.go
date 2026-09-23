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
// should read 5. It does not always: without a release barrier the five
// goroutines' HTTP round-trips did not reliably overlap, so a first version
// of this test (no barrier, single iteration) flaked green roughly 7 times
// out of 8 in this environment. Hardened the same way W21's own race pin
// was (start-channel barrier + repeated iterations, asserting "at least one
// iteration reproduces it" rather than "every iteration reproduces it") —
// this asserts the CURRENT broken behavior across enough iterations that a
// lost update reliably shows up at least once. Once W15's fix (a
// CAS-guarded UPDATE or a wrapping transaction) lands, this assertion must
// be inverted to require finalUsed == n on every iteration and this pin
// removed.
func TestIncrementFailureBudget_PinsConcurrentDistinctChildIDsLoseUpdates(t *testing.T) {
	const n = 5
	const iterations = 100
	lostAnUpdate := 0

	for iter := 0; iter < iterations; iter++ {
		tm := wk15Manager(t, fmt.Sprintf("wk15race%d", iter))
		mux := wk14Mux(tm)
		srv := httptest.NewServer(mux)

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

		// start is a release barrier: every request is built and ready
		// before any of them fire, so all n goroutines hit the single
		// sqlite connection (SetMaxOpenConns(1) in wk15Manager) as close to
		// simultaneously as the Go scheduler allows, rather than one
		// lagging behind the others by however long it took to construct
		// its own request — see W21's identical pattern for the same
		// reason.
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			body, _ := json.Marshal(map[string]any{"child_run_id": fmt.Sprintf("child-%d", i)})
			go func(body []byte) {
				defer wg.Done()
				<-start
				resp, err := srv.Client().Post(srv.URL+"/workflow-runs/"+rootID+"/failure-budget/increment", "application/json", bytes.NewReader(body))
				if err != nil {
					t.Errorf("increment: %v", err)
					return
				}
				resp.Body.Close()
			}(body)
		}
		close(start)
		wg.Wait()

		getResp := wk14Do(t, mux, "GET", "/workflow-runs/"+rootID, nil)
		var final map[string]any
		if err := json.Unmarshal(getResp.Body.Bytes(), &final); err != nil {
			t.Fatalf("unmarshal final: %v", err)
		}
		usedF, _ := final["failure_pipelines_used"].(float64)
		counted, _ := final["counted_child_run_ids"].([]any)
		srv.Close()

		if int(usedF) < n || len(counted) < n {
			lostAnUpdate++
		}
	}

	t.Logf("lost at least one concurrent increment in %d/%d iterations", lostAnUpdate, iterations)
	// PINS THE BUG: with a correct CAS/transactional guard, every iteration
	// would land all n increments and this count would be 0.
	if lostAnUpdate == 0 {
		t.Fatalf("W15 appears fixed: 0/%d iterations lost an update — update this test's assertion to require == %d on every iteration and remove this pin", iterations, n)
	}
}
