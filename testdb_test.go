package mwanachamataskmanager_test

import (
	"testing"

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
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

	tables := mwanachamataskmanager.DefaultTableNames("test")
	if err := mwanachamataskmanager.Migrate(db, tables); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	mgr, err := mwanachamataskmanager.NewTaskManager(db, tables, pub)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr
}

func TestNewTaskManager_NilDB(t *testing.T) {
	if _, err := mwanachamataskmanager.NewTaskManager(nil, mwanachamataskmanager.DefaultTableNames("test"), nil); err == nil {
		t.Fatal("expected error for nil db")
	}
}
