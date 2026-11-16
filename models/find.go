package models

type FindFilter struct {
	Tag     TagList `query:"tag"`
	Backlog string  `query:"backlog"`
	Step    string  `query:"step"`
	Q       string  `query:"q"`
	Limit   int     `query:"limit"`
	Offset  int     `query:"offset"`

	IncludeClosed bool `query:"include_closed"`

	RequesterID string `query:"-" json:"-"`
}

type FoundTasks struct {
	Total int           `json:"total"`
	Items []PendingItem `json:"items"`
}
