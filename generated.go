package mwanachamataskmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"gorm.io/gorm"
)

type (
	TaskTemplate         = models.TaskTemplate
	GenerateTasks        = models.GenerateTasks
	GeneratedTask        = models.GeneratedTask
	GeneratedTaskOutcome = models.GeneratedTaskOutcome
	TasksGenerated       = models.TasksGenerated
)

const MaxGeneratedSubjects = 5000

var (
	ErrInvalidGeneratedTasks = errors.New("generating tasks needs a run, a template with a title, an RFC 3339 due time when one is given, and at least one subject key, none blank and none repeated")
	ErrTooManySubjects       = fmt.Errorf("generating tasks takes at most %d subject keys in one call", MaxGeneratedSubjects)
	errGeneratedTaskNotFound = errors.New("generated task not found")
)

func generatedSubjectKey(run, subject string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s%s", len(run), run, subject)))
	return hex.EncodeToString(sum[:])
}

func normalizeGenerateTasks(req GenerateTasks) (GenerateTasks, error) {
	if len(req.Subjects) > MaxGeneratedSubjects {
		return GenerateTasks{}, ErrTooManySubjects
	}
	if strings.TrimSpace(req.Run) == "" || strings.TrimSpace(req.Template.Title) == "" || len(req.Subjects) == 0 {
		return GenerateTasks{}, ErrInvalidGeneratedTasks
	}
	if req.Template.DueAt != "" {
		due, err := time.Parse(time.RFC3339, req.Template.DueAt)
		if err != nil {
			return GenerateTasks{}, fmt.Errorf("%w: due_at %q", ErrInvalidGeneratedTasks, req.Template.DueAt)
		}
		req.Template.DueAt = due.UTC().Format(time.RFC3339)
	}
	seen := make(map[string]bool, len(req.Subjects))
	for _, s := range req.Subjects {
		if strings.TrimSpace(s) == "" {
			return GenerateTasks{}, fmt.Errorf("%w: a blank subject key", ErrInvalidGeneratedTasks)
		}
		if seen[s] {
			return GenerateTasks{}, fmt.Errorf("%w: subject %q appears twice", ErrInvalidGeneratedTasks, s)
		}
		seen[s] = true
	}
	return req, nil
}

func (m *taskManager) GenerateTasks(ctx context.Context, req GenerateTasks) (TasksGenerated, error) {
	req, err := normalizeGenerateTasks(req)
	if err != nil {
		return TasksGenerated{}, err
	}
	var out TasksGenerated
	var held heldEvents
	txErr := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txm, err := m.inTx(tx, &held)
		if err != nil {
			return err
		}
		out = TasksGenerated{Items: make([]GeneratedTaskOutcome, 0, len(req.Subjects))}
		for _, subject := range req.Subjects {
			key := generatedSubjectKey(req.Run, subject)
			existing, found, err := txm.generatedTask(ctx, key)
			if err != nil {
				return err
			}
			if found {
				out.Items = append(out.Items, GeneratedTaskOutcome{GeneratedTask: existing})
				continue
			}
			task, err := txm.CreateTask(ctx, Task{
				Title:          req.Template.Title,
				Description:    req.Template.Description,
				Priority:       req.Template.Priority,
				DueAt:          req.Template.DueAt,
				EstimatedHours: req.Template.EstimatedHours,
				LinkedRef:      subject,
			})
			if err != nil {
				return err
			}
			made := GeneratedTask{
				SubjectKey:  key,
				Run:         req.Run,
				Subject:     subject,
				TaskID:      task.ID,
				GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			}
			if err := txm.store.Insert(ctx, roleGeneratedTask, made); err != nil {
				return fmt.Errorf("GenerateTasks: %w", err)
			}
			out.Items = append(out.Items, GeneratedTaskOutcome{GeneratedTask: made, Created: true})
		}
		return nil
	})
	if txErr != nil {
		return TasksGenerated{}, txErr
	}
	for _, e := range held {
		m.publish(ctx, e.topic, e.payload)
	}
	return out, nil
}

func (m *taskManager) generatedTask(ctx context.Context, key string) (GeneratedTask, bool, error) {
	var made GeneratedTask
	q := m.store.Query(ctx, roleGeneratedTask).Where("subject_key = ?", key)
	switch err := m.store.Take(q, roleGeneratedTask, &made, errGeneratedTaskNotFound); {
	case err == nil:
		return made, true, nil
	case errors.Is(err, errGeneratedTaskNotFound):
		return GeneratedTask{}, false, nil
	default:
		return GeneratedTask{}, false, fmt.Errorf("GenerateTasks: %w", err)
	}
}
