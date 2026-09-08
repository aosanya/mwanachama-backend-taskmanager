package models

// Deliverable is a specification of what a Task or TaskTodo must produce.
// It is a pure definition — no runtime state. ParentID is a polymorphic
// owner reference (Task or TaskTodo), so it is not a real foreign key.
type Deliverable struct {
	ID string `json:"id"`

	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	DeliverableType string `json:"deliverable_type,omitempty"`

	// ParentID is the denormalised owner ID (Task or TaskTodo).
	ParentID string `json:"parent_id"`

	// Ordinality is the 1-based position within the owning entity's deliverables.
	Ordinality int `json:"ordinality"`

	// WorkflowRunID is inherited from the parent at creation time.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
