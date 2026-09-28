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

func w20Setup(t *testing.T) (mwanachamataskmanager.TaskManager, *gorm.DB, testTables, string) {
	t.Helper()
	dsn := fmt.Sprintf("file:w20pin%d?mode=memory&cache=shared", time.Now().UnixNano())

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

	workSpec, tables := specForInstance(t, "../spec/examples/work.taskmanager.json", "w20pin")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	tm, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return tm, db, tables, dsn
}

// TestW20_UpdateProjectRefusesAnAlreadyDeletedRow guards board row W20, fixed
// 2026-09-28 — the same TOCTOU shape as W19 and W21, on project.go's
// UpdateProject, which read through GetProject (`deleted = false`) and then
// wrote through a bare `Where("id = ?")`. Its write now carries
// `AND deleted = false` and reports ErrProjectNotFound when that matches no
// row.
//
// Like W19's, this replaced a 200-iteration race whose final state could not
// be told apart from a legitimate update-then-delete ordering. See
// w19_task_update_delete_race_test.go for why the interleave is forced.
func TestW20_UpdateProjectRefusesAnAlreadyDeletedRow(t *testing.T) {
	tm, db, tables, dsn := w20Setup(t)
	ctx := context.Background()

	project, err := tm.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Original"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	side, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("side gorm.Open: %v", err)
	}

	deleted := false
	err = db.Callback().Update().Before("gorm:update").Register("w20:delete_between_read_and_write",
		func(tx *gorm.DB) {
			if deleted || tx.Statement.Table != tables.Projects {
				return
			}
			deleted = true
			if err := side.Exec("UPDATE "+tables.Projects+" SET deleted = 1 WHERE id = ?", project.ID).Error; err != nil {
				t.Errorf("forced soft delete: %v", err)
			}
		})
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}

	project.Name = "raced-update"
	_, updErr := tm.UpdateProject(ctx, project)
	if !deleted {
		t.Fatal("the forced soft delete never fired, so nothing was interleaved")
	}
	if !errors.Is(updErr, mwanachamataskmanager.ErrProjectNotFound) {
		t.Errorf("UpdateProject returned %v, want ErrProjectNotFound", updErr)
	}

	var row struct {
		Name    string
		Deleted bool
	}
	if err := side.Table(tables.Projects).Select("name, deleted").
		Where("id = ?", project.ID).Scan(&row).Error; err != nil {
		t.Fatalf("raw select: %v", err)
	}
	if !row.Deleted {
		t.Fatal("precondition: the row should have been soft-deleted mid-call")
	}
	if row.Name != "Original" {
		t.Errorf("name = %q, want %q — the racing update landed on an already-deleted row", row.Name, "Original")
	}
}

// An ordinary update, with nothing deleting underneath it, still writes.
func TestW20_UpdateProjectStillWritesWhenNothingDeletesUnderneath(t *testing.T) {
	tm, _, _, _ := w20Setup(t)
	ctx := context.Background()

	project, err := tm.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Original"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	project.Name = "Edited"
	updated, err := tm.UpdateProject(ctx, project)
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if updated.Name != "Edited" {
		t.Errorf("name = %q, want Edited", updated.Name)
	}
	read, err := tm.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if read.Name != "Edited" {
		t.Errorf("persisted name = %q, want Edited", read.Name)
	}
}

// Updating a project that was already deleted before the call is refused too.
func TestW20_UpdateProjectRefusesADeletedProjectOutright(t *testing.T) {
	tm, _, _, _ := w20Setup(t)
	ctx := context.Background()

	project, err := tm.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Original"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if err := tm.DeleteProject(ctx, project.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	project.Name = "Edited"
	if _, err := tm.UpdateProject(ctx, project); !errors.Is(err, mwanachamataskmanager.ErrProjectNotFound) {
		t.Errorf("err = %v, want ErrProjectNotFound", err)
	}
}
