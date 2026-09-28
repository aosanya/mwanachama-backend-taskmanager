package mwanachamataskmanager_test

import (
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/spec"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// newTestManager builds a [mwanachamataskmanager.TaskManager] backed by a
// fresh in-memory sqlite database, migrated the same way a real deployment
// would via [mwanachamataskmanager.Migrate]. Replaces the old hand-maintained
// fakeDataManager: exercising real GORM/SQL behavior catches more than a Go
// map fake ever could, while staying fully in-process — no containers, no
// POSTGRES_URL, consistent with this repo's existing separation between fast
// unit tests here and the opt-in postgres_integration_test.go.
func newTestManager(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	return newTestManagerWithPublisher(t, nil)
}

// newTestManagerWithPublisher is [newTestManager] with an explicit
// [mwanachamataskmanager.Publisher] (e.g. a *recordingPublisher) instead of
// the default nil (events skipped).
func newTestManagerWithPublisher(t *testing.T, pub mwanachamataskmanager.Publisher) mwanachamataskmanager.TaskManager {
	t.Helper()
	mgr, _, _ := newTestManagerWithDB(t, pub)
	return mgr
}

// newTestManagerWithDB is [newTestManagerWithPublisher] with the database and
// its table names handed back too, for a test that has to read or write a row
// the manager's own API deliberately will not — a run left in rolling_back by
// a rollback that never finished, for one.
func newTestManagerWithDB(t *testing.T, pub mwanachamataskmanager.Publisher) (mwanachamataskmanager.TaskManager, *gorm.DB, testTables) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

	workSpec, tables := specForInstance(t, "spec/examples/work.taskmanager.json", "test")
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
	workSpec, _ := specForInstance(t, "spec/examples/work.taskmanager.json", "test")
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
