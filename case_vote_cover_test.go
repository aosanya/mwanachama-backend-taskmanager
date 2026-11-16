package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func openCaseBy(t *testing.T, mgr mwanachamataskmanager.TaskManager, requester string) mwanachamataskmanager.Case {
	t.Helper()
	c, err := mgr.OpenCase(context.Background(), requester, mwanachamataskmanager.Case{SubjectKind: "market", SubjectID: requester + "-subject"})
	if err != nil {
		t.Fatalf("OpenCase: %v", err)
	}
	return c
}

func TestCaseVotes_TheRequesterCountsAsTheFirstVote(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	c := openCaseBy(t, mgr, "amina")

	ranked, err := mgr.VoteCase(ctx, c.ID, "amina", "amina", "member")
	if err != nil {
		t.Fatal(err)
	}
	if ranked.Votes != 1 {
		t.Fatalf("the requester voting for their own case: got %d votes, want 1", ranked.Votes)
	}
	votes, err := mgr.ListCaseVotes(ctx, c.ID)
	if err != nil || len(votes) != 0 {
		t.Fatalf("the requester's own vote is not stored: got %d, err %v", len(votes), err)
	}
}

func TestCaseVotes_EachMemberVotesOnceAndTheCountRises(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	c := openCaseBy(t, mgr, "amina")

	first, err := mgr.VoteCase(ctx, c.ID, "otieno", "otieno", "member")
	if err != nil || first.Votes != 2 {
		t.Fatalf("second person's vote: votes %d, err %v, want 2", first.Votes, err)
	}
	again, err := mgr.VoteCase(ctx, c.ID, "otieno", "otieno", "")
	if err != nil || again.Votes != 2 {
		t.Fatalf("voting twice must not count twice: votes %d, err %v, want 2", again.Votes, err)
	}
	third, err := mgr.VoteCase(ctx, c.ID, "wanjiru", "wanjiru", "member")
	if err != nil || third.Votes != 3 {
		t.Fatalf("third person's vote: votes %d, err %v, want 3", third.Votes, err)
	}
	votes, err := mgr.ListCaseVotes(ctx, c.ID)
	if err != nil || len(votes) != 2 {
		t.Fatalf("stored votes: got %d, err %v, want 2", len(votes), err)
	}
}

func TestCaseVotes_ABlankMemberIsTheOneCastingIt(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	c := openCaseBy(t, mgr, "amina")

	ranked, err := mgr.VoteCase(ctx, c.ID, "", "otieno", "member")
	if err != nil || ranked.Votes != 2 {
		t.Fatalf("a vote with no member named: votes %d, err %v, want 2", ranked.Votes, err)
	}
	votes, err := mgr.ListCaseVotes(ctx, c.ID)
	if err != nil || len(votes) != 1 || votes[0].MemberID != "otieno" || votes[0].CastBy != "otieno" {
		t.Fatalf("the stored vote should be otieno's own: %+v, err %v", votes, err)
	}
}

func TestCaseVotes_AVoteNeedsACaseAMemberAndAMemberVoter(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	c := openCaseBy(t, mgr, "amina")

	cases := []struct {
		name                   string
		caseID, member, castBy string
		kind                   string
		want                   error
	}{
		{"no case", "", "otieno", "otieno", "member", mwanachamataskmanager.ErrInvalidVote},
		{"nobody casting", c.ID, "", "", "member", mwanachamataskmanager.ErrInvalidVote},
		{"an agent voting", c.ID, "otieno", "bot", "agent", mwanachamataskmanager.ErrVoterNotMember},
		{"an owner voting", c.ID, "otieno", "boss", "owner", mwanachamataskmanager.ErrVoterNotMember},
		{"a case that does not exist", "missing", "otieno", "otieno", "member", mwanachamataskmanager.ErrCaseNotFound},
	}
	for _, tc := range cases {
		if _, err := mgr.VoteCase(ctx, tc.caseID, tc.member, tc.castBy, tc.kind); !errors.Is(err, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestCaseVotes_ACaseThatIsClosedTakesNoMoreVotesAndGivesNoneBack(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	c := openCaseBy(t, mgr, "amina")
	if _, err := mgr.VoteCase(ctx, c.ID, "otieno", "otieno", "member"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.EndCase(ctx, c.ID, mwanachamataskmanager.CaseStatusDone, "sorted"); err != nil {
		t.Fatal(err)
	}

	if _, err := mgr.VoteCase(ctx, c.ID, "wanjiru", "wanjiru", "member"); !errors.Is(err, mwanachamataskmanager.ErrCaseNotOpen) {
		t.Fatalf("voting on a closed case: got %v, want ErrCaseNotOpen", err)
	}
	if _, err := mgr.TakeBackVote(ctx, c.ID, "otieno"); !errors.Is(err, mwanachamataskmanager.ErrCaseNotOpen) {
		t.Fatalf("taking a vote back from a closed case: got %v, want ErrCaseNotOpen", err)
	}
}

func TestCaseVotes_ATakenBackVoteLowersTheCountAndTheRequesterCannotTakeTheirsBack(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	c := openCaseBy(t, mgr, "amina")
	if _, err := mgr.VoteCase(ctx, c.ID, "otieno", "otieno", "member"); err != nil {
		t.Fatal(err)
	}

	ranked, err := mgr.TakeBackVote(ctx, c.ID, "otieno")
	if err != nil || ranked.Votes != 1 {
		t.Fatalf("taking a vote back: votes %d, err %v, want 1", ranked.Votes, err)
	}
	if _, err := mgr.TakeBackVote(ctx, c.ID, "otieno"); !errors.Is(err, mwanachamataskmanager.ErrVoteNotFound) {
		t.Fatalf("taking back a vote twice: got %v, want ErrVoteNotFound", err)
	}
	if _, err := mgr.TakeBackVote(ctx, c.ID, "amina"); !errors.Is(err, mwanachamataskmanager.ErrRequesterVoteStays) {
		t.Fatalf("the requester taking their vote back: got %v, want ErrRequesterVoteStays", err)
	}
	if _, err := mgr.TakeBackVote(ctx, "", "otieno"); !errors.Is(err, mwanachamataskmanager.ErrInvalidVote) {
		t.Fatalf("taking back with no case: got %v, want ErrInvalidVote", err)
	}
	if _, err := mgr.TakeBackVote(ctx, "missing", "otieno"); !errors.Is(err, mwanachamataskmanager.ErrCaseNotFound) {
		t.Fatalf("taking back on a missing case: got %v, want ErrCaseNotFound", err)
	}
}

func TestCaseVotes_ListingVotesOnAMissingCaseIsRefused(t *testing.T) {
	mgr := newTestManager(t)
	if _, err := mgr.ListCaseVotes(context.Background(), "missing"); !errors.Is(err, mwanachamataskmanager.ErrCaseNotFound) {
		t.Fatalf("got %v, want ErrCaseNotFound", err)
	}
}

func TestCaseVotes_CasesAreRankedByVotesThenByWhoOpenedFirst(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	quiet := openCaseBy(t, mgr, "amina")
	popular := openCaseBy(t, mgr, "otieno")
	middle := openCaseBy(t, mgr, "wanjiru")
	for _, member := range []string{"a", "b", "c"} {
		if _, err := mgr.VoteCase(ctx, popular.ID, member, member, "member"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mgr.VoteCase(ctx, middle.ID, "a", "a", "member"); err != nil {
		t.Fatal(err)
	}

	ranked, err := mgr.ListCasesByVotes(ctx, mwanachamataskmanager.CaseFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked) != 3 {
		t.Fatalf("got %d open cases, want 3", len(ranked))
	}
	if ranked[0].ID != popular.ID || ranked[0].Votes != 4 {
		t.Fatalf("first should be the case with the most votes: %+v", ranked[0])
	}
	if ranked[1].ID != middle.ID || ranked[1].Votes != 2 {
		t.Fatalf("second should be the case with one vote besides its requester's: %+v", ranked[1])
	}
	if ranked[2].ID != quiet.ID || ranked[2].Votes != 1 {
		t.Fatalf("last should be the case with only its requester's vote: %+v", ranked[2])
	}
}

func TestCaseVotes_RankingListsOpenCasesByDefaultAndNoneWhenThereAreNone(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	none, err := mgr.ListCasesByVotes(ctx, mwanachamataskmanager.CaseFilter{})
	if err != nil || len(none) != 0 {
		t.Fatalf("no cases: got %d, err %v", len(none), err)
	}
	closed := openCaseBy(t, mgr, "amina")
	if _, err := mgr.EndCase(ctx, closed.ID, mwanachamataskmanager.CaseStatusCancelled, "no"); err != nil {
		t.Fatal(err)
	}
	open := openCaseBy(t, mgr, "otieno")

	ranked, err := mgr.ListCasesByVotes(ctx, mwanachamataskmanager.CaseFilter{})
	if err != nil || len(ranked) != 1 || ranked[0].ID != open.ID {
		t.Fatalf("default is open cases only: %+v, err %v", ranked, err)
	}
	cancelled, err := mgr.ListCasesByVotes(ctx, mwanachamataskmanager.CaseFilter{Status: mwanachamataskmanager.CaseStatusCancelled})
	if err != nil || len(cancelled) != 1 || cancelled[0].ID != closed.ID {
		t.Fatalf("asking for cancelled cases: %+v, err %v", cancelled, err)
	}
}
