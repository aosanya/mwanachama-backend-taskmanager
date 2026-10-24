package mwanachamataskmanager_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func storedStatesFixture(t *testing.T) (mwanachamataskmanager.TaskManager, *gorm.DB, *spec.Spec) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s, _ := specForInstance(t, "spec/examples/work.taskmanager.json", "test")
	if err := spec.Migrate(db, s); err != nil {
		t.Fatal(err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	return mgr, db, s
}

func agencyStates(names ...string) []any {
	statuses := [][]any{{"pending"}, {"in_progress"}, {"completed"}}
	out := []any{}
	for i, n := range names {
		out = append(out, map[string]any{"name": n, "statuses": statuses[i]})
	}
	return out
}

func TestAnInstanceWithNoStoredLayersReadsTheDefaultFive(t *testing.T) {
	mgr, _, _ := storedStatesFixture(t)
	task, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "To do" {
		t.Errorf("state %q, want To do", task.State)
	}
}

func TestReadsAnswerTheAgencysDefinedState(t *testing.T) {
	mgr, db, s := storedStatesFixture(t)
	ctx := context.Background()
	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mwanachamataskmanager.DefineTaskStates(db, s, agencyStates("Backlog", "Active", "Shipped")); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "Backlog" {
		t.Errorf("GetTask state %q, want Backlog", got.State)
	}
	list, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].State != "Backlog" {
		t.Errorf("ListTasks %+v", list)
	}
}

func TestTheOwnersOverrideWinsOverTheDefinitionAndClearingItRestoresIt(t *testing.T) {
	mgr, db, s := storedStatesFixture(t)
	ctx := context.Background()
	if err := mwanachamataskmanager.DefineTaskStates(db, s, agencyStates("Backlog")); err != nil {
		t.Fatal(err)
	}
	if _, err := mwanachamataskmanager.OverrideTaskStates(db, s, agencyStates("Inbox"), "owner-1", "2026-10-24T08:00:00Z"); err != nil {
		t.Fatal(err)
	}
	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if task.State != "Inbox" {
		t.Errorf("state %q, want Inbox", task.State)
	}
	_, cleared, err := mwanachamataskmanager.ClearTaskStatesOverride(db, s, "owner-1", "2026-10-24T09:00:00Z")
	if err != nil || !cleared {
		t.Fatalf("cleared %v err %v", cleared, err)
	}
	got, err := mgr.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "Backlog" {
		t.Errorf("state %q after clearing, want Backlog", got.State)
	}
	changes, err := spec.SettingChanges(db, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Errorf("%d changes recorded, want 2", len(changes))
	}
}

func TestDefiningTaskStatesKeepsTheOtherDefinedSettings(t *testing.T) {
	_, db, s := storedStatesFixture(t)
	if err := spec.PutDefinitionSettings(db, s, map[string]any{"overdue_grace_hours": 6}); err != nil {
		t.Fatal(err)
	}
	if err := mwanachamataskmanager.DefineTaskStates(db, s, agencyStates("Backlog")); err != nil {
		t.Fatal(err)
	}
	definition, _, err := spec.LoadSettingLayers(db, s)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := definition["overdue_grace_hours"]; !ok {
		t.Errorf("definition lost overdue_grace_hours: %+v", definition)
	}
}

func TestAStateListThatCannotBeReadIsRefusedBeforeItIsStored(t *testing.T) {
	_, db, s := storedStatesFixture(t)
	bad := []any{map[string]any{"name": "A", "statuses": []any{"nope"}}}
	if err := mwanachamataskmanager.DefineTaskStates(db, s, bad); err == nil || !strings.HasPrefix(err.Error(), "task states:") {
		t.Errorf("define: %v", err)
	}
	if _, err := mwanachamataskmanager.OverrideTaskStates(db, s, bad, "o", "2026-10-24T08:00:00Z"); err == nil || !strings.HasPrefix(err.Error(), "task states:") {
		t.Errorf("override: %v", err)
	}
	states, err := mwanachamataskmanager.StoredTaskStates(db, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != len(models.DefaultTaskStates()) {
		t.Errorf("stored states %+v, want the default five", states)
	}
}

func TestAStoredValueThatNoLongerParsesFailsTheReadNotSilently(t *testing.T) {
	mgr, db, s := storedStatesFixture(t)
	ctx := context.Background()
	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.PutDefinitionSettings(db, s, map[string]any{"task_states": []any{}}); err != nil {
		t.Skipf("the engine refuses an empty list itself: %v", err)
	}
	if _, err := mgr.GetTask(ctx, task.ID); err == nil || !strings.Contains(err.Error(), "task states:") {
		t.Errorf("GetTask err %v, want a task states refusal", err)
	}
}
