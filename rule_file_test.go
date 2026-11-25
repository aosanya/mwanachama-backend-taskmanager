package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type fixedPathChecker struct {
	exists bool
	err    error
	asked  []string
}

func (c *fixedPathChecker) PathExists(_ context.Context, repo, path string) (bool, error) {
	c.asked = append(c.asked, repo+":"+path)
	return c.exists, c.err
}

func TestRuleFiles_ALinkReadsBackAndIsRemoved(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "Routes stay thin")

	added, err := mgr.AddRuleFile(ctx, "owner", rule.Code, "mwanachama-backend-taskmanager", "/rule.go", 58, "How a record is made")
	if err != nil {
		t.Fatal(err)
	}
	if added.File.Path != "rule.go" || added.File.Line != 58 || added.File.Note != "How a record is made" || added.File.Repo != "mwanachama-backend-taskmanager" {
		t.Fatalf("added %+v", added.File)
	}
	if added.Warning != "" {
		t.Fatalf("warning without a checker: %q", added.Warning)
	}
	files, err := mgr.ListRuleFiles(ctx, "owner", rule.Code)
	if err != nil || len(files) != 1 || files[0].ID != added.File.ID {
		t.Fatalf("list %+v %v", files, err)
	}
	if err := mgr.RemoveRuleFile(ctx, "owner", rule.Code, added.File.ID); err != nil {
		t.Fatal(err)
	}
	files, _ = mgr.ListRuleFiles(ctx, "owner", rule.Code)
	if len(files) != 0 {
		t.Fatalf("after remove %+v", files)
	}
	if err := mgr.RemoveRuleFile(ctx, "owner", rule.Code, added.File.ID); !errors.Is(err, mwanachamataskmanager.ErrRuleFileNotFound) {
		t.Fatalf("second remove: %v", err)
	}
}

func TestRuleFiles_TheSameFileTwiceIsRefusedButAnotherLineOfItIsNot(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "Routes stay thin")

	if _, err := mgr.AddRuleFile(ctx, "owner", rule.Code, "repo", "a/b.go", 0, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AddRuleFile(ctx, "owner", rule.Code, "repo", "a//b.go", 0, "again"); !errors.Is(err, mwanachamataskmanager.ErrDuplicateRuleFile) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := mgr.AddRuleFile(ctx, "owner", rule.Code, "repo", "a/b.go", 12, ""); err != nil {
		t.Fatalf("another line: %v", err)
	}
	other := mustCreateRule(t, mgr, "rule", "Another rule")
	if _, err := mgr.AddRuleFile(ctx, "owner", other.Code, "repo", "a/b.go", 0, ""); err != nil {
		t.Fatalf("another rule: %v", err)
	}
}

func TestRuleFiles_AnInvalidLinkIsRefused(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "Routes stay thin")

	cases := []struct {
		name, repo, path string
		line             int
	}{
		{"no repo", "", "a.go", 0},
		{"no path", "repo", " / ", 0},
		{"path leaves the repo", "repo", "../a.go", 0},
		{"negative line", "repo", "a.go", -1},
	}
	for _, c := range cases {
		if _, err := mgr.AddRuleFile(ctx, "owner", rule.Code, c.repo, c.path, c.line, ""); !errors.Is(err, mwanachamataskmanager.ErrInvalidRuleFile) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if _, err := mgr.AddRuleFile(ctx, "owner", "R-999", "repo", "a.go", 0, ""); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Errorf("unknown rule: %v", err)
	}
}

func TestRuleFiles_AMissingPathWarnsAndStillSaves(t *testing.T) {
	ctx := context.Background()
	checker := &fixedPathChecker{exists: false}
	mgr := mwanachamataskmanager.WithRepoPathChecker(newTestManager(t), checker)
	rule := mustCreateRule(t, mgr, "rule", "Routes stay thin")

	added, err := mgr.AddRuleFile(ctx, "owner", rule.Code, "repo", "gone.go", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if added.Warning == "" {
		t.Fatal("a missing path gave no warning")
	}
	if len(checker.asked) != 1 || checker.asked[0] != "repo:gone.go" {
		t.Fatalf("asked %v", checker.asked)
	}
	files, _ := mgr.ListRuleFiles(ctx, "owner", rule.Code)
	if len(files) != 1 {
		t.Fatalf("a warned link is still saved, got %d", len(files))
	}
}

func TestRuleFiles_AnExistingPathHasNoWarningAndACheckerFailureWarns(t *testing.T) {
	ctx := context.Background()
	checker := &fixedPathChecker{exists: true}
	mgr := mwanachamataskmanager.WithRepoPathChecker(newTestManager(t), checker)
	rule := mustCreateRule(t, mgr, "rule", "Routes stay thin")

	added, err := mgr.AddRuleFile(ctx, "owner", rule.Code, "repo", "here.go", 0, "")
	if err != nil || added.Warning != "" {
		t.Fatalf("existing: %+v %v", added, err)
	}
	checker.err = errors.New("custodian unreachable")
	added, err = mgr.AddRuleFile(ctx, "owner", rule.Code, "repo", "there.go", 0, "")
	if err != nil || added.Warning == "" {
		t.Fatalf("checker failure: %+v %v", added, err)
	}
}

func TestRuleFiles_TheListFlagsAPathTheCheckerSaysIsGoneAndNoOtherWithoutACheckerNoFlag(t *testing.T) {
	ctx := context.Background()
	checker := &fixedPathChecker{exists: false}
	mgr := mwanachamataskmanager.WithRepoPathChecker(newTestManager(t), checker)
	rule := mustCreateRule(t, mgr, "rule", "Routes stay thin")
	if _, err := mgr.AddRuleFile(ctx, "owner", rule.Code, "repo", "gone.go", 0, ""); err != nil {
		t.Fatal(err)
	}
	files, err := mgr.ListRuleFiles(ctx, "owner", rule.Code)
	if err != nil || len(files) != 1 || !files[0].Missing {
		t.Fatalf("gone: %+v %v", files, err)
	}
	checker.exists = true
	files, _ = mgr.ListRuleFiles(ctx, "owner", rule.Code)
	if files[0].Missing {
		t.Fatalf("present path flagged: %+v", files)
	}
	checker.exists, checker.err = false, errors.New("custodian unreachable")
	files, _ = mgr.ListRuleFiles(ctx, "owner", rule.Code)
	if files[0].Missing {
		t.Fatalf("an unanswered check flagged the path: %+v", files)
	}

	plain := newTestManager(t)
	other := mustCreateRule(t, plain, "rule", "Routes stay thin")
	if _, err := plain.AddRuleFile(ctx, "owner", other.Code, "repo", "gone.go", 0, ""); err != nil {
		t.Fatal(err)
	}
	files, _ = plain.ListRuleFiles(ctx, "owner", other.Code)
	if files[0].Missing {
		t.Fatalf("flag without a checker: %+v", files)
	}
}
