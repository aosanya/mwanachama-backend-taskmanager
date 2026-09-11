// tag_test.go exercises Tag.Code minting: Tags are created on demand via
// upsertTagByName (called from CreateTask/UpdateTask's setTaskTags), the
// only real creation path for Tag rows in this package.
package mwanachamataskmanager_test

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
)

// newTagTestManager is [newTestManager] but also returns the underlying
// *gorm.DB and table names so tests can read raw gormstore.TagRow rows
// directly — Tag has no public read-by-name accessor on [mwanachamataskmanager.TaskManager].
func newTagTestManager(t *testing.T) (mwanachamataskmanager.TaskManager, *gorm.DB, mwanachamataskmanager.TableNames) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	tables := mwanachamataskmanager.DefaultTableNames("test")
	if err := mwanachamataskmanager.Migrate(db, tables); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, tables, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr, db, tables
}

func getTagRowByName(t *testing.T, db *gorm.DB, tables mwanachamataskmanager.TableNames, name string) gormstore.TagRow {
	t.Helper()
	var row gormstore.TagRow
	if err := db.Table(tables.Tags).Where("name = ?", name).First(&row).Error; err != nil {
		t.Fatalf("read tag %q: %v", name, err)
	}
	return row
}

// TestCreateTask_TagCode_SetAndSequential asserts a new Tag row gets a
// non-empty Code at creation time, and that codes mint sequentially
// ("TG-1", "TG-2", ...) across separate new tag names.
func TestCreateTask_TagCode_SetAndSequential(t *testing.T) {
	mgr, db, tables := newTagTestManager(t)
	ctx := context.Background()

	if _, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "t1", Tags: []string{"alpha"}}); err != nil {
		t.Fatalf("CreateTask 1: %v", err)
	}
	if _, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "t2", Tags: []string{"beta"}}); err != nil {
		t.Fatalf("CreateTask 2: %v", err)
	}

	alpha := getTagRowByName(t, db, tables, "alpha")
	beta := getTagRowByName(t, db, tables, "beta")

	if alpha.Code == "" {
		t.Error("alpha tag Code is empty")
	}
	if beta.Code == "" {
		t.Error("beta tag Code is empty")
	}
	if alpha.Code != "TG-1" {
		t.Errorf("alpha Code = %q, want TG-1", alpha.Code)
	}
	if beta.Code != "TG-2" {
		t.Errorf("beta Code = %q, want TG-2", beta.Code)
	}
}

// TestTagCode_UnchangedWhenTagReused asserts that re-using an existing tag
// name (via a second CreateTask, and via UpdateTask on the first task)
// never re-mints or otherwise changes the tag's Code — upsertTagByName's
// find-branch must leave the row untouched.
func TestTagCode_UnchangedWhenTagReused(t *testing.T) {
	mgr, db, tables := newTagTestManager(t)
	ctx := context.Background()

	task1, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "t1", Tags: []string{"shared"}})
	if err != nil {
		t.Fatalf("CreateTask 1: %v", err)
	}
	original := getTagRowByName(t, db, tables, "shared")
	if original.Code == "" {
		t.Fatal("shared tag Code is empty after first create")
	}

	// A second task reusing the same tag name must not re-mint its Code.
	if _, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "t2", Tags: []string{"shared"}}); err != nil {
		t.Fatalf("CreateTask 2: %v", err)
	}
	afterSecondCreate := getTagRowByName(t, db, tables, "shared")
	if afterSecondCreate.Code != original.Code {
		t.Errorf("Code changed after reuse via CreateTask: got %q, want %q", afterSecondCreate.Code, original.Code)
	}

	// UpdateTask re-running setTaskTags over the same tag name must also
	// leave the Code untouched.
	task1.Tags = []string{"shared"}
	task1.Description = "updated"
	if _, err := mgr.UpdateTask(ctx, task1); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	afterUpdate := getTagRowByName(t, db, tables, "shared")
	if afterUpdate.Code != original.Code {
		t.Errorf("Code changed after UpdateTask: got %q, want %q", afterUpdate.Code, original.Code)
	}
}
