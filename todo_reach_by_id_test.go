package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type todoBoard struct {
	reachBoard
	todos map[string]string
	runID string
	agent string
}

func newTodoBoard(t *testing.T) todoBoard {
	t.Helper()
	ctx := context.Background()
	b := todoBoard{reachBoard: newReachBoard(t), todos: map[string]string{}}
	run, err := b.mgr.CreateWorkflowRun(ctx, "release", "manual", "admin")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := b.mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "bot"})
	if err != nil {
		t.Fatal(err)
	}
	b.runID, b.agent = run.ID, agent.ID
	for _, title := range []string{emeaTask, apacTask} {
		added, err := b.mgr.AddTodos(ctx, "", b.idOf(title), mwanachamataskmanager.AddTodosRequest{Todos: []mwanachamataskmanager.TodoSpec{{Title: title, Instructions: "do it"}}})
		if err != nil {
			t.Fatal(err)
		}
		todoID := added.Todos[0].ID
		b.todos[title] = todoID
		if err := b.mgr.LinkTodoToRun(ctx, run.ID, todoID); err != nil {
			t.Fatal(err)
		}
		if _, err := b.mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{Label: mwanachamataskmanager.RelLabelTodoAssignedTo, FromID: todoID, ToID: agent.ID}); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

func (b todoBoard) traverse(t *testing.T, person, vertex, label string, dir mwanachamataskmanager.Direction) []mwanachamataskmanager.Relationship {
	t.Helper()
	edges, err := b.mgr.TraverseRelationshipsFor(context.Background(), person, vertex, label, dir)
	if err != nil {
		t.Fatalf("%q traverses %s: %v", person, label, err)
	}
	return edges
}

func TestTodoByID_TraversingFromAnUnreachedTodoFindsNothing(t *testing.T) {
	b := newTodoBoard(t)
	todo := b.todos[emeaTask]
	walks := map[string]mwanachamataskmanager.Direction{
		mwanachamataskmanager.RelLabelTodoAssignedTo: mwanachamataskmanager.DirectionOutbound,
		mwanachamataskmanager.RelLabelStartedTodo:    mwanachamataskmanager.DirectionInbound,
		mwanachamataskmanager.RelLabelHasTodo:        mwanachamataskmanager.DirectionInbound,
	}
	for label, dir := range walks {
		for person, want := range map[string]int{"emea": 1, "admin": 1, "": 1, "apac": 0, "none": 0} {
			if got := b.traverse(t, person, todo, label, dir); len(got) != want {
				t.Errorf("%q over %s from the emea todo: %d edges, want %d", person, label, len(got), want)
			}
		}
	}
}

func TestTodoByID_TraversingTowardsTodosListsOnlyReachedOnes(t *testing.T) {
	b := newTodoBoard(t)
	reached := map[string][]string{
		"emea":  {b.todos[emeaTask]},
		"apac":  {b.todos[apacTask]},
		"admin": {b.todos[emeaTask], b.todos[apacTask]},
		"":      {b.todos[emeaTask], b.todos[apacTask]},
		"none":  {},
	}
	for person, want := range reached {
		fromRun := []string{}
		for _, edge := range b.traverse(t, person, b.runID, mwanachamataskmanager.RelLabelStartedTodo, mwanachamataskmanager.DirectionOutbound) {
			fromRun = append(fromRun, edge.ToID)
		}
		toAgent := []string{}
		for _, edge := range b.traverse(t, person, b.agent, mwanachamataskmanager.RelLabelTodoAssignedTo, mwanachamataskmanager.DirectionInbound) {
			toAgent = append(toAgent, edge.FromID)
		}
		for what, got := range map[string][]string{"todos of the run": fromRun, "todos of the agent": toAgent} {
			if !sameStrings(sorted(got), sorted(want)) {
				t.Errorf("%q %s: %v, want %v", person, what, got, want)
			}
		}
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTodoByID_WaitingOnAnUnreachedTodoReadsAsATodoThatDoesNotExist(t *testing.T) {
	b := newTodoBoard(t)
	ctx := context.Background()
	hidden, own := b.todos[apacTask], b.todos[emeaTask]
	create := func(person, dependency string) error {
		_, err := b.mgr.CreateTaskTodoAs(ctx, person, mwanachamataskmanager.TaskTodo{Title: "later", Instructions: "do it", ParentTaskID: b.idOf(emeaTask), DependsOn: []string{dependency}})
		return err
	}
	edit := func(person, dependency string) error {
		depends := []string{dependency}
		_, err := b.mgr.EditTodoFor(ctx, person, own, mwanachamataskmanager.TodoChange{DependsOn: &depends})
		return err
	}
	for what, call := range map[string]func(string, string) error{"create": create, "edit": edit} {
		for _, dependency := range []string{hidden, "no-such-todo"} {
			if err := call("emea", dependency); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoDependencyNotFound) {
				t.Errorf("%s as emea waiting on %s: %v, want dependency not found", what, dependency, err)
			}
		}
		for _, person := range []string{"admin", ""} {
			if err := call(person, hidden); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoCrossIssue) {
				t.Errorf("%s as %q waiting on a reached todo of another issue: %v, want cross issue", what, person, err)
			}
		}
	}
}

func TestTodoByID_ReleasingWithAnUnreachedTodoReadsAsNotFound(t *testing.T) {
	b := newTodoBoard(t)
	ctx := context.Background()
	if _, err := b.mgr.MoveTaskToStep(ctx, b.idOf(emeaTask), "emea-board", "in-progress"); err != nil {
		t.Fatal(err)
	}
	release := func(person, todoID string) error {
		_, err := b.mgr.ReleaseTaskAs(ctx, person, mwanachamataskmanager.ReleaseTask{TaskID: b.idOf(emeaTask), Mode: "blocked", Reason: "stopped", TodoID: todoID})
		return err
	}
	if err := release("emea", b.todos[apacTask]); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoNotFound) {
		t.Errorf("emea releases naming an apac todo: %v, want todo not found", err)
	}
	if err := release("admin", b.todos[apacTask]); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoCrossIssue) {
		t.Errorf("admin releases naming a todo of another issue: %v, want cross issue", err)
	}
	if err := release("emea", b.todos[emeaTask]); err != nil {
		t.Errorf("emea releases naming its own todo: %v", err)
	}
}
