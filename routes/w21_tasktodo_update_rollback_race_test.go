package routes_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// TestW21_UpdateTaskTodoStatusRefusesAnAlreadyDeletedRow guards board row
// W21, fixed 2026-09-28. It widened W19/W20/assetmanager-A14's TOCTOU shape onto
// TaskTodo — a widening lead W20's own note left unchased, on a mistaken
// premise: W20 assumed TaskTodo's only delete path
// (DeleteWorkflowRunArtifacts) is a hard delete, so a racing
// UpdateTaskTodoStatus write would silently affect 0 rows rather than
// succeed. Reading workflow_run_rollback.go directly shows that premise is
// wrong: TaskTodos anchored to a run are SOFT-deleted there
// (`UpdateColumn("deleted", true)`) — the row survives, so `todo.go`'s
// `UpdateTaskTodoStatus` (its own `Updates` call has no `deleted` guard,
// identical to W19's `UpdateTask`/W20's `UpdateProject`) can and does write
// into it after the row was marked deleted. Its write now carries
// `AND deleted = false` and reports ErrTaskTodoNotFound when that matches no
// row, so the update is refused rather than landing on a row no read would
// ever return.
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
// This asserted the broken behaviour until the fix; it now asserts that the
// write affects no rows and the persisted status still reads "pending".
func TestW21_UpdateTaskTodoStatusRefusesAnAlreadyDeletedRow(t *testing.T) {
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
	if !errors.Is(updErr, mwanachamataskmanager.ErrTaskTodoNotFound) {
		t.Errorf("UpdateTaskTodoStatus returned %v, want ErrTaskTodoNotFound", updErr)
	}

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
	if row.Status != string(mwanachamataskmanager.TodoStatusPending) {
		t.Errorf("status = %q, want %q — the racing update landed on an already-deleted row",
			row.Status, mwanachamataskmanager.TodoStatusPending)
	}
}
