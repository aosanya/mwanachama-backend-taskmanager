package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	tm "github.com/aosanya/mwanachama-backend-taskmanager"
)

func openVotingCase(t *testing.T, m tm.TaskManager, requester string) tm.Case {
	t.Helper()
	c, err := m.OpenCase(context.Background(), requester, tm.Case{Title: "fix the borehole"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTheRequesterCountsAsTheFirstVoteAndOthersAddToIt(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	c := openVotingCase(t, m, "amina")
	got, err := m.VoteCase(ctx, c.ID, "juma", "juma", "")
	if err != nil || got.Votes != 2 {
		t.Fatalf("after juma = %+v, %v; want 2 votes", got, err)
	}
	got, err = m.VoteCase(ctx, c.ID, "neema", "neema", "member")
	if err != nil || got.Votes != 3 {
		t.Fatalf("after neema = %+v, %v; want 3 votes", got, err)
	}
}

func TestAMemberVotesOnACaseOnceHoweverOftenTheVoteIsCast(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	c := openVotingCase(t, m, "amina")
	for i := 0; i < 3; i++ {
		if _, err := m.VoteCase(ctx, c.ID, "juma", "juma", ""); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.VoteCase(ctx, c.ID, "juma", "hub-steward", "")
	if err != nil || got.Votes != 2 {
		t.Fatalf("votes = %+v, %v; want 2", got, err)
	}
	votes, err := m.ListCaseVotes(ctx, c.ID)
	if err != nil || len(votes) != 1 || votes[0].CastBy != "juma" {
		t.Fatalf("stored votes = %+v, %v; the first cast stands", votes, err)
	}
}

func TestTheRequesterVotingOnTheirOwnCaseAddsNothing(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	c := openVotingCase(t, m, "amina")
	got, err := m.VoteCase(ctx, c.ID, "amina", "amina", "")
	if err != nil || got.Votes != 1 {
		t.Fatalf("votes = %+v, %v; want 1", got, err)
	}
	if votes, _ := m.ListCaseVotes(ctx, c.ID); len(votes) != 0 {
		t.Fatalf("stored %+v; the requester's vote is never stored", votes)
	}
}

func TestAVoteCastForAMemberRecordsTheMemberAndWhoCastIt(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	c := openVotingCase(t, m, "amina")
	if _, err := m.VoteCase(ctx, c.ID, "juma", "hub-steward", ""); err != nil {
		t.Fatal(err)
	}
	votes, err := m.ListCaseVotes(ctx, c.ID)
	if err != nil || len(votes) != 1 {
		t.Fatalf("votes = %+v, %v", votes, err)
	}
	v := votes[0]
	if v.MemberID != "juma" || v.CastBy != "hub-steward" || v.CaseID != c.ID || v.VotedAt == "" {
		t.Fatalf("vote = %+v", v)
	}
}

func TestWithNoMemberGivenTheCallerVotesForThemselves(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	c := openVotingCase(t, m, "amina")
	if _, err := m.VoteCase(ctx, c.ID, "", "juma", ""); err != nil {
		t.Fatal(err)
	}
	votes, _ := m.ListCaseVotes(ctx, c.ID)
	if len(votes) != 1 || votes[0].MemberID != "juma" || votes[0].CastBy != "juma" {
		t.Fatalf("votes = %+v", votes)
	}
}

func TestOwnersAndAgentsAreRefusedAsVoters(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	c := openVotingCase(t, m, "amina")
	for _, kind := range []string{"owner", "agent", "staff", "anything"} {
		if _, err := m.VoteCase(ctx, c.ID, "boss", "boss", kind); !errors.Is(err, tm.ErrVoterNotMember) {
			t.Fatalf("kind %q err = %v, want ErrVoterNotMember", kind, err)
		}
	}
	if votes, _ := m.ListCaseVotes(ctx, c.ID); len(votes) != 0 {
		t.Fatalf("refused voters left %+v", votes)
	}
}

func TestAVoteNeedsACaseAndSomeoneCastingIt(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	c := openVotingCase(t, m, "amina")
	if _, err := m.VoteCase(ctx, c.ID, "juma", "", ""); !errors.Is(err, tm.ErrInvalidVote) {
		t.Fatalf("no caster err = %v", err)
	}
	if _, err := m.VoteCase(ctx, "", "juma", "juma", ""); !errors.Is(err, tm.ErrInvalidVote) {
		t.Fatalf("no case err = %v", err)
	}
}

func TestVotingOnAnUnknownOrClosedCaseIsRefused(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	if _, err := m.VoteCase(ctx, "nope", "juma", "juma", ""); !errors.Is(err, tm.ErrCaseNotFound) {
		t.Fatalf("unknown err = %v", err)
	}
	c := openVotingCase(t, m, "amina")
	if _, err := m.EndCase(ctx, c.ID, tm.CaseStatusCancelled, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.VoteCase(ctx, c.ID, "juma", "juma", ""); !errors.Is(err, tm.ErrCaseNotOpen) {
		t.Fatalf("closed err = %v", err)
	}
	if _, err := m.TakeBackVote(ctx, c.ID, "juma"); !errors.Is(err, tm.ErrCaseNotOpen) {
		t.Fatalf("take back on closed err = %v", err)
	}
}

func TestATakenBackVoteNoLongerCountsAndCanBeCastAgain(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	c := openVotingCase(t, m, "amina")
	if _, err := m.VoteCase(ctx, c.ID, "juma", "juma", ""); err != nil {
		t.Fatal(err)
	}
	got, err := m.TakeBackVote(ctx, c.ID, "juma")
	if err != nil || got.Votes != 1 {
		t.Fatalf("after take back = %+v, %v; want 1", got, err)
	}
	if _, err := m.TakeBackVote(ctx, c.ID, "juma"); !errors.Is(err, tm.ErrVoteNotFound) {
		t.Fatalf("second take back err = %v", err)
	}
	got, err = m.VoteCase(ctx, c.ID, "juma", "juma", "")
	if err != nil || got.Votes != 2 {
		t.Fatalf("after revote = %+v, %v; want 2", got, err)
	}
}

func TestTheRequestersOwnVoteIsNotTakenBack(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	c := openVotingCase(t, m, "amina")
	if _, err := m.TakeBackVote(ctx, c.ID, "amina"); !errors.Is(err, tm.ErrRequesterVoteStays) {
		t.Fatalf("err = %v, want ErrRequesterVoteStays", err)
	}
}

func TestVotesOnOneCaseDoNotCountOnAnother(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	a := openVotingCase(t, m, "amina")
	b := openVotingCase(t, m, "amina")
	if _, err := m.VoteCase(ctx, a.ID, "juma", "juma", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.VoteCase(ctx, b.ID, "juma", "juma", ""); err != nil {
		t.Fatal(err)
	}
	got, err := m.TakeBackVote(ctx, a.ID, "juma")
	if err != nil || got.Votes != 1 {
		t.Fatalf("a = %+v, %v", got, err)
	}
	if again, _ := m.VoteCase(ctx, b.ID, "juma", "juma", ""); again.Votes != 2 {
		t.Fatalf("b = %+v; taking back on a must not touch b", again)
	}
}

func TestCasesListMostVotedFirstAndTheEarlierOpenedWinsATie(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	first := openVotingCase(t, m, "amina")
	second := openVotingCase(t, m, "amina")
	third := openVotingCase(t, m, "amina")
	time.Sleep(1100 * time.Millisecond)
	fourth := openVotingCase(t, m, "amina")
	for _, who := range []string{"a", "b"} {
		if _, err := m.VoteCase(ctx, third.ID, who, who, ""); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{second.ID, fourth.ID} {
		if _, err := m.VoteCase(ctx, id, "a", "a", ""); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.ListCasesByVotes(ctx, tm.CaseFilter{})
	if err != nil || len(got) != 4 {
		t.Fatalf("ranked = %+v, %v", got, err)
	}
	want := []struct {
		id    string
		votes int
	}{{third.ID, 3}, {second.ID, 2}, {fourth.ID, 2}, {first.ID, 1}}
	for i, w := range want {
		if got[i].ID != w.id || got[i].Votes != w.votes {
			t.Fatalf("rank %d = %s/%d, want %s/%d", i, got[i].ID, got[i].Votes, w.id, w.votes)
		}
	}
}

func TestOnlyOpenCasesAreRankedUnlessAStatusIsAsked(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	open := openVotingCase(t, m, "amina")
	closed := openVotingCase(t, m, "amina")
	if _, err := m.EndCase(ctx, closed.ID, tm.CaseStatusCancelled, ""); err != nil {
		t.Fatal(err)
	}
	got, err := m.ListCasesByVotes(ctx, tm.CaseFilter{})
	if err != nil || len(got) != 1 || got[0].ID != open.ID {
		t.Fatalf("default = %+v, %v", got, err)
	}
	got, err = m.ListCasesByVotes(ctx, tm.CaseFilter{Status: tm.CaseStatusCancelled})
	if err != nil || len(got) != 1 || got[0].ID != closed.ID {
		t.Fatalf("cancelled = %+v, %v", got, err)
	}
}

func TestRankingNarrowsBySubject(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	for _, subject := range []string{"issue", "issue", "idea"} {
		if _, err := m.OpenCase(ctx, "amina", tm.Case{SubjectKind: subject}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.ListCasesByVotes(ctx, tm.CaseFilter{SubjectKind: "issue"})
	if err != nil || len(got) != 2 {
		t.Fatalf("issues = %+v, %v", got, err)
	}
}
