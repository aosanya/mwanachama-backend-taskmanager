package models

// AcceptanceCriteria is a verifiable condition that must be satisfied
// before the owning Task or TaskTodo is considered done. A reviewer writes
// the runtime result against each criterion. ParentID is a polymorphic
// owner reference (Task or TaskTodo), so it is not a real foreign key.
type AcceptanceCriteria struct {
	ID string `json:"id"`

	// Code is a stable, human-readable identifier (e.g. "AC-1") minted once
	// at creation. It never changes, even when Title is later edited.
	Code string `json:"code"`

	Title       string `json:"title"`
	Description string `json:"description,omitempty"`

	// ParentID is the denormalised owner ID (Task or TaskTodo).
	ParentID string `json:"parent_id"`

	// Ordinality is the 1-based position within the owning entity's criteria.
	Ordinality int `json:"ordinality"`

	// WorkflowRunID is inherited from the parent at creation time.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`

	// Result is the runtime outcome written by the reviewer: "passed",
	// "failed", "skipped", or "blocked". Empty until the reviewer runs.
	Result      string `json:"result,omitempty"`
	ResultNotes string `json:"result_notes,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
