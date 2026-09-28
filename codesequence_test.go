package mwanachamataskmanager_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// sharedCacheManager opens a database several connections can reach at once.
// This repo's usual `:memory:` single-connection harness serialises every
// call's whole transaction and so cannot expose a counter race at all, which
// is why W22 went unpinned for a week.
func sharedCacheManager(t *testing.T, name string) (mwanachamataskmanager.TaskManager, *gorm.DB, testTables) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s%d?mode=memory&cache=shared", name, time.Now().UnixNano())

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	workSpec, tables := specForInstance(t, "spec/examples/work.taskmanager.json", name)
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr, db, tables
}

// TestConcurrentTodoCreatesEachGetTheirOwnCode guards board row W22, fixed
// 2026-09-28. The counter was read, incremented in Go and written back with
// no precondition and no lock, so two concurrent CreateTaskTodo calls both
// read the same number: on SQLite one of them failed outright with a raw,
// unclassified driver error (19/30 iterations of the original probe —
// "database table is locked: database is deadlocked"), and on Postgres the
// second writer would have proceeded once the first committed, minting two
// rows sharing one Code.
//
// The counter is now advanced by the database itself — a seeding insert that
// does nothing on conflict, then `next_number = next_number + 1`, then a read
// of the value this caller claimed. The write comes first, so a second caller
// waits on the write lock instead of deadlocking on a read-to-write upgrade,
// and the increment is atomic rather than computed in Go.
func TestConcurrentTodoCreatesEachGetTheirOwnCode(t *testing.T) {
	mgr, db, tables := sharedCacheManager(t, "w22codes")
	ctx := context.Background()

	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "parent"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	const iterations, perIteration = 30, 2
	for i := 0; i < iterations; i++ {
		var wg sync.WaitGroup
		errs := make([]error, perIteration)
		start := make(chan struct{})
		for g := 0; g < perIteration; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				<-start
				_, err := mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
					Title:        fmt.Sprintf("todo-%d-%d", i, g),
					Instructions: "do the thing",
					ParentTaskID: task.ID,
				})
				errs[g] = err
			}(g)
		}
		close(start)
		wg.Wait()
		for g, err := range errs {
			if err != nil {
				t.Fatalf("iteration %d, goroutine %d: creating two todos at once failed: %v", i, g, err)
			}
		}
	}

	var codes []string
	if err := db.Table(tables.TaskTodos).Order("id").Pluck("code", &codes).Error; err != nil {
		t.Fatalf("read codes: %v", err)
	}
	if len(codes) != iterations*perIteration {
		t.Fatalf("created %d todos, want %d", len(codes), iterations*perIteration)
	}
	seen := map[string]int{}
	for _, code := range codes {
		if !strings.HasPrefix(code, "TD-") {
			t.Errorf("code %q is not a todo code", code)
		}
		seen[code]++
	}
	for code, n := range seen {
		if n > 1 {
			t.Errorf("code %q was minted %d times — the counter handed one number to two rows", code, n)
		}
	}
}

// Two concurrently-created tags with distinct names each get their own code
// too. The tag path has its own retry for a name collision, which is a
// different race from the counter's and must not be masking this one.
func TestConcurrentDistinctTagsEachGetTheirOwnCode(t *testing.T) {
	mgr, db, tables := sharedCacheManager(t, "w22tags")
	ctx := context.Background()

	const n = 12
	var wg sync.WaitGroup
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{
				Title: fmt.Sprintf("task-%d", i),
				Tags:  []string{fmt.Sprintf("tag-%d", i)},
			})
			errs[i] = err
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("task %d: %v", i, err)
		}
	}

	var codes []string
	if err := db.Table(tables.Tags).Order("id").Pluck("code", &codes).Error; err != nil {
		t.Fatalf("read codes: %v", err)
	}
	seen := map[string]int{}
	for _, code := range codes {
		seen[code]++
	}
	for code, count := range seen {
		if count > 1 {
			t.Errorf("tag code %q was minted %d times", code, count)
		}
	}
}

// Codes stay sequential and gapless when nothing is racing, which is what a
// person reading "TD-7" off a board is relying on.
func TestCodesAreSequential(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "parent"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	for i := 1; i <= 5; i++ {
		todo, err := mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
			Title: fmt.Sprintf("step %d", i), Instructions: "x", ParentTaskID: task.ID,
		})
		if err != nil {
			t.Fatalf("CreateTaskTodo %d: %v", i, err)
		}
		if want := fmt.Sprintf("TD-%d", i); todo.Code != want {
			t.Errorf("code = %q, want %q", todo.Code, want)
		}
	}
}
