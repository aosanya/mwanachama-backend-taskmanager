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
	LinkedRef    string       `query:"linked_ref"`
	IDs          []string     `query:"ids"`
	Sort         string       `query:"sort"`

	MinDaysInStep int `query:"min_days_in_step,min=0"`

	IncludeClosed bool `query:"include_closed"`

	Fields []string `query:"fields"`

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

	EnteredStepAt string `json:"entered_step_at,omitempty"`
	DaysInStep    int    `json:"days_in_step"`
}

type PendingTasks struct {
	Total      int            `json:"total"`
	ByStep     []PendingCount `json:"by_step"`
	ByPriority []PendingCount `json:"by_priority"`
	Items      []PendingItem  `json:"items"`

	Fields []string `json:"-"`
}
