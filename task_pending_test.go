package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func mustCreate(t *testing.T, mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task) mwanachamataskmanager.Task {
	t.Helper()
	out, err := mgr.CreateTask(context.Background(), task)
	if err != nil {
		t.Fatalf("CreateTask %q: %v", task.Title, err)
	}
	return out
}

func pendingOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, filter mwanachamataskmanager.PendingFilter) mwanachamataskmanager.PendingTasks {
	t.Helper()
	out, err := mgr.PendingTasks(context.Background(), filter)
	if err != nil {
		t.Fatalf("PendingTasks %+v: %v", filter, err)
	}
	return out
}

func itemTitles(out mwanachamataskmanager.PendingTasks) []string {
	titles := make([]string, len(out.Items))
	for i, item := range out.Items {
		titles[i] = item.Title
	}
	return titles
}

func countOf(counts []mwanachamataskmanager.PendingCount, name string) int {
	for _, c := range counts {
		if c.Name == name {
			return c.Count
		}
	}
	return -1
}

func seedPending(t *testing.T, mgr mwanachamataskmanager.TaskManager) {
	t.Helper()
	ctx := context.Background()
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "DEV-001 first", Priority: mwanachamataskmanager.TaskPriorityCritical, Tags: []string{"Blocked", "mwanachama-backend-git"}, Description: strings.Repeat("long ", 500)})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "DEV-002 second", Priority: mwanachamataskmanager.TaskPriorityHigh, Tags: []string{"Ready", "mwanachama-web-shared"}})
	third := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "DEV-003 third", Priority: mwanachamataskmanager.TaskPriorityHigh, Tags: []string{"Ready"}})
	third.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, third); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	done := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "DEV-004 finished", Priority: mwanachamataskmanager.TaskPriorityLow})
	done.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, done); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	done.Status = mwanachamataskmanager.TaskStatusCompleted
	if _, err := mgr.UpdateTask(ctx, done); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	cancelled := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "DEV-005 dropped"})
	cancelled.Status = mwanachamataskmanager.TaskStatusCancelled
	if _, err := mgr.UpdateTask(ctx, cancelled); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
}

func TestPendingTasks_CountsOnlyIssuesThatAreNotCompletedOrCancelled(t *testing.T) {
	mgr := newTestManager(t)
	seedPending(t, mgr)

	out := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{})

	if out.Total != 3 {
		t.Fatalf("total %d, want 3 (%v)", out.Total, itemTitles(out))
	}
	if got := countOf(out.ByPriority, "critical"); got != 1 {
		t.Fatalf("critical %d, want 1: %+v", got, out.ByPriority)
	}
	if got := countOf(out.ByPriority, "high"); got != 2 {
		t.Fatalf("high %d, want 2: %+v", got, out.ByPriority)
	}
	if got := countOf(out.ByPriority, "low"); got != -1 {
		t.Fatalf("a finished low issue is counted: %+v", out.ByPriority)
	}
	if out.ByPriority[0].Name != "critical" || out.ByPriority[1].Name != "high" {
		t.Fatalf("priorities not in order: %+v", out.ByPriority)
	}
}

func TestPendingTasks_CountsEachStepInStepOrder(t *testing.T) {
	mgr := newTestManager(t)
	seedPending(t, mgr)

	out := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{})

	if len(out.ByStep) != 2 || out.ByStep[0].Name != "To do" || out.ByStep[0].Count != 2 || out.ByStep[1].Name != "Doing" || out.ByStep[1].Count != 1 {
		t.Fatalf("by_step %+v, want To do 2 then Doing 1", out.ByStep)
	}
}

func TestPendingTasks_ItemsAreCompactAndHaveNoDescription(t *testing.T) {
	mgr := newTestManager(t)
	seedPending(t, mgr)

	out := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{})
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "description") || strings.Contains(string(raw), "long long") {
		t.Fatalf("a description is in the answer: %s", raw)
	}
	first := out.Items[0]
	if first.Title != "DEV-001 first" || first.Priority != "critical" || first.Backlog != "Backlog" || first.Step != "To do" || first.ID == "" {
		t.Fatalf("first item %+v", first)
	}
	if !first.Blocked {
		t.Fatalf("the Blocked tag does not make the issue blocked: %+v", first)
	}
	if out.Items[1].Blocked {
		t.Fatalf("a Ready issue is blocked: %+v", out.Items[1])
	}
	if got := strings.Join(first.Tags, ","); !strings.Contains(got, "Blocked") || !strings.Contains(got, "mwanachama-backend-git") {
		t.Fatalf("tags %v", first.Tags)
	}
}

func TestPendingTasks_ItemsFollowRankAndPageWithOffset(t *testing.T) {
	mgr := newTestManager(t)
	for i := 1; i <= 60; i++ {
		mustCreate(t, mgr, mwanachamataskmanager.Task{Title: fmt.Sprintf("DEV-%03d", i)})
	}

	firstPage := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{})
	if firstPage.Total != 60 || len(firstPage.Items) != 25 {
		t.Fatalf("total %d items %d, want 60 and the default 25", firstPage.Total, len(firstPage.Items))
	}
	ranks := map[int]bool{}
	for i, item := range firstPage.Items {
		if i > 0 && item.Rank < firstPage.Items[i-1].Rank {
			t.Fatalf("items are not in rank order: %+v", firstPage.Items)
		}
		ranks[item.Rank] = true
	}
	second := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{Limit: 25, Offset: 25})
	third := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{Limit: 25, Offset: 50})
	if len(second.Items) != 25 || len(third.Items) != 10 {
		t.Fatalf("pages of %d and %d, want 25 and 10", len(second.Items), len(third.Items))
	}
	seen := map[string]bool{}
	for _, page := range []mwanachamataskmanager.PendingTasks{firstPage, second, third} {
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatalf("%s is on two pages", item.Title)
			}
			seen[item.ID] = true
		}
	}
	if len(seen) != 60 {
		t.Fatalf("%d issues over three pages, want 60", len(seen))
	}
	if got := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{Limit: 1000}); len(got.Items) != 60 {
		t.Fatalf("limit above the cap returned %d items, want all 60 (cap is 100)", len(got.Items))
	}
	if got := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{Limit: 5}); len(got.Items) != 5 || got.Total != 60 {
		t.Fatalf("limit 5 gave %d items and total %d", len(got.Items), got.Total)
	}
}

func TestPendingTasks_RefusesANegativeOffset(t *testing.T) {
	mgr := newTestManager(t)
	if _, err := mgr.PendingTasks(context.Background(), mwanachamataskmanager.PendingFilter{Offset: -1}); err == nil {
		t.Fatal("a negative offset was accepted")
	}
}

func TestPendingTasks_NarrowsByTagPriorityAndWords(t *testing.T) {
	mgr := newTestManager(t)
	seedPending(t, mgr)

	cases := []struct {
		name   string
		filter mwanachamataskmanager.PendingFilter
		want   []string
	}{
		{"one tag", mwanachamataskmanager.PendingFilter{Tag: []string{"ready"}}, []string{"DEV-002 second", "DEV-003 third"}},
		{"two tags must both match", mwanachamataskmanager.PendingFilter{Tag: []string{"Ready", "mwanachama-web-shared"}}, []string{"DEV-002 second"}},
		{"priority", mwanachamataskmanager.PendingFilter{Priority: mwanachamataskmanager.TaskPriorityCritical}, []string{"DEV-001 first"}},
		{"word in the title", mwanachamataskmanager.PendingFilter{Q: "second"}, []string{"DEV-002 second"}},
		{"word in a tag", mwanachamataskmanager.PendingFilter{Q: "GIT"}, []string{"DEV-001 first"}},
		{"every word must match", mwanachamataskmanager.PendingFilter{Q: "dev blocked"}, []string{"DEV-001 first"}},
		{"words across title and tag", mwanachamataskmanager.PendingFilter{Q: "third ready"}, []string{"DEV-003 third"}},
		{"nothing matches", mwanachamataskmanager.PendingFilter{Q: "zzz"}, []string{}},
		{"a percent is not a wildcard", mwanachamataskmanager.PendingFilter{Q: "%"}, []string{}},
	}
	for _, c := range cases {
		out := pendingOf(t, mgr, c.filter)
		got := itemTitles(out)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
		if out.Total != len(c.want) {
			t.Errorf("%s: total %d, want %d", c.name, out.Total, len(c.want))
		}
	}
}

func TestPendingTasks_NarrowsByBacklogAndStep(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in the first"})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in software", Backlog: "software", Step: "open"})

	soft := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{Backlog: "Software"})
	if got := itemTitles(soft); len(got) != 1 || got[0] != "in software" || soft.Items[0].Backlog != "Software" || soft.Items[0].Step != "Open" {
		t.Fatalf("software items %v %+v", got, soft.Items)
	}
	step := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{Backlog: "software", Step: "open"})
	if step.Total != 1 {
		t.Fatalf("step total %d", step.Total)
	}
	all := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{})
	if all.Total != 2 || countOf(all.ByStep, "Open") != 1 || countOf(all.ByStep, "To do") != 1 {
		t.Fatalf("all %+v", all)
	}
	if _, err := mgr.PendingTasks(context.Background(), mwanachamataskmanager.PendingFilter{Backlog: "nope"}); err == nil {
		t.Fatal("an unknown backlog was accepted")
	}
}

func TestPendingTasks_ShowsAPersonOnlyTheBacklogsTheirGroupsCanSee(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in the first"})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in software", Backlog: "software", Step: "open"})
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see"); err != nil {
		t.Fatal(err)
	}

	for person, want := range map[string]int{"owner": 2, "dev": 1, "none": 0, "": 2} {
		out := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{RequesterID: person})
		if out.Total != want || len(out.Items) != want {
			t.Errorf("%q: total %d items %d, want %d", person, out.Total, len(out.Items), want)
		}
		if got := countOf(out.ByStep, "To do"); person == "dev" && got != -1 {
			t.Errorf("dev is counted in a backlog it cannot see: %+v", out.ByStep)
		}
	}
	dev := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{RequesterID: "dev"})
	if dev.Items[0].Title != "in software" || countOf(dev.ByStep, "Open") != 1 {
		t.Fatalf("dev sees %+v", dev)
	}
}

func TestPendingTasks_IsFarCheaperThanPagingTheList(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds 2,000 issues")
	}
	mgr := newTestManager(t)
	ctx := context.Background()
	for i := 1; i <= 2000; i++ {
		mustCreate(t, mgr, mwanachamataskmanager.Task{
			Title: fmt.Sprintf("DEV-%04d a short title", i), Description: strings.Repeat("detail ", 300),
			Tags: []string{"mwanachama-frontend-kazi", "Ready"},
		})
	}

	start := time.Now()
	out, err := mgr.PendingTasks(ctx, mwanachamataskmanager.PendingFilter{})
	pending := time.Since(start)
	if err != nil || out.Total != 2000 || len(out.Items) != 25 {
		t.Fatalf("pending: %v total %d items %d", err, out.Total, len(out.Items))
	}

	pendingBytes, _ := json.Marshal(out)

	start = time.Now()
	read, pagedBytes := 0, 0
	for offset := 0; ; offset += 500 {
		page, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Offset: offset})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(page)
		pagedBytes += len(raw)
		read += len(page)
		if len(page) < 500 {
			break
		}
	}
	paging := time.Since(start)
	if read != 2000 {
		t.Fatalf("paging read %d", read)
	}
	t.Logf("pending %v and %d bytes, paging the list %v and %d bytes", pending, len(pendingBytes), paging, pagedBytes)
	if len(pendingBytes)*20 > pagedBytes {
		t.Fatalf("pending answers with %d bytes against %d for paging the list", len(pendingBytes), pagedBytes)
	}
	if pending*5 > paging {
		t.Fatalf("pending took %v against %v for paging the list: not far cheaper", pending, paging)
	}
}

func TestPendingTasks_CountsDoneAndCancelledByStepOnlyWhenAsked(t *testing.T) {
	mgr := newTestManager(t)
	seedPending(t, mgr)

	asked := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{IncludeClosed: true})

	want := map[string]int{"To do": 2, "Doing": 1, "Done": 1, "Cancelled": 1}
	if len(asked.ByStep) != len(want) {
		t.Fatalf("by_step %+v, want %v", asked.ByStep, want)
	}
	for name, count := range want {
		if got := countOf(asked.ByStep, name); got != count {
			t.Fatalf("%s %d, want %d: %+v", name, got, count, asked.ByStep)
		}
	}
	if names := []string{asked.ByStep[0].Name, asked.ByStep[1].Name, asked.ByStep[2].Name, asked.ByStep[3].Name}; strings.Join(names, ",") != "To do,Doing,Done,Cancelled" {
		t.Fatalf("by_step not in step order: %+v", asked.ByStep)
	}
	if asked.Total != 3 || len(asked.Items) != 3 {
		t.Fatalf("total %d items %d, want the 3 open issues only", asked.Total, len(asked.Items))
	}

	plain := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{})
	if countOf(plain.ByStep, "Done") != -1 || countOf(plain.ByStep, "Cancelled") != -1 {
		t.Fatalf("closed steps are counted without being asked: %+v", plain.ByStep)
	}
}

func TestPendingTasks_ClosedCountsFollowTheTagFilter(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "open mine", Tags: []string{"mine"}})
	for _, tags := range [][]string{{"mine"}, {"other"}} {
		task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "closed " + tags[0], Tags: tags})
		task.Status = mwanachamataskmanager.TaskStatusCancelled
		if _, err := mgr.UpdateTask(ctx, task); err != nil {
			t.Fatalf("UpdateTask: %v", err)
		}
	}

	out := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{Tag: []string{"mine"}, IncludeClosed: true})

	if got := countOf(out.ByStep, "Cancelled"); got != 1 {
		t.Fatalf("Cancelled %d, want 1: %+v", got, out.ByStep)
	}
}
