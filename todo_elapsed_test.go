package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

var elapsedSteps = []string{"To do", "In progress", "Review", "Done"}

func newElapsedFixture(t *testing.T) reviewFixture {
	t.Helper()
	return newReviewFixture(t, elapsedSteps, "to-do", 2)
}

func (f reviewFixture) viewOf(t *testing.T, piece int) mwanachamataskmanager.TodoView {
	t.Helper()
	list, err := f.mgr.ListTodosOfIssue(context.Background(), "", f.issue.ID)
	if err != nil {
		t.Fatalf("ListTodosOfIssue: %v", err)
	}
	for _, view := range list.Items {
		if view.ID == f.pieces[piece].ID {
			return view
		}
	}
	t.Fatalf("piece %d is not in the list", piece)
	return mwanachamataskmanager.TodoView{}
}

func (f reviewFixture) stamp(t *testing.T, piece int, startedAt, doneAt any) {
	t.Helper()
	err := f.db.Table(f.table).Where("id = ?", f.pieces[piece].ID).
		Updates(map[string]any{"started_at": startedAt, "done_at": doneAt}).Error
	if err != nil {
		t.Fatalf("stamp: %v", err)
	}
}

func hoursAgo(hours float64) string {
	return time.Now().UTC().Add(-time.Duration(hours * float64(time.Hour))).Format(time.RFC3339)
}

func TestATodoNotStartedHasNoTimes(t *testing.T) {
	f := newElapsedFixture(t)
	view := f.viewOf(t, 0)
	if view.StartedAt != "" || view.DoneAt != "" || view.ElapsedSeconds != nil {
		t.Fatalf("a todo not started has started_at %q, done_at %q, elapsed %v", view.StartedAt, view.DoneAt, view.ElapsedSeconds)
	}
}

func TestSettingInProgressStampsStartedAtOnce(t *testing.T) {
	f := newElapsedFixture(t)
	f.setState(t, 0, "In progress")
	view := f.viewOf(t, 0)
	if view.StartedAt == "" {
		t.Fatal("started_at is empty after In progress")
	}
	if view.DoneAt != "" {
		t.Fatalf("done_at is %q while running", view.DoneAt)
	}
	if view.ElapsedSeconds == nil || *view.ElapsedSeconds < 0 || *view.ElapsedSeconds > 5 {
		t.Fatalf("a todo just started has elapsed %v, want 0 to 5", view.ElapsedSeconds)
	}
	earlier := hoursAgo(2)
	f.stamp(t, 0, earlier, "")
	f.setState(t, 0, "In progress")
	if again := f.viewOf(t, 0); again.StartedAt != earlier {
		t.Fatalf("setting In progress twice moved started_at from %q to %q", earlier, again.StartedAt)
	}
}

func TestARunningTodoCountsUpFromItsStart(t *testing.T) {
	f := newElapsedFixture(t)
	f.setState(t, 0, "In progress")
	f.stamp(t, 0, hoursAgo(2), "")
	view := f.viewOf(t, 0)
	if view.ElapsedSeconds == nil || *view.ElapsedSeconds < 7200 || *view.ElapsedSeconds > 7210 {
		t.Fatalf("elapsed of a todo started two hours ago is %v, want 7200 to 7210", view.ElapsedSeconds)
	}
	time.Sleep(1100 * time.Millisecond)
	later := f.viewOf(t, 0)
	if *later.ElapsedSeconds <= *view.ElapsedSeconds {
		t.Fatalf("elapsed did not grow: %d then %d", *view.ElapsedSeconds, *later.ElapsedSeconds)
	}
}

func TestADoneTodoStopsAtItsDoneTime(t *testing.T) {
	f := newElapsedFixture(t)
	f.setState(t, 0, "In progress")
	f.stamp(t, 0, hoursAgo(3), "")
	f.setState(t, 0, "Done")
	done := f.viewOf(t, 0)
	if done.DoneAt == "" || done.StartedAt == "" {
		t.Fatalf("a Done todo has started_at %q and done_at %q", done.StartedAt, done.DoneAt)
	}
	if done.ElapsedSeconds == nil || *done.ElapsedSeconds < 10800 || *done.ElapsedSeconds > 10810 {
		t.Fatalf("elapsed of a todo done after three hours is %v, want 10800 to 10810", done.ElapsedSeconds)
	}
	started := time.Now().UTC().Add(-3 * time.Hour)
	f.stamp(t, 0, started.Format(time.RFC3339), started.Add(90*time.Minute).Format(time.RFC3339))
	time.Sleep(1100 * time.Millisecond)
	frozen := f.viewOf(t, 0)
	if frozen.ElapsedSeconds == nil || *frozen.ElapsedSeconds != 5400 {
		t.Fatalf("elapsed of a todo done 90 minutes after its start is %v, want 5400", frozen.ElapsedSeconds)
	}
}

func TestReopeningToDoClearsBothTimes(t *testing.T) {
	f := newElapsedFixture(t)
	f.setState(t, 0, "In progress")
	f.setState(t, 0, "Done")
	f.setState(t, 0, "To do")
	view := f.viewOf(t, 0)
	if view.StartedAt != "" || view.DoneAt != "" || view.ElapsedSeconds != nil {
		t.Fatalf("a reopened todo keeps started_at %q, done_at %q, elapsed %v", view.StartedAt, view.DoneAt, view.ElapsedSeconds)
	}
}

func TestReopeningADoneTodoToInProgressKeepsItsStartAndDropsItsEnd(t *testing.T) {
	f := newElapsedFixture(t)
	f.setState(t, 0, "In progress")
	f.stamp(t, 0, hoursAgo(4), "")
	f.setState(t, 0, "Done")
	f.setState(t, 0, "In progress")
	view := f.viewOf(t, 0)
	if view.DoneAt != "" {
		t.Fatalf("done_at is %q after reopening to In progress", view.DoneAt)
	}
	if view.ElapsedSeconds == nil || *view.ElapsedSeconds < 14400 || *view.ElapsedSeconds > 14410 {
		t.Fatalf("elapsed after reopening is %v, want the time since the first start, 14400 to 14410", view.ElapsedSeconds)
	}
}

func TestDoneStraightFromToDoWasNeverStarted(t *testing.T) {
	f := newElapsedFixture(t)
	f.setState(t, 0, "Done")
	view := f.viewOf(t, 0)
	if view.StartedAt != "" || view.ElapsedSeconds != nil {
		t.Fatalf("a todo never started has started_at %q and elapsed %v", view.StartedAt, view.ElapsedSeconds)
	}
	if view.DoneAt == "" {
		t.Fatal("done_at is empty on a Done todo")
	}
}

func TestBlockedByHandKeepsStartAndDropsEnd(t *testing.T) {
	f := newElapsedFixture(t)
	f.setState(t, 0, "In progress")
	f.stamp(t, 0, hoursAgo(1), "")
	if _, err := f.mgr.SetTodoStateFor(context.Background(), "", f.pieces[0].ID, "Blocked", "waiting for the owner"); err != nil {
		t.Fatalf("SetTodoStateFor Blocked: %v", err)
	}
	view := f.viewOf(t, 0)
	if view.StartedAt == "" || view.DoneAt != "" {
		t.Fatalf("a todo blocked by hand has started_at %q and done_at %q", view.StartedAt, view.DoneAt)
	}
}

func TestRowsWrittenBeforeTheTimesExistReadWithoutThem(t *testing.T) {
	f := newElapsedFixture(t)
	f.stamp(t, 0, nil, nil)
	f.stamp(t, 1, nil, nil)
	list, err := f.mgr.ListTodosOfIssue(context.Background(), "", f.issue.ID)
	if err != nil {
		t.Fatalf("ListTodosOfIssue: %v", err)
	}
	for _, view := range list.Items {
		if view.StartedAt != "" || view.DoneAt != "" || view.ElapsedSeconds != nil {
			t.Fatalf("%s reads started_at %q, done_at %q, elapsed %v", view.Code, view.StartedAt, view.DoneAt, view.ElapsedSeconds)
		}
	}
}

func TestTheJSONOmitsTheTimesUntilStartedAndCarriesThemAfter(t *testing.T) {
	f := newElapsedFixture(t)
	raw, err := json.Marshal(f.viewOf(t, 0))
	if err != nil {
		t.Fatal(err)
	}
	var plain map[string]any
	if err := json.Unmarshal(raw, &plain); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"started_at", "done_at", "elapsed_seconds"} {
		if _, present := plain[key]; present {
			t.Fatalf("%s is in the JSON of a todo not started", key)
		}
	}
	f.setState(t, 0, "In progress")
	raw, err = json.Marshal(f.viewOf(t, 0))
	if err != nil {
		t.Fatal(err)
	}
	var started map[string]any
	if err := json.Unmarshal(raw, &started); err != nil {
		t.Fatal(err)
	}
	if _, present := started["started_at"]; !present {
		t.Fatal("started_at is missing from the JSON of a running todo")
	}
	if _, isNumber := started["elapsed_seconds"].(float64); !isNumber {
		t.Fatalf("elapsed_seconds is %v, want a number", started["elapsed_seconds"])
	}
	if _, present := started["done_at"]; present {
		t.Fatal("done_at is in the JSON of a running todo")
	}
}

func TestTheReadyListCarriesTheElapsedSecondsOfARunningTodo(t *testing.T) {
	f := newElapsedFixture(t)
	f.setState(t, 0, "In progress")
	f.stamp(t, 0, hoursAgo(1), "")
	ready, err := f.mgr.ReadyTodosFor(context.Background(), "", f.issue.ID)
	if err != nil {
		t.Fatalf("ReadyTodosFor: %v", err)
	}
	for _, item := range ready.Items {
		switch item.ID {
		case f.pieces[0].ID:
			if item.ElapsedSeconds == nil || *item.ElapsedSeconds < 3600 || *item.ElapsedSeconds > 3610 {
				t.Fatalf("ready running todo has elapsed %v, want 3600 to 3610", item.ElapsedSeconds)
			}
		case f.pieces[1].ID:
			if item.ElapsedSeconds != nil {
				t.Fatalf("ready todo not started has elapsed %v", item.ElapsedSeconds)
			}
		}
	}
}
