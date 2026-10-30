package mwanachamataskmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const memberVoterKind = "member"

var (
	ErrInvalidVote        = errors.New("a vote needs a case, a member and someone casting it")
	ErrVoterNotMember     = errors.New("only a member votes; an owner or an agent does not")
	ErrVoteNotFound       = errors.New("vote not found")
	ErrRequesterVoteStays = errors.New("the requester's own vote counts as the first and is not taken back")
)

func caseVoteKey(caseID, memberID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s%s", len(caseID), caseID, memberID)))
	return hex.EncodeToString(sum[:])
}

func (m *taskManager) openCaseToVoteOn(ctx context.Context, caseID string) (Case, error) {
	c, err := m.GetCase(ctx, caseID)
	if err != nil {
		return Case{}, err
	}
	if c.Status != CaseStatusOpen {
		return Case{}, ErrCaseNotOpen
	}
	return c, nil
}

func (m *taskManager) caseVote(ctx context.Context, key string) (CaseVote, bool, error) {
	var v CaseVote
	q := m.store.Query(ctx, roleCaseVote).Where("vote_key = ?", key)
	err := m.store.Take(q, roleCaseVote, &v, ErrVoteNotFound)
	if errors.Is(err, ErrVoteNotFound) {
		return CaseVote{}, false, nil
	}
	if err != nil {
		return CaseVote{}, false, err
	}
	return v, true, nil
}

func (m *taskManager) votesOn(ctx context.Context, caseID string) (int, error) {
	var stored int64
	if err := m.store.Query(ctx, roleCaseVote).Where("case_id = ?", caseID).Count(&stored).Error; err != nil {
		return 0, err
	}
	return int(stored) + 1, nil
}

func (m *taskManager) rankedCase(ctx context.Context, c Case) (RankedCase, error) {
	n, err := m.votesOn(ctx, c.ID)
	if err != nil {
		return RankedCase{}, err
	}
	return RankedCase{Case: c, Votes: n}, nil
}

func (m *taskManager) VoteCase(ctx context.Context, caseID, memberID, castBy, memberKind string) (RankedCase, error) {
	if memberID == "" {
		memberID = castBy
	}
	if caseID == "" || memberID == "" || castBy == "" {
		return RankedCase{}, ErrInvalidVote
	}
	if memberKind != "" && memberKind != memberVoterKind {
		return RankedCase{}, ErrVoterNotMember
	}
	c, err := m.openCaseToVoteOn(ctx, caseID)
	if err != nil {
		return RankedCase{}, err
	}
	if memberID != c.RequesterID {
		if err := m.recordVote(ctx, c.ID, memberID, castBy); err != nil {
			return RankedCase{}, fmt.Errorf("VoteCase: %w", err)
		}
	}
	ranked, err := m.rankedCase(ctx, c)
	if err != nil {
		return RankedCase{}, fmt.Errorf("VoteCase: %w", err)
	}
	return ranked, nil
}

func (m *taskManager) recordVote(ctx context.Context, caseID, memberID, castBy string) error {
	key := caseVoteKey(caseID, memberID)
	if _, found, err := m.caseVote(ctx, key); err != nil || found {
		return err
	}
	vote := CaseVote{
		VoteKey: key, CaseID: caseID, MemberID: memberID, CastBy: castBy,
		VotedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := m.store.Insert(ctx, roleCaseVote, vote); err != nil {
		if _, found, lookupErr := m.caseVote(ctx, key); lookupErr == nil && found {
			return nil
		}
		return err
	}
	return nil
}

func (m *taskManager) TakeBackVote(ctx context.Context, caseID, memberID string) (RankedCase, error) {
	if caseID == "" || memberID == "" {
		return RankedCase{}, ErrInvalidVote
	}
	c, err := m.openCaseToVoteOn(ctx, caseID)
	if err != nil {
		return RankedCase{}, err
	}
	if memberID == c.RequesterID {
		return RankedCase{}, ErrRequesterVoteStays
	}
	res := m.store.Query(ctx, roleCaseVote).Where("vote_key = ?", caseVoteKey(caseID, memberID)).Delete(nil)
	if res.Error != nil {
		return RankedCase{}, fmt.Errorf("TakeBackVote: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return RankedCase{}, ErrVoteNotFound
	}
	ranked, err := m.rankedCase(ctx, c)
	if err != nil {
		return RankedCase{}, fmt.Errorf("TakeBackVote: %w", err)
	}
	return ranked, nil
}

func (m *taskManager) ListCaseVotes(ctx context.Context, caseID string) ([]CaseVote, error) {
	if _, err := m.GetCase(ctx, caseID); err != nil {
		return nil, err
	}
	q := m.store.Query(ctx, roleCaseVote).Where("case_id = ?", caseID).Order("voted_at").Order("member_id").Limit(maxListPage)
	out, err := specstore.List[CaseVote](m.store, q, roleCaseVote)
	if err != nil {
		return nil, fmt.Errorf("ListCaseVotes: %w", err)
	}
	return out, nil
}

func (m *taskManager) ListCasesByVotes(ctx context.Context, filter CaseFilter) ([]RankedCase, error) {
	if filter.Status == "" {
		filter.Status = CaseStatusOpen
	}
	cases, err := m.ListCases(ctx, filter)
	if err != nil {
		return nil, err
	}
	counts, err := m.storedVotesByCase(ctx, cases)
	if err != nil {
		return nil, fmt.Errorf("ListCasesByVotes: %w", err)
	}
	out := make([]RankedCase, len(cases))
	for i, c := range cases {
		out[i] = RankedCase{Case: c, Votes: counts[c.ID] + 1}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Votes != out[j].Votes {
			return out[i].Votes > out[j].Votes
		}
		if out[i].OpenedAt != out[j].OpenedAt {
			return out[i].OpenedAt < out[j].OpenedAt
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *taskManager) storedVotesByCase(ctx context.Context, cases []Case) (map[string]int, error) {
	out := make(map[string]int, len(cases))
	if len(cases) == 0 {
		return out, nil
	}
	ids := make([]string, len(cases))
	for i, c := range cases {
		ids[i] = c.ID
	}
	var rows []struct {
		CaseID string
		Votes  int
	}
	err := m.store.Query(ctx, roleCaseVote).
		Select("case_id, count(*) AS votes").
		Where("case_id IN ?", ids).
		Group("case_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.CaseID] = r.Votes
	}
	return out, nil
}
