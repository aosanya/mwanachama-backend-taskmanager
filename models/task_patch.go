package models

type TaskPatch struct {
	ID             string        `json:"-"`
	Title          *string       `json:"title,omitempty"`
	Description    *string       `json:"description,omitempty"`
	Status         *TaskStatus   `json:"status,omitempty"`
	Priority       *TaskPriority `json:"priority,omitempty"`
	Effort         *TaskEffort   `json:"effort,omitempty"`
	Tags           *[]string     `json:"tags,omitempty"`
	ProjectName    *string       `json:"project_name,omitempty"`
	DueAt          *string       `json:"due_at,omitempty"`
	EstimatedHours *float64      `json:"estimated_hours,omitempty"`
	Context        *string       `json:"context,omitempty"`
}
