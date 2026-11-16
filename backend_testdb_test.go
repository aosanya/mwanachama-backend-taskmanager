package mwanachamataskmanager_test

import (
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/spec"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func newTestManagerWithPublisher(t *testing.T, pub mwanachamataskmanager.Publisher) mwanachamataskmanager.TaskManager {
	t.Helper()
	mgr, _, _ := newTestManagerWithTables(t, pub)
	return mgr
}

func newTestManagerWithTables(t *testing.T, pub mwanachamataskmanager.Publisher) (mwanachamataskmanager.TaskManager, *gorm.DB, testTables) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

	workSpec, tables := specAndTablesForInstance(t, "spec/examples/work.taskmanager.json", "test")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, pub)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr, db, tables
}

func TestNewTaskManager_NilDB(t *testing.T) {
	workSpec, _ := specAndTablesForInstance(t, "spec/examples/work.taskmanager.json", "test")
	if _, err := mwanachamataskmanager.NewTaskManager(nil, workSpec, nil); err == nil {
		t.Fatal("expected error for nil db")
	}
}

func TestNewTaskManager_NilSpec(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	if _, err := mwanachamataskmanager.NewTaskManager(db, nil, nil); err == nil {
		t.Fatal("expected error for nil spec")
	}
}
