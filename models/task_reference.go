package models

type TaskReference struct {
	ID        string `json:"id"`
	TaskID    string `json:"task_id"`
	Kind      string `json:"kind"`
	Repo      string `json:"repo,omitempty"`
	Target    string `json:"target"`
	Source    string `json:"source"`
	Confirmed bool   `json:"confirmed"`
	AddedBy   string `json:"added_by"`
	CreatedAt string `json:"created_at"`
}
