package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestTaskReferences_AnIssueStartsWithNone(t *testing.T) {
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	refs, err := mgr.ListTaskReferences(context.Background(), "", task.ID)
	if err != nil || len(refs) != 0 {
		t.Fatalf("got %d references, err %v, want none", len(refs), err)
	}
}

func TestTaskReferences_AddingAFileKeepsRepoPathWriterAndTimeAndIsConfirmed(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	ref, err := mgr.AddTaskReference(ctx, "owner", task.ID, "file", "hub-steward-swe", "loader/importer.go")
	if err != nil {
		t.Fatal(err)
	}
	if ref.ID == "" || ref.TaskID != task.ID || ref.Kind != "file" || ref.Repo != "hub-steward-swe" || ref.Target != "loader/importer.go" ||
		ref.Source != "person" || !ref.Confirmed || ref.AddedBy != "owner" || ref.CreatedAt == "" {
		t.Fatalf("got %+v", ref)
	}
	refs, err := mgr.ListTaskReferences(ctx, "owner", task.ID)
	if err != nil || len(refs) != 1 || refs[0].ID != ref.ID {
		t.Fatalf("got %+v, err %v", refs, err)
	}
}

func TestTaskReferences_EveryKindIsAcceptedAndAPathIsKeptOnTheMainBranchRoot(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	for _, c := range []struct{ kind, repo, target, want string }{
		{"file", "hub-steward", "/code/app/main.go", "code/app/main.go"},
		{"folder", "hub-steward", "code/app/", "code/app"},
		{"issue", "", "HS-005", "HS-005"},
		{"screen", "", "agency_list_mounts", "agency_list_mounts"},
		{"link", "", "https://example.test/spec", "https://example.test/spec"},
	} {
		ref, err := mgr.AddTaskReference(ctx, "owner", task.ID, c.kind, c.repo, c.target)
		if err != nil || ref.Target != c.want {
			t.Fatalf("%s: got %+v, err %v", c.kind, ref, err)
		}
	}
	refs, err := mgr.ListTaskReferences(ctx, "owner", task.ID)
	if err != nil || len(refs) != 5 {
		t.Fatalf("got %d, err %v", len(refs), err)
	}
	for i, want := range []string{"file", "folder", "issue", "screen", "link"} {
		if refs[i].Kind != want {
			t.Fatalf("reference %d is %q, want %q (oldest first)", i, refs[i].Kind, want)
		}
	}
}

func TestTaskReferences_AnEntryNeedsAKnownKindATargetAndForRepoKindsARepo(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	for name, c := range map[string]struct{ kind, repo, target string }{
		"unknown kind":      {"video", "", "x"},
		"blank target":      {"file", "hub-steward", "   "},
		"file without repo": {"file", "", "a.go"},
		"folder no repo":    {"folder", "", "src"},
		"path leaves repo":  {"file", "hub-steward", "../secret"},
		"root path":         {"file", "hub-steward", "/"},
		"link not a url":    {"link", "", "example.test"},
		"link bad scheme":   {"link", "", "javascript:alert(1)"},
	} {
		if _, err := mgr.AddTaskReference(ctx, "owner", task.ID, c.kind, c.repo, c.target); !errors.Is(err, mwanachamataskmanager.ErrInvalidTaskReference) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := mgr.AddTaskReference(ctx, "owner", "no-such-issue", "file", "r", "a.go"); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("unknown issue: %v", err)
	}
}

func TestTaskReferences_TheSameEntryTwiceIsRefusedButAnotherIssueMayHaveIt(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	other := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "b"})

	if _, err := mgr.AddTaskReference(ctx, "owner", task.ID, "file", "hub-steward", "a.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AddTaskReference(ctx, "owner", task.ID, "file", "hub-steward", "./a.go"); !errors.Is(err, mwanachamataskmanager.ErrDuplicateTaskReference) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := mgr.AddTaskReference(ctx, "owner", task.ID, "file", "hub-steward-swe", "a.go"); err != nil {
		t.Fatalf("same path in another repo: %v", err)
	}
	if _, err := mgr.AddTaskReference(ctx, "owner", other.ID, "file", "hub-steward", "a.go"); err != nil {
		t.Fatalf("another issue: %v", err)
	}
}

func TestTaskReferences_RemovingDropsOneEntryAndNoOtherAndAMissingOneIsNotFound(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	other := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "b"})

	keep, _ := mgr.AddTaskReference(ctx, "owner", task.ID, "file", "r", "keep.go")
	drop, _ := mgr.AddTaskReference(ctx, "owner", task.ID, "file", "r", "drop.go")
	elsewhere, _ := mgr.AddTaskReference(ctx, "owner", other.ID, "file", "r", "drop.go")

	if err := mgr.RemoveTaskReference(ctx, "owner", task.ID, drop.ID); err != nil {
		t.Fatal(err)
	}
	refs, err := mgr.ListTaskReferences(ctx, "owner", task.ID)
	if err != nil || len(refs) != 1 || refs[0].ID != keep.ID {
		t.Fatalf("got %+v, err %v", refs, err)
	}
	if err := mgr.RemoveTaskReference(ctx, "owner", task.ID, drop.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskReferenceNotFound) {
		t.Fatalf("remove twice: %v", err)
	}
	if err := mgr.RemoveTaskReference(ctx, "owner", task.ID, elsewhere.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskReferenceNotFound) {
		t.Fatalf("remove another issue's entry: %v", err)
	}
	if _, err := mgr.AddTaskReference(ctx, "owner", task.ID, "file", "r", "drop.go"); err != nil {
		t.Fatalf("add again after removing: %v", err)
	}
}

func TestTaskReferences_ACanSeeReaderMayAddAndRemoveButAnUnseenIssueIsNotFound(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	task := softwareTask(t, mgr)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see"); err != nil {
		t.Fatal(err)
	}

	ref, err := mgr.AddTaskReference(ctx, "dev", task.ID, "file", "hub-steward", "a.go")
	if err != nil {
		t.Fatalf("a Can see reader adds: %v", err)
	}
	if err := mgr.RemoveTaskReference(ctx, "dev", task.ID, ref.ID); err != nil {
		t.Fatalf("a Can see reader removes: %v", err)
	}
	if _, err := mgr.AddTaskReference(ctx, "none", task.ID, "file", "hub-steward", "a.go"); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("an unseen issue adds: %v", err)
	}
	if _, err := mgr.ListTaskReferences(ctx, "none", task.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("an unseen issue lists: %v", err)
	}
	if err := mgr.RemoveTaskReference(ctx, "none", task.ID, ref.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("an unseen issue removes: %v", err)
	}
}

func TestTaskReferences_AnAssistantProposalIsSavedUnconfirmedAndMarkedAsTheAssistants(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	ref, err := mgr.ProposeTaskReference(ctx, "assistant", task.ID, "file", "hub-steward", "./loader/importer.go")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Source != "assistant" || ref.Confirmed || ref.Target != "loader/importer.go" || ref.AddedBy != "assistant" || ref.CreatedAt == "" {
		t.Fatalf("got %+v", ref)
	}
	refs, err := mgr.ListTaskReferences(ctx, "owner", task.ID)
	if err != nil || len(refs) != 1 || refs[0].Confirmed || refs[0].Source != "assistant" {
		t.Fatalf("got %+v, err %v", refs, err)
	}
}

func TestTaskReferences_ProposingWhatTheIssueAlreadyHasIsRefusedAndABadProposalIsInvalid(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	if _, err := mgr.AddTaskReference(ctx, "owner", task.ID, "file", "hub-steward", "a.go"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ProposeTaskReference(ctx, "assistant", task.ID, "file", "hub-steward", "a.go"); !errors.Is(err, mwanachamataskmanager.ErrDuplicateTaskReference) {
		t.Fatalf("proposing what a person added: %v", err)
	}
	if _, err := mgr.ProposeTaskReference(ctx, "assistant", task.ID, "file", "", "a.go"); !errors.Is(err, mwanachamataskmanager.ErrInvalidTaskReference) {
		t.Fatalf("no repo: %v", err)
	}
	if _, err := mgr.ProposeTaskReference(ctx, "assistant", "no-such-issue", "link", "", "https://example.test"); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("unknown issue: %v", err)
	}
}

func TestTaskReferences_ConfirmingKeepsTheAssistantAsSourceAndIsRepeatable(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	other := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "b"})
	proposed, _ := mgr.ProposeTaskReference(ctx, "assistant", task.ID, "folder", "hub-steward", "code/app")
	elsewhere, _ := mgr.ProposeTaskReference(ctx, "assistant", other.ID, "folder", "hub-steward", "code/app")

	ref, err := mgr.ConfirmTaskReference(ctx, "owner", task.ID, proposed.ID)
	if err != nil || !ref.Confirmed || ref.Source != "assistant" || ref.AddedBy != "assistant" {
		t.Fatalf("got %+v, err %v", ref, err)
	}
	if again, err := mgr.ConfirmTaskReference(ctx, "owner", task.ID, proposed.ID); err != nil || !again.Confirmed {
		t.Fatalf("confirm again: %+v, %v", again, err)
	}
	refs, _ := mgr.ListTaskReferences(ctx, "owner", task.ID)
	if len(refs) != 1 || !refs[0].Confirmed {
		t.Fatalf("got %+v", refs)
	}
	others, _ := mgr.ListTaskReferences(ctx, "owner", other.ID)
	if len(others) != 1 || others[0].Confirmed {
		t.Fatalf("another issue's proposal was confirmed: %+v", others)
	}
	if _, err := mgr.ConfirmTaskReference(ctx, "owner", task.ID, elsewhere.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskReferenceNotFound) {
		t.Fatalf("confirm another issue's entry: %v", err)
	}
	if _, err := mgr.ConfirmTaskReference(ctx, "owner", task.ID, "nothing"); !errors.Is(err, mwanachamataskmanager.ErrTaskReferenceNotFound) {
		t.Fatalf("confirm missing: %v", err)
	}
}

func TestTaskReferences_ACanSeeReaderMayProposeAndConfirmButAnUnseenIssueIsNotFound(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	task := softwareTask(t, mgr)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see"); err != nil {
		t.Fatal(err)
	}

	ref, err := mgr.ProposeTaskReference(ctx, "dev", task.ID, "link", "", "https://example.test/spec")
	if err != nil {
		t.Fatalf("a Can see reader proposes: %v", err)
	}
	if _, err := mgr.ConfirmTaskReference(ctx, "dev", task.ID, ref.ID); err != nil {
		t.Fatalf("a Can see reader confirms: %v", err)
	}
	if _, err := mgr.ProposeTaskReference(ctx, "none", task.ID, "link", "", "https://example.test/other"); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("an unseen issue proposes: %v", err)
	}
	if _, err := mgr.ConfirmTaskReference(ctx, "none", task.ID, ref.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("an unseen issue confirms: %v", err)
	}
}
