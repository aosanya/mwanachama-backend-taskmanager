package models

type PendingFilter struct {
	Tag      TagList      `query:"tag"`
	Backlog  string       `query:"backlog"`
	Step     string       `query:"step"`
	Priority TaskPriority `query:"priority"`
	Q        string       `query:"q"`
	Limit    int          `query:"limit"`
	Offset   int          `query:"offset"`

	RequesterID string `query:"-" json:"-"`
}

type PendingCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type PendingItem struct {
	ID       string       `json:"id"`
	Title    string       `json:"title"`
	State    string       `json:"state,omitempty"`
	Backlog  string       `json:"backlog"`
	Step     string       `json:"step"`
	Priority TaskPriority `json:"priority"`
	Tags     []string     `json:"tags"`
	Rank     int          `json:"rank"`
	Blocked  bool         `json:"blocked"`
}

type PendingTasks struct {
	Total      int            `json:"total"`
	ByStep     []PendingCount `json:"by_step"`
	ByPriority []PendingCount `json:"by_priority"`
	Items      []PendingItem  `json:"items"`
}
