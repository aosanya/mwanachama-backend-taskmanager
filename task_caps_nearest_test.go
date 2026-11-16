package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func usageOf(input, output, seconds int64) mwanachamataskmanager.TaskUsageWrite {
	return mwanachamataskmanager.TaskUsageWrite{InputTokens: input, OutputTokens: output, SecondsUsed: seconds}
}

func nearestOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, filter mwanachamataskmanager.CapsNearestFilter) map[string]mwanachamataskmanager.CapsNearest {
	t.Helper()
	got, err := mgr.NearestCaps(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	byTask := map[string]mwanachamataskmanager.CapsNearest{}
	for _, item := range got.Items {
		byTask[item.TaskID] = item
	}
	return byTask
}

func TestNearestCaps_EachIssueNamesTheCapItIsClosestToWithItsRatio(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	tokens := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "tokens"})
	clock := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "clock"})
	for id, used := range map[string]mwanachamataskmanager.TaskUsageWrite{
		tokens.ID: usageOf(400000, 10000, 30),
		clock.ID:  usageOf(1000, 100, 720),
	} {
		if _, err := mgr.WriteTaskUsage(ctx, "", id, used); err != nil {
			t.Fatal(err)
		}
	}

	got := nearestOf(t, mgr, mwanachamataskmanager.CapsNearestFilter{TaskIDs: []string{tokens.ID, clock.ID}})

	a := got[tokens.ID].Nearest
	if a == nil || a.Key != "input_tokens" || a.Used != 400000 || a.Cap != 500000 || a.Ratio != 0.8 {
		t.Fatalf("tokens: %+v", a)
	}
	b := got[clock.ID].Nearest
	if b == nil || b.Key != "runtime_seconds" || b.Used != 720 || b.Cap != 900 || b.Ratio != 0.8 {
		t.Fatalf("clock: %+v", b)
	}
}

func TestNearestCaps_AnIssueNoWorkerHasRunCarriesItsCapsAndNoNearest(t *testing.T) {
	mgr := newTestManager(t)
	idle := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "idle"})

	item, found := nearestOf(t, mgr, mwanachamataskmanager.CapsNearestFilter{TaskIDs: []string{idle.ID}})[idle.ID]

	if !found || item.Nearest != nil || item.InForce != capsOf(500000, 100000, 900) {
		t.Fatalf("got %+v, found %v", item, found)
	}
}

func TestNearestCaps_AnIssueOwnNumbersReplaceTheDefaultForTheRatio(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	own := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "own"})
	if _, err := mgr.SetTaskCaps(ctx, "", own.ID, numbers(2000, 500, 60)); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.WriteTaskUsage(ctx, "", own.ID, usageOf(1000, 400, 6)); err != nil {
		t.Fatal(err)
	}

	item := nearestOf(t, mgr, mwanachamataskmanager.CapsNearestFilter{TaskIDs: []string{own.ID}})[own.ID]

	if item.InForce != capsOf(2000, 500, 60) || item.Nearest == nil || item.Nearest.Key != "output_tokens" || item.Nearest.Cap != 500 || item.Nearest.Ratio != 0.8 {
		t.Fatalf("got %+v", item)
	}
}

func TestNearestCaps_APastTheCapRatioIsAboveOne(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	over := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "over"})
	if _, err := mgr.WriteTaskUsage(ctx, "", over.ID, usageOf(1, 1, 1800)); err != nil {
		t.Fatal(err)
	}

	item := nearestOf(t, mgr, mwanachamataskmanager.CapsNearestFilter{TaskIDs: []string{over.ID}})[over.ID]

	if item.Nearest == nil || item.Nearest.Key != "runtime_seconds" || item.Nearest.Ratio != 2 {
		t.Fatalf("got %+v", item)
	}
}

func TestNearestCaps_ABacklogNamesEveryIssueInItAndAnUnknownIdIsLeftOut(t *testing.T) {
	mgr := newTestManager(t)
	a := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	b := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "b"})

	byBacklog := nearestOf(t, mgr, mwanachamataskmanager.CapsNearestFilter{Backlog: "backlog"})
	if _, ok := byBacklog[a.ID]; !ok {
		t.Fatalf("backlog read missed %s: %+v", a.ID, byBacklog)
	}
	if _, ok := byBacklog[b.ID]; !ok {
		t.Fatalf("backlog read missed %s: %+v", b.ID, byBacklog)
	}

	byIDs := nearestOf(t, mgr, mwanachamataskmanager.CapsNearestFilter{TaskIDs: []string{a.ID, "no-such-issue"}})
	if len(byIDs) != 1 {
		t.Fatalf("got %+v", byIDs)
	}
}

func TestNearestCaps_NeitherIdsNorBacklogAndTooManyIdsAreRefused(t *testing.T) {
	mgr := newTestManager(t)
	if _, err := mgr.NearestCaps(context.Background(), mwanachamataskmanager.CapsNearestFilter{}); !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
		t.Fatalf("empty: %v", err)
	}
	tooMany := make([]string, 501)
	if _, err := mgr.NearestCaps(context.Background(), mwanachamataskmanager.CapsNearestFilter{TaskIDs: tooMany}); !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
		t.Fatalf("501 ids: %v", err)
	}
}
