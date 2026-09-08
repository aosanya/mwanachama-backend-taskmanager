package gormstore

import (
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TaskRow is the GORM row for a [models.Task].
//
// AssignedAgentID, WorkflowRunID, and ParentTaskID are the denormalised
// column equivalents of the entitygraph edges (assigned_to, started_task,
// subtask_of) — the domain type already treats "" as "unset" throughout, so
// these stay plain indexed strings rather than nullable *string columns.
//
// Tags is intentionally NOT a column here: it is resolved at read time from
// the TaskTag join table (see relationships.go), matching the pre-GORM
// behaviour where the stored "tags" property was always overwritten by a
// has_tag traversal on every read.
type TaskRow struct {
	ID               string `gorm:"primaryKey"`
	Title            string
	Description      string
	Status           string
	Priority         string
	DueAt            string
	EstimatedHours   float64
	Context          string
	CreatedAt        string
	UpdatedAt        string
	CompletedAt      string
	TaskName         string
	ProjectName      string
	SeparateBranch   bool
	BranchName       string
	AssignedAgentID  string `gorm:"index"`
	WorkflowRunID    string `gorm:"index"`
	RecoveryRunsUsed int
	BlockerNote      string
	DirectionHistory string
	ParentTaskID     string `gorm:"index"`

	// Deleted marks a soft-deleted task, matching entitygraph.DataManager's
	// DeleteEntity semantics (always soft-delete, regardless of entity
	// type). Not exposed on models.Task — GetTask/ListTasks filter it out
	// the same way the pre-GORM implementation never surfaced it either.
	Deleted bool `gorm:"index"`
}

func (r *TaskRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// TaskToRow converts a domain Task to its row shape. Tags is not carried —
// see TaskRow's doc.
func TaskToRow(t models.Task) TaskRow {
	return TaskRow{
		ID:               t.ID,
		Title:            t.Title,
		Description:      t.Description,
		Status:           string(t.Status),
		Priority:         string(t.Priority),
		DueAt:            t.DueAt,
		EstimatedHours:   t.EstimatedHours,
		Context:          t.Context,
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
		CompletedAt:      t.CompletedAt,
		TaskName:         t.TaskName,
		ProjectName:      t.ProjectName,
		SeparateBranch:   t.SeparateBranch,
		BranchName:       t.BranchName,
		AssignedAgentID:  t.AssignedTo,
		WorkflowRunID:    t.WorkflowRunID,
		RecoveryRunsUsed: t.RecoveryRunsUsed,
		BlockerNote:      t.BlockerNote,
		DirectionHistory: t.DirectionHistory,
		ParentTaskID:     t.ParentTaskID,
	}
}

// TaskFromRow converts a row back to the domain Task. Tags is left nil —
// callers resolve it separately from the TaskTag join table.
func TaskFromRow(r TaskRow) models.Task {
	return models.Task{
		ID:               r.ID,
		Title:            r.Title,
		Description:      r.Description,
		Status:           models.TaskStatus(r.Status),
		Priority:         models.TaskPriority(r.Priority),
		DueAt:            r.DueAt,
		EstimatedHours:   r.EstimatedHours,
		Context:          r.Context,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		CompletedAt:      r.CompletedAt,
		TaskName:         r.TaskName,
		ProjectName:      r.ProjectName,
		SeparateBranch:   r.SeparateBranch,
		BranchName:       r.BranchName,
		AssignedTo:       r.AssignedAgentID,
		WorkflowRunID:    r.WorkflowRunID,
		RecoveryRunsUsed: r.RecoveryRunsUsed,
		BlockerNote:      r.BlockerNote,
		DirectionHistory: r.DirectionHistory,
		ParentTaskID:     r.ParentTaskID,
	}
}
