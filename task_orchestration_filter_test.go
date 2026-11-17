package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type orchestrationBoard struct {
	fixture todoFixture
	ids     map[string]string
}

func seedOrchestrationBoard(t *testing.T) orchestrationBoard {
	t.Helper()
	f := newTodoFixture(t)
	ids := map[string]string{}
	add := func(task mwanachamataskmanager.Task) {
		ids[task.Title] = mustCreate(t, f.mgr, task).ID
	}
	add(mwanachamataskmanager.Task{Title: "alpha", Priority: mwanachamataskmanager.TaskPriorityHigh, Effort: mwanachamataskmanager.TaskEffortLow, Files: []string{"app/a/"}, AssignedTo: "agent-1", Tags: []string{"Ready"}})
	add(mwanachamataskmanager.Task{Title: "beta", Priority: mwanachamataskmanager.TaskPriorityCritical, Effort: mwanachamataskmanager.TaskEffortHigh, Tags: []string{"Blocked"}})
	add(mwanachamataskmanager.Task{Title: "gamma", Priority: mwanachamataskmanager.TaskPriorityLow, Tags: []string{"Sinbin", "Ready"}})
	add(mwanachamataskmanager.Task{Title: "delta", Priority: mwanachamataskmanager.TaskPriorityMedium, Effort: mwanachamataskmanager.TaskEffortLow, Files: []string{"app/b.go"}, AssignedTo: "agent-2", Tags: []string{"Waiting"}})
	add(mwanachamataskmanager.Task{Title: "epsilon", Priority: mwanachamataskmanager.TaskPriorityMedium, Tags: []string{"Ready"}})

	f.piece(t, ids["gamma"], "gamma open", 1)
	f.piece(t, ids["gamma"], "gamma shut", 2)
	done := f.piece(t, ids["delta"], "delta only", 1)
	if _, err := f.mgr.UpdateTaskTodoStatus(context.Background(), done.ID, mwanachamataskmanager.TodoStatusCompleted); err != nil {
		t.Fatalf("complete: %v", err)
	}
	return orchestrationBoard{fixture: f, ids: ids}
}

func (b orchestrationBoard) listed(t *testing.T, filter mwanachamataskmanager.TaskFilter) map[string]bool {
	t.Helper()
	return listTitles(t, b.fixture.mgr, filter)
}

func (b orchestrationBoard) pending(t *testing.T, filter mwanachamataskmanager.PendingFilter) []string {
	t.Helper()
	return itemTitles(pendingOf(t, b.fixture.mgr, filter))
}

func truth(v bool) *bool { return &v }

func sameTitles(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestOrchestrationFilters_ListAndPendingAgreeForEachFilter(t *testing.T) {
	b := seedOrchestrationBoard(t)
	tomorrow := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	yesterday := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)

	cases := []struct {
		name    string
		list    mwanachamataskmanager.TaskFilter
		pending mwanachamataskmanager.PendingFilter
		want    []string
	}{
		{"assignee", mwanachamataskmanager.TaskFilter{Assignee: "agent-1"}, mwanachamataskmanager.PendingFilter{Assignee: "agent-1"}, []string{"alpha"}},
		{"unassigned", mwanachamataskmanager.TaskFilter{Unassigned: true}, mwanachamataskmanager.PendingFilter{Unassigned: true}, []string{"beta", "gamma", "epsilon"}},
		{"blocked", mwanachamataskmanager.TaskFilter{Blocked: truth(true)}, mwanachamataskmanager.PendingFilter{Blocked: truth(true)}, []string{"beta"}},
		{"not blocked", mwanachamataskmanager.TaskFilter{Blocked: truth(false)}, mwanachamataskmanager.PendingFilter{Blocked: truth(false)}, []string{"alpha", "gamma", "delta", "epsilon"}},
		{"sinbin", mwanachamataskmanager.TaskFilter{Sinbin: truth(true)}, mwanachamataskmanager.PendingFilter{Sinbin: truth(true)}, []string{"gamma"}},
		{"waiting", mwanachamataskmanager.TaskFilter{Waiting: truth(true)}, mwanachamataskmanager.PendingFilter{Waiting: truth(true)}, []string{"delta"}},
		{"not waiting", mwanachamataskmanager.TaskFilter{Waiting: truth(false)}, mwanachamataskmanager.PendingFilter{Waiting: truth(false)}, []string{"alpha", "beta", "gamma", "epsilon"}},
		{"has todos", mwanachamataskmanager.TaskFilter{HasTodos: truth(true)}, mwanachamataskmanager.PendingFilter{HasTodos: truth(true)}, []string{"gamma", "delta"}},
		{"no todos", mwanachamataskmanager.TaskFilter{HasTodos: truth(false)}, mwanachamataskmanager.PendingFilter{HasTodos: truth(false)}, []string{"alpha", "beta", "epsilon"}},
		{"todos open", mwanachamataskmanager.TaskFilter{TodosOpen: true}, mwanachamataskmanager.PendingFilter{TodosOpen: true}, []string{"gamma"}},
		{"effort low", mwanachamataskmanager.TaskFilter{Effort: "low"}, mwanachamataskmanager.PendingFilter{Effort: "low"}, []string{"alpha", "delta"}},
		{"effort unknown", mwanachamataskmanager.TaskFilter{Effort: "unknown"}, mwanachamataskmanager.PendingFilter{Effort: "unknown"}, []string{"gamma", "epsilon"}},
		{"has files", mwanachamataskmanager.TaskFilter{HasFiles: truth(true)}, mwanachamataskmanager.PendingFilter{HasFiles: truth(true)}, []string{"alpha", "delta"}},
		{"no files", mwanachamataskmanager.TaskFilter{HasFiles: truth(false)}, mwanachamataskmanager.PendingFilter{HasFiles: truth(false)}, []string{"beta", "gamma", "epsilon"}},
		{"created since yesterday", mwanachamataskmanager.TaskFilter{CreatedSince: yesterday}, mwanachamataskmanager.PendingFilter{CreatedSince: yesterday}, []string{"alpha", "beta", "gamma", "delta", "epsilon"}},
		{"created since tomorrow", mwanachamataskmanager.TaskFilter{CreatedSince: tomorrow}, mwanachamataskmanager.PendingFilter{CreatedSince: tomorrow}, nil},
		{"updated since tomorrow", mwanachamataskmanager.TaskFilter{UpdatedSince: tomorrow}, mwanachamataskmanager.PendingFilter{UpdatedSince: tomorrow}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantTitles(t, b.listed(t, tc.list), tc.want...)
			got := b.pending(t, tc.pending)
			if len(got) != len(tc.want) {
				t.Fatalf("pending %v, want %v", got, tc.want)
			}
			seen := map[string]bool{}
			for _, title := range got {
				seen[title] = true
			}
			for _, title := range tc.want {
				if !seen[title] {
					t.Fatalf("pending %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestOrchestrationFilters_ByIDs(t *testing.T) {
	b := seedOrchestrationBoard(t)
	ids := []string{b.ids["beta"], b.ids["epsilon"], "no-such-issue"}

	wantTitles(t, b.listed(t, mwanachamataskmanager.TaskFilter{IDs: ids}), "beta", "epsilon")
	if got := b.pending(t, mwanachamataskmanager.PendingFilter{IDs: ids}); len(got) != 2 {
		t.Fatalf("pending by ids %v, want 2", got)
	}
}

func TestOrchestrationFilters_TwoTogetherNarrowFurther(t *testing.T) {
	b := seedOrchestrationBoard(t)

	wantTitles(t, b.listed(t, mwanachamataskmanager.TaskFilter{Effort: "low", Assignee: "agent-2"}), "delta")
	wantTitles(t, b.listed(t, mwanachamataskmanager.TaskFilter{Unassigned: true, HasTodos: truth(true)}), "gamma")
	wantTitles(t, b.listed(t, mwanachamataskmanager.TaskFilter{Tag: []string{"Ready"}, HasFiles: truth(false)}), "gamma", "epsilon")
	wantTitles(t, b.listed(t, mwanachamataskmanager.TaskFilter{Effort: "low", Assignee: "agent-2", HasFiles: truth(false)}))
	got := b.pending(t, mwanachamataskmanager.PendingFilter{Priority: mwanachamataskmanager.TaskPriorityMedium, Unassigned: true})
	if !sameTitles(got, []string{"epsilon"}) {
		t.Fatalf("pending %v, want [epsilon]", got)
	}
}

func TestOrchestrationFilters_PendingCountsFollowTheFilter(t *testing.T) {
	b := seedOrchestrationBoard(t)

	out := pendingOf(t, b.fixture.mgr, mwanachamataskmanager.PendingFilter{Unassigned: true})

	if out.Total != 3 {
		t.Fatalf("total %d, want 3", out.Total)
	}
	if got := countOf(out.ByPriority, "critical"); got != 1 {
		t.Fatalf("critical %d, want 1: %+v", got, out.ByPriority)
	}
}

func TestOrchestrationFilters_SortOrdersTheRows(t *testing.T) {
	b := seedOrchestrationBoard(t)

	byPriority := b.pending(t, mwanachamataskmanager.PendingFilter{Sort: "priority"})
	if byPriority[0] != "beta" || byPriority[len(byPriority)-1] != "gamma" {
		t.Fatalf("by priority %v, want beta first and gamma last", byPriority)
	}
	tasks, err := b.fixture.mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{Sort: "priority"})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if tasks[0].Title != "beta" || tasks[len(tasks)-1].Title != "gamma" {
		t.Fatalf("list by priority starts %q ends %q", tasks[0].Title, tasks[len(tasks)-1].Title)
	}
	byRank := b.pending(t, mwanachamataskmanager.PendingFilter{Sort: "rank"})
	plain := b.pending(t, mwanachamataskmanager.PendingFilter{})
	if !sameTitles(byRank, plain) {
		t.Fatalf("rank %v differs from default %v", byRank, plain)
	}
}

func TestOrchestrationFilters_BadValuesAreRefused(t *testing.T) {
	b := seedOrchestrationBoard(t)
	ctx := context.Background()
	tooMany := make([]string, 101)
	for i := range tooMany {
		tooMany[i] = "x"
	}
	list := []mwanachamataskmanager.TaskFilter{
		{Effort: "gigantic"},
		{CreatedSince: "last week"},
		{UpdatedSince: "2026-13-45"},
		{Sort: "alphabetical"},
		{IDs: tooMany},
		{Assignee: "agent-1", Unassigned: true},
	}
	for _, filter := range list {
		if _, err := b.fixture.mgr.ListTasks(ctx, filter); !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
			t.Errorf("list %+v gave %v, want ErrInvalidFilter", filter, err)
		}
	}
	pending := []mwanachamataskmanager.PendingFilter{
		{Effort: "gigantic"},
		{CreatedSince: "last week"},
		{Sort: "alphabetical"},
		{IDs: tooMany},
		{Assignee: "agent-1", Unassigned: true},
	}
	for _, filter := range pending {
		if _, err := b.fixture.mgr.PendingTasks(ctx, filter); !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
			t.Errorf("pending %+v gave %v, want ErrInvalidFilter", filter, err)
		}
	}
	ready := []mwanachamataskmanager.NextReadyFilter{{Effort: "gigantic"}, {UpdatedSince: "soon"}, {Assignee: "a", Unassigned: true}}
	for _, filter := range ready {
		if _, err := b.fixture.mgr.NextReady(ctx, filter); !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
			t.Errorf("next_ready %+v gave %v, want ErrInvalidFilter", filter, err)
		}
	}
}

func TestOrchestrationFilters_NextReadyNarrowsByEffortPriorityRepoAndFiles(t *testing.T) {
	f := newTodoFixture(t)
	ctx := context.Background()
	mustCreate(t, f.mgr, mwanachamataskmanager.Task{Title: "small", Priority: mwanachamataskmanager.TaskPriorityHigh, Effort: mwanachamataskmanager.TaskEffortLow, ProjectName: "repo-a", Files: []string{"a/"}, Tags: []string{"Ready"}})
	mustCreate(t, f.mgr, mwanachamataskmanager.Task{Title: "big", Priority: mwanachamataskmanager.TaskPriorityLow, Effort: mwanachamataskmanager.TaskEffortHigh, ProjectName: "repo-b", Tags: []string{"Ready"}})

	titles := func(filter mwanachamataskmanager.NextReadyFilter) []string {
		filter.Limit = 5
		out, err := f.mgr.NextReady(ctx, filter)
		if err != nil {
			t.Fatalf("NextReady %+v: %v", filter, err)
		}
		names := []string{}
		for _, item := range out.Items {
			names = append(names, item.Title)
		}
		return names
	}

	if got := titles(mwanachamataskmanager.NextReadyFilter{}); len(got) != 2 {
		t.Fatalf("unfiltered %v, want both", got)
	}
	if got := titles(mwanachamataskmanager.NextReadyFilter{Effort: "low"}); !sameTitles(got, []string{"small"}) {
		t.Fatalf("effort low %v", got)
	}
	if got := titles(mwanachamataskmanager.NextReadyFilter{Priority: mwanachamataskmanager.TaskPriorityLow}); !sameTitles(got, []string{"big"}) {
		t.Fatalf("priority low %v", got)
	}
	if got := titles(mwanachamataskmanager.NextReadyFilter{Repo: "repo-a"}); !sameTitles(got, []string{"small"}) {
		t.Fatalf("repo-a %v", got)
	}
	if got := titles(mwanachamataskmanager.NextReadyFilter{HasFiles: truth(false)}); !sameTitles(got, []string{"big"}) {
		t.Fatalf("no files %v", got)
	}
	if got := titles(mwanachamataskmanager.NextReadyFilter{Effort: "low", Repo: "repo-b"}); len(got) != 0 {
		t.Fatalf("effort low in repo-b %v, want none", got)
	}
	out, err := f.mgr.NextReady(ctx, mwanachamataskmanager.NextReadyFilter{Effort: "low"})
	if err != nil || out.TotalReady != 1 {
		t.Fatalf("total_ready %d err %v, want 1", out.TotalReady, err)
	}
}
