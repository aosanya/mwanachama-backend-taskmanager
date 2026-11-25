package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func groupTagsOfPeople() mwanachamataskmanager.GroupTags {
	byPerson := map[string][]string{
		"owner": {"all"},
		"dev":   {"emea"},
		"both":  {"emea", "apac"},
		"none":  nil,
	}
	return func(_ context.Context, id string) ([]string, error) { return byPerson[id], nil }
}

func taggingManager(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	mgr := restrictedManager(t)
	mgr.UseGroupTags(groupTagsOfPeople())
	return mgr
}

func backlogSummaryOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) models.BacklogSummary {
	t.Helper()
	list, err := mgr.ListBacklogs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list.Items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("no backlog %q", id)
	return models.BacklogSummary{}
}

func TestBacklogTags_ABacklogTakesTheAssumedGroupsTagsOfItsMaker(t *testing.T) {
	ctx := context.Background()
	mgr := taggingManager(t)
	made, err := mgr.CreateBacklogAs(ctx, "dev", "Sprint 1", []string{"Open"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(made.Tags, []string{"emea"}) {
		t.Fatalf("tags %v, want [emea]", made.Tags)
	}
	if got := backlogSummaryOf(t, mgr, made.ID); !reflect.DeepEqual(got.Tags, []string{"emea"}) {
		t.Fatalf("stored tags %v, want [emea]", got.Tags)
	}
}

func TestBacklogTags_ABacklogMadeWithNoPersonCarriesNone(t *testing.T) {
	mgr := taggingManager(t)
	made, err := mgr.CreateBacklogAs(context.Background(), "", "Hub made", []string{"Open"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(made.Tags) != 0 {
		t.Fatalf("tags %v, want none", made.Tags)
	}
}

func TestBacklogTags_ASmartBacklogCarriesNone(t *testing.T) {
	mgr := taggingManager(t)
	made, err := mgr.CreateBacklogAs(context.Background(), "dev", "Imports", nil, rulesOf("all", titleHas("Import")))
	if err != nil {
		t.Fatal(err)
	}
	if len(made.Tags) != 0 {
		t.Fatalf("smart backlog tags %v, want none", made.Tags)
	}
}

func TestBacklogTags_AnEditorAddsATagOfTheirAssumedGroup(t *testing.T) {
	ctx := context.Background()
	mgr := taggingManager(t)
	made, err := mgr.CreateBacklogAs(ctx, "dev", "Sprint 1", []string{"Open"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, made.ID, docsGroup, "edit"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, made.ID, devGroup, "edit"); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.AddBacklogTagAs(ctx, "both", made.ID, "apac")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Tags, []string{"emea", "apac"}) {
		t.Fatalf("tags %v, want [emea apac]", got.Tags)
	}
	again, err := mgr.AddBacklogTagAs(ctx, "both", made.ID, "apac")
	if err != nil || !reflect.DeepEqual(again.Tags, []string{"emea", "apac"}) {
		t.Fatalf("adding twice: %v %v", again.Tags, err)
	}
}

func TestBacklogTags_ThePersonWhoCannotEditIsRefused(t *testing.T) {
	ctx := context.Background()
	mgr := taggingManager(t)
	made, err := mgr.CreateBacklogAs(ctx, "owner", "Sprint 1", []string{"Open"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, made.ID, devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	_, err = mgr.AddBacklogTagAs(ctx, "dev", made.ID, "emea")
	wantForbidden(t, "add by a mover", err)
}

func TestBacklogTags_ATagTheAssumedGroupLacksIsRefused(t *testing.T) {
	ctx := context.Background()
	mgr := taggingManager(t)
	made, err := mgr.CreateBacklogAs(ctx, "dev", "Sprint 1", []string{"Open"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, made.ID, devGroup, "edit"); err != nil {
		t.Fatal(err)
	}
	_, err = mgr.AddBacklogTagAs(ctx, "dev", made.ID, "apac")
	if !errors.Is(err, mwanachamataskmanager.ErrTagNotOfGroup) {
		t.Fatalf("got %v, want ErrTagNotOfGroup", err)
	}
	if got := backlogSummaryOf(t, mgr, made.ID); !reflect.DeepEqual(got.Tags, []string{"emea"}) {
		t.Fatalf("tags changed to %v", got.Tags)
	}
}

func TestBacklogTags_ASmartBacklogTakesNoTag(t *testing.T) {
	ctx := context.Background()
	mgr := taggingManager(t)
	made, err := mgr.CreateBacklogAs(ctx, "owner", "Imports", nil, rulesOf("all", titleHas("Import")))
	if err != nil {
		t.Fatal(err)
	}
	_, err = mgr.AddBacklogTagAs(ctx, "owner", made.ID, "all")
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("got %v, want ErrInvalidBacklog", err)
	}
}

func TestBacklogTags_AReplaceCannotTakeATagOff(t *testing.T) {
	ctx := context.Background()
	mgr := taggingManager(t)
	made, err := mgr.CreateBacklogAs(ctx, "dev", "Sprint 1", []string{"Open"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	list, err := mgr.ListBacklogs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var replacement []models.Backlog
	for _, item := range list.Items {
		b := models.Backlog{ID: item.ID, Name: item.Name, Kind: item.Kind, Rules: item.Rules}
		for _, s := range item.Steps {
			b.Steps = append(b.Steps, models.BacklogStep{ID: s.ID, Name: s.Name})
		}
		replacement = append(replacement, b)
	}
	if _, err := mgr.ReplaceBacklogs(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	if got := backlogSummaryOf(t, mgr, made.ID); !reflect.DeepEqual(got.Tags, []string{"emea"}) {
		t.Fatalf("tags after replace %v, want [emea]", got.Tags)
	}
}

func TestBacklogTags_NoOperationTakesATagOff(t *testing.T) {
	raw, err := os.ReadFile("taskmanager.operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Operations map[string]struct {
			Method string `json:"method"`
			Path   string `json:"path"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	adds := 0
	for name, op := range doc.Operations {
		if !strings.HasPrefix(op.Path, "/backlogs/{backlogID}/tags") {
			continue
		}
		if op.Method != "POST" {
			t.Fatalf("%s is %s %s: only an add exists", name, op.Method, op.Path)
		}
		adds++
	}
	if adds != 1 {
		t.Fatalf("%d add routes, want 1", adds)
	}
}

func TestBacklogTags_ALabelPatchLeavesTheBacklogTagsAlone(t *testing.T) {
	ctx := context.Background()
	mgr := taggingManager(t)
	made, err := mgr.CreateBacklogAs(ctx, "dev", "Sprint 1", []string{"Open"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "t", Backlog: made.ID, Step: "open"})
	labels := []string{"emea", "apac", "all"}
	if _, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: task.ID, Tags: &labels}); err != nil {
		t.Fatal(err)
	}
	if got := backlogSummaryOf(t, mgr, made.ID); !reflect.DeepEqual(got.Tags, []string{"emea"}) {
		t.Fatalf("backlog tags %v, want [emea]", got.Tags)
	}
}
