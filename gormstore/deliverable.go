package gormstore

import (
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DeliverableRow is the GORM row for a [models.Deliverable]. ParentID is a
// polymorphic owner reference (Task or TaskTodo) — plain indexed column, no
// FK constraint.
type DeliverableRow struct {
	ID              string `gorm:"primaryKey"`
	Code            string `gorm:"uniqueIndex"`
	Title           string
	Description     string
	DeliverableType string
	ParentID        string `gorm:"index"`
	Ordinality      int
	WorkflowRunID   string `gorm:"index"`
	CreatedAt       string
	UpdatedAt       string
}

func (r *DeliverableRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

func DeliverableToRow(d models.Deliverable) DeliverableRow {
	return DeliverableRow{
		ID:              d.ID,
		Code:            d.Code,
		Title:           d.Title,
		Description:     d.Description,
		DeliverableType: d.DeliverableType,
		ParentID:        d.ParentID,
		Ordinality:      d.Ordinality,
		WorkflowRunID:   d.WorkflowRunID,
		CreatedAt:       d.CreatedAt,
		UpdatedAt:       d.UpdatedAt,
	}
}

func DeliverableFromRow(r DeliverableRow) models.Deliverable {
	return models.Deliverable{
		ID:              r.ID,
		Code:            r.Code,
		Title:           r.Title,
		Description:     r.Description,
		DeliverableType: r.DeliverableType,
		ParentID:        r.ParentID,
		Ordinality:      r.Ordinality,
		WorkflowRunID:   r.WorkflowRunID,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
	}
}
