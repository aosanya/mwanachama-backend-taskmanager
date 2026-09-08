package gormstore

import (
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AgentRow is the GORM row for a [models.Agent]. AgentID carries the
// natural-key uniqueness UpsertAgent relies on for find-or-create.
type AgentRow struct {
	ID          string `gorm:"primaryKey"`
	AgentID     string `gorm:"uniqueIndex"`
	DisplayName string
	Capability  string
	RoleName    string
	CreatedAt   string
	UpdatedAt   string
}

func (r *AgentRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

func AgentToRow(a models.Agent) AgentRow {
	return AgentRow{
		ID:          a.ID,
		AgentID:     a.AgentID,
		DisplayName: a.DisplayName,
		Capability:  a.Capability,
		RoleName:    a.RoleName,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
}

func AgentFromRow(r AgentRow) models.Agent {
	return models.Agent{
		ID:          r.ID,
		AgentID:     r.AgentID,
		DisplayName: r.DisplayName,
		Capability:  r.Capability,
		RoleName:    r.RoleName,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
