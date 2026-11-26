package mwanachamataskmanager_test

import (
	"context"
	"reflect"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	sprintLabel    = "sprint 1"
	emeaSprintTask = "emea sprint work"
	apacSprintTask = "apac sprint work"
)

var smartExpectations = map[string][]string{
	"emea":  {emeaSprintTask},
	"apac":  {apacSprintTask},
	"admin": {apacSprintTask, emeaSprintTask},
	"both":  {apacSprintTask, emeaSprintTask},
	"none":  {},
	"":      {apacSprintTask, emeaSprintTask},
}

func labelHas(label string) models.BacklogRule {
	return models.BacklogRule{Field: "tag", Operator: "has", Value: label}
}

func newSmartReachBoard(t *testing.T) (reachBoard, string) {
	t.Helper()
	ctx := context.Background()
	b := newReachBoard(t)
	for title, place := range map[string][2]string{emeaSprintTask: {"emea-board", "emea-area"}, apacSprintTask: {"apac-board", "apac-area"}} {
		if _, err := b.mgr.CreateTaskAs(ctx, "", mwanachamataskmanager.Task{Title: title, Backlog: place[0], Step: "to-do", Tags: []string{"Ready", sprintLabel, place[1]}}); err != nil {
			t.Fatal(err)
		}
	}
	made, err := b.mgr.CreateBacklogWithRules(ctx, "Sprint one", nil, rulesOf("all", labelHas(sprintLabel)))
	if err != nil {
		t.Fatal(err)
	}
	return b, made.ID
}

func listedCount(t *testing.T, mgr mwanachamataskmanager.TaskManager, person, backlogID string) (int, bool) {
	t.Helper()
	list, err := mgr.ListBacklogsFor(context.Background(), person)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list.Items {
		if item.ID == backlogID {
			return item.Issues, true
		}
	}
	return 0, false
}

func wantSameTitles(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(sorted(got), sorted(want)) {
		t.Fatalf("%s: got %v, want %v", what, sorted(got), sorted(want))
	}
}

func TestSmartReach_TwoGroupsReadDifferentCountsAndListsFromTheSameRules(t *testing.T) {
	b, smart := newSmartReachBoard(t)
	ctx := context.Background()
	for person, want := range smartExpectations {
		t.Run("as "+person, func(t *testing.T) {
			count, listed := listedCount(t, b.mgr, person, smart)
			if listed != (person != "none") || count != len(want) {
				t.Fatalf("backlogs list: listed %v with %d, want %d", listed, count, len(want))
			}
			counted, err := b.mgr.CountBacklogRules(ctx, person, rulesOf("all", labelHas(sprintLabel)))
			if err != nil || counted.Count != len(want) {
				t.Fatalf("count of the rules: %+v %v, want %d", counted, err, len(want))
			}
			tasks, err := b.mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Backlog: smart, RequesterID: person})
			if err != nil {
				t.Fatal(err)
			}
			titles := []string{}
			for _, task := range tasks {
				titles = append(titles, task.Title)
			}
			wantSameTitles(t, "list_tasks", titles, want)
		})
	}
}

func TestSmartReach_PendingAndFindOnASmartBacklogShowOnlyReachedIssues(t *testing.T) {
	b, smart := newSmartReachBoard(t)
	ctx := context.Background()
	for person, want := range smartExpectations {
		if person == "none" {
			continue
		}
		t.Run("as "+person, func(t *testing.T) {
			pending, err := b.mgr.PendingTasks(ctx, mwanachamataskmanager.PendingFilter{Backlog: smart, RequesterID: person})
			if err != nil {
				t.Fatal(err)
			}
			wantSameTitles(t, "pending", pendingTitles(pending.Items), want)
			found, err := b.mgr.FindTasks(ctx, mwanachamataskmanager.FindFilter{Q: "sprint", Backlog: smart, RequesterID: person})
			if err != nil {
				t.Fatal(err)
			}
			titles := []string{}
			for _, item := range found.Items {
				titles = append(titles, item.Title)
			}
			wantSameTitles(t, "find", titles, want)
		})
	}
}

func TestSmartReach_ATagRuleMatchesTheLabelAndNeverTheReachTagOfTheSameName(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	mgr.UseGroupTags(func(_ context.Context, id string) ([]string, error) { return []string{sprintLabel}, nil })
	if _, err := mgr.CreateBacklogAs(ctx, "sprinter", "Reached by sprint 1", []string{"To do"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateBacklogAs(ctx, "", "Untagged", []string{"To do"}, nil); err != nil {
		t.Fatal(err)
	}
	for _, task := range []mwanachamataskmanager.Task{
		{Title: "reached, no label", Backlog: "reached-by-sprint-1", Step: "to-do"},
		{Title: "reached and labelled", Backlog: "reached-by-sprint-1", Step: "to-do", Tags: []string{sprintLabel}},
		{Title: "labelled elsewhere", Backlog: "untagged", Step: "to-do", Tags: []string{sprintLabel}},
	} {
		mustCreate(t, mgr, task)
	}
	smart, err := mgr.CreateBacklogWithRules(ctx, "Sprint one", nil, rulesOf("all", labelHas(sprintLabel)))
	if err != nil {
		t.Fatal(err)
	}
	for person, want := range map[string][]string{"": {"reached and labelled", "labelled elsewhere"}, "sprinter": {"reached and labelled"}} {
		tasks, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Backlog: smart.ID, RequesterID: person})
		if err != nil {
			t.Fatal(err)
		}
		titles := []string{}
		for _, task := range tasks {
			titles = append(titles, task.Title)
		}
		wantSameTitles(t, "as "+person, titles, want)
	}
}

func TestSmartReach_ABacklogWriteAnswersWithTheWritersOwnCount(t *testing.T) {
	ctx := context.Background()
	writes := map[string]func(mgr mwanachamataskmanager.TaskManager, person, smart string) (models.BacklogSummary, error){
		"rename": func(mgr mwanachamataskmanager.TaskManager, person, smart string) (models.BacklogSummary, error) {
			return mgr.RenameBacklogAs(ctx, person, smart, "Sprint renamed")
		},
		"archive": func(mgr mwanachamataskmanager.TaskManager, person, smart string) (models.BacklogSummary, error) {
			return mgr.ArchiveBacklogAs(ctx, person, smart)
		},
		"unarchive": func(mgr mwanachamataskmanager.TaskManager, person, smart string) (models.BacklogSummary, error) {
			if _, err := mgr.ArchiveBacklog(ctx, smart); err != nil {
				return models.BacklogSummary{}, err
			}
			return mgr.UnarchiveBacklogAs(ctx, person, smart)
		},
		"create": func(mgr mwanachamataskmanager.TaskManager, person, _ string) (models.BacklogSummary, error) {
			return mgr.CreateBacklogAs(ctx, person, "Another sprint", nil, rulesOf("any", labelHas(sprintLabel)))
		},
	}
	for name, write := range writes {
		for person, want := range map[string]int{"emea": 1, "admin": 2} {
			t.Run(name+" as "+person, func(t *testing.T) {
				b, smart := newSmartReachBoard(t)
				b.mgr.UseGroups(func(context.Context, string) ([]string, error) { return []string{"owners"}, nil })
				b.mgr.UseGroupExists(knownGroups("owners"))
				if err := b.mgr.SetBacklogOwnerGroup(ctx, "owners"); err != nil {
					t.Fatal(err)
				}
				got, err := write(b.mgr, person, smart)
				if err != nil {
					t.Fatal(err)
				}
				if got.Issues != want {
					t.Fatalf("answered %d issues, want %d", got.Issues, want)
				}
			})
		}
	}
}

func TestSmartReach_RulesAndQuestionsOfASmartBacklogComeFromReachedTasksOnly(t *testing.T) {
	b, smart := newSmartReachBoard(t)
	ctx := context.Background()
	for _, area := range []string{"emea-area", "apac-area"} {
		for kind, role := range map[string]string{"rule": "", "decision": "", "question": "owner"} {
			if _, err := b.mgr.CreateRule(ctx, "", mwanachamataskmanager.RuleRecord{Kind: kind, TargetRole: role, Title: area + " " + kind, Tags: []string{area}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	lists := map[string]func(context.Context, string, string, mwanachamataskmanager.RuleFilter) ([]mwanachamataskmanager.RuleRecord, error){
		"rule":     b.mgr.ListBacklogRules,
		"decision": b.mgr.ListBacklogDecisions,
		"question": b.mgr.ListBacklogQuestions,
	}
	for person, areas := range map[string][]string{"emea": {"emea-area"}, "apac": {"apac-area"}, "admin": {"apac-area", "emea-area"}} {
		for kind, list := range lists {
			t.Run(kind+" as "+person, func(t *testing.T) {
				records, err := list(ctx, person, smart, mwanachamataskmanager.RuleFilter{})
				if err != nil {
					t.Fatal(err)
				}
				got, want := []string{}, []string{}
				for _, record := range records {
					got = append(got, record.Title)
				}
				for _, area := range areas {
					want = append(want, area+" "+kind)
				}
				wantSameTitles(t, "backlog "+kind+" list", got, want)
			})
		}
	}
}
