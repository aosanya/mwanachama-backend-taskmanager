package gormstore

import (
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProjectRow is the GORM row for a [models.Project].
type ProjectRow struct {
	ID          string `gorm:"primaryKey"`
	Name        string
	ProjectName string `gorm:"index"`
	Description string
	RepoName    string
	GithubRepo  string
	TaskPrefix  string
	CreatedAt   string
	UpdatedAt   string

	// Deleted marks a soft-deleted project — see TaskRow.Deleted's doc.
	Deleted bool `gorm:"index"`
}

func (r *ProjectRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

func ProjectToRow(p models.Project) ProjectRow {
	return ProjectRow{
		ID:          p.ID,
		Name:        p.Name,
		ProjectName: p.ProjectName,
		Description: p.Description,
		RepoName:    p.RepoName,
		GithubRepo:  p.GithubRepo,
		TaskPrefix:  p.TaskPrefix,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func ProjectFromRow(r ProjectRow) models.Project {
	return models.Project{
		ID:          r.ID,
		Name:        r.Name,
		ProjectName: r.ProjectName,
		Description: r.Description,
		RepoName:    r.RepoName,
		GithubRepo:  r.GithubRepo,
		TaskPrefix:  r.TaskPrefix,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
