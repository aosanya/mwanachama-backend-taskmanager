package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

const realLengthInstanceID = "agy0d6e4079"

type boardCopy struct {
	live mwanachamataskmanager.TaskManager
	db   *gorm.DB
	spec *spec.Spec
	ctx  context.Context
}

func newBoardCopy(t *testing.T) boardCopy {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := mwanachamataskmanager.SpecForMount(realLengthInstanceID, "board")
	if err != nil {
		t.Fatal(err)
	}
	if err := mwanachamataskmanager.Provision(db, s); err != nil {
		t.Fatal(err)
	}
	live, err := mwanachamataskmanager.NewTaskManager(db, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	return boardCopy{live: live, db: db, spec: s, ctx: context.Background()}
}

func (c boardCopy) run(t *testing.T, id string) mwanachamataskmanager.TaskManager {
	t.Helper()
	m, err := c.live.ForRun(id)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (c boardCopy) liveRows(t *testing.T) int64 {
	t.Helper()
	var total int64
	for _, o := range c.spec.Objects {
		var n int64
		if err := c.db.Table(c.spec.TableFor(o)).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		total += n
	}
	return total
}

func (c boardCopy) testRows(t *testing.T) int64 {
	t.Helper()
	var total int64
	for _, o := range c.spec.Objects {
		var n int64
		if err := c.db.Table(c.spec.TestTableFor(o)).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		total += n
	}
	return total
}

func playScenario(t *testing.T, c boardCopy, m mwanachamataskmanager.TaskManager, title string) mwanachamataskmanager.Task {
	t.Helper()
	project, err := m.CreateProject(c.ctx, mwanachamataskmanager.Project{Name: title + "-project"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := m.CreateTask(c.ctx, mwanachamataskmanager.Task{Title: title, Tags: []string{"urgent"}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := m.CreateTask(c.ctx, mwanachamataskmanager.Task{Title: title + "-after"})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []mwanachamataskmanager.Relationship{
		{Label: mwanachamataskmanager.RelLabelMemberOf, FromID: task.ID, ToID: project.ID},
		{Label: mwanachamataskmanager.RelLabelBlocks, FromID: task.ID, ToID: other.ID},
	} {
		if _, err := m.CreateRelationship(c.ctx, rel); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.CreateWorkflowRun(c.ctx, title+"-run", "test.started", "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateTaskTodo(c.ctx, mwanachamataskmanager.TaskTodo{ParentTaskID: task.ID, Title: title + "-todo", Instructions: "do it"}); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestARunThroughTheBoardLeavesNoRowInAnyLiveTable(t *testing.T) {
	c := newBoardCopy(t)
	playScenario(t, c, c.run(t, "run-1"), "ship")

	if got := c.liveRows(t); got != 0 {
		t.Fatalf("live rows = %d after a run, want 0", got)
	}
	rows, err := specstore.RowsOfRun(c.ctx, c.db, c.spec, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	stored := map[string]int64{}
	for _, r := range rows {
		stored[r.Object] = r.Rows
	}
	for _, object := range []string{"work_item", "objective", "label", "step", "workflow_run", "objective_membership", "gate", "labelling", "code_sequence"} {
		if stored[object] == 0 {
			t.Fatalf("run-1 stored nothing in %s: %+v", object, rows)
		}
	}
}

func TestALiveWriteLeavesTheTestCopyEmpty(t *testing.T) {
	c := newBoardCopy(t)
	playScenario(t, c, c.live, "ship")

	if got := c.testRows(t); got != 0 {
		t.Fatalf("test copy holds %d rows after a live write, want 0", got)
	}
	if c.liveRows(t) == 0 {
		t.Fatal("the live write stored nothing")
	}
}

func TestTwoRunsAndTheLiveBoardKeepTheirTasksApart(t *testing.T) {
	c := newBoardCopy(t)
	first, second := c.run(t, "run-1"), c.run(t, "run-2")
	a := playScenario(t, c, first, "alpha")
	playScenario(t, c, second, "beta")
	playScenario(t, c, second, "gamma")
	playScenario(t, c, c.live, "live")

	count := func(m mwanachamataskmanager.TaskManager) int {
		tasks, err := m.ListTasks(c.ctx, mwanachamataskmanager.TaskFilter{})
		if err != nil {
			t.Fatal(err)
		}
		return len(tasks)
	}
	if got := count(first); got != 2 {
		t.Fatalf("run-1 sees %d tasks, want 2", got)
	}
	if got := count(second); got != 4 {
		t.Fatalf("run-2 sees %d tasks, want 4", got)
	}
	if got := count(c.live); got != 2 {
		t.Fatalf("live sees %d tasks, want 2", got)
	}
	if _, err := second.GetTask(c.ctx, a.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("run-2 reading run-1's task = %v, want ErrTaskNotFound", err)
	}
	if _, err := c.live.GetTask(c.ctx, a.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("live reading run-1's task = %v, want ErrTaskNotFound", err)
	}
}

func TestRelationshipsInARunAreInvisibleToTheOtherRun(t *testing.T) {
	c := newBoardCopy(t)
	first, second := c.run(t, "run-1"), c.run(t, "run-2")
	a := playScenario(t, c, first, "alpha")

	mine, err := first.TraverseRelationships(c.ctx, a.ID, mwanachamataskmanager.RelLabelBlocks, mwanachamataskmanager.DirectionOutbound)
	if err != nil || len(mine) != 1 {
		t.Fatalf("run-1 blocks = %+v, %v; want 1", mine, err)
	}
	theirs, err := second.TraverseRelationships(c.ctx, a.ID, mwanachamataskmanager.RelLabelBlocks, mwanachamataskmanager.DirectionOutbound)
	if err != nil || len(theirs) != 0 {
		t.Fatalf("run-2 blocks = %+v, %v; want none", theirs, err)
	}
}

func TestCancellingInOneRunLeavesTheOtherRunsTaskAlone(t *testing.T) {
	c := newBoardCopy(t)
	first, second := c.run(t, "run-1"), c.run(t, "run-2")
	a := playScenario(t, c, first, "alpha")
	b := playScenario(t, c, second, "beta")

	if err := first.DeleteTask(c.ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := second.GetTask(c.ctx, b.ID); err != nil {
		t.Fatalf("run-2's task after run-1 deleted its own = %v", err)
	}
}

func TestRemovingARunRemovesItsRowsAndKeepsTheOthers(t *testing.T) {
	c := newBoardCopy(t)
	first, second := c.run(t, "run-1"), c.run(t, "run-2")
	playScenario(t, c, first, "alpha")
	b := playScenario(t, c, second, "beta")

	removed, err := specstore.RemoveRun(c.ctx, c.db, c.spec, "run-1")
	if err != nil || removed == 0 {
		t.Fatalf("removed %d rows, %v", removed, err)
	}
	if rows, _ := specstore.RowsOfRun(c.ctx, c.db, c.spec, "run-1"); len(rows) != 0 {
		t.Fatalf("run-1 still holds %+v", rows)
	}
	if _, err := second.GetTask(c.ctx, b.ID); err != nil {
		t.Fatalf("run-2 lost its task: %v", err)
	}
}

func TestBatchesOpenedInARunStayInTheRun(t *testing.T) {
	c := newBoardCopy(t)
	run := c.run(t, "run-1")

	if _, err := run.GenerateTasks(c.ctx, generateRequest("a", "b")); err != nil {
		t.Fatal(err)
	}
	if _, err := run.OpenSourcedTasks(c.ctx, sourcedRequest()); err != nil {
		t.Fatal(err)
	}

	if got := c.liveRows(t); got != 0 {
		t.Fatalf("live rows = %d after batches in a run, want 0", got)
	}
	tasks, err := run.ListTasks(c.ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil || len(tasks) != 4 {
		t.Fatalf("run sees %d tasks, %v; want 4", len(tasks), err)
	}
}

func TestATaskInARunReadsItsTagsBack(t *testing.T) {
	c := newBoardCopy(t)
	run := c.run(t, "run-1")
	playScenario(t, c, run, "ship")

	tasks, err := run.ListTasks(c.ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.Title == "ship" && (len(task.Tags) != 1 || task.Tags[0] != "urgent") {
			t.Fatalf("tags of the run's task = %v, want [urgent]", task.Tags)
		}
	}
}

func TestARunWithoutAnIDIsRefused(t *testing.T) {
	c := newBoardCopy(t)
	if _, err := c.live.ForRun(""); !errors.Is(err, specstore.ErrRunIDEmpty) {
		t.Fatalf("ForRun(\"\") = %v, want ErrRunIDEmpty", err)
	}
}

func TestTheBoardSpecLoadsAndItsTestCopyIsCreatedUnderARealLengthInstanceID(t *testing.T) {
	c := newBoardCopy(t)
	for _, o := range c.spec.Objects {
		if !c.db.Migrator().HasTable(c.spec.TestTableFor(o)) {
			t.Errorf("no test table %s for %s", c.spec.TestTableFor(o), o.Name)
		}
		for _, name := range []string{c.spec.TableFor(o), c.spec.TestTableFor(o)} {
			if len(name) > spec.MaxIdentifier {
				t.Errorf("table %q is %d bytes, over %d", name, len(name), spec.MaxIdentifier)
			}
		}
		for _, idx := range o.Indexes {
			for _, name := range []string{c.spec.IndexFor(o, idx), c.spec.TestIndexFor(o, idx)} {
				if len(name) > spec.MaxIdentifier {
					t.Errorf("index %q is %d bytes, over %d", name, len(name), spec.MaxIdentifier)
				}
			}
		}
	}
}
