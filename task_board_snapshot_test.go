package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func snapshotOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, filter mwanachamataskmanager.BoardSnapshotFilter) mwanachamataskmanager.BoardSnapshot {
	t.Helper()
	out, err := mgr.BoardSnapshot(context.Background(), filter)
	if err != nil {
		t.Fatalf("BoardSnapshot %+v: %v", filter, err)
	}
	return out
}

func stepOf(snap mwanachamataskmanager.BoardSnapshot, name string) (mwanachamataskmanager.SnapshotStep, bool) {
	for _, s := range snap.Steps {
		if s.Name == name {
			return s, true
		}
	}
	return mwanachamataskmanager.SnapshotStep{}, false
}

func startIssue(t *testing.T, mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task) mwanachamataskmanager.Task {
	t.Helper()
	made := mustCreate(t, mgr, task)
	made.Status = mwanachamataskmanager.TaskStatusInProgress
	updated, err := mgr.UpdateTask(context.Background(), made)
	if err != nil {
		t.Fatalf("UpdateTask %q: %v", task.Title, err)
	}
	return updated
}

func TestBoardSnapshot_StepCountsEqualWhatPendingReportsForTheSameBoard(t *testing.T) {
	mgr := newTestManager(t)
	seedPending(t, mgr)

	snap := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})
	pending := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{})

	if len(pending.ByStep) == 0 {
		t.Fatalf("seed produced no steps: %+v", pending)
	}
	for _, want := range pending.ByStep {
		got, ok := stepOf(snap, want.Name)
		if !ok || got.Count != want.Count {
			t.Fatalf("step %q: snapshot %+v (found %v), pending count %d; steps %+v", want.Name, got, ok, want.Count, snap.Steps)
		}
	}
}

func TestBoardSnapshot_AnEmptyBoardAnswersZerosAndEmptyLists(t *testing.T) {
	mgr := newTestManager(t)

	snap := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})

	for _, step := range snap.Steps {
		if step.Count != 0 || step.OldestDays != 0 || step.OldestCode != "" {
			t.Fatalf("empty board has a non-zero step: %+v", step)
		}
	}
	if snap.Ready != 0 || snap.TodoProgress.Total != 0 || snap.TodoProgress.Done != 0 {
		t.Fatalf("empty board has counts: %+v", snap)
	}
	if snap.Agents == nil || snap.Stalled == nil || snap.BlockedChains == nil || snap.Steps == nil {
		t.Fatalf("empty board answers null lists: %+v", snap)
	}
	if len(snap.Agents) != 0 || len(snap.Stalled) != 0 || len(snap.BlockedChains) != 0 {
		t.Fatalf("empty board lists are not empty: %+v", snap)
	}
}

func TestBoardSnapshot_AnUnknownBacklogIsRefused(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.BoardSnapshot(context.Background(), mwanachamataskmanager.BoardSnapshotFilter{Backlog: "Nowhere"})

	if !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("error = %v, want ErrUnknownBacklog", err)
	}
}

func TestBoardSnapshot_ANegativeStalledDaysIsRefused(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.BoardSnapshot(context.Background(), mwanachamataskmanager.BoardSnapshotFilter{StalledDays: -1})

	if !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
		t.Fatalf("error = %v, want ErrInvalidFilter", err)
	}
}

func TestBoardSnapshot_AnInProgressIssueIsStalledOnlyPastTheThreshold(t *testing.T) {
	mgr, db, table := newTestManagerWithDB(t)
	old := startIssue(t, mgr, mwanachamataskmanager.Task{Title: "three days quiet"})
	startIssue(t, mgr, mwanachamataskmanager.Task{Title: "just touched"})
	threeDaysAgo := time.Now().UTC().AddDate(0, 0, -3).Format(time.RFC3339)
	if err := db.Table(table).Where("id = ?", old.ID).UpdateColumn("updated_at", threeDaysAgo).Error; err != nil {
		t.Fatalf("age the issue: %v", err)
	}

	byDefault := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})
	if len(byDefault.Stalled) != 1 || byDefault.Stalled[0].Code != old.Code || byDefault.Stalled[0].Days != 3 {
		t.Fatalf("default threshold (2 days) stalled = %+v, want only %s at 3 days", byDefault.Stalled, old.Code)
	}
	if byDefault.Stalled[0].Title != "three days quiet" {
		t.Fatalf("stalled title = %q", byDefault.Stalled[0].Title)
	}
	if got := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{StalledDays: 5}).Stalled; len(got) != 0 {
		t.Fatalf("5-day threshold stalled = %+v, want none", got)
	}
	if got := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{StalledDays: 1}).Stalled; len(got) != 1 {
		t.Fatalf("1-day threshold stalled = %+v, want the one old issue", got)
	}
}

func TestBoardSnapshot_ATodoIssueNotInProgressIsNeverStalled(t *testing.T) {
	mgr, db, table := newTestManagerWithDB(t)
	idle := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "waiting its turn"})
	longAgo := time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339)
	if err := db.Table(table).Where("id = ?", idle.ID).UpdateColumn("updated_at", longAgo).Error; err != nil {
		t.Fatalf("age the issue: %v", err)
	}

	snap := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})

	if len(snap.Stalled) != 0 {
		t.Fatalf("stalled = %+v, want none for an issue not in progress", snap.Stalled)
	}
	todo, ok := stepOf(snap, "To do")
	if !ok || todo.Count != 1 || todo.OldestDays != 30 || todo.OldestCode != idle.Code {
		t.Fatalf("To do step = %+v (found %v), want 1 issue, oldest %s at 30 days", todo, ok, idle.Code)
	}
}

func TestBoardSnapshot_ATwoStepBlockedChainIsReportedWithBothLinks(t *testing.T) {
	mgr := newTestManager(t)
	first := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "first link", Tags: []string{"Blocked"}})
	second := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "second link", Tags: []string{"Blocked"}})
	root := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "root cause"})
	linkDependsOn(t, mgr, first, second)
	linkDependsOn(t, mgr, second, root)

	snap := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})

	if len(snap.BlockedChains) != 2 {
		t.Fatalf("blocked chains = %+v, want both blocked issues", snap.BlockedChains)
	}
	byCode := map[string]mwanachamataskmanager.SnapshotBlocked{}
	for _, link := range snap.BlockedChains {
		byCode[link.Code] = link
	}
	head := byCode[first.Code]
	if len(head.WaitsOn) != 1 || head.WaitsOn[0].Code != second.Code || !head.WaitsOn[0].Blocked {
		t.Fatalf("first link waits on %+v, want %s (itself blocked)", head.WaitsOn, second.Code)
	}
	tail := byCode[second.Code]
	if len(tail.WaitsOn) != 1 || tail.WaitsOn[0].Code != root.Code || tail.WaitsOn[0].Blocked {
		t.Fatalf("second link waits on %+v, want %s (not blocked)", tail.WaitsOn, root.Code)
	}
}

func TestBoardSnapshot_ABlockedIssueWithNothingToWaitOnIsStillListed(t *testing.T) {
	mgr := newTestManager(t)
	alone := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "stuck for a reason", Tags: []string{"Blocked"}})

	snap := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})

	if len(snap.BlockedChains) != 1 || snap.BlockedChains[0].Code != alone.Code || len(snap.BlockedChains[0].WaitsOn) != 0 {
		t.Fatalf("blocked chains = %+v, want %s with no waits_on", snap.BlockedChains, alone.Code)
	}
}

func TestBoardSnapshot_ReadyCountsOnlyReadyIssuesThatAreNotBlockedWaitingOrSinbin(t *testing.T) {
	mgr := newTestManager(t)
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "free", Tags: []string{"Ready"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "free too", Tags: []string{"Ready"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "also blocked", Tags: []string{"Ready", "Blocked"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "also sinbin", Tags: []string{"Ready", "Sinbin"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "also waiting", Tags: []string{"Ready", "Waiting"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "not ready"})

	snap := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})

	if snap.Ready != 2 {
		t.Fatalf("ready = %d, want 2", snap.Ready)
	}
	next, err := mgr.NextReady(context.Background(), mwanachamataskmanager.NextReadyFilter{Limit: 5})
	if err != nil {
		t.Fatalf("NextReady: %v", err)
	}
	if next.TotalReady != snap.Ready {
		t.Fatalf("snapshot ready %d differs from next_ready total_ready %d", snap.Ready, next.TotalReady)
	}
}

func TestBoardSnapshot_ClosedIssuesAreNotCountedAnywhere(t *testing.T) {
	mgr := newTestManager(t)
	done := startIssue(t, mgr, mwanachamataskmanager.Task{Title: "finished", Tags: []string{"Ready", "Blocked"}})
	done.Status = mwanachamataskmanager.TaskStatusCompleted
	if _, err := mgr.UpdateTask(context.Background(), done); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	snap := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})

	if len(snap.BlockedChains) != 0 || snap.Ready != 0 || len(snap.Stalled) != 0 {
		t.Fatalf("a completed issue leaked into the snapshot: %+v", snap)
	}
}

func TestBoardSnapshot_AgentsShowTheirInProgressIssues(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "worker"}); err != nil {
		t.Fatal(err)
	}
	agent, err := mgr.GetAgent(ctx, "worker")
	if err != nil {
		t.Fatal(err)
	}
	first := startIssue(t, mgr, mwanachamataskmanager.Task{Title: "one"})
	second := startIssue(t, mgr, mwanachamataskmanager.Task{Title: "two"})
	startIssue(t, mgr, mwanachamataskmanager.Task{Title: "nobody's"})
	for _, issue := range []mwanachamataskmanager.Task{first, second} {
		if err := mgr.AssignTask(ctx, issue.ID, "worker", ""); err != nil {
			t.Fatalf("AssignTask: %v", err)
		}
	}

	snap := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})

	var mine *mwanachamataskmanager.SnapshotAgent
	unassigned := 0
	for i, a := range snap.Agents {
		if a.Agent == agent.ID {
			mine = &snap.Agents[i]
		}
		if a.Agent == "" {
			unassigned = a.InProgress
		}
	}
	if mine == nil || mine.InProgress != 2 || len(mine.Codes) != 2 {
		t.Fatalf("agents = %+v, want %s with 2 in progress", snap.Agents, agent.ID)
	}
	if unassigned != 1 {
		t.Fatalf("unassigned in progress = %d, want 1: %+v", unassigned, snap.Agents)
	}
}

func TestBoardSnapshot_TodoProgressSumsTheTodosOfInProgressIssuesOnly(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	active := startIssue(t, mgr, mwanachamataskmanager.Task{Title: "active"})
	idle := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "idle"})
	var pieces []mwanachamataskmanager.TaskTodo
	for i, parent := range []string{active.ID, active.ID, active.ID, idle.ID} {
		piece, err := mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{Title: "piece", Instructions: "do it", ParentTaskID: parent, Ordinality: i + 1})
		if err != nil {
			t.Fatalf("CreateTaskTodo: %v", err)
		}
		pieces = append(pieces, piece)
	}
	if _, err := mgr.UpdateTaskTodoStatus(ctx, pieces[0].ID, mwanachamataskmanager.TodoStatusCompleted); err != nil {
		t.Fatalf("complete a todo: %v", err)
	}

	snap := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})

	if snap.TodoProgress.Done != 1 || snap.TodoProgress.Total != 3 {
		t.Fatalf("todo progress = %+v, want 1 of 3", snap.TodoProgress)
	}
}
