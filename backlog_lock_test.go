package mwanachamataskmanager_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func newFileBackedManager(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "hub.db") + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	workSpec, _ := specForInstance(t, "spec/examples/work.taskmanager.json", "test")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr
}

func TestConcurrentBacklogEditsAllSurvive(t *testing.T) {
	mgr := newFileBackedManager(t)
	ctx := context.Background()
	const writers = 8

	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = mgr.CreateBacklog(ctx, fmt.Sprintf("Backlog %d", i), []string{"Open", "Closed"})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("writer %d: %v", i, err)
		}
	}
	list, err := mgr.ListBacklogs(ctx)
	if err != nil {
		t.Fatalf("ListBacklogs: %v", err)
	}
	if got, want := len(list.Items), writers+1; got != want {
		t.Fatalf("backlogs after %d concurrent adds = %d, want %d", writers, got, want)
	}
}
