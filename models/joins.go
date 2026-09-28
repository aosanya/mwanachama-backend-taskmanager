package models

type Blocker struct {
	FromTaskID string `json:"from_task_id"`
	ToTaskID   string `json:"to_task_id"`
	CreatedAt  string `json:"created_at"`
	Reason     string `json:"reason,omitempty"`
}

type Dependency struct {
	FromTaskID string `json:"from_task_id"`
	ToTaskID   string `json:"to_task_id"`
	CreatedAt  string `json:"created_at"`
	Reason     string `json:"reason,omitempty"`
}

type Membership struct {
	TaskID    string `json:"task_id"`
	ProjectID string `json:"project_id"`
	AddedAt   string `json:"added_at"`
}

type Tagging struct {
	TaskID   string `json:"task_id"`
	TagID    string `json:"tag_id"`
	TaggedAt string `json:"tagged_at"`
}

type CodeSequence struct {
	EntityType string `json:"entity_type"`
	NextNumber int    `json:"next_number"`
}
