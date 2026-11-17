package models

type TaskNote struct {
	ID        string   `json:"id"`
	TaskID    string   `json:"task_id"`
	AuthorID  string   `json:"author_id"`
	Kind      string   `json:"kind"`
	Text      string   `json:"text"`
	Links     []string `json:"links,omitempty"`
	Automatic bool     `json:"automatic,omitempty"`
	CreatedAt string   `json:"created_at"`
}

type TaskNoteFilter struct {
	After string  `query:"after"`
	Kind  TagList `query:"kind"`
	Limit int     `query:"limit,min=1"`

	RequesterID string `query:"-" json:"-"`
	TaskID      string `query:"-" json:"-"`
}
