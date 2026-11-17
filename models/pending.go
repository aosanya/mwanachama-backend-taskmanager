package models

type PendingFilter struct {
	Tag          TagList      `query:"tag"`
	Backlog      string       `query:"backlog"`
	Step         string       `query:"step"`
	Priority     TaskPriority `query:"priority"`
	Q            string       `query:"q"`
	Limit        int          `query:"limit"`
	Offset       int          `query:"offset"`
	Assignee     string       `query:"assignee"`
	Unassigned   bool         `query:"unassigned"`
	Blocked      *bool        `query:"blocked"`
	Waiting      *bool        `query:"waiting"`
	Sinbin       *bool        `query:"sinbin"`
	HasTodos     *bool        `query:"has_todos"`
	TodosOpen    bool         `query:"todos_open"`
	Effort       string       `query:"effort"`
	CreatedSince string       `query:"created_since"`
	UpdatedSince string       `query:"updated_since"`
	HasFiles     *bool        `query:"has_files"`
	IDs          []string     `query:"ids"`
	Sort         string       `query:"sort"`

	IncludeClosed bool `query:"include_closed"`

	RequesterID string `query:"-" json:"-"`
}

type PendingCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type PendingItem struct {
	ID       string       `json:"id"`
	Code     string       `json:"code"`
	Title    string       `json:"title"`
	State    string       `json:"state,omitempty"`
	Backlog  string       `json:"backlog"`
	Step     string       `json:"step"`
	Priority TaskPriority `json:"priority"`
	Tags     []string     `json:"tags"`
	Rank     int          `json:"rank"`
	Blocked  bool         `json:"blocked"`
	Waiting  bool         `json:"waiting"`
	Sinbin   bool         `json:"sinbin"`
}

type PendingTasks struct {
	Total      int            `json:"total"`
	ByStep     []PendingCount `json:"by_step"`
	ByPriority []PendingCount `json:"by_priority"`
	Items      []PendingItem  `json:"items"`
}
