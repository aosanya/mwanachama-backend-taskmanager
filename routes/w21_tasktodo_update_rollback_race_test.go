package routes_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func w21RaceManagerAndDB(t *testing.T, prefix string) (mwanachamataskmanager.TaskManager, *gorm.DB, mwanachamataskmanager.TableNames) {
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
	return mgr, db, tables
}

func w21RaceMux(tm mwanachamataskmanager.TaskManager) *http.ServeMux {
	m := http.NewServeMux()
	for _, rt := range routes.Routes(tm) {
		m.Handle(rt.Pattern(""), rt.Handler)
	}
	return m
}

// TestPinsW21_UpdateTaskTodoStatusCanWriteIntoAnAlreadyDeletedRow pins a new
// board row (W21), widening W19/W20/assetmanager-A14's TOCTOU shape onto
// TaskTodo — a widening lead W20's own note left unchased, on a mistaken
// premise: W20 assumed TaskTodo's only delete path
// (DeleteWorkflowRunArtifacts) is a hard delete, so a racing
// UpdateTaskTodoStatus write would silently affect 0 rows rather than
// succeed. Reading workflow_run_rollback.go directly shows that premise is
// wrong: TaskTodos anchored to a run are SOFT-deleted there
// (`UpdateColumn("deleted", true)`, confirmed by gormstore/tasktodo.go's own
// `TaskTodoRow.Deleted` doc comment) — the row survives, so
// `todo.go`'s `UpdateTaskTodoStatus` (its own `Updates` call has no
// `deleted` guard, identical to W19's `UpdateTask`/W20's `UpdateProject`)
// can and does write into it after the row is marked deleted.
//
// Driven through the real `routes.Routes(tm)` mux behind `httptest.
// NewServer`, with two real *http.Client requests fired concurrently — PUT
// /todos/{todoID}/status racing DELETE /workflow-runs/{runID}/artifacts —
// repeated across 200 fresh todos so the test does not depend on hitting
// one particular interleaving.
//
// Once W21 is fixed (the same CAS shape as W19/W20/A14: guard the status
// Update's WHERE clause with `AND deleted = ?` / `false`, check
// RowsAffected, return ErrTaskTodoNotFound on 0 rows affected), the
// written-into-deleted count here should drop to 0 and this test should be
// rewritten to assert exactly that.
func TestPinsW21_UpdateTaskTodoStatusCanWriteIntoAnAlreadyDeletedRow(t *testing.T) {
	const iterations = 500
	bothSucceeded := 0
	writtenIntoDeletedRow := 0

	for i := 0; i < iterations; i++ {
		tm, db, tables := w21RaceManagerAndDB(t, fmt.Sprintf("w21race%d", i))
		srv := httptest.NewServer(w21RaceMux(tm))
		client := srv.Client()
		ctx := context.Background()

		task, err := tm.CreateTask(ctx, mwanachamataskmanager.Task{Title: "parent"})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		run, err := tm.CreateWorkflowRun(ctx, "", "", "")
		if err != nil {
			t.Fatalf("CreateWorkflowRun: %v", err)
		}
		todo, err := tm.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
			Title:         "original",
			Instructions:  "do the thing",
			ParentTaskID:  task.ID,
			WorkflowRunID: run.ID,
		})
		if err != nil {
			t.Fatalf("CreateTaskTodo: %v", err)
		}

		var wg sync.WaitGroup
		var updateStatus, deleteStatus int
		var updateErr, deleteErr error
		// start is a release barrier: both requests are built and ready
		// before either fires, so the two goroutines hit the single sqlite
		// connection (SetMaxOpenConns(1) above) as close to simultaneously
		// as the Go scheduler allows, rather than one lagging behind the
		// other by however long it took to construct its own request.
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			var buf bytes.Buffer
			_ = json.NewEncoder(&buf).Encode(map[string]any{"status": "completed"})
			req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/todos/%s/status", srv.URL, todo.ID), &buf)
			req.Header.Set("Content-Type", "application/json")
			<-start
			resp, err := client.Do(req)
			if err != nil {
				updateErr = err
				return
			}
			defer resp.Body.Close()
			updateStatus = resp.StatusCode
		}()
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/workflow-runs/%s/artifacts", srv.URL, run.ID), nil)
			<-start
			resp, err := client.Do(req)
			if err != nil {
				deleteErr = err
				return
			}
			defer resp.Body.Close()
			deleteStatus = resp.StatusCode
		}()
		close(start)
		wg.Wait()
		if updateErr != nil {
			t.Fatalf("update Do: %v", updateErr)
		}
		if deleteErr != nil {
			t.Fatalf("delete Do: %v", deleteErr)
		}

		if updateStatus == http.StatusOK && deleteStatus == http.StatusNoContent {
			bothSucceeded++
			var row struct {
				Status  string
				Deleted bool
			}
			if err := db.Table(tables.TaskTodos).Select("status, deleted").Where("id = ?", todo.ID).Scan(&row).Error; err != nil {
				t.Fatalf("raw select: %v", err)
			}
			if row.Deleted && row.Status == "completed" {
				writtenIntoDeletedRow++
			}
		}

		srv.Close()
	}

	t.Logf("UpdateTaskTodoStatus returned 200 concurrently with DeleteWorkflowRunArtifacts succeeding: %d/%d iterations", bothSucceeded, iterations)
	t.Logf("of those, the persisted todo row ended up deleted=true with the racing status update silently applied anyway: %d/%d", writtenIntoDeletedRow, bothSucceeded)

	if writtenIntoDeletedRow == 0 {
		t.Fatalf("expected at least one iteration where a soft-deleted TaskTodo row still absorbed a racing status update (race not reproduced across %d iterations — current broken behavior no longer confirmed)", iterations)
	}
}
