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

// W19, W20 and W21 all force the interleave rather than racing for it:
// a callback soft-deletes the row from a second connection just before the
// method's own UPDATE lands. Racing real requests reproduced at roughly one
// iteration in a hundred for W21, and for W19 and W20 could not be told apart
// from a benign update-then-delete ordering at all. The defect is not
// probabilistic; only the scheduling that exposes it is.
func w19Setup(t *testing.T) (mwanachamataskmanager.TaskManager, *gorm.DB, testTables, string) {
	t.Helper()
	dsn := fmt.Sprintf("file:w19pin%d?mode=memory&cache=shared", time.Now().UnixNano())

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

	workSpec, tables := specForInstance(t, "../spec/examples/work.taskmanager.json", "w19pin")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	tm, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return tm, db, tables, dsn
}

// TestW19_UpdateTaskRefusesAnAlreadyDeletedRow guards board row W19, fixed
// 2026-09-28. task_impl_task.go's UpdateTask read the current row through
// GetTask (`WHERE id = ? AND deleted = false`) and then wrote through a bare
// `Where("id = ?")`, so a DeleteTask landing in the gap was invisible to the
// write, which applied the caller's new Title and returned 200. The write now
// carries `AND deleted = false` and reports ErrTaskNotFound when that matches
// no row.
//
// This replaced a 200-iteration race that could not distinguish the defect
// from a legitimate update-then-delete ordering — it reported ~73/200
// "written into a deleted row" even after the fix, because the final state of
// both is the same. The interleave is forced instead, so the assertion is
// about the defect rather than about scheduling.
func TestW19_UpdateTaskRefusesAnAlreadyDeletedRow(t *testing.T) {
	tm, db, tables, dsn := w19Setup(t)
	ctx := context.Background()

	task, err := tm.CreateTask(ctx, mwanachamataskmanager.Task{Title: "original", Description: "as written"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	side, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("side gorm.Open: %v", err)
	}

	deleted := false
	err = db.Callback().Update().Before("gorm:update").Register("w19:delete_between_read_and_write",
		func(tx *gorm.DB) {
			if deleted || tx.Statement.Table != tables.Tasks {
				return
			}
			deleted = true
			if err := side.Exec("UPDATE "+tables.Tasks+" SET deleted = 1 WHERE id = ?", task.ID).Error; err != nil {
				t.Errorf("forced soft delete: %v", err)
			}
		})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}

	task.Title = "raced-update"
	_, updErr := tm.UpdateTask(ctx, task)
	if !deleted {
		t.Fatal("the forced soft delete never fired, so nothing was interleaved")
	}
	if !errors.Is(updErr, mwanachamataskmanager.ErrTaskNotFound) {
		t.Errorf("UpdateTask returned %v, want ErrTaskNotFound", updErr)
	}

	var row struct {
		Title   string
		Deleted bool
	}
	if err := side.Table(tables.Tasks).Select("title, deleted").
		Where("id = ?", task.ID).Scan(&row).Error; err != nil {
		t.Fatalf("raw select: %v", err)
	}
	if !row.Deleted {
		t.Fatal("precondition: the row should have been soft-deleted mid-call")
	}
	if row.Title != "original" {
		t.Errorf("title = %q, want %q — the racing update landed on an already-deleted row", row.Title, "original")
	}
}

// An ordinary update, with nothing deleting underneath it, still writes.
func TestW19_UpdateTaskStillWritesWhenNothingDeletesUnderneath(t *testing.T) {
	tm, _, _, _ := w19Setup(t)
	ctx := context.Background()

	task, err := tm.CreateTask(ctx, mwanachamataskmanager.Task{Title: "original"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	task.Title = "edited"
	updated, err := tm.UpdateTask(ctx, task)
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.Title != "edited" {
		t.Errorf("title = %q, want edited", updated.Title)
	}
	read, err := tm.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if read.Title != "edited" {
		t.Errorf("persisted title = %q, want edited", read.Title)
	}
}

// Updating a task that was already deleted before the call is refused too.
func TestW19_UpdateTaskRefusesADeletedTaskOutright(t *testing.T) {
	tm, _, _, _ := w19Setup(t)
	ctx := context.Background()

	task, err := tm.CreateTask(ctx, mwanachamataskmanager.Task{Title: "original"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := tm.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	task.Title = "edited"
	if _, err := tm.UpdateTask(ctx, task); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Errorf("err = %v, want ErrTaskNotFound", err)
	}
}
