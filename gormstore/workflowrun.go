package gormstore

import (
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// WorkflowRunRow is the GORM row for a [models.WorkflowRun].
//
// FailureReason has no corresponding models.WorkflowRun field — the
// pre-GORM implementation wrote it as an extra property on failed/
// rollback_failed transitions for external/debugging visibility but never
// read it back through any Go path. Preserved here as a write-only column
// for parity; workflowrun_impl.go sets it directly, bypassing
// WorkflowRunToRow.
type WorkflowRunRow struct {
	ID                    string `gorm:"primaryKey"`
	Name                  string `gorm:"uniqueIndex"`
	Status                string
	TriggerEvent          string
	Initiator             string
	Notes                 string
	AgentRunIDs           datatypes.JSON
	FunctionJobIDs        datatypes.JSON
	BranchNames           datatypes.JSON
	TerminalEvent         string
	StartedAt             string
	CompletedAt           string
	CreatedAt             string
	UpdatedAt             string
	ParentWorkflowRunID   string `gorm:"index"`
	RootWorkflowRunID     string `gorm:"index"`
	FailurePipelineBudget int
	FailurePipelinesUsed  int
	CountedChildRunIDs    datatypes.JSON
	CancelledBy           string
	CancelReason          string
	CancellingUntil       string
	LastEventAt           string `gorm:"index"`
	TimeoutPublished      bool
	PausedAt              string
	CurrentStepID         string
	CurrentStepStartedAt  string
	FailureReason         string
}

func (r *WorkflowRunRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

// WorkflowRunToRow converts a domain WorkflowRun to its row shape.
// FailureReason is not set — callers that need it set it directly on the
// returned row (see this file's doc).
func WorkflowRunToRow(r models.WorkflowRun) WorkflowRunRow {
	return WorkflowRunRow{
		ID:                    r.ID,
		Name:                  r.Name,
		Status:                string(r.Status),
		TriggerEvent:          r.TriggerEvent,
		Initiator:             r.Initiator,
		Notes:                 r.Notes,
		AgentRunIDs:           stringsToJSON(r.AgentRunIDs),
		FunctionJobIDs:        stringsToJSON(r.FunctionJobIDs),
		BranchNames:           stringsToJSON(r.BranchNames),
		TerminalEvent:         r.TerminalEvent,
		StartedAt:             r.StartedAt,
		CompletedAt:           r.CompletedAt,
		CreatedAt:             r.CreatedAt,
		UpdatedAt:             r.UpdatedAt,
		ParentWorkflowRunID:   r.ParentWorkflowRunID,
		RootWorkflowRunID:     r.RootWorkflowRunID,
		FailurePipelineBudget: r.FailurePipelineBudget,
		FailurePipelinesUsed:  r.FailurePipelinesUsed,
		CountedChildRunIDs:    stringsToJSON(r.CountedChildRunIDs),
		CancelledBy:           r.CancelledBy,
		CancelReason:          r.CancelReason,
		CancellingUntil:       r.CancellingUntil,
		LastEventAt:           r.LastEventAt,
		TimeoutPublished:      r.TimeoutPublished,
		PausedAt:              r.PausedAt,
		CurrentStepID:         r.CurrentStepID,
		CurrentStepStartedAt:  r.CurrentStepStartedAt,
	}
}

// WorkflowRunFromRow converts a row back to the domain WorkflowRun.
func WorkflowRunFromRow(r WorkflowRunRow) models.WorkflowRun {
	return models.WorkflowRun{
		ID:                    r.ID,
		Name:                  r.Name,
		Status:                models.WorkflowRunStatus(r.Status),
		TriggerEvent:          r.TriggerEvent,
		Initiator:             r.Initiator,
		Notes:                 r.Notes,
		AgentRunIDs:           jsonToStrings(r.AgentRunIDs),
		FunctionJobIDs:        jsonToStrings(r.FunctionJobIDs),
		BranchNames:           jsonToStrings(r.BranchNames),
		TerminalEvent:         r.TerminalEvent,
		StartedAt:             r.StartedAt,
		CompletedAt:           r.CompletedAt,
		CreatedAt:             r.CreatedAt,
		UpdatedAt:             r.UpdatedAt,
		ParentWorkflowRunID:   r.ParentWorkflowRunID,
		RootWorkflowRunID:     r.RootWorkflowRunID,
		FailurePipelineBudget: r.FailurePipelineBudget,
		FailurePipelinesUsed:  r.FailurePipelinesUsed,
		CountedChildRunIDs:    jsonToStrings(r.CountedChildRunIDs),
		CancelledBy:           r.CancelledBy,
		CancelReason:          r.CancelReason,
		CancellingUntil:       r.CancellingUntil,
		LastEventAt:           r.LastEventAt,
		TimeoutPublished:      r.TimeoutPublished,
		PausedAt:              r.PausedAt,
		CurrentStepID:         r.CurrentStepID,
		CurrentStepStartedAt:  r.CurrentStepStartedAt,
	}
}
