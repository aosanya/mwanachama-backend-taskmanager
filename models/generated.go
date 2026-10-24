package models

type TaskTemplate struct {
	Title          string       `json:"title"`
	Description    string       `json:"description,omitempty"`
	Priority       TaskPriority `json:"priority,omitempty"`
	DueAt          string       `json:"due_at,omitempty"`
	EstimatedHours float64      `json:"estimated_hours,omitempty"`
}

type GenerateTasks struct {
	Run      string       `json:"run"`
	Template TaskTemplate `json:"template"`
	Subjects []string     `json:"subjects"`
}

type GeneratedTask struct {
	SubjectKey  string `json:"subject_key"`
	Run         string `json:"run"`
	Subject     string `json:"subject"`
	TaskID      string `json:"task_id"`
	GeneratedAt string `json:"generated_at"`
}

type GeneratedTaskOutcome struct {
	GeneratedTask
	Created bool `json:"created"`
}

type TasksGenerated struct {
	Items []GeneratedTaskOutcome `json:"items"`
}
