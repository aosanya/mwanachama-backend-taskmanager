package models

type CaseStatus string

const (
	CaseStatusOpen      CaseStatus = "open"
	CaseStatusDone      CaseStatus = "done"
	CaseStatusCancelled CaseStatus = "cancelled"
)

type Case struct {
	ID          string     `json:"id"`
	RequesterID string     `json:"requester_id"`
	SubjectKind string     `json:"subject_kind,omitempty"`
	SubjectID   string     `json:"subject_id,omitempty"`
	Title       string     `json:"title,omitempty"`
	Status      CaseStatus `json:"status"`
	OpenedAt    string     `json:"opened_at"`
	ClosedAt    string     `json:"closed_at,omitempty"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
}

type CaseTask struct {
	TaskID   string `json:"task_id"`
	CaseID   string `json:"case_id"`
	LinkedAt string `json:"linked_at"`
}

type CaseFilter struct {
	RequesterID string     `json:"requester_id,omitempty"`
	SubjectKind string     `json:"subject_kind,omitempty"`
	SubjectID   string     `json:"subject_id,omitempty"`
	Status      CaseStatus `json:"status,omitempty"`
}
