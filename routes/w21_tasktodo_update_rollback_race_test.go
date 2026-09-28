package routes_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// TestPinsW21_UpdateTaskTodoStatusCanWriteIntoAnAlreadyDeletedRow pins a
// board row (W21), widening W19/W20/assetmanager-A14's TOCTOU shape onto
// TaskTodo — a widening lead W20's own note left unchased, on a mistaken
// premise: W20 assumed TaskTodo's only delete path
// (DeleteWorkflowRunArtifacts) is a hard delete, so a racing
// UpdateTaskTodoStatus write would silently affect 0 rows rather than
// succeed. Reading workflow_run_rollback.go directly shows that premise is
// wrong: TaskTodos anchored to a run are SOFT-deleted there
// (`UpdateColumn("deleted", true)`) — the row survives, so `todo.go`'s
// `UpdateTaskTodoStatus` (its own `Updates` call has no `deleted` guard,
// identical to W19's `UpdateTask`/W20's `UpdateProject`) can and does write
// into it after the row is marked deleted.
//
// The interleave is forced rather than raced. Firing two concurrent HTTP
// requests reproduced this at roughly one iteration in a hundred, which
// made the pin flaky enough to fail runs on its own; the defect is not
// probabilistic, only the scheduling that exposes it is. A GORM callback
// registered before the status UPDATE soft-deletes the row from a second
// connection, landing the delete in exactly the window between
// UpdateTaskTodoStatus's own read and its own write — the same window two
// racing callers hit by luck.
//
// Once W21 is fixed (the same CAS shape as W19/W20/A14: guard the status
// Update's WHERE clause with `AND deleted = ?` / `false`, check
// RowsAffected, return ErrTaskTodoNotFound on 0 rows affected), the write
// below will affect no rows, the persisted status will still read "pending",
// and this test should be rewritten to assert exactly that.
func TestPinsW21_UpdateTaskTodoStatusCanWriteIntoAnAlreadyDeletedRow(t *testing.T) {
	dsn := fmt.Sprintf("file:w21pin%d?mode=memory&cache=shared", time.Now().UnixNano())

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = sqlDB.Close() })

	workSpec, tables := specForInstance(t, "../spec/examples/work.taskmanager.json", "w21pin")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	tm, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}

	ctx := context.Background()
	run, err := tm.CreateWorkflowRun(ctx, "w21-pin-run", "trigger", "initiator")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	task, err := tm.CreateTask(ctx, mwanachamataskmanager.Task{Title: "parent", WorkflowRunID: run.ID})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
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

	side, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("side gorm.Open: %v", err)
	}

	deleted := false
	err = db.Callback().Update().Before("gorm:update").Register("w21:soft_delete_between_read_and_write",
		func(tx *gorm.DB) {
			if deleted || tx.Statement.Table != tables.TaskTodos {
				return
			}
			deleted = true
			if err := side.Exec("UPDATE "+tables.TaskTodos+" SET deleted = 1 WHERE id = ?", todo.ID).Error; err != nil {
				t.Errorf("forced soft delete: %v", err)
			}
		})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}

	_, updErr := tm.UpdateTaskTodoStatus(ctx, todo.ID, mwanachamataskmanager.TodoStatusCompleted)
	if !deleted {
		t.Fatal("the forced soft delete never fired, so nothing was interleaved")
	}
	t.Logf("UpdateTaskTodoStatus returned: %v", updErr)

	var row struct {
		Status  string
		Deleted bool
	}
	if err := side.Table(tables.TaskTodos).Select("status, deleted").
		Where("id = ?", todo.ID).Scan(&row).Error; err != nil {
		t.Fatalf("raw select: %v", err)
	}
	t.Logf("persisted todo row: status=%q deleted=%v", row.Status, row.Deleted)

	if !row.Deleted {
		t.Fatal("precondition: the row should have been soft-deleted mid-call")
	}
	if row.Status != string(mwanachamataskmanager.TodoStatusCompleted) {
		t.Fatalf("status = %q, want %q — the racing update no longer lands on a deleted row, so W21 looks fixed and this pin should be rewritten to assert the guard",
			row.Status, mwanachamataskmanager.TodoStatusCompleted)
	}
}
