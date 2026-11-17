package mwanachamataskmanager_test

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func workerHolding(t *testing.T, mgr mwanachamataskmanager.TaskManager, tags ...string) mwanachamataskmanager.Task {
	t.Helper()
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "held", Tags: tags})
	if _, err := mgr.MoveTaskToStep(context.Background(), task.ID, "backlog", "doing"); err != nil {
		t.Fatal(err)
	}
	return task
}

func ownCaps(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string, input, output, runtime int64) {
	t.Helper()
	if _, err := mgr.SetTaskCaps(context.Background(), "", id, numbers(input, output, runtime)); err != nil {
		t.Fatal(err)
	}
}

func usage(input, output, seconds int64) mwanachamataskmanager.TaskUsageWrite {
	return mwanachamataskmanager.TaskUsageWrite{InputTokens: input, OutputTokens: output, SecondsUsed: seconds}
}

func logOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) []mwanachamataskmanager.TaskNote {
	t.Helper()
	notes, err := mgr.ListTaskNotes(context.Background(), "", id)
	if err != nil {
		t.Fatal(err)
	}
	kept := notes[:0:0]
	for _, note := range notes {
		if !isChangeEntry(note) {
			kept = append(kept, note)
		}
	}
	return kept
}

func isChangeEntry(note mwanachamataskmanager.TaskNote) bool {
	if note.Kind != "automatic" || !note.Automatic {
		return false
	}
	if note.Text == "Deleted" {
		return true
	}
	for _, label := range []string{"Moved ", "Assignee ", "Status ", "Tags "} {
		if strings.HasPrefix(note.Text, label) && strings.Contains(note.Text, " to ") {
			return true
		}
	}
	return false
}

func sortedTags(task mwanachamataskmanager.Task) []string {
	tags := append([]string{}, task.Tags...)
	sort.Strings(tags)
	return tags
}

func TestUsageWrite_OverACapStopsTheIssueIntoSinbinInToDoAndRepliesStop(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := workerHolding(t, mgr, "Ready", "hub-steward-swe")
	ownCaps(t, mgr, task.ID, 1000, 200, 60)

	reply, err := mgr.WriteTaskUsage(ctx, "worker", task.ID, usage(1540, 70, 7))
	if err != nil {
		t.Fatal(err)
	}
	if !reply.Stop || !reflect.DeepEqual(reply.Passed, []string{"input_tokens"}) {
		t.Fatalf("reply stop %v passed %v", reply.Stop, reply.Passed)
	}
	got, err := mgr.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sortedTags(got), []string{"Sinbin", "hub-steward-swe"}) {
		t.Fatalf("tags %v", got.Tags)
	}
	if got.Status != mwanachamataskmanager.TaskStatusPending {
		t.Fatalf("status %s", got.Status)
	}
	wantPlace(t, mgr, task.ID, "to-do")
	notes := logOf(t, mgr, task.ID)
	if len(notes) != 1 || notes[0].Kind != "worker" {
		t.Fatalf("log %+v", notes)
	}
}

func TestUsageWrite_TheEntryNamesTheCapTheThreeFiguresTheLastStepWhatIsLeftAndTheFiles(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := workerHolding(t, mgr, "Ready")
	ownCaps(t, mgr, task.ID, 1000, 200, 60)
	for _, text := range []string{"Taken", "Failing test written"} {
		if _, err := mgr.AddTaskNote(ctx, "worker", task.ID, "worker", text, nil); err != nil {
			t.Fatal(err)
		}
	}
	write := usage(1540, 70, 7)
	files := []string{"app/src/a.ts", "hooks/b.py"}
	write.FilesUncommitted = &files
	if _, err := mgr.WriteTaskUsage(ctx, "worker", task.ID, write); err != nil {
		t.Fatal(err)
	}
	notes := logOf(t, mgr, task.ID)
	entry := notes[len(notes)-1].Text
	for _, part := range []string{
		"Stopped at a cap", "Cap passed: Input tokens", "Input tokens 1,540 of 1,000", "Output tokens 70 of 200", "Runtime (working time) 7 of 60",
		"Last step finished: Failing test written", "Left:", "app/src/a.ts, hooks/b.py", "neither commits nor discards", "Sinbin", "To do",
	} {
		if !strings.Contains(entry, part) {
			t.Fatalf("entry lacks %q: %s", part, entry)
		}
	}
	if len(notes) != 3 {
		t.Fatalf("want the two worker entries and one stop entry, got %d", len(notes))
	}
}

func TestUsageWrite_FilesAreNoneWhenReportedEmptyAndNotReportedWhenTheRunnerSaidNothing(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	said := workerHolding(t, mgr, "Ready")
	silent := workerHolding(t, mgr, "Ready")
	ownCaps(t, mgr, said.ID, 1000, 200, 60)
	ownCaps(t, mgr, silent.ID, 1000, 200, 60)
	reported := usage(1540, 70, 7)
	none := []string{}
	reported.FilesUncommitted = &none
	if _, err := mgr.WriteTaskUsage(ctx, "", said.ID, reported); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.WriteTaskUsage(ctx, "", silent.ID, usage(1540, 70, 7)); err != nil {
		t.Fatal(err)
	}
	if text := logOf(t, mgr, said.ID)[0].Text; !strings.Contains(text, "Files changed, not committed: none") {
		t.Fatal(text)
	}
	if text := logOf(t, mgr, silent.ID)[0].Text; !strings.Contains(text, "Files changed, not committed: not reported by the runner") {
		t.Fatal(text)
	}
}

func TestUsageWrite_EveryCapStopsAtTheCapAndNotJustUnderIt(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		write  mwanachamataskmanager.TaskUsageWrite
		passed []string
	}{
		{"input just under", usage(999, 10, 10), nil},
		{"input at the cap", usage(1000, 10, 10), []string{"input_tokens"}},
		{"output just under", usage(10, 199, 10), nil},
		{"output at the cap", usage(10, 200, 10), []string{"output_tokens"}},
		{"runtime just under", usage(10, 10, 59), nil},
		{"runtime at the cap", usage(10, 10, 60), []string{"runtime_seconds"}},
		{"two at once", usage(2000, 300, 10), []string{"input_tokens", "output_tokens"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mgr := newTestManager(t)
			task := workerHolding(t, mgr, "Ready")
			ownCaps(t, mgr, task.ID, 1000, 200, 60)
			reply, err := mgr.WriteTaskUsage(ctx, "", task.ID, c.write)
			if err != nil {
				t.Fatal(err)
			}
			if reply.Stop != (c.passed != nil) || !reflect.DeepEqual(reply.Passed, c.passed) {
				t.Fatalf("stop %v passed %v, want %v", reply.Stop, reply.Passed, c.passed)
			}
			if c.passed == nil {
				wantPlace(t, mgr, task.ID, "doing")
				if len(logOf(t, mgr, task.ID)) != 0 {
					t.Fatal("an entry was written under every cap")
				}
			}
		})
	}
}

func TestUsageWrite_TheDefaultCapsApplyWhenTheIssueHasNone(t *testing.T) {
	mgr := newTestManager(t)
	task := workerHolding(t, mgr, "Ready")
	reply, err := mgr.WriteTaskUsage(context.Background(), "", task.ID, usage(500000, 10, 10))
	if err != nil || !reply.Stop {
		t.Fatalf("reply %+v, err %v", reply, err)
	}
}

func TestUsageWrite_AWorkerWaitingForTheOwnerIsNotStoppedOnRuntimeButIsOnTokens(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := workerHolding(t, mgr, "Ready")
	ownCaps(t, mgr, task.ID, 1000, 200, 60)
	waiting := usage(10, 10, 600)
	waiting.ClockStopped = true
	reply, err := mgr.WriteTaskUsage(ctx, "", task.ID, waiting)
	if err != nil || reply.Stop {
		t.Fatalf("reply %+v, err %v", reply, err)
	}
	wantPlace(t, mgr, task.ID, "doing")
	waiting.InputTokens = 1000
	if reply, _ = mgr.WriteTaskUsage(ctx, "", task.ID, waiting); !reply.Stop || !reflect.DeepEqual(reply.Passed, []string{"input_tokens"}) {
		t.Fatalf("reply %+v", reply)
	}
}

func TestUsageWrite_TheClockStoppedAtTheCapPassesNoRuntimeCapAndThePlayedClockDoes(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := workerHolding(t, mgr, "Ready")
	ownCaps(t, mgr, task.ID, 1000, 200, 60)
	waiting := usage(10, 10, 60)
	waiting.ClockStopped = true
	if reply, err := mgr.WriteTaskUsage(ctx, "", task.ID, waiting); err != nil || reply.Stop || len(reply.Passed) != 0 {
		t.Fatalf("reply %+v, err %v", reply, err)
	}
	wantPlace(t, mgr, task.ID, "doing")
	answered := usage(10, 10, 61)
	reply, err := mgr.WriteTaskUsage(ctx, "", task.ID, answered)
	if err != nil || !reply.Stop || !reflect.DeepEqual(reply.Passed, []string{"runtime_seconds"}) {
		t.Fatalf("reply %+v, err %v", reply, err)
	}
	wantPlace(t, mgr, task.ID, "to-do")
}

func TestUsageWrite_AnUnansweredQuestionNeverEndsAWorkerOnRuntimeHoweverLongTheFigure(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := workerHolding(t, mgr, "Ready")
	ownCaps(t, mgr, task.ID, 1000, 200, 60)
	asking := usage(999, 199, 86400)
	asking.ClockStopped = true
	reply, err := mgr.WriteTaskUsage(ctx, "", task.ID, asking)
	if err != nil || reply.Stop {
		t.Fatalf("reply %+v, err %v", reply, err)
	}
	if n := len(logOf(t, mgr, task.ID)); n != 0 {
		t.Fatalf("%d entries", n)
	}
	wantPlace(t, mgr, task.ID, "doing")
}

func TestUsageWrite_AWriteAfterTheStopRepliesStopAgainWithoutASecondEntry(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := workerHolding(t, mgr, "Ready")
	ownCaps(t, mgr, task.ID, 1000, 200, 60)
	for i := 0; i < 3; i++ {
		reply, err := mgr.WriteTaskUsage(ctx, "", task.ID, usage(1540+int64(i), 70, 7))
		if err != nil || !reply.Stop {
			t.Fatalf("write %d: reply %+v, err %v", i, reply, err)
		}
	}
	if n := len(logOf(t, mgr, task.ID)); n != 1 {
		t.Fatalf("%d entries", n)
	}
	got, _ := mgr.GetTask(ctx, task.ID)
	if !reflect.DeepEqual(sortedTags(got), []string{"Sinbin"}) {
		t.Fatalf("tags %v", got.Tags)
	}
}

func TestUsageWrite_AnIssueNotInProgressIsLeftAloneButTheReplyStillSaysStop(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "idle", Tags: []string{"Ready"}})
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "backlog", "to-do"); err != nil {
		t.Fatal(err)
	}
	reply, err := mgr.WriteTaskUsage(ctx, "", task.ID, usage(900000, 10, 10))
	if err != nil || !reply.Stop {
		t.Fatalf("reply %+v, err %v", reply, err)
	}
	got, _ := mgr.GetTask(ctx, task.ID)
	if !reflect.DeepEqual(got.Tags, []string{"Ready"}) || len(logOf(t, mgr, task.ID)) != 0 {
		t.Fatalf("the issue changed: %+v", got)
	}
}

func TestUsageWrite_SinbinReplacesEveryOtherMarkAndTheOtherTagsStay(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := workerHolding(t, mgr, "Ready", "Waiting", "Blocked", "Docs", "hub-steward-swe")
	ownCaps(t, mgr, task.ID, 1000, 200, 60)
	if _, err := mgr.WriteTaskUsage(ctx, "", task.ID, usage(1540, 70, 7)); err != nil {
		t.Fatal(err)
	}
	got, _ := mgr.GetTask(ctx, task.ID)
	if !reflect.DeepEqual(sortedTags(got), []string{"Docs", "Sinbin", "hub-steward-swe"}) {
		t.Fatalf("tags %v", got.Tags)
	}
}

func TestUsageWrite_AStoppedIssueIsNotPickedUpAsReady(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := workerHolding(t, mgr, "Ready")
	ownCaps(t, mgr, task.ID, 1000, 200, 60)
	if _, err := mgr.WriteTaskUsage(ctx, "", task.ID, usage(1540, 70, 7)); err != nil {
		t.Fatal(err)
	}
	tasks, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Tag: []string{"Ready"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("still Ready: %+v", tasks)
	}
}

func TestUsageWrite_ANewRunByWritingZerosDoesNotStop(t *testing.T) {
	mgr := newTestManager(t)
	task := workerHolding(t, mgr, "Ready")
	reply, err := mgr.WriteTaskUsage(context.Background(), "", task.ID, usage(0, 0, 0))
	if err != nil || reply.Stop || len(reply.Passed) != 0 {
		t.Fatalf("reply %+v, err %v", reply, err)
	}
}
