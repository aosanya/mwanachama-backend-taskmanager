package gormstore

import (
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TagRow is the GORM row for a [models.Tag]. Name is unique — the natural
// key UpsertEntity's original find-or-create relied on.
type TagRow struct {
	ID          string `gorm:"primaryKey"`
	Name        string `gorm:"uniqueIndex"`
	Color       string
	Description string
	CreatedAt   string
	UpdatedAt   string
}

func (r *TagRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

func TagToRow(t models.Tag) TagRow {
	return TagRow{
		ID:          t.ID,
		Name:        t.Name,
		Color:       t.Color,
		Description: t.Description,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

func TagFromRow(r TagRow) models.Tag {
	return models.Tag{
		ID:          r.ID,
		Name:        r.Name,
		Color:       r.Color,
		Description: r.Description,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
