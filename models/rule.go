package models

type RuleRecord struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Code         string   `json:"code"`
	Title        string   `json:"title"`
	Body         string   `json:"body,omitempty"`
	Status       string   `json:"status"`
	Tags         []string `json:"tags,omitempty"`
	Supersedes   string   `json:"supersedes,omitempty"`
	SupersededBy string   `json:"superseded_by,omitempty"`
	DecidedBy    string   `json:"decided_by,omitempty"`
	DecidedAt    string   `json:"decided_at,omitempty"`
	Priority     string   `json:"priority,omitempty"`
	AnsweredBy   string   `json:"answered_by,omitempty"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

type RulePatch struct {
	Title     *string   `json:"title,omitempty"`
	Body      *string   `json:"body,omitempty"`
	Tags      *[]string `json:"tags,omitempty"`
	DecidedBy *string   `json:"decided_by,omitempty"`
	DecidedAt *string   `json:"decided_at,omitempty"`
	Priority  *string   `json:"priority,omitempty"`
}

type RuleFilter struct {
	Kind     string   `query:"kind"`
	Statuses []string `query:"status"`
	Tags     []string `query:"tag"`
}
