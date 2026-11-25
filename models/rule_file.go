package models

type RuleFile struct {
	ID        string `json:"id"`
	RuleID    string `json:"rule_id"`
	Repo      string `json:"repo"`
	Path      string `json:"path"`
	Line      int    `json:"line,omitempty"`
	Note      string `json:"note,omitempty"`
	AddedBy   string `json:"added_by"`
	CreatedAt string `json:"created_at"`
}

type RuleFileLink struct {
	RuleFile
	Missing bool `json:"missing,omitempty"`
}

type RuleFileResult struct {
	File    RuleFile `json:"file"`
	Warning string   `json:"warning,omitempty"`
}
