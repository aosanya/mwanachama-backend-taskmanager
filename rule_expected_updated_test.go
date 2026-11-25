package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestRuleExpectedUpdatedAt_ACreateReadUpdateRoundTripAcceptsTheStampJustRead(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := mustCreateRule(t, mgr, "rule", "No comments")
	read := mustReadRule(t, mgr, made.Code)
	title := "No comments in code"

	updated, err := mgr.UpdateRule(ctx, "owner", made.Code, mwanachamataskmanager.RulePatch{Title: &title, ExpectedUpdatedAt: &read.UpdatedAt})
	if err != nil || updated.Title != title {
		t.Fatalf("got %+v, %v", updated, err)
	}
	if again := mustReadRule(t, mgr, made.Code); again.Title != title || again.UpdatedAt != updated.UpdatedAt {
		t.Fatalf("stored %+v, returned %+v", again, updated)
	}
}

func TestRuleExpectedUpdatedAt_ASecondUpdateFromTheSameReadIsRefusedAndWritesNothing(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := mustCreateRule(t, mgr, "rule", "No comments")
	read := mustReadRule(t, mgr, made.Code)
	first, second := "first", "second"

	if _, err := mgr.UpdateRule(ctx, "owner", made.Code, mwanachamataskmanager.RulePatch{Title: &first, ExpectedUpdatedAt: &read.UpdatedAt}); err != nil {
		t.Fatal(err)
	}
	_, err := mgr.UpdateRule(ctx, "owner", made.Code, mwanachamataskmanager.RulePatch{Title: &second, ExpectedUpdatedAt: &read.UpdatedAt})
	if !errors.Is(err, mwanachamataskmanager.ErrRuleStale) {
		t.Fatalf("err %v, want ErrRuleStale", err)
	}
	if kept := mustReadRule(t, mgr, made.Code); kept.Title != first {
		t.Fatalf("title %q, want %q", kept.Title, first)
	}
}

func TestRuleExpectedUpdatedAt_ARefusalIsNotABadInputError(t *testing.T) {
	if errors.Is(mwanachamataskmanager.ErrRuleStale, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatal("ErrRuleStale must not wrap ErrInvalidRule")
	}
}

func TestRuleExpectedUpdatedAt_AnUpdateWithoutItStillSucceedsAfterAnotherUpdate(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := mustCreateRule(t, mgr, "rule", "No comments")
	first, second := "first", "second"
	for _, title := range []*string{&first, &second} {
		if _, err := mgr.UpdateRule(ctx, "owner", made.Code, mwanachamataskmanager.RulePatch{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	if kept := mustReadRule(t, mgr, made.Code); kept.Title != second {
		t.Fatalf("title %q", kept.Title)
	}
}

func TestRuleExpectedUpdatedAt_EveryWriteMovesTheStampEvenWithinOneSecond(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := mustCreateRule(t, mgr, "rule", "No comments")
	previous := made.UpdatedAt
	for _, title := range []string{"a", "b", "c"} {
		title := title
		updated, err := mgr.UpdateRule(ctx, "owner", made.Code, mwanachamataskmanager.RulePatch{Title: &title})
		if err != nil {
			t.Fatal(err)
		}
		if updated.UpdatedAt <= previous {
			t.Fatalf("updated_at %q did not move past %q", updated.UpdatedAt, previous)
		}
		previous = updated.UpdatedAt
	}
}

func TestRuleExpectedUpdatedAt_AStatusChangeMovesTheStampSoAnEarlierReadIsStale(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := mustCreateRule(t, mgr, "rule", "No comments")
	read := mustReadRule(t, mgr, made.Code)
	mustSetStatus(t, mgr, made.Code, "in_force")
	title := "late"
	if _, err := mgr.UpdateRule(ctx, "owner", made.Code, mwanachamataskmanager.RulePatch{Title: &title, ExpectedUpdatedAt: &read.UpdatedAt}); !errors.Is(err, mwanachamataskmanager.ErrRuleStale) {
		t.Fatalf("err %v, want ErrRuleStale", err)
	}
}

func TestRuleExpectedUpdatedAt_ABlankValueIsRefusedAsBadInput(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := mustCreateRule(t, mgr, "rule", "No comments")
	blank, title := "  ", "x"
	if _, err := mgr.UpdateRule(ctx, "owner", made.Code, mwanachamataskmanager.RulePatch{Title: &title, ExpectedUpdatedAt: &blank}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("err %v, want ErrInvalidRule", err)
	}
}
