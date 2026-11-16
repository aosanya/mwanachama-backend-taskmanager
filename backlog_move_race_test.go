package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func twoManagersOverOneFile(t *testing.T) []mwanachamataskmanager.TaskManager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "moves.db")
	dsn := "file:" + path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	workSpec := specForInstance(t, "spec/examples/agency.taskmanager.json", "test")
	managers := make([]mwanachamataskmanager.TaskManager, 2)
	for i := range managers {
		db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatalf("gorm.Open: %v", err)
		}
		if i == 0 {
			if err := spec.Migrate(db, workSpec); err != nil {
				t.Fatalf("Migrate: %v", err)
			}
		}
		mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
		if err != nil {
			t.Fatalf("NewTaskManager: %v", err)
		}
		managers[i] = mgr
	}
	return managers
}

func TestMoveTaskToStep_MovesAtOnceNeverLeaveAStepWithTheStatusOfAnother(t *testing.T) {
	managers := twoManagersOverOneFile(t)
	ctx := context.Background()
	steps := []string{"to-do", "doing", "done", "cancelled"}
	statusOf := map[string]mwanachamataskmanager.TaskStatus{
		"to-do":     mwanachamataskmanager.TaskStatusPending,
		"doing":     mwanachamataskmanager.TaskStatusInProgress,
		"done":      mwanachamataskmanager.TaskStatusCompleted,
		"cancelled": mwanachamataskmanager.TaskStatusCancelled,
	}
	for round := 0; round < 12; round++ {
		task := mustCreate(t, managers[0], mwanachamataskmanager.Task{Title: fmt.Sprintf("M%d", round)})
		var wg sync.WaitGroup
		errs := make([]error, 10)
		start := make(chan struct{})
		for i := range errs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, errs[i] = managers[i%2].MoveTaskToStep(ctx, task.ID, "backlog", steps[i%len(steps)])
			}(i)
		}
		close(start)
		wg.Wait()
		for _, err := range errs {
			if err != nil && !errors.Is(err, mwanachamataskmanager.ErrTaskMoveConflict) && !errors.Is(err, mwanachamataskmanager.ErrNoPathToState) {
				t.Errorf("round %d: a lost move answered %v, want the conflict", round, err)
			}
		}
		got, err := managers[0].GetTask(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if want, known := statusOf[got.Step]; known && got.Status != want {
			t.Errorf("round %d: task sits in step %s with status %s", round, got.Step, got.Status)
		}
	}
}
