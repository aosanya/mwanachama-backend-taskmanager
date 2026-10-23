package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

const overdueAsOf = "2026-10-20T12:00:00Z"

func newOverdueManager(t *testing.T, graceHours any) mwanachamataskmanager.TaskManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := specForInstance(t, "spec/examples/work.taskmanager.json", "test")
	if graceHours != nil {
		for i := range s.Settings {
			if s.Settings[i].Name == "overdue_grace_hours" {
				s.Settings[i].Default = graceHours
			}
		}
	}
	if err := spec.Migrate(db, s); err != nil {
		t.Fatal(err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func createDue(t *testing.T, mgr mwanachamataskmanager.TaskManager, title, due string) mwanachamataskmanager.Task {
	t.Helper()
	task, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: title, DueAt: due})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func overdueTitles(t *testing.T, mgr mwanachamataskmanager.TaskManager) []string {
	t.Helper()
	got, err := mgr.ListOverdueTasks(context.Background(), mwanachamataskmanager.OverdueFilter{AsOf: overdueAsOf})
	if err != nil {
		t.Fatal(err)
	}
	titles := []string{}
	for _, o := range got {
		titles = append(titles, o.Title)
	}
	return titles
}

func TestTheBlueprintDeclaresAnOverdueGraceOfZeroHours(t *testing.T) {
	s, err := mwanachamataskmanager.SpecFor("w33")
	if err != nil {
		t.Fatal(err)
	}
	st, ok := s.Setting("overdue_grace_hours")
	if !ok || st.Type != spec.TypeInt {
		t.Fatalf("overdue_grace_hours declared %v as %+v, want an int setting", ok, st)
	}
	if n, _ := st.Default.(float64); n != 0 {
		t.Errorf("default %v, want 0", st.Default)
	}
}

func TestOverdueListCarriesOpenTasksPastTheirDueDateLongestOverdueFirst(t *testing.T) {
	mgr := newOverdueManager(t, nil)
	ctx := context.Background()
	createDue(t, mgr, "a day late", "2026-10-19T12:00:00Z")
	createDue(t, mgr, "a week late", "2026-10-13T12:00:00Z")
	createDue(t, mgr, "not yet due", "2026-10-21T12:00:00Z")
	createDue(t, mgr, "no due date", "")
	done := createDue(t, mgr, "late but done", "2026-10-10T12:00:00Z")
	done.Status = mwanachamataskmanager.TaskStatusInProgress
	done, err := mgr.UpdateTask(ctx, done)
	if err != nil {
		t.Fatal(err)
	}
	done.Status = mwanachamataskmanager.TaskStatusCompleted
	if _, err := mgr.UpdateTask(ctx, done); err != nil {
		t.Fatal(err)
	}
	cancelled := createDue(t, mgr, "late but cancelled", "2026-10-10T12:00:00Z")
	cancelled.Status = mwanachamataskmanager.TaskStatusCancelled
	if _, err := mgr.UpdateTask(ctx, cancelled); err != nil {
		t.Fatal(err)
	}

	got, err := mgr.ListOverdueTasks(ctx, mwanachamataskmanager.OverdueFilter{AsOf: overdueAsOf})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "a week late" || got[1].Title != "a day late" {
		t.Fatalf("got %+v, want the week-late task then the day-late one", got)
	}
	if got[0].OverdueHours != 168 || got[1].OverdueHours != 24 {
		t.Errorf("overdue hours %d and %d, want 168 and 24", got[0].OverdueHours, got[1].OverdueHours)
	}
	if got[0].State != "To do" {
		t.Errorf("state %q, want To do", got[0].State)
	}
}

func TestTheClockIsTheCallersToSet(t *testing.T) {
	mgr := newOverdueManager(t, nil)
	createDue(t, mgr, "due tomorrow", "2026-10-21T12:00:00Z")
	if titles := overdueTitles(t, mgr); len(titles) != 0 {
		t.Fatalf("as of %s got %v, want nothing", overdueAsOf, titles)
	}
	got, err := mgr.ListOverdueTasks(context.Background(), mwanachamataskmanager.OverdueFilter{AsOf: "2026-10-22T12:00:00Z"})
	if err != nil || len(got) != 1 {
		t.Fatalf("a day later got %v, %v, want the task", got, err)
	}
}

func TestTheGraceSettingAgesOpenWorkBeforeItIsListed(t *testing.T) {
	mgr := newOverdueManager(t, float64(48))
	createDue(t, mgr, "a day late", "2026-10-19T12:00:00Z")
	createDue(t, mgr, "three days late", "2026-10-17T12:00:00Z")
	titles := overdueTitles(t, mgr)
	if len(titles) != 1 || titles[0] != "three days late" {
		t.Fatalf("got %v, want only the task past the 48 hour grace", titles)
	}
}

func TestANegativeGraceIsRefusedWhenTheManagerIsBuilt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := specForInstance(t, "spec/examples/work.taskmanager.json", "test")
	for i := range s.Settings {
		if s.Settings[i].Name == "overdue_grace_hours" {
			s.Settings[i].Default = float64(-1)
		}
	}
	if _, err := mwanachamataskmanager.NewTaskManager(db, s, nil); err == nil {
		t.Fatal("a negative grace built a manager")
	}
}

func TestAnUnreadableAsOfIsRefused(t *testing.T) {
	mgr := newOverdueManager(t, nil)
	_, err := mgr.ListOverdueTasks(context.Background(), mwanachamataskmanager.OverdueFilter{AsOf: "yesterday"})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidOverdueFilter) {
		t.Fatalf("got %v, want ErrInvalidOverdueFilter", err)
	}
}
