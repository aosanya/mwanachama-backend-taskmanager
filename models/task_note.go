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
