package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func generateRequest(subjects ...string) mwanachamataskmanager.GenerateTasks {
	return mwanachamataskmanager.GenerateTasks{
		Run: "agency/agy1/ward-survey-2026",
		Template: mwanachamataskmanager.TaskTemplate{
			Title:       "Visit the ward office",
			Description: "Collect the signed register",
			Priority:    "high",
			DueAt:       "2026-12-01T09:00:00+03:00",
		},
		Subjects: subjects,
	}
}

func TestGenerateTasksCreatesOneTaskPerSubjectCarryingItsKey(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	got, err := m.GenerateTasks(ctx, generateRequest("actor/agy1/ward-1", "actor/agy1/ward-2"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(got.Items))
	}
	for i, subject := range []string{"actor/agy1/ward-1", "actor/agy1/ward-2"} {
		o := got.Items[i]
		if !o.Created || o.Subject != subject {
			t.Fatalf("item %d = %+v", i, o)
		}
		task, err := m.GetTask(ctx, o.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if task.LinkedRef != subject || task.Title != "Visit the ward office" || task.DueAt != "2026-12-01T06:00:00Z" || task.Priority != "high" {
			t.Fatalf("task %d = %+v", i, task)
		}
	}
}

func TestGenerateTasksReplayCreatesNothingAgain(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	first, err := m.GenerateTasks(ctx, generateRequest("a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := m.GenerateTasks(ctx, generateRequest("a", "b", "c"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if again.Items[i].Created || again.Items[i].TaskID != first.Items[i].TaskID {
			t.Fatalf("item %d regenerated: %+v", i, again.Items[i])
		}
	}
	if !again.Items[2].Created {
		t.Fatal("a new subject on a replay is generated")
	}
	other := generateRequest("a")
	other.Run = "agency/agy1/ward-survey-2027"
	fresh, err := m.GenerateTasks(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh.Items[0].Created {
		t.Fatal("the same subject under another run gets its own task")
	}
	tasks, err := m.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 4 {
		t.Fatalf("tasks = %d, want 4", len(tasks))
	}
}

func TestGenerateTasksRefusesABadRequestWholly(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	noRun := generateRequest("a")
	noRun.Run = ""
	noTitle := generateRequest("a")
	noTitle.Template.Title = ""
	badDue := generateRequest("a")
	badDue.Template.DueAt = "soon"
	cases := map[string]mwanachamataskmanager.GenerateTasks{
		"no run":           noRun,
		"no title":         noTitle,
		"bad due":          badDue,
		"no subjects":      generateRequest(),
		"blank subject":    generateRequest("a", " "),
		"repeated subject": generateRequest("a", "b", "a"),
	}
	for name, req := range cases {
		if _, err := m.GenerateTasks(ctx, req); !errors.Is(err, mwanachamataskmanager.ErrInvalidGeneratedTasks) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	tasks, err := m.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("a refused request created %d tasks", len(tasks))
	}
}

func TestGenerateTasksRefusesMoreThanTheCap(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	subjects := make([]string, mwanachamataskmanager.MaxGeneratedSubjects+1)
	for i := range subjects {
		subjects[i] = fmt.Sprintf("s-%d", i)
	}
	if _, err := m.GenerateTasks(ctx, generateRequest(subjects...)); !errors.Is(err, mwanachamataskmanager.ErrTooManySubjects) {
		t.Fatalf("err = %v", err)
	}
	tasks, err := m.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("an over-cap request created %d tasks", len(tasks))
	}
}
