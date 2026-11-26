package models

type RuleNote struct {
	ID             string   `json:"id"`
	RuleID         string   `json:"rule_id"`
	RuleCode       string   `json:"rule_code"`
	Kind           string   `json:"kind"`
	Source         string   `json:"source"`
	State          string   `json:"state,omitempty"`
	Text           string   `json:"text"`
	RelatedCodes   []string `json:"related_codes"`
	Links          []string `json:"links"`
	AuthorID       string   `json:"author_id"`
	CreatedAt      string   `json:"created_at"`
	ResolvedAt     string   `json:"resolved_at,omitempty"`
	ResolvedBy     string   `json:"resolved_by,omitempty"`
	Resolution     string   `json:"resolution,omitempty"`
	ResolvedByCode string   `json:"resolved_by_code,omitempty"`
}

type RuleNoteFilter struct {
	Rule    string  `query:"rule"`
	Kind    TagList `query:"kind"`
	State   string  `query:"state"`
	Related string  `query:"related"`
	Since   string  `query:"since"`
	Limit   int     `query:"limit,min=1"`

	RequesterID string `query:"-" json:"-"`
}

type RuleWithNotes struct {
	RuleRecord
	Compatible []string   `json:"compatible"`
	Notes      []RuleNote `json:"notes"`
	NoteCount  int        `json:"note_count"`
}

type RuleNoteDraft struct {
	Kind         string   `json:"kind"`
	Source       string   `json:"source"`
	Text         string   `json:"text"`
	RelatedCodes []string `json:"related_codes"`
	Links        []string `json:"links"`

	RequesterID string `json:"-"`
	RuleCode    string `json:"-"`
}

type RuleNoteResolution struct {
	Resolution     string `json:"resolution"`
	ResolvedByCode string `json:"resolved_by_code"`

	RequesterID string `json:"-"`
	NoteID      string `json:"-"`
}
