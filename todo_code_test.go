// todo_code_test.go exercises TaskTodo.Code minting via CreateTaskTodo, the
// real task-decomposition creation path for TaskTodo rows.
package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// TestCreateTaskTodo_CodeSetAndSequential asserts each new TaskTodo gets a
// non-empty Code at creation, minted sequentially ("TD-1", "TD-2", ...) and
// left unchanged by a later status update.
func TestCreateTaskTodo_CodeSetAndSequential(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "parent"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	todo1, err := mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
		Title: "step 1", Instructions: "do it", ParentTaskID: task.ID,
	})
	if err != nil {
		t.Fatalf("CreateTaskTodo 1: %v", err)
	}
	todo2, err := mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
		Title: "step 2", Instructions: "do it too", ParentTaskID: task.ID,
	})
	if err != nil {
		t.Fatalf("CreateTaskTodo 2: %v", err)
	}

	if todo1.Code == "" || todo2.Code == "" {
		t.Fatalf("expected non-empty Codes, got %q and %q", todo1.Code, todo2.Code)
	}
	if todo1.Code != "TD-1" || todo2.Code != "TD-2" {
		t.Errorf("Codes = %q, %q, want TD-1, TD-2", todo1.Code, todo2.Code)
	}

	updated, err := mgr.UpdateTaskTodoStatus(ctx, todo1.ID, mwanachamataskmanager.TodoStatusCompleted)
	if err != nil {
		t.Fatalf("UpdateTaskTodoStatus: %v", err)
	}
	if updated.Code != todo1.Code {
		t.Errorf("Code changed after status update: got %q, want %q", updated.Code, todo1.Code)
	}
}
