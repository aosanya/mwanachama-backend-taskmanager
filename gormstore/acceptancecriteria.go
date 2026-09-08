package gormstore

import (
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AcceptanceCriteriaRow is the GORM row for a [models.AcceptanceCriteria].
// ParentID is a polymorphic owner reference (Task or TaskTodo) — plain
// indexed column, no FK constraint.
type AcceptanceCriteriaRow struct {
	ID            string `gorm:"primaryKey"`
	Title         string
	Description   string
	ParentID      string `gorm:"index"`
	Ordinality    int
	WorkflowRunID string `gorm:"index"`
	Result        string
	ResultNotes   string
	CreatedAt     string
	UpdatedAt     string
}

func (r *AcceptanceCriteriaRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

func AcceptanceCriteriaToRow(a models.AcceptanceCriteria) AcceptanceCriteriaRow {
	return AcceptanceCriteriaRow{
		ID:            a.ID,
		Title:         a.Title,
		Description:   a.Description,
		ParentID:      a.ParentID,
		Ordinality:    a.Ordinality,
		WorkflowRunID: a.WorkflowRunID,
		Result:        a.Result,
		ResultNotes:   a.ResultNotes,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
}

func AcceptanceCriteriaFromRow(r AcceptanceCriteriaRow) models.AcceptanceCriteria {
	return models.AcceptanceCriteria{
		ID:            r.ID,
		Title:         r.Title,
		Description:   r.Description,
		ParentID:      r.ParentID,
		Ordinality:    r.Ordinality,
		WorkflowRunID: r.WorkflowRunID,
		Result:        r.Result,
		ResultNotes:   r.ResultNotes,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}
