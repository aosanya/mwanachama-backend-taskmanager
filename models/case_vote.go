package models

type CaseVote struct {
	VoteKey  string `json:"vote_key"`
	CaseID   string `json:"case_id"`
	MemberID string `json:"member_id"`
	CastBy   string `json:"cast_by"`
	VotedAt  string `json:"voted_at"`
}

type RankedCase struct {
	Case
	Votes int `json:"votes"`
}
