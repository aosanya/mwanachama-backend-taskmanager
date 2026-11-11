package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const (
	noteByPerson = "person"
	noteByWorker = "worker"
	noteTimeForm = "2006-01-02T15:04:05.000000000Z07:00"
)

var ErrInvalidTaskNote = errors.New("a note needs text, and its kind is person or worker")

func (m *taskManager) AddTaskNote(ctx context.Context, requesterID, taskID, kind, text string, links []string) (TaskNote, error) {
	if kind == "" {
		kind = noteByPerson
	}
	text = strings.TrimSpace(text)
	if text == "" || (kind != noteByPerson && kind != noteByWorker) {
		return TaskNote{}, ErrInvalidTaskNote
	}
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return TaskNote{}, err
	}
	note := TaskNote{
		ID: newSpecID(), TaskID: taskID, AuthorID: requesterID, Kind: kind,
		Text: text, Links: keptLinks(links), CreatedAt: time.Now().UTC().Format(noteTimeForm),
	}
	if err := m.store.Insert(ctx, roleTaskNote, note); err != nil {
		return TaskNote{}, fmt.Errorf("AddTaskNote: %w", err)
	}
	return note, nil
}

func keptLinks(links []string) []string {
	var kept []string
	for _, link := range links {
		if link = strings.TrimSpace(link); link != "" {
			kept = append(kept, link)
		}
	}
	return kept
}

func (m *taskManager) ListTaskNotes(ctx context.Context, requesterID, taskID string) ([]TaskNote, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return nil, err
	}
	q := m.store.Query(ctx, roleTaskNote).Where("task_id = ?", taskID).Order("created_at").Order("id").Limit(maxListPage)
	out, err := specstore.List[TaskNote](m.store, q, roleTaskNote)
	if err != nil {
		return nil, fmt.Errorf("ListTaskNotes: %w", err)
	}
	return out, nil
}
