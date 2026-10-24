package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func sourcedRequest() mwanachamataskmanager.OpenSourcedTasks {
	return mwanachamataskmanager.OpenSourcedTasks{
		Source: "forms/agy1/result-7",
		Items: []mwanachamataskmanager.SourcedTaskItem{
			{Key: "1", Title: "Repair the borehole", DueAt: "2026-11-30T00:00:00Z"},
			{Title: "Publish the budget", Description: "In the ward office", DueAt: "2026-12-15T09:00:00+03:00"},
		},
	}
}

func TestOpenSourcedTasksOpensOneDatedTaskPerItem(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	got, err := m.OpenSourcedTasks(ctx, sourcedRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(got.Items))
	}
	for i, want := range []string{"2026-11-30T00:00:00Z", "2026-12-15T06:00:00Z"} {
		o := got.Items[i]
		if !o.Opened || o.TaskID == "" {
			t.Fatalf("item %d not opened: %+v", i, o)
		}
		task, err := m.GetTask(ctx, o.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		if task.DueAt != want || task.LinkedRef != "forms/agy1/result-7" {
			t.Fatalf("task %d = due %q ref %q", i, task.DueAt, task.LinkedRef)
		}
	}
	if got.Items[1].Key != "Publish the budget" {
		t.Fatalf("a keyless item is keyed by its title, got %q", got.Items[1].Key)
	}
}

func TestOpenSourcedTasksReplayOpensNothingAgain(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	first, err := m.OpenSourcedTasks(ctx, sourcedRequest())
	if err != nil {
		t.Fatal(err)
	}
	req := sourcedRequest()
	req.Items = append(req.Items, mwanachamataskmanager.SourcedTaskItem{Key: "3", Title: "Light the market", DueAt: "2027-01-01T00:00:00Z"})
	again, err := m.OpenSourcedTasks(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if again.Items[i].Opened || again.Items[i].TaskID != first.Items[i].TaskID {
			t.Fatalf("item %d reopened: %+v", i, again.Items[i])
		}
	}
	if !again.Items[2].Opened {
		t.Fatal("a new item on a replay is opened")
	}
	other := sourcedRequest()
	other.Source = "forms/agy1/result-8"
	fresh, err := m.OpenSourcedTasks(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh.Items[0].Opened {
		t.Fatal("the same key under another source opens its own task")
	}
}

func TestOpenSourcedTasksRefusesABadRequestWholly(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	cases := map[string]mwanachamataskmanager.OpenSourcedTasks{
		"no source":    {Items: sourcedRequest().Items},
		"no items":     {Source: "s"},
		"bad due":      {Source: "s", Items: []mwanachamataskmanager.SourcedTaskItem{{Title: "a", DueAt: "soon"}}},
		"no title":     {Source: "s", Items: []mwanachamataskmanager.SourcedTaskItem{{DueAt: "2026-11-30T00:00:00Z"}}},
		"repeated key": {Source: "s", Items: []mwanachamataskmanager.SourcedTaskItem{{Key: "k", Title: "a", DueAt: "2026-11-30T00:00:00Z"}, {Key: "k", Title: "b", DueAt: "2026-11-30T00:00:00Z"}}},
	}
	for name, req := range cases {
		if _, err := m.OpenSourcedTasks(ctx, req); !errors.Is(err, mwanachamataskmanager.ErrInvalidSourcedTasks) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	tasks, err := m.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("a refused request opened %d tasks", len(tasks))
	}
}
