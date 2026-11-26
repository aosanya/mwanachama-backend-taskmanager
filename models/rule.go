package models

import "encoding/json"

type RuleRecord struct {
	ID           string           `json:"id"`
	Kind         string           `json:"kind"`
	Code         string           `json:"code"`
	Title        string           `json:"title"`
	Body         string           `json:"body,omitempty"`
	Status       string           `json:"status"`
	Tags         []string         `json:"tags,omitempty"`
	AppliesWhen  json.RawMessage  `json:"applies_when,omitempty"`
	Supersedes   string           `json:"supersedes,omitempty"`
	SupersededBy string           `json:"superseded_by,omitempty"`
	DecidedBy    string           `json:"decided_by,omitempty"`
	DecidedAt    string           `json:"decided_at,omitempty"`
	Priority     string           `json:"priority,omitempty"`
	AnsweredBy   string           `json:"answered_by,omitempty"`
	TargetRole   string           `json:"target_role,omitempty"`
	Options      []QuestionOption `json:"options,omitempty"`
	Answer       string           `json:"answer,omitempty"`
	CreatedAt    string           `json:"created_at"`
	UpdatedAt    string           `json:"updated_at"`
}

type QuestionOption struct {
	Text        string `json:"text"`
	Recommended bool   `json:"recommended,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type QuestionAnswer struct {
	DecisionCode string `json:"decision_code,omitempty"`
	Option       *int   `json:"option,omitempty"`
	Other        string `json:"other,omitempty"`
}

type RulePatch struct {
	Title       *string          `json:"title,omitempty"`
	Body        *string          `json:"body,omitempty"`
	Tags        *[]string        `json:"tags,omitempty"`
	AppliesWhen *json.RawMessage `json:"applies_when,omitempty"`
	DecidedBy   *string          `json:"decided_by,omitempty"`
	DecidedAt   *string          `json:"decided_at,omitempty"`
	Priority    *string          `json:"priority,omitempty"`

	ExpectedUpdatedAt *string `json:"expected_updated_at,omitempty"`
}

type RuleFilter struct {
	Kind     string   `query:"kind"`
	Statuses []string `query:"status"`
	Tags     []string `query:"tag"`
}
