package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	tm "github.com/aosanya/mwanachama-backend-taskmanager"
)

func countTopic(p *recordingPublisher, topic string) int {
	n := 0
	for _, e := range p.events {
		if e == topic {
			n++
		}
	}
	return n
}

func TestOpeningACaseTakesTheRequesterFromTheCallerAndPublishesOpened(t *testing.T) {
	ctx := context.Background()
	m, pub := newManagerWithPublisher(t)
	c, err := m.OpenCase(ctx, "resident-1", tm.Case{
		ID: "forged", RequesterID: "someone-else", Status: tm.CaseStatusDone,
		SubjectKind: "form", SubjectID: "f-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.ID == "forged" || c.RequesterID != "resident-1" || c.Status != tm.CaseStatusOpen {
		t.Fatalf("server-owned fields not owned: %+v", c)
	}
	got, err := m.GetCase(ctx, c.ID)
	if err != nil || got.SubjectKind != "form" || got.SubjectID != "f-1" {
		t.Fatalf("GetCase = %+v, %v", got, err)
	}
	if countTopic(pub, tm.TopicCaseOpened) != 1 {
		t.Fatalf("case.opened published %d times", countTopic(pub, tm.TopicCaseOpened))
	}
}

func TestOpeningACaseWithNoRequesterIsRefused(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.OpenCase(context.Background(), "", tm.Case{}); !errors.Is(err, tm.ErrInvalidCase) {
		t.Fatalf("err = %v, want ErrInvalidCase", err)
	}
}

func TestAnUnknownCaseIsNotFound(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.GetCase(context.Background(), "nope"); !errors.Is(err, tm.ErrCaseNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestListCasesNarrowsByRequesterAndStatus(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	for _, r := range []string{"a", "a", "b"} {
		if _, err := m.OpenCase(ctx, r, tm.Case{}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.ListCases(ctx, tm.CaseFilter{RequesterID: "a", Status: tm.CaseStatusOpen})
	if err != nil || len(got) != 2 {
		t.Fatalf("ListCases = %d, %v", len(got), err)
	}
	none, _ := m.ListCases(ctx, tm.CaseFilter{Status: tm.CaseStatusDone})
	if len(none) != 0 {
		t.Fatalf("done cases = %d", len(none))
	}
}

func setStatus(t *testing.T, m tm.TaskManager, id string, s tm.TaskStatus) {
	t.Helper()
	task, err := m.GetTask(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	task.Status = s
	if _, err := m.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("UpdateTask %s -> %s: %v", id, s, err)
	}
}

func TestACaseClosesDoneWhenItsLastOpenTaskIsCompletedOrCancelled(t *testing.T) {
	ctx := context.Background()
	m, pub := newManagerWithPublisher(t)
	c, _ := m.OpenCase(ctx, "resident-1", tm.Case{})
	var ids []string
	for _, title := range []string{"one", "two"} {
		task, err := m.CreateTask(ctx, tm.Task{Title: title})
		if err != nil {
			t.Fatal(err)
		}
		if err := m.LinkCaseTask(ctx, c.ID, task.ID); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, task.ID)
	}
	tasks, err := m.ListCaseTasks(ctx, c.ID)
	if err != nil || len(tasks) != 2 {
		t.Fatalf("ListCaseTasks = %d, %v", len(tasks), err)
	}

	setStatus(t, m, ids[0], tm.TaskStatusInProgress)
	setStatus(t, m, ids[0], tm.TaskStatusCompleted)
	if got, _ := m.GetCase(ctx, c.ID); got.Status != tm.CaseStatusOpen {
		t.Fatalf("case closed with a task still open: %s", got.Status)
	}

	setStatus(t, m, ids[1], tm.TaskStatusCancelled)
	got, _ := m.GetCase(ctx, c.ID)
	if got.Status != tm.CaseStatusDone || got.ClosedAt == "" {
		t.Fatalf("case = %+v, want done with closed_at", got)
	}
	if countTopic(pub, tm.TopicCaseClosed) != 1 {
		t.Fatalf("case.closed published %d times", countTopic(pub, tm.TopicCaseClosed))
	}
}

func TestAFailedTaskKeepsItsCaseOpen(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	c, _ := m.OpenCase(ctx, "r", tm.Case{})
	task, _ := m.CreateTask(ctx, tm.Task{Title: "x"})
	_ = m.LinkCaseTask(ctx, c.ID, task.ID)
	setStatus(t, m, task.ID, tm.TaskStatusInProgress)
	setStatus(t, m, task.ID, tm.TaskStatusFailed)
	if got, _ := m.GetCase(ctx, c.ID); got.Status != tm.CaseStatusOpen {
		t.Fatalf("status = %s, want open", got.Status)
	}
}

func TestATaskServesOneCaseAndAClosedCaseTakesNoTasks(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	a, _ := m.OpenCase(ctx, "r", tm.Case{})
	b, _ := m.OpenCase(ctx, "r", tm.Case{})
	task, _ := m.CreateTask(ctx, tm.Task{Title: "x"})
	if err := m.LinkCaseTask(ctx, a.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkCaseTask(ctx, a.ID, task.ID); err != nil {
		t.Fatalf("relinking the same pair = %v, want nil", err)
	}
	if err := m.LinkCaseTask(ctx, b.ID, task.ID); !errors.Is(err, tm.ErrTaskAlreadyInCase) {
		t.Fatalf("err = %v, want ErrTaskAlreadyInCase", err)
	}
	setStatus(t, m, task.ID, tm.TaskStatusCancelled)
	other, _ := m.CreateTask(ctx, tm.Task{Title: "y"})
	if err := m.LinkCaseTask(ctx, a.ID, other.ID); !errors.Is(err, tm.ErrCaseNotOpen) {
		t.Fatalf("err = %v, want ErrCaseNotOpen", err)
	}
}
