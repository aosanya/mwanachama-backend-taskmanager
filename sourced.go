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
	SourcedTaskItem    = models.SourcedTaskItem
	OpenSourcedTasks   = models.OpenSourcedTasks
	SourcedTask        = models.SourcedTask
	SourcedTaskOutcome = models.SourcedTaskOutcome
	SourcedTasksOpened = models.SourcedTasksOpened
)

var (
	ErrInvalidSourcedTasks = errors.New("opening tasks from a source needs a source and at least one item, each with a title, an RFC 3339 due time and a key no other item in the request shares")
	errSourcedTaskNotFound = errors.New("sourced task not found")
)

func sourcedItemKey(source, key string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s%s", len(source), source, key)))
	return hex.EncodeToString(sum[:])
}

func normalizeSourcedItems(req OpenSourcedTasks) ([]SourcedTaskItem, error) {
	if strings.TrimSpace(req.Source) == "" || len(req.Items) == 0 || len(req.Items) > maxListPage {
		return nil, ErrInvalidSourcedTasks
	}
	seen := map[string]bool{}
	items := make([]SourcedTaskItem, 0, len(req.Items))
	for _, it := range req.Items {
		if strings.TrimSpace(it.Title) == "" {
			return nil, ErrInvalidSourcedTasks
		}
		due, err := time.Parse(time.RFC3339, it.DueAt)
		if err != nil {
			return nil, fmt.Errorf("%w: due_at %q", ErrInvalidSourcedTasks, it.DueAt)
		}
		it.DueAt = due.UTC().Format(time.RFC3339)
		if it.Key == "" {
			it.Key = it.Title
		}
		if seen[it.Key] {
			return nil, fmt.Errorf("%w: key %q appears twice", ErrInvalidSourcedTasks, it.Key)
		}
		seen[it.Key] = true
		items = append(items, it)
	}
	return items, nil
}

func (m *taskManager) OpenSourcedTasks(ctx context.Context, req OpenSourcedTasks) (SourcedTasksOpened, error) {
	items, err := normalizeSourcedItems(req)
	if err != nil {
		return SourcedTasksOpened{}, err
	}
	var out SourcedTasksOpened
	var held heldEvents
	txErr := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txStore, err := newStore(tx, m.store.Spec(), carriers())
		if err != nil {
			return err
		}
		txm := &taskManager{db: tx, store: txStore, publisher: &held}
		out = SourcedTasksOpened{Items: make([]SourcedTaskOutcome, 0, len(items))}
		for _, it := range items {
			itemKey := sourcedItemKey(req.Source, it.Key)
			existing, found, err := txm.sourcedTask(ctx, itemKey)
			if err != nil {
				return err
			}
			if found {
				out.Items = append(out.Items, SourcedTaskOutcome{SourcedTask: existing})
				continue
			}
			task, err := txm.CreateTask(ctx, Task{
				Title:       it.Title,
				Description: it.Description,
				DueAt:       it.DueAt,
				LinkedRef:   req.Source,
			})
			if err != nil {
				return err
			}
			made := SourcedTask{
				ItemKey:  itemKey,
				Source:   req.Source,
				Key:      it.Key,
				TaskID:   task.ID,
				DueAt:    it.DueAt,
				OpenedAt: time.Now().UTC().Format(time.RFC3339),
			}
			if err := txStore.Insert(ctx, roleSourcedTask, made); err != nil {
				return fmt.Errorf("OpenSourcedTasks: %w", err)
			}
			out.Items = append(out.Items, SourcedTaskOutcome{SourcedTask: made, Opened: true})
		}
		return nil
	})
	if txErr != nil {
		return SourcedTasksOpened{}, txErr
	}
	for _, e := range held {
		m.publish(ctx, e.topic, e.payload)
	}
	return out, nil
}

func (m *taskManager) sourcedTask(ctx context.Context, itemKey string) (SourcedTask, bool, error) {
	var made SourcedTask
	q := m.store.Query(ctx, roleSourcedTask).Where("item_key = ?", itemKey)
	switch err := m.store.Take(q, roleSourcedTask, &made, errSourcedTaskNotFound); {
	case err == nil:
		return made, true, nil
	case errors.Is(err, errSourcedTaskNotFound):
		return SourcedTask{}, false, nil
	default:
		return SourcedTask{}, false, fmt.Errorf("OpenSourcedTasks: %w", err)
	}
}
