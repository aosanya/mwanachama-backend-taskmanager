package gormstore

import (
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ImportProjectJobRow is the GORM row for a [models.ImportProjectJob].
// ProgressSteps is deliberately not a column — it is in-memory-only, tracked
// by the owning goroutine while it runs (see the package's import_impl.go).
type ImportProjectJobRow struct {
	ID           string `gorm:"primaryKey"`
	Status       string
	ErrorMessage string
	TasksCreated int
	DepsCreated  int
	ProjectName  string
	CreatedAt    string
	UpdatedAt    string
}

func (r *ImportProjectJobRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

func ImportProjectJobToRow(j models.ImportProjectJob) ImportProjectJobRow {
	return ImportProjectJobRow{
		ID:           j.ID,
		Status:       j.Status,
		ErrorMessage: j.ErrorMessage,
		TasksCreated: j.TasksCreated,
		DepsCreated:  j.DepsCreated,
		ProjectName:  j.ProjectName,
		CreatedAt:    j.CreatedAt,
		UpdatedAt:    j.UpdatedAt,
	}
}

func ImportProjectJobFromRow(r ImportProjectJobRow) models.ImportProjectJob {
	return models.ImportProjectJob{
		ID:           r.ID,
		Status:       r.Status,
		ErrorMessage: r.ErrorMessage,
		TasksCreated: r.TasksCreated,
		DepsCreated:  r.DepsCreated,
		ProjectName:  r.ProjectName,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}
