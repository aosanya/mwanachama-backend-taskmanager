package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestRules_SupersedeKeepsTheOldRecordAndPointsBothWays(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	old := mustCreateRule(t, mgr, "rule", "A comment says why")
	mustSetStatus(t, mgr, old.Code, "in_force")

	next, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "No comments in code", Tags: []string{"code"}})
	if err != nil {
		t.Fatal(err)
	}
	if next.Code != "R-2" || next.Kind != "rule" || next.Status != "in_force" || next.Supersedes != old.Code {
		t.Fatalf("replacement %+v", next)
	}
	was, err := mgr.GetRuleByCode(ctx, "owner", old.Code)
	if err != nil || was.Status != "superseded" || was.SupersededBy != next.Code || was.Title != "A comment says why" {
		t.Fatalf("old %+v, err %v", was, err)
	}
}

func TestRules_ADecisionIsSupersededIntoDecided(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	old := mustCreateRule(t, mgr, "decision", "Old choice")
	mustSetStatus(t, mgr, old.Code, "decided")
	next, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "New choice"})
	if err != nil || next.Code != "D-2" || next.Status != "decided" {
		t.Fatalf("got %+v, err %v", next, err)
	}
}

func TestRules_OnlyAnActiveRecordCanBeSuperseded(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	proposed := mustCreateRule(t, mgr, "rule", "p")
	if _, err := mgr.SupersedeRule(ctx, "owner", proposed.Code, mwanachamataskmanager.RuleRecord{Title: "x"}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("proposed: err %v, want ErrInvalidRule", err)
	}
	active := mustCreateRule(t, mgr, "rule", "a")
	mustSetStatus(t, mgr, active.Code, "in_force")
	if _, err := mgr.SupersedeRule(ctx, "owner", active.Code, mwanachamataskmanager.RuleRecord{Title: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.SupersedeRule(ctx, "owner", active.Code, mwanachamataskmanager.RuleRecord{Title: "c"}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("twice: err %v, want ErrInvalidRule", err)
	}
	if _, err := mgr.SupersedeRule(ctx, "owner", "R-99", mwanachamataskmanager.RuleRecord{Title: "x"}); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Fatalf("unknown: err %v, want ErrRuleNotFound", err)
	}
}

func TestRules_ACodeIsNeverGivenAgainAfterARetirement(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	first := mustCreateRule(t, mgr, "rule", "a")
	mustSetStatus(t, mgr, first.Code, "in_force")
	mustSetStatus(t, mgr, first.Code, "retired")
	second := mustCreateRule(t, mgr, "rule", "b")
	if second.Code == first.Code {
		t.Fatalf("code %s given twice", second.Code)
	}
	_ = ctx
}

func mustSetStatus(t *testing.T, mgr mwanachamataskmanager.TaskManager, code, status string) {
	t.Helper()
	if _, err := mgr.SetRuleStatus(context.Background(), "owner", code, status); err != nil {
		t.Fatalf("%s to %s: %v", code, status, err)
	}
}

func codes(rules []mwanachamataskmanager.RuleRecord) string {
	var out []string
	for _, r := range rules {
		out = append(out, r.Code)
	}
	return strings.Join(out, ",")
}

func TestRules_TwoCreatesAtOnceNeverShareACode(t *testing.T) {
	ctx := context.Background()
	mgr := newFileBackedTestManager(t)
	const n = 8
	made := make(chan string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "x"})
			if err == nil {
				made <- r.Code
			}
		}()
	}
	wg.Wait()
	close(made)
	seen := map[string]bool{}
	for code := range made {
		if seen[code] {
			t.Fatalf("code %s given twice", code)
		}
		seen[code] = true
	}
	if len(seen) == 0 {
		t.Fatal("no create succeeded")
	}
}

func newFileBackedTestManager(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/rules.db?_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	workSpec := specForInstance(t, "spec/examples/agency.taskmanager.json", "test")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatal(err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}
