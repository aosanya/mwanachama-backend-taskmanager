package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func capsOf(input, output, runtime int64) mwanachamataskmanager.WorkerCaps {
	return mwanachamataskmanager.WorkerCaps{InputTokens: input, OutputTokens: output, RuntimeSeconds: runtime}
}

func numbers(input, output, runtime int64) mwanachamataskmanager.TaskCapsChange {
	return mwanachamataskmanager.TaskCapsChange{InputTokens: &input, OutputTokens: &output, RuntimeSeconds: &runtime}
}

func TestTaskCaps_AnIssueNoWorkerHasRunUnderTheDefaults(t *testing.T) {
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	got, err := mgr.GetTaskCaps(context.Background(), "", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := capsOf(500000, 100000, 900)
	if got.Default != want || got.InForce != want || got.Own != nil || got.Used != nil {
		t.Fatalf("got %+v", got)
	}
}

func TestTaskCaps_TheDefaultIsAHubSettingThatAnOwnerChanges(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	runtime := int64(1200)
	changed, err := mgr.SetWorkerCapsDefault(ctx, "", mwanachamataskmanager.TaskCapsChange{RuntimeSeconds: &runtime})
	if err != nil || changed != capsOf(500000, 100000, 1200) {
		t.Fatalf("got %+v, err %v", changed, err)
	}
	read, err := mgr.GetWorkerCapsDefault(ctx)
	if err != nil || read != changed {
		t.Fatalf("got %+v, err %v", read, err)
	}
	got, err := mgr.GetTaskCaps(ctx, "", task.ID)
	if err != nil || got.InForce != changed {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

func TestTaskCaps_ANumberBelowOneIsRefusedForTheDefaultAndTheIssue(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	if _, err := mgr.SetWorkerCapsDefault(ctx, "", numbers(0, 100, 100)); !errors.Is(err, mwanachamataskmanager.ErrInvalidCaps) {
		t.Fatalf("default zero: %v", err)
	}
	if _, err := mgr.SetTaskCaps(ctx, "", task.ID, numbers(10, -1, 10)); !errors.Is(err, mwanachamataskmanager.ErrInvalidCaps) {
		t.Fatalf("issue negative: %v", err)
	}
	if _, err := mgr.SetTaskCaps(ctx, "", task.ID, mwanachamataskmanager.TaskCapsChange{}); !errors.Is(err, mwanachamataskmanager.ErrInvalidCaps) {
		t.Fatalf("issue empty: %v", err)
	}
}

func TestTaskCaps_AnIssueOwnNumbersReplaceTheDefaultsAndAMissingOneFallsBack(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	other := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "b"})

	got, err := mgr.SetTaskCaps(ctx, "", task.ID, numbers(2000, 500, 60))
	if err != nil {
		t.Fatal(err)
	}
	if got.InForce != capsOf(2000, 500, 60) || got.Default != capsOf(500000, 100000, 900) || got.Own == nil || *got.Own != capsOf(2000, 500, 60) {
		t.Fatalf("got %+v", got)
	}

	only := int64(30)
	got, err = mgr.SetTaskCaps(ctx, "", other.ID, mwanachamataskmanager.TaskCapsChange{RuntimeSeconds: &only})
	if err != nil || got.InForce != capsOf(500000, 100000, 30) {
		t.Fatalf("got %+v, err %v", got, err)
	}
	again, err := mgr.GetTaskCaps(ctx, "", task.ID)
	if err != nil || again.InForce != capsOf(2000, 500, 60) {
		t.Fatalf("the first issue changed: %+v, err %v", again, err)
	}
}

func TestTaskCaps_UseTheDefaultRemovesTheIssueOwnNumbers(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if _, err := mgr.SetTaskCaps(ctx, "", task.ID, numbers(2000, 500, 60)); err != nil {
		t.Fatal(err)
	}

	got, err := mgr.ClearTaskCaps(ctx, "", task.ID)
	if err != nil || got.Own != nil || got.InForce != capsOf(500000, 100000, 900) {
		t.Fatalf("got %+v, err %v", got, err)
	}
	if _, err := mgr.ClearTaskCaps(ctx, "", task.ID); err != nil {
		t.Fatalf("clearing twice: %v", err)
	}
}

func TestTaskCaps_TheConsumptionIsWrittenWhileTheWorkerRunsAndReadWithTheCaps(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	first, err := mgr.WriteTaskUsage(ctx, "worker-1", task.ID, mwanachamataskmanager.TaskUsageWrite{InputTokens: 120000, OutputTokens: 8000, SecondsUsed: 95})
	if err != nil {
		t.Fatal(err)
	}
	if first.Used == nil || first.Used.InputTokens != 120000 || first.Used.OutputTokens != 8000 || first.Used.SecondsUsed != 95 || first.Used.ClockStopped || first.Used.UpdatedAt == "" {
		t.Fatalf("got %+v", first.Used)
	}

	second, err := mgr.WriteTaskUsage(ctx, "worker-1", task.ID, mwanachamataskmanager.TaskUsageWrite{InputTokens: 180000, OutputTokens: 9000, SecondsUsed: 95, ClockStopped: true})
	if err != nil {
		t.Fatal(err)
	}
	read, err := mgr.GetTaskCaps(ctx, "worker-1", task.ID)
	if err != nil || read.Used == nil || *read.Used != *second.Used {
		t.Fatalf("read %+v, wrote %+v, err %v", read.Used, second.Used, err)
	}
	if read.Used.InputTokens != 180000 || !read.Used.ClockStopped {
		t.Fatalf("the second write did not replace the first: %+v", read.Used)
	}
	if read.InForce != capsOf(500000, 100000, 900) {
		t.Fatalf("the caps moved with the consumption: %+v", read.InForce)
	}
}

func TestTaskCaps_ANewRunStartsFromNothingByWritingZeros(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if _, err := mgr.WriteTaskUsage(ctx, "", task.ID, mwanachamataskmanager.TaskUsageWrite{InputTokens: 9, OutputTokens: 9, SecondsUsed: 9}); err != nil {
		t.Fatal(err)
	}

	got, err := mgr.WriteTaskUsage(ctx, "", task.ID, mwanachamataskmanager.TaskUsageWrite{})
	if err != nil || got.Used == nil || got.Used.InputTokens != 0 || got.Used.SecondsUsed != 0 {
		t.Fatalf("got %+v, err %v", got.Used, err)
	}
}

func TestTaskCaps_NegativeConsumptionAndAnUnknownIssueAreRefused(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	if _, err := mgr.WriteTaskUsage(ctx, "", task.ID, mwanachamataskmanager.TaskUsageWrite{InputTokens: -1}); !errors.Is(err, mwanachamataskmanager.ErrInvalidCaps) {
		t.Fatalf("negative: %v", err)
	}
	if _, err := mgr.WriteTaskUsage(ctx, "", "no-such-issue", mwanachamataskmanager.TaskUsageWrite{}); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("unknown issue write: %v", err)
	}
	if _, err := mgr.GetTaskCaps(ctx, "", "no-such-issue"); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("unknown issue read: %v", err)
	}
	if _, err := mgr.SetTaskCaps(ctx, "", "no-such-issue", numbers(1, 1, 1)); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("unknown issue set: %v", err)
	}
}
