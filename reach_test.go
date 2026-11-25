package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	emeaTask = "emea task"
	apacTask = "apac task"
	openTask = "untagged task"
)

type reachBoard struct {
	mgr    mwanachamataskmanager.TaskManager
	issues map[string]mwanachamataskmanager.Task
}

func reachPeople() mwanachamataskmanager.GroupTags {
	byPerson := map[string][]string{
		"emea":  {"emea"},
		"apac":  {"apac"},
		"admin": {"all"},
		"both":  {"emea", "apac"},
		"none":  nil,
	}
	return func(_ context.Context, id string) ([]string, error) { return byPerson[id], nil }
}

func newReachBoard(t *testing.T) reachBoard {
	t.Helper()
	ctx := context.Background()
	mgr := newTestManager(t)
	mgr.UseGroupTags(reachPeople())
	for _, b := range []struct{ maker, name string }{{"emea", "EMEA board"}, {"apac", "APAC board"}, {"", "Open board"}} {
		if _, err := mgr.CreateBacklogAs(ctx, b.maker, b.name, []string{"To do", "In progress"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	issues := map[string]mwanachamataskmanager.Task{}
	for title, backlog := range map[string]string{emeaTask: "emea-board", apacTask: "apac-board", openTask: "open-board"} {
		made, err := mgr.CreateTaskAs(ctx, "", mwanachamataskmanager.Task{Title: title, Backlog: backlog, Step: "to-do", Tags: []string{"Ready"}})
		if err != nil {
			t.Fatal(err)
		}
		issues[title] = made
	}
	return reachBoard{mgr: mgr, issues: issues}
}

func (b reachBoard) idOf(title string) string { return b.issues[title].ID }

var reachExpectations = map[string][]string{
	"emea":  {emeaTask},
	"apac":  {apacTask},
	"admin": {apacTask, emeaTask, openTask},
	"both":  {apacTask, emeaTask},
	"none":  {},
	"":      {apacTask, emeaTask, openTask},
}

func sorted(titles []string) []string {
	out := append([]string{}, titles...)
	sort.Strings(out)
	if out == nil {
		out = []string{}
	}
	return out
}

func eachPerson(t *testing.T, check func(t *testing.T, person string, want []string)) {
	t.Helper()
	for person, want := range reachExpectations {
		t.Run("as "+person, func(t *testing.T) { check(t, person, want) })
	}
}

func onlyBoardTitles(titles []string) []string {
	kept := []string{}
	for _, title := range titles {
		if title == emeaTask || title == apacTask || title == openTask {
			kept = append(kept, title)
		}
	}
	return sorted(kept)
}

func wantReached(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(onlyBoardTitles(got), sorted(want)) {
		t.Fatalf("%s: got %v, want %v", what, onlyBoardTitles(got), sorted(want))
	}
}

func pendingTitles(items []models.PendingItem) []string {
	out := []string{}
	for _, i := range items {
		out = append(out, i.Title)
	}
	return out
}

func TestReach_ListTasksShowsOnlyWhatTheGroupReaches(t *testing.T) {
	b := newReachBoard(t)
	eachPerson(t, func(t *testing.T, person string, want []string) {
		tasks, err := b.mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{RequesterID: person})
		if err != nil {
			t.Fatal(err)
		}
		titles := []string{}
		for _, task := range tasks {
			titles = append(titles, task.Title)
		}
		wantReached(t, "ListTasks", titles, want)
	})
}

func TestReach_PendingAndFindShowOnlyWhatTheGroupReaches(t *testing.T) {
	b := newReachBoard(t)
	eachPerson(t, func(t *testing.T, person string, want []string) {
		pending, err := b.mgr.PendingTasks(context.Background(), mwanachamataskmanager.PendingFilter{RequesterID: person})
		if err != nil {
			t.Fatal(err)
		}
		wantReached(t, "PendingTasks", pendingTitles(pending.Items), want)
		found, err := b.mgr.FindTasks(context.Background(), mwanachamataskmanager.FindFilter{Q: "task", RequesterID: person})
		if err != nil {
			t.Fatal(err)
		}
		wantReached(t, "FindTasks", pendingTitles(found.Items), want)
	})
}

func TestReach_GetTaskForRefusesATaskTheGroupDoesNotReach(t *testing.T) {
	b := newReachBoard(t)
	eachPerson(t, func(t *testing.T, person string, want []string) {
		for _, title := range []string{emeaTask, apacTask, openTask} {
			_, err := b.mgr.GetTaskFor(context.Background(), person, b.idOf(title))
			reached := containsTitle(want, title)
			if reached && err != nil {
				t.Fatalf("%s should reach %s: %v", person, title, err)
			}
			if !reached && !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
				t.Fatalf("%s should not reach %s: %v", person, title, err)
			}
		}
	})
}

func containsTitle(titles []string, title string) bool {
	for _, t := range titles {
		if t == title {
			return true
		}
	}
	return false
}

func TestReach_NextReadyAndTheBatchGiveOnlyReachedIssues(t *testing.T) {
	b := newReachBoard(t)
	eachPerson(t, func(t *testing.T, person string, want []string) {
		ready, err := b.mgr.NextReady(context.Background(), mwanachamataskmanager.NextReadyFilter{Limit: 5, RequesterID: person})
		if err != nil {
			t.Fatal(err)
		}
		titles := []string{}
		for _, item := range ready.Items {
			titles = append(titles, item.Title)
		}
		wantReached(t, "NextReady", titles, want)
		batch, err := b.mgr.NextReadyBatch(context.Background(), mwanachamataskmanager.NextReadyBatchFilter{Count: 10, IncludeUnfiled: true, RequesterID: person})
		if err != nil {
			t.Fatal(err)
		}
		titles = nil
		for _, item := range batch.Items {
			titles = append(titles, item.Title)
		}
		wantReached(t, "NextReadyBatch", titles, want)
	})
}

func TestReach_ANamedBacklogTheGroupDoesNotReachIsUnknown(t *testing.T) {
	b := newReachBoard(t)
	ctx := context.Background()
	if _, err := b.mgr.NextReady(ctx, mwanachamataskmanager.NextReadyFilter{Backlog: "apac-board", RequesterID: "emea"}); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("NextReady: %v", err)
	}
	if _, err := b.mgr.NextReadyBatch(ctx, mwanachamataskmanager.NextReadyBatchFilter{Backlog: "apac-board", RequesterID: "emea"}); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("NextReadyBatch: %v", err)
	}
	if _, err := b.mgr.PendingTodos(ctx, mwanachamataskmanager.PendingTodosFilter{Backlog: "apac-board", RequesterID: "emea"}); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("PendingTodos: %v", err)
	}
	if _, err := b.mgr.BoardSnapshot(ctx, mwanachamataskmanager.BoardSnapshotFilter{Backlog: "apac-board", RequesterID: "emea"}); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("BoardSnapshot: %v", err)
	}
}

func TestReach_ClaimNextGivesNothingToAGroupThatReachesNoReadyIssue(t *testing.T) {
	b := newReachBoard(t)
	ctx := context.Background()
	if _, err := b.mgr.ClaimNext(ctx, "none", mwanachamataskmanager.ClaimNextRequest{Agent: "bot"}); err == nil {
		t.Fatal("an agent with no group claimed an issue")
	}
	claimed, err := b.mgr.ClaimNext(ctx, "apac", mwanachamataskmanager.ClaimNextRequest{Agent: "bot"})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Issue.ID != b.idOf(apacTask) {
		t.Fatalf("claimed %s, want the apac task", claimed.Issue.ID)
	}
}

func TestReach_NotesAndTodosFollowTheirIssue(t *testing.T) {
	b := newReachBoard(t)
	ctx := context.Background()
	id := b.idOf(emeaTask)
	if _, err := b.mgr.AddTodos(ctx, "emea", id, mwanachamataskmanager.AddTodosRequest{Todos: []mwanachamataskmanager.TodoSpec{{Title: "one", Instructions: "do it"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.mgr.AddTaskNote(ctx, "emea", id, "person", "hello", nil); err != nil {
		t.Fatal(err)
	}
	for _, person := range []string{"emea", "admin", "both"} {
		todos, err := b.mgr.ListTodosOfIssue(ctx, person, id)
		if err != nil || todos.Total != 1 {
			t.Fatalf("%s todos: %+v %v", person, todos, err)
		}
		if notes, err := b.mgr.ListTaskNotes(ctx, person, id); err != nil || len(notes) == 0 {
			t.Fatalf("%s notes: %v %v", person, notes, err)
		}
	}
	for _, person := range []string{"apac", "none"} {
		if _, err := b.mgr.ListTodosOfIssue(ctx, person, id); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
			t.Fatalf("%s todos: %v", person, err)
		}
		if _, err := b.mgr.ListTaskNotes(ctx, person, id); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
			t.Fatalf("%s notes: %v", person, err)
		}
		if _, err := b.mgr.AddTaskNote(ctx, person, id, "person", "x", nil); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
			t.Fatalf("%s add note: %v", person, err)
		}
		counts, err := b.mgr.CountTodosOfIssues(ctx, mwanachamataskmanager.TodoCountsFilter{TaskIDs: models.TagList{id}, RequesterID: person})
		if err != nil || len(counts.Items) != 0 {
			t.Fatalf("%s counts: %+v %v", person, counts, err)
		}
	}
}

func TestReach_ABacklogListCountsOnlyReachedBacklogs(t *testing.T) {
	b := newReachBoard(t)
	eachPerson(t, func(t *testing.T, person string, want []string) {
		list, err := b.mgr.ListBacklogsFor(context.Background(), person)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, item := range list.Items {
			seen[item.ID] = true
		}
		for id, title := range map[string]string{"emea-board": emeaTask, "apac-board": apacTask, "open-board": openTask} {
			if seen[id] != containsTitle(want, title) {
				t.Fatalf("%s: backlog %s listed %v", person, id, seen[id])
			}
		}
	})
}

func TestReach_AnEmptyCallerIsNotLimitedAndHasNoOwnerPass(t *testing.T) {
	b := newReachBoard(t)
	ctx := context.Background()
	tasks, err := b.mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil || len(tasks) < 3 {
		t.Fatalf("no person: %d tasks, %v", len(tasks), err)
	}
	b.mgr.UseGroupTags(reachPeople())
	b.mgr.UseGroups(func(context.Context, string) ([]string, error) { return []string{"g-owner"}, nil })
	b.mgr.UseGroupExists(knownGroups("g-owner"))
	if err := b.mgr.SetBacklogOwnerGroup(ctx, "g-owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.mgr.GetTaskFor(ctx, "none", b.idOf(emeaTask)); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("an owner-group seat without a shared tag reached the issue: %v", err)
	}
}

func TestReach_MoveAndEditStayPerGroupAndReachComesFirst(t *testing.T) {
	b := newReachBoard(t)
	ctx := context.Background()
	b.mgr.UseGroups(func(_ context.Context, id string) ([]string, error) { return []string{"g-" + id}, nil })
	b.mgr.UseGroupExists(knownGroups("g-owner", "g-emea", "g-apac"))
	if err := b.mgr.SetBacklogOwnerGroup(ctx, "g-owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.mgr.GiveBacklogAccess(ctx, "emea-board", "g-apac", "move"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.mgr.GiveBacklogAccess(ctx, "emea-board", "g-emea", "move"); err != nil {
		t.Fatal(err)
	}
	_, err := b.mgr.PatchTaskAs(ctx, "apac", mwanachamataskmanager.TaskPatch{ID: b.idOf(emeaTask), Title: reachPtr("changed")})
	if err == nil {
		t.Fatal("a group with move on a backlog it does not reach still wrote")
	}
	if _, err := b.mgr.PatchTaskAs(ctx, "emea", mwanachamataskmanager.TaskPatch{ID: b.idOf(emeaTask), Title: reachPtr("changed")}); err != nil {
		t.Fatalf("a reaching group with move could not write: %v", err)
	}
}

func TestReach_TheSeeLevelIsNoLongerGiven(t *testing.T) {
	b := newReachBoard(t)
	ctx := context.Background()
	b.mgr.UseGroupExists(knownGroups("g-owner", "g-emea"))
	if err := b.mgr.SetBacklogOwnerGroup(ctx, "g-owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.mgr.GiveBacklogAccess(ctx, "emea-board", "g-emea", "see"); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("see was given: %v", err)
	}
}

func reachPtr[T any](v T) *T { return &v }

func TestReach_TheCallerlessOperationsFollowTheirIssue(t *testing.T) {
	b := newReachBoard(t)
	ctx := context.Background()
	id := b.idOf(emeaTask)
	added, err := b.mgr.AddTodos(ctx, "emea", id, mwanachamataskmanager.AddTodosRequest{Todos: []mwanachamataskmanager.TodoSpec{{Title: "one", Instructions: "do it"}}})
	if err != nil {
		t.Fatal(err)
	}
	todoID := added.Todos[0].ID
	for _, person := range []string{"emea", "admin", "both", ""} {
		if _, err := b.mgr.GetTaskTodoFor(ctx, person, todoID); err != nil {
			t.Fatalf("%q reads the todo: %v", person, err)
		}
		if _, err := b.mgr.ListProjectsForTaskFor(ctx, person, id); err != nil {
			t.Fatalf("%q lists projects: %v", person, err)
		}
		if _, err := b.mgr.ListDeliverablesForTaskFor(ctx, person, id); err != nil {
			t.Fatalf("%q lists deliverables: %v", person, err)
		}
		if _, err := b.mgr.ListAcceptanceCriteriaForTaskFor(ctx, person, id); err != nil {
			t.Fatalf("%q lists criteria: %v", person, err)
		}
	}
	for _, person := range []string{"apac", "none"} {
		if _, err := b.mgr.GetTaskTodoFor(ctx, person, todoID); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoNotFound) {
			t.Fatalf("%s reads the todo: %v", person, err)
		}
		if _, err := b.mgr.ListProjectsForTaskFor(ctx, person, id); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
			t.Fatalf("%s lists projects: %v", person, err)
		}
		if _, err := b.mgr.ListDeliverablesForTaskFor(ctx, person, id); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
			t.Fatalf("%s lists deliverables: %v", person, err)
		}
		if _, err := b.mgr.ListAcceptanceCriteriaForTaskFor(ctx, person, id); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
			t.Fatalf("%s lists criteria: %v", person, err)
		}
		if err := b.mgr.DispatchTaskTodoAs(ctx, person, todoID); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoNotFound) {
			t.Fatalf("%s dispatches: %v", person, err)
		}
		_, err := b.mgr.CreateTaskTodoAs(ctx, person, mwanachamataskmanager.TaskTodo{Title: "x", Instructions: "y", ParentTaskID: id})
		if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
			t.Fatalf("%s creates a todo: %v", person, err)
		}
	}
}

func TestReach_ARunListsAndClosesOverOnlyReachedIssuesAndTodos(t *testing.T) {
	b := newReachBoard(t)
	ctx := context.Background()
	run, err := b.mgr.CreateWorkflowRun(ctx, "release", "manual", "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{emeaTask, apacTask} {
		if err := b.mgr.LinkTaskToRunAs(ctx, "", run.ID, b.idOf(title)); err != nil {
			t.Fatal(err)
		}
		added, err := b.mgr.AddTodos(ctx, "", b.idOf(title), mwanachamataskmanager.AddTodosRequest{Todos: []mwanachamataskmanager.TodoSpec{{Title: title, Instructions: "do it"}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := b.mgr.LinkTodoToRunAs(ctx, "", run.ID, added.Todos[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]int{"emea": 1, "apac": 1, "both": 2, "admin": 2, "none": 0, "": 2}
	for person, n := range want {
		closure, err := b.mgr.GetWorkflowRunClosureFor(ctx, person, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(closure.Tasks) != n || len(closure.Todos) != n {
			t.Fatalf("%q closure holds %d tasks and %d todos, want %d each", person, len(closure.Tasks), len(closure.Todos), n)
		}
	}
	todos, err := b.mgr.ListTaskTodosFor(ctx, "emea", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, todo := range todos {
		if todo.ParentTaskID == b.idOf(apacTask) {
			t.Fatal("emea listed a todo of the apac issue")
		}
	}
}
