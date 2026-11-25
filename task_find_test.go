package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func findOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, filter mwanachamataskmanager.FindFilter) mwanachamataskmanager.FoundTasks {
	t.Helper()
	out, err := mgr.FindTasks(context.Background(), filter)
	if err != nil {
		t.Fatalf("FindTasks %+v: %v", filter, err)
	}
	return out
}

func foundCodes(out mwanachamataskmanager.FoundTasks) []string {
	codes := make([]string, len(out.Items))
	for i, item := range out.Items {
		codes[i] = item.Code
	}
	return codes
}

func seedFind(t *testing.T, mgr mwanachamataskmanager.TaskManager) map[string]mwanachamataskmanager.Task {
	t.Helper()
	ctx := context.Background()
	made := map[string]mwanachamataskmanager.Task{}
	for _, task := range []mwanachamataskmanager.Task{
		{Title: "Add a login button", Code: "SW4", ProjectName: "swe", Tags: []string{"Ready", "app"}, Description: "the button sits in the header"},
		{Title: "Fix the footer", Code: "SW40", ProjectName: "swe", Tags: []string{"bug"}, Description: "Duplicate of SW4 in spirit"},
		{Title: "Fix the sidebar", Code: "SW41", ProjectName: "swe", Description: "A Narrow screen hides the Sidebar"},
		{Title: "Other repo row", Code: "ZZ4", ProjectName: "zz"},
		{Title: "Follow up", Code: "SW9", ProjectName: "swe", Description: "see SW4 and SW41 first"},
		{Title: "Finished chore", Code: "SW5", ProjectName: "swe", Tags: []string{"app"}},
	} {
		made[task.Code] = mustCreate(t, mgr, task)
	}
	finished := made["SW5"]
	finished.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, finished); err != nil {
		t.Fatal(err)
	}
	finished.Status = mwanachamataskmanager.TaskStatusCompleted
	if _, err := mgr.UpdateTask(ctx, finished); err != nil {
		t.Fatal(err)
	}
	return made
}

func TestFindTasks_AnIssueIdAnswersWithThatIssueAlone(t *testing.T) {
	mgr := newTestManager(t)
	made := seedFind(t, mgr)

	out := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: made["SW41"].ID})

	if out.Total != 1 || len(out.Items) != 1 || out.Items[0].Code != "SW41" || out.Items[0].ID != made["SW41"].ID {
		t.Fatalf("got %+v, want SW41 alone", out)
	}
}

func TestFindTasks_AnIdOfAClosedIssueIsFoundOnlyWhenClosedAreAsked(t *testing.T) {
	mgr := newTestManager(t)
	made := seedFind(t, mgr)

	if out := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: made["SW5"].ID}); out.Total != 0 {
		t.Fatalf("a closed issue was found without include_closed: %+v", out)
	}
	if out := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: made["SW5"].ID, IncludeClosed: true}); out.Total != 1 {
		t.Fatalf("a closed issue was not found with include_closed: %+v", out)
	}
}

func TestFindTasks_AnExactCodeIsFoundWithoutRegardToCaseAndAnswersFirst(t *testing.T) {
	mgr := newTestManager(t)
	seedFind(t, mgr)

	out := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "sw4"})

	if got := foundCodes(out); !slices.Equal(got, []string{"SW4", "SW40", "SW41"}) {
		t.Fatalf("got %v, want SW4 first, then its prefix matches", got)
	}
	if out.Total != 3 {
		t.Fatalf("total %d, want 3", out.Total)
	}
}

func TestFindTasks_ACodePrefixFindsEveryCodeThatStartsWithIt(t *testing.T) {
	mgr := newTestManager(t)
	seedFind(t, mgr)

	out := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "SW4"})

	if got := foundCodes(out); !slices.Equal(got, []string{"SW4", "SW40", "SW41"}) {
		t.Fatalf("got %v", got)
	}
	if got := foundCodes(findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "ZZ"})); !slices.Equal(got, []string{"ZZ4"}) {
		t.Fatalf("a prefix of letters alone found %v", got)
	}
}

func TestFindTasks_ACodeShapedQueryDoesNotMatchAnIssueThatOnlyMentionsTheCode(t *testing.T) {
	mgr := newTestManager(t)
	seedFind(t, mgr)

	got := foundCodes(findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "SW41"}))

	if !slices.Equal(got, []string{"SW41"}) {
		t.Fatalf("got %v, want SW41 only, not SW9 that names it in its description", got)
	}
}

func TestFindTasks_WordsAreFoundInTheDescriptionWithoutRegardToCase(t *testing.T) {
	mgr := newTestManager(t)
	seedFind(t, mgr)

	got := foundCodes(findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "narrow SIDEBAR"}))

	if !slices.Equal(got, []string{"SW41"}) {
		t.Fatalf("got %v, want SW41", got)
	}
	if got := foundCodes(findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "header button"})); !slices.Equal(got, []string{"SW4"}) {
		t.Fatalf("title and description words found %v", got)
	}
}

func TestFindTasks_EveryWordMustAppearAcrossTitleAndTag(t *testing.T) {
	mgr := newTestManager(t)
	seedFind(t, mgr)

	got := foundCodes(findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "login app"}))

	if !slices.Equal(got, []string{"SW4"}) {
		t.Fatalf("got %v, want SW4 (title word login, tag app)", got)
	}
	if got := foundCodes(findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "login bug"})); len(got) != 0 {
		t.Fatalf("a word that no field of one issue holds matched %v", got)
	}
}

func TestFindTasks_AWildcardInTheQueryIsTakenLiterally(t *testing.T) {
	mgr := newTestManager(t)
	seedFind(t, mgr)

	if out := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "%"}); out.Total != 0 {
		t.Fatalf("a percent sign matched %+v", out)
	}
}

func TestFindTasks_ClosedIssuesAreFoundOnlyWhenAsked(t *testing.T) {
	mgr := newTestManager(t)
	seedFind(t, mgr)

	if got := foundCodes(findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "chore"})); len(got) != 0 {
		t.Fatalf("a closed issue was found without include_closed: %v", got)
	}
	if got := foundCodes(findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "chore", IncludeClosed: true})); !slices.Equal(got, []string{"SW5"}) {
		t.Fatalf("got %v with include_closed", got)
	}
	if got := foundCodes(findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "SW5", IncludeClosed: true})); !slices.Equal(got, []string{"SW5"}) {
		t.Fatalf("a closed code found %v", got)
	}
}

func TestFindTasks_NarrowsByTagStepAndBacklogNamedByNameOrId(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "fix in the first", Tags: []string{"app"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "fix in software", Backlog: "software", Step: "open", Tags: []string{"app"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "fix shipped", Backlog: "software", Step: "shipped"})

	titles := func(filter mwanachamataskmanager.FindFilter) []string {
		out := findOf(t, mgr, filter)
		names := make([]string, len(out.Items))
		for i, item := range out.Items {
			names[i] = item.Title
		}
		return names
	}
	if got := titles(mwanachamataskmanager.FindFilter{Q: "fix", Backlog: "Software"}); len(got) != 2 {
		t.Fatalf("backlog by name found %v", got)
	}
	if got := titles(mwanachamataskmanager.FindFilter{Q: "fix", Backlog: "software"}); len(got) != 2 {
		t.Fatalf("backlog by id found %v", got)
	}
	if got := titles(mwanachamataskmanager.FindFilter{Q: "fix", Backlog: "software", Step: "shipped"}); !slices.Equal(got, []string{"fix shipped"}) {
		t.Fatalf("step found %v", got)
	}
	if got := titles(mwanachamataskmanager.FindFilter{Q: "fix", Tag: []string{"app"}}); len(got) != 2 {
		t.Fatalf("tag found %v", got)
	}
	if _, err := mgr.FindTasks(context.Background(), mwanachamataskmanager.FindFilter{Q: "fix", Backlog: "nope"}); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("an unknown backlog gave %v", err)
	}
}

func TestFindTasks_PagesInRankOrderAndTotalCountsEveryMatch(t *testing.T) {
	mgr := newTestManager(t)
	for _, title := range []string{"page one", "page two", "page three", "page four", "page five"} {
		mustCreate(t, mgr, mwanachamataskmanager.Task{Title: title})
	}

	first := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "page", Limit: 2})
	second := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "page", Limit: 2, Offset: 2})
	last := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "page", Limit: 2, Offset: 4})

	if first.Total != 5 || second.Total != 5 || last.Total != 5 {
		t.Fatalf("totals %d %d %d, want 5", first.Total, second.Total, last.Total)
	}
	var titles []string
	for _, page := range []mwanachamataskmanager.FoundTasks{first, second, last} {
		for _, item := range page.Items {
			titles = append(titles, item.Title)
		}
	}
	if want := []string{"page one", "page two", "page three", "page four", "page five"}; !slices.Equal(titles, want) {
		t.Fatalf("pages gave %v, want %v", titles, want)
	}
}

func TestFindTasks_LimitDefaultsTo25AndStopsAt100(t *testing.T) {
	mgr := newTestManager(t)
	for i := 0; i < 105; i++ {
		mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "bulk row"})
	}

	if got := len(findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "bulk"}).Items); got != 25 {
		t.Fatalf("default limit gave %d", got)
	}
	if got := len(findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "bulk", Limit: 500}).Items); got != 100 {
		t.Fatalf("a limit over the cap gave %d", got)
	}
}

func TestFindTasks_ItemsAreTheCompactShapeOfPendingWithoutADescription(t *testing.T) {
	mgr := newTestManager(t)
	seedFind(t, mgr)

	found := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "login"})
	pending := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{Q: "login"})

	if len(found.Items) != 1 || len(pending.Items) != 1 {
		t.Fatalf("found %d, pending %d", len(found.Items), len(pending.Items))
	}
	var _ mwanachamataskmanager.PendingItem = found.Items[0]
	if found.Items[0].ID != pending.Items[0].ID || found.Items[0].Rank != pending.Items[0].Rank || found.Items[0].State != pending.Items[0].State ||
		found.Items[0].Backlog != pending.Items[0].Backlog || found.Items[0].Step != pending.Items[0].Step || !slices.Equal(found.Items[0].Tags, pending.Items[0].Tags) {
		t.Fatalf("found %+v differs from pending %+v", found.Items[0], pending.Items[0])
	}
}

func TestFindTasks_MarksTheSameFlagsAsPending(t *testing.T) {
	mgr := newTestManager(t)
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "stuck", Tags: []string{"Blocked"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "stopped", Tags: []string{"Sinbin"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "later", Tags: []string{"Waiting"}})

	out := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "stuck stopped later"})
	if out.Total != 0 {
		t.Fatalf("words of three issues matched one: %+v", out)
	}
	if item := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "stuck"}).Items[0]; !item.Blocked || item.Waiting || item.Sinbin {
		t.Fatalf("stuck %+v", item)
	}
	if item := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "stopped"}).Items[0]; !item.Sinbin || item.Blocked {
		t.Fatalf("stopped %+v", item)
	}
	if item := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: "later"}).Items[0]; !item.Waiting {
		t.Fatalf("later %+v", item)
	}
}

func TestFindTasks_ShowsAPersonOnlyTheBacklogsTheirGroupsCanSee(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "secret in the first", Code: "AA1"})
	hidden := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "secret in software", Code: "AA2", Backlog: "software", Step: "open"})
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	if _, err := giveSee(mgr, ctx, "software", devGroup); err != nil {
		t.Fatal(err)
	}

	for person, want := range map[string]int{"owner": 2, "dev": 1, "none": 0, "": 2} {
		for _, q := range []string{"secret", "AA", hidden.ID} {
			out := findOf(t, mgr, mwanachamataskmanager.FindFilter{Q: q, RequesterID: person})
			wantFor := want
			if q == hidden.ID {
				wantFor = map[string]int{"owner": 1, "dev": 1, "none": 0, "": 1}[person]
			}
			if out.Total != wantFor || len(out.Items) != wantFor {
				t.Errorf("%q asking %q: total %d items %d, want %d", person, q, out.Total, len(out.Items), wantFor)
			}
		}
	}
}

func TestFindTasks_RefusesABlankQueryAndANegativeOffset(t *testing.T) {
	mgr := newTestManager(t)

	for _, filter := range []mwanachamataskmanager.FindFilter{{}, {Q: "   "}, {Q: "x", Offset: -1}} {
		if _, err := mgr.FindTasks(context.Background(), filter); !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
			t.Errorf("%+v gave %v, want ErrInvalidFilter", filter, err)
		}
	}
}
