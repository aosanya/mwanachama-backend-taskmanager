// workflow_run_id_test.go — exercises the workflow_run_id propagation rules:
// persistence + edge write on create, list filter, chain-through on assign,
// mismatch rejection, and event payload propagation.
package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestCreateTask_WithWorkflowRunID_PersistsAndLinks(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, err := mgr.CreateWorkflowRun(ctx, "wfr-test", "next.requested", "tester")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{
		Title:         "t1",
		WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.WorkflowRunID != run.ID {
		t.Errorf("WorkflowRunID = %q, want %q", task.WorkflowRunID, run.ID)
	}

	// Re-read to confirm the property landed in storage, not just on the
	// returned struct.
	got, err := mgr.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.WorkflowRunID != run.ID {
		t.Errorf("re-read WorkflowRunID = %q, want %q", got.WorkflowRunID, run.ID)
	}

	// The started_task edge from run → task must exist after CreateTask.
	edges, err := mgr.TraverseRelationships(ctx, run.ID, mwanachamataskmanager.RelLabelStartedTask, mwanachamataskmanager.DirectionOutbound)
	if err != nil {
		t.Fatalf("traverse started_task: %v", err)
	}
	found := false
	for _, e := range edges {
		if e.ToID == task.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("started_task edge missing run=%s task=%s edges=%+v", run.ID, task.ID, edges)
	}
}

func TestCreateTask_WithoutWorkflowRunID_LeavesEmpty(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	task, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: "t"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.WorkflowRunID != "" {
		t.Errorf("WorkflowRunID = %q, want empty", task.WorkflowRunID)
	}
}

func TestListTasks_WorkflowRunIDFilter_ReturnsOnlyMatches(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	runA, _ := mgr.CreateWorkflowRun(ctx, "wfr-a", "", "")
	runB, _ := mgr.CreateWorkflowRun(ctx, "wfr-b", "", "")

	a1, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "a1", WorkflowRunID: runA.ID})
	_, _ = mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "a2", WorkflowRunID: runA.ID})
	_, _ = mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "b1", WorkflowRunID: runB.ID})
	_, _ = mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "free"})

	got, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{WorkflowRunID: runA.ID})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filtered count = %d, want 2 (got %+v)", len(got), got)
	}
	for _, tk := range got {
		if tk.WorkflowRunID != runA.ID {
			t.Errorf("task %s has WorkflowRunID %q, want %q", tk.ID, tk.WorkflowRunID, runA.ID)
		}
	}

	// Sanity: unfiltered list returns all four.
	all, _ := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if len(all) != 4 {
		t.Errorf("unfiltered count = %d, want 4", len(all))
	}
	_ = a1 // silence unused (the ID could be used for ordering but is not asserted)
}

func TestAssignTask_InheritsRunIDWhenStoredEmpty(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "wfr-inherit", "", "")
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "t"})
	agent, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "dev-01"})

	if err := mgr.AssignTask(ctx, task.ID, agent.ID, run.ID); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	got, _ := mgr.GetTask(ctx, task.ID)
	if got.WorkflowRunID != run.ID {
		t.Errorf("Task.WorkflowRunID = %q, want %q (inherited)", got.WorkflowRunID, run.ID)
	}

	// started_task edge must have been written by AssignTask too.
	edges, _ := mgr.TraverseRelationships(ctx, run.ID, mwanachamataskmanager.RelLabelStartedTask, mwanachamataskmanager.DirectionOutbound)
	found := false
	for _, e := range edges {
		if e.ToID == task.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("AssignTask did not write started_task edge run=%s task=%s", run.ID, task.ID)
	}

	// task.assigned payload must carry the run-id.
	ev, ok := findEvent(pub.full, mwanachamataskmanager.TopicTaskAssigned)
	if !ok {
		t.Fatal("no task.assigned event")
	}
	p := ev.Payload.(mwanachamataskmanager.TaskAssignedPayload)
	if p.WorkflowRunID != run.ID {
		t.Errorf("TaskAssignedPayload.WorkflowRunID = %q, want %q", p.WorkflowRunID, run.ID)
	}
}

func TestAssignTask_SameRunID_IsIdempotent(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "wfr-same", "", "")
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "t", WorkflowRunID: run.ID})
	agent, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "dev-01"})

	if err := mgr.AssignTask(ctx, task.ID, agent.ID, run.ID); err != nil {
		t.Fatalf("AssignTask with same run-id: %v", err)
	}
	got, _ := mgr.GetTask(ctx, task.ID)
	if got.WorkflowRunID != run.ID {
		t.Errorf("WorkflowRunID drifted to %q", got.WorkflowRunID)
	}
}

func TestAssignTask_MismatchedRunID_ReturnsErrWorkflowRunMismatch(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	runA, _ := mgr.CreateWorkflowRun(ctx, "wfr-A", "", "")
	runB, _ := mgr.CreateWorkflowRun(ctx, "wfr-B", "", "")
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "t", WorkflowRunID: runA.ID})
	agent, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "dev-01"})

	err := mgr.AssignTask(ctx, task.ID, agent.ID, runB.ID)
	if !errors.Is(err, mwanachamataskmanager.ErrWorkflowRunMismatch) {
		t.Fatalf("got %v, want ErrWorkflowRunMismatch", err)
	}

	got, _ := mgr.GetTask(ctx, task.ID)
	if got.WorkflowRunID != runA.ID {
		t.Errorf("WorkflowRunID changed despite rejected assign: got %q want %q", got.WorkflowRunID, runA.ID)
	}
}

func TestAssignTask_EmptyRunID_PreservesStored(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "wfr-pres", "", "")
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "t", WorkflowRunID: run.ID})
	agent, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "dev-01"})

	if err := mgr.AssignTask(ctx, task.ID, agent.ID, ""); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	got, _ := mgr.GetTask(ctx, task.ID)
	if got.WorkflowRunID != run.ID {
		t.Errorf("WorkflowRunID = %q, want %q (preserved)", got.WorkflowRunID, run.ID)
	}
}

func TestCreateTaskTodo_PersistsWorkflowRunID(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "wfr-todo", "", "")
	parent, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "p", WorkflowRunID: run.ID})

	todo, err := mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
		Title:         "t",
		Instructions:  "do it",
		ParentTaskID:  parent.ID,
		Ordinality:    1,
		WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTaskTodo: %v", err)
	}
	if todo.WorkflowRunID != run.ID {
		t.Errorf("todo WorkflowRunID = %q, want %q", todo.WorkflowRunID, run.ID)
	}

	got, _ := mgr.GetTaskTodo(ctx, todo.ID)
	if got.WorkflowRunID != run.ID {
		t.Errorf("re-read todo WorkflowRunID = %q, want %q", got.WorkflowRunID, run.ID)
	}
}

func TestTaskCreatedPayload_CarriesWorkflowRunID(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "wfr-ev", "", "")
	created, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "t", WorkflowRunID: run.ID})

	ev, ok := findEvent(pub.full, mwanachamataskmanager.TopicTaskCreated)
	if !ok {
		t.Fatal("no task.created event")
	}
	p := ev.Payload.(mwanachamataskmanager.TaskCreatedPayload)
	if p.TaskID != created.ID {
		t.Errorf("payload TaskID = %q, want %q", p.TaskID, created.ID)
	}
	if p.WorkflowRunID != run.ID {
		t.Errorf("payload WorkflowRunID = %q, want %q", p.WorkflowRunID, run.ID)
	}
}
