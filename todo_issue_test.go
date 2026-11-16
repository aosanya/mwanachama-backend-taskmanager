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

type todoFixture struct {
	mgr   mwanachamataskmanager.TaskManager
	db    *gorm.DB
	spec  *spec.Spec
	table string
}

func newTodoFixture(t *testing.T) todoFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	workSpec := specForInstance(t, "spec/examples/agency.taskmanager.json", "test")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatal(err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatal(err)
	}
	object, ok := workSpec.ByRole("task_todo")
	if !ok {
		t.Fatal("spec declares no task_todo object")
	}
	return todoFixture{mgr: mgr, db: db, spec: workSpec, table: workSpec.TableFor(object)}
}

func (f todoFixture) piece(t *testing.T, issue, title string, ordinality int, waitsOn ...string) mwanachamataskmanager.TaskTodo {
	t.Helper()
	made, err := f.mgr.CreateTaskTodo(context.Background(), mwanachamataskmanager.TaskTodo{
		Title: title, Instructions: "do " + title, ParentTaskID: issue, Ordinality: ordinality, DependsOn: waitsOn,
	})
	if err != nil {
		t.Fatalf("create %q: %v", title, err)
	}
	return made
}

func todoTitles(todos []mwanachamataskmanager.TaskTodo) []string {
	out := make([]string, len(todos))
	for at, todo := range todos {
		out[at] = todo.Title
	}
	return out
}

func sameTodoIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for at := range a {
		if a[at] != b[at] {
			return false
		}
	}
	return true
}

func TestListIssueTodosReturnsLiveTodosOfOneIssueInOrder(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	f.piece(t, "issue-a", "third", 3)
	f.piece(t, "issue-a", "first", 1)
	gone := f.piece(t, "issue-a", "removed", 4)
	f.piece(t, "issue-a", "second", 2)
	f.piece(t, "issue-b", "elsewhere", 1)
	if err := f.mgr.DeleteTaskTodo(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}

	got, err := f.mgr.ListIssueTodos(ctx, "issue-a")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"first", "second", "third"}; !sameTodoIDs(todoTitles(got), want) {
		t.Fatalf("list = %v, want %v", todoTitles(got), want)
	}
}

func TestCreateWithoutOrdinalityAppendsToTheEndOfTheList(t *testing.T) {
	f := newTodoFixture(t)
	f.piece(t, "issue-a", "one", 0)
	f.piece(t, "issue-a", "two", 0)
	got, err := f.mgr.ListIssueTodos(context.Background(), "issue-a")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Ordinality != 1 || got[1].Ordinality != 2 {
		t.Fatalf("ordinalities = %d, %d, want 1, 2", got[0].Ordinality, got[1].Ordinality)
	}
}

func TestBlockedReasonRoundTripsAndIsClearedWhenLifted(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	piece := f.piece(t, "issue-a", "wait for the vendor", 1)

	blocked, err := f.mgr.SetTaskTodoStatus(ctx, piece.ID, mwanachamataskmanager.TodoStatusBlocked, "vendor has not replied")
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Status != mwanachamataskmanager.TodoStatusBlocked || blocked.BlockedReason != "vendor has not replied" {
		t.Fatalf("after block: %+v", blocked)
	}
	read, err := f.mgr.GetTaskTodo(ctx, piece.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.BlockedReason != "vendor has not replied" {
		t.Fatalf("read back reason = %q", read.BlockedReason)
	}

	lifted, err := f.mgr.SetTaskTodoStatus(ctx, piece.ID, mwanachamataskmanager.TodoStatusPending, "")
	if err != nil {
		t.Fatal(err)
	}
	if lifted.BlockedReason != "" || lifted.Status != mwanachamataskmanager.TodoStatusPending {
		t.Fatalf("after lift: %+v", lifted)
	}
}

func TestBlockedByHandNeedsAReason(t *testing.T) {
	f := newTodoFixture(t)
	piece := f.piece(t, "issue-a", "one", 1)
	_, err := f.mgr.SetTaskTodoStatus(context.Background(), piece.ID, mwanachamataskmanager.TodoStatusBlocked, "")
	if !errors.Is(err, mwanachamataskmanager.ErrTaskTodoBlockedReasonRequired) {
		t.Fatalf("err = %v, want ErrTaskTodoBlockedReasonRequired", err)
	}
}

func TestUpdateStatusClearsTheReason(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	piece := f.piece(t, "issue-a", "one", 1)
	if _, err := f.mgr.SetTaskTodoStatus(ctx, piece.ID, mwanachamataskmanager.TodoStatusBlocked, "reason"); err != nil {
		t.Fatal(err)
	}
	done, err := f.mgr.UpdateTaskTodoStatus(ctx, piece.ID, mwanachamataskmanager.TodoStatusCompleted)
	if err != nil {
		t.Fatal(err)
	}
	if done.BlockedReason != "" {
		t.Fatalf("reason kept: %q", done.BlockedReason)
	}
}

func TestCreateResolvesOrdinalitiesToIds(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	first := f.piece(t, "issue-a", "first", 1)
	second := f.piece(t, "issue-a", "second", 2)
	made, err := f.mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
		Title: "third", Instructions: "do third", ParentTaskID: "issue-a", Ordinality: 3,
		DependsOnOrdinalities: []int{1, 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sameTodoIDs(made.DependsOn, []string{first.ID, second.ID}) {
		t.Fatalf("depends_on = %v, want ids %v", made.DependsOn, []string{first.ID, second.ID})
	}
	if made.Status != mwanachamataskmanager.TodoStatusBlocked {
		t.Fatalf("status = %s, want blocked", made.Status)
	}
	if len(made.DependsOnOrdinalities) != 0 {
		t.Fatalf("ordinalities leaked into the stored todo: %v", made.DependsOnOrdinalities)
	}
}

func TestCreateRefusesAnOrdinalityThatIsNotInTheIssue(t *testing.T) {
	f := newTodoFixture(t)
	f.piece(t, "issue-a", "first", 1)
	_, err := f.mgr.CreateTaskTodo(context.Background(), mwanachamataskmanager.TaskTodo{
		Title: "second", Instructions: "x", ParentTaskID: "issue-a", Ordinality: 2, DependsOnOrdinalities: []int{9},
	})
	if !errors.Is(err, mwanachamataskmanager.ErrTaskTodoDependencyNotFound) {
		t.Fatalf("err = %v, want ErrTaskTodoDependencyNotFound", err)
	}
}

func TestDependsOnSurvivesAReorder(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	first := f.piece(t, "issue-a", "first", 1)
	second := f.piece(t, "issue-a", "second", 2, first.ID)
	third := f.piece(t, "issue-a", "third", 3, second.ID)

	reordered, err := f.mgr.ReorderTaskTodos(ctx, "issue-a", []string{third.ID, first.ID, second.ID})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"third", "first", "second"}; !sameTodoIDs(todoTitles(reordered), want) {
		t.Fatalf("order = %v, want %v", todoTitles(reordered), want)
	}
	for at, todo := range reordered {
		if todo.Ordinality != at+1 {
			t.Fatalf("%s ordinality = %d, want %d", todo.Title, todo.Ordinality, at+1)
		}
	}
	again, err := f.mgr.GetTaskTodo(ctx, third.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sameTodoIDs(again.DependsOn, []string{second.ID}) {
		t.Fatalf("third now waits on %v, want %v", again.DependsOn, []string{second.ID})
	}
}

func TestReorderMustNameEveryLiveTodoOnce(t *testing.T) {
	f := newTodoFixture(t)
	first := f.piece(t, "issue-a", "first", 1)
	f.piece(t, "issue-a", "second", 2)
	other := f.piece(t, "issue-b", "other", 1)
	for name, ids := range map[string][]string{
		"missing one":      {first.ID},
		"a foreign id":     {first.ID, other.ID},
		"a repeated id":    {first.ID, first.ID},
		"an unknown id":    {first.ID, "no-such-id"},
		"an empty request": {},
	} {
		if _, err := f.mgr.ReorderTaskTodos(context.Background(), "issue-a", ids); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoOrderMismatch) {
			t.Fatalf("%s: err = %v, want ErrTaskTodoOrderMismatch", name, err)
		}
	}
}

func TestDeleteRemovesTheIdFromDependentsAndHidesTheTodoFromEveryRead(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	first := f.piece(t, "issue-a", "first", 1)
	second := f.piece(t, "issue-a", "second", 2)
	third := f.piece(t, "issue-a", "third", 3, first.ID, second.ID)

	if err := f.mgr.DeleteTaskTodo(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.GetTaskTodo(ctx, first.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoNotFound) {
		t.Fatalf("get deleted: err = %v", err)
	}
	if err := f.mgr.DeleteTaskTodo(ctx, first.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoNotFound) {
		t.Fatalf("delete twice: err = %v", err)
	}
	if _, err := f.mgr.UpdateTaskTodoStatus(ctx, first.ID, mwanachamataskmanager.TodoStatusCompleted); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoNotFound) {
		t.Fatalf("status of deleted: err = %v", err)
	}
	listed, err := f.mgr.ListIssueTodos(ctx, "issue-a")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"second", "third"}; !sameTodoIDs(todoTitles(listed), want) {
		t.Fatalf("list = %v, want %v", todoTitles(listed), want)
	}
	counts, err := f.mgr.CountIssueTodos(ctx, []string{"issue-a"})
	if err != nil {
		t.Fatal(err)
	}
	if counts["issue-a"].Total != 2 {
		t.Fatalf("total = %d, want 2", counts["issue-a"].Total)
	}
	left, err := f.mgr.GetTaskTodo(ctx, third.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sameTodoIDs(left.DependsOn, []string{second.ID}) {
		t.Fatalf("third waits on %v, want %v", left.DependsOn, []string{second.ID})
	}
	all, err := f.mgr.ListTaskTodos(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, todo := range all {
		if todo.ID == first.ID {
			t.Fatal("ListTaskTodos returned a deleted todo")
		}
	}
}

func TestCountIssueTodosAnswersForManyIssuesInOneCall(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	a1 := f.piece(t, "issue-a", "a1", 1)
	a2 := f.piece(t, "issue-a", "a2", 2)
	f.piece(t, "issue-a", "a3", 3)
	b1 := f.piece(t, "issue-b", "b1", 1)
	f.piece(t, "issue-c", "c1", 1)
	for _, id := range []string{a1.ID, a2.ID, b1.ID} {
		if _, err := f.mgr.UpdateTaskTodoStatus(ctx, id, mwanachamataskmanager.TodoStatusCompleted); err != nil {
			t.Fatal(err)
		}
	}

	got, err := f.mgr.CountIssueTodos(ctx, []string{"issue-a", "issue-b", "issue-none"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]mwanachamataskmanager.TodoCount{
		"issue-a": {Done: 2, Total: 3},
		"issue-b": {Done: 1, Total: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for issue, count := range want {
		if got[issue] != count {
			t.Fatalf("%s = %+v, want %+v", issue, got[issue], count)
		}
	}
	if empty, err := f.mgr.CountIssueTodos(ctx, nil); err != nil || len(empty) != 0 {
		t.Fatalf("no ids: %v, %v", empty, err)
	}
}

func TestCycleIsRefused(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	first := f.piece(t, "issue-a", "first", 1)
	second := f.piece(t, "issue-a", "second", 2, first.ID)
	third := f.piece(t, "issue-a", "third", 3, second.ID)

	cases := map[string][2]any{
		"onto itself":        {first.ID, []string{first.ID}},
		"a two-piece loop":   {first.ID, []string{second.ID}},
		"a three-piece loop": {first.ID, []string{third.ID}},
	}
	for name, c := range cases {
		patch := mwanachamataskmanager.TaskTodoPatch{DependsOn: pointerTo(c[1].([]string))}
		if _, err := f.mgr.UpdateTaskTodo(ctx, c[0].(string), patch); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoCycle) {
			t.Fatalf("%s: err = %v, want ErrTaskTodoCycle", name, err)
		}
		if err := f.mgr.CheckTodoCycle(ctx, c[0].(string), c[1].([]string)); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoCycle) {
			t.Fatalf("%s: check err = %v, want ErrTaskTodoCycle", name, err)
		}
	}
	unchanged, err := f.mgr.GetTaskTodo(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(unchanged.DependsOn) != 0 {
		t.Fatalf("a refused change was stored: %v", unchanged.DependsOn)
	}
	if err := f.mgr.CheckTodoCycle(ctx, third.ID, []string{first.ID}); err != nil {
		t.Fatalf("a legal edge was refused: %v", err)
	}
}

func TestCrossIssueDependencyIsRefused(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	mine := f.piece(t, "issue-a", "mine", 1)
	foreign := f.piece(t, "issue-b", "foreign", 1)

	patch := mwanachamataskmanager.TaskTodoPatch{DependsOn: pointerTo([]string{foreign.ID})}
	if _, err := f.mgr.UpdateTaskTodo(ctx, mine.ID, patch); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoCrossIssue) {
		t.Fatalf("update: err = %v, want ErrTaskTodoCrossIssue", err)
	}
	if err := f.mgr.CheckTodoSameIssue(ctx, "issue-a", []string{foreign.ID}); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoCrossIssue) {
		t.Fatalf("check: err = %v, want ErrTaskTodoCrossIssue", err)
	}
	_, err := f.mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
		Title: "new", Instructions: "x", ParentTaskID: "issue-a", DependsOn: []string{foreign.ID},
	})
	if !errors.Is(err, mwanachamataskmanager.ErrTaskTodoCrossIssue) {
		t.Fatalf("create: err = %v, want ErrTaskTodoCrossIssue", err)
	}
	if err := f.mgr.CheckTodoSameIssue(ctx, "issue-a", []string{"no-such-id"}); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoDependencyNotFound) {
		t.Fatalf("unknown id: err = %v, want ErrTaskTodoDependencyNotFound", err)
	}
}

func TestUpdateTaskTodoEditsTitleDescriptionAndWhatItWaitsOn(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	first := f.piece(t, "issue-a", "first", 1)
	second := f.piece(t, "issue-a", "second", 2)

	renamed := "second, renamed"
	about := "why it exists"
	updated, err := f.mgr.UpdateTaskTodo(ctx, second.ID, mwanachamataskmanager.TaskTodoPatch{
		Title: &renamed, Description: &about, DependsOn: pointerTo([]string{first.ID}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != renamed || updated.Description != about || !sameTodoIDs(updated.DependsOn, []string{first.ID}) {
		t.Fatalf("updated = %+v", updated)
	}
	if updated.Code != second.Code {
		t.Fatalf("code changed from %s to %s", second.Code, updated.Code)
	}
	if _, err := f.mgr.UpdateTaskTodo(ctx, "no-such-id", mwanachamataskmanager.TaskTodoPatch{Title: &renamed}); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoNotFound) {
		t.Fatalf("unknown todo: err = %v", err)
	}
	blank := ""
	if _, err := f.mgr.UpdateTaskTodo(ctx, second.ID, mwanachamataskmanager.TaskTodoPatch{Title: &blank}); !errors.Is(err, mwanachamataskmanager.ErrInvalidTask) {
		t.Fatalf("blank title: err = %v", err)
	}
}

func TestLegacyRowsKeepingOrdinalitiesAreMigratedToIdsWhenTheManagerStarts(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	first := f.piece(t, "issue-a", "first", 1)
	second := f.piece(t, "issue-a", "second", 2)
	third := f.piece(t, "issue-a", "third", 3)
	otherFirst := f.piece(t, "issue-b", "other first", 1)
	otherSecond := f.piece(t, "issue-b", "other second", 2)
	dangling := f.piece(t, "issue-b", "dangling", 3)

	legacy := map[string]string{
		second.ID:      "[1]",
		third.ID:       "[1,2]",
		otherSecond.ID: "[1]",
		dangling.ID:    "[2,9]",
	}
	for id, ordinalities := range legacy {
		if err := f.db.Exec("update "+f.table+" set depends_on = ? where id = ?", ordinalities, id).Error; err != nil {
			t.Fatal(err)
		}
	}

	for run := 0; run < 2; run++ {
		mgr, err := mwanachamataskmanager.NewTaskManager(f.db, f.spec, nil)
		if err != nil {
			t.Fatalf("start %d: %v", run+1, err)
		}
		want := map[string][]string{
			first.ID:       nil,
			second.ID:      {first.ID},
			third.ID:       {first.ID, second.ID},
			otherFirst.ID:  nil,
			otherSecond.ID: {otherFirst.ID},
			dangling.ID:    {otherSecond.ID},
		}
		for id, ids := range want {
			got, err := mgr.GetTaskTodo(ctx, id)
			if err != nil {
				t.Fatalf("start %d: read %s: %v", run+1, id, err)
			}
			if !sameTodoIDs(got.DependsOn, ids) {
				t.Fatalf("start %d: %s waits on %v, want %v", run+1, got.Title, got.DependsOn, ids)
			}
		}
	}
}

func TestLegacyRowsOfADeletedTodoAreMigratedToo(t *testing.T) {
	f := newTodoFixture(t)
	first := f.piece(t, "issue-a", "first", 1)
	second := f.piece(t, "issue-a", "second", 2)
	if err := f.db.Exec("update "+f.table+" set depends_on = '[1]', deleted = ? where id = ?", true, second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := mwanachamataskmanager.NewTaskManager(f.db, f.spec, nil); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := f.db.Raw("select depends_on from "+f.table+" where id = ?", second.ID).Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if want := `["` + first.ID + `"]`; stored != want {
		t.Fatalf("stored = %s, want %s", stored, want)
	}
}

func pointerTo[T any](v T) *T { return &v }
