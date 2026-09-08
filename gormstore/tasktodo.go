package gormstore

import (
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// TaskTodoRow is the GORM row for a [models.TaskTodo].
type TaskTodoRow struct {
	ID             string `gorm:"primaryKey"`
	Title          string
	Description    string
	Instructions   string
	Ordinality     int
	CanRunParallel bool
	DependsOn      datatypes.JSON
	Status         string
	ParentTaskID   string `gorm:"index"`
	DecompRunID    string
	AgentID        string
	Precalls       string
	CreatedAt      string
	UpdatedAt      string
	WorkflowRunID  string `gorm:"index"`
}

func (r *TaskTodoRow) BeforeCreate(_ *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

func TaskTodoToRow(t models.TaskTodo) TaskTodoRow {
	return TaskTodoRow{
		ID:             t.ID,
		Title:          t.Title,
		Description:    t.Description,
		Instructions:   t.Instructions,
		Ordinality:     t.Ordinality,
		CanRunParallel: t.CanRunParallel,
		DependsOn:      intsToJSON(t.DependsOn),
		Status:         string(t.Status),
		ParentTaskID:   t.ParentTaskID,
		DecompRunID:    t.DecompRunID,
		AgentID:        t.AgentID,
		Precalls:       t.Precalls,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
		WorkflowRunID:  t.WorkflowRunID,
	}
}

func TaskTodoFromRow(r TaskTodoRow) models.TaskTodo {
	return models.TaskTodo{
		ID:             r.ID,
		Title:          r.Title,
		Description:    r.Description,
		Instructions:   r.Instructions,
		Ordinality:     r.Ordinality,
		CanRunParallel: r.CanRunParallel,
		DependsOn:      jsonToInts(r.DependsOn),
		Status:         models.TodoStatus(r.Status),
		ParentTaskID:   r.ParentTaskID,
		DecompRunID:    r.DecompRunID,
		AgentID:        r.AgentID,
		Precalls:       r.Precalls,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
		WorkflowRunID:  r.WorkflowRunID,
	}
}
