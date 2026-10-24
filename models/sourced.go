package models

type SourcedTaskItem struct {
	Key         string `json:"key,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	DueAt       string `json:"due_at"`
}

type OpenSourcedTasks struct {
	Source string            `json:"source"`
	Items  []SourcedTaskItem `json:"items"`
}

type SourcedTask struct {
	ItemKey  string `json:"item_key"`
	Source   string `json:"source"`
	Key      string `json:"key"`
	TaskID   string `json:"task_id"`
	DueAt    string `json:"due_at"`
	OpenedAt string `json:"opened_at"`
}

type SourcedTaskOutcome struct {
	SourcedTask
	Opened bool `json:"opened"`
}

type SourcedTasksOpened struct {
	Items []SourcedTaskOutcome `json:"items"`
}
