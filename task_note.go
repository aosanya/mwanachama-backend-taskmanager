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
	noteByPerson  = "person"
	noteByWorker  = "worker"
	noteAutomatic = "automatic"
	maxNoteLimit  = 200
	noteTimeForm  = "2006-01-02T15:04:05.000000000Z07:00"
)

var ErrInvalidTaskNote = errors.New("a note needs text, and its kind is person or worker")

var ErrInvalidTaskNoteFilter = errors.New("invalid note filter")

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
	return m.allTaskNotes(ctx, taskID)
}

func (m *taskManager) allTaskNotes(ctx context.Context, taskID string) ([]TaskNote, error) {
	q := m.store.Query(ctx, roleTaskNote).Where("task_id = ?", taskID).Order("created_at").Order("id").Limit(maxListPage)
	out, err := specstore.List[TaskNote](m.store, q, roleTaskNote)
	if err != nil {
		return nil, fmt.Errorf("ListTaskNotes: %w", err)
	}
	return out, nil
}

func (m *taskManager) ListTaskNotesFiltered(ctx context.Context, filter TaskNoteFilter) ([]TaskNote, error) {
	if filter.Limit < 0 || filter.Limit > maxNoteLimit {
		return nil, fmt.Errorf("limit %d is not between 1 and %d: %w", filter.Limit, maxNoteLimit, ErrInvalidTaskNoteFilter)
	}
	kinds, err := noteKindSet(filter.Kind)
	if err != nil {
		return nil, err
	}
	if _, err := m.GetTaskFor(ctx, filter.RequesterID, filter.TaskID); err != nil {
		return nil, err
	}
	all, err := m.allTaskNotes(ctx, filter.TaskID)
	if err != nil {
		return nil, err
	}
	newer, err := notesAfter(all, filter.After)
	if err != nil {
		return nil, err
	}
	kept := newer[:0:0]
	for _, note := range newer {
		if len(kinds) == 0 || kinds[noteKindOf(note)] {
			kept = append(kept, note)
		}
	}
	if filter.Limit > 0 && len(kept) > filter.Limit {
		kept = kept[len(kept)-filter.Limit:]
	}
	return kept, nil
}

func noteKindOf(note TaskNote) string {
	if note.Automatic {
		return noteAutomatic
	}
	return note.Kind
}

func noteKindSet(kinds []string) (map[string]bool, error) {
	set := map[string]bool{}
	for _, kind := range kinds {
		if kind != noteByPerson && kind != noteByWorker && kind != noteAutomatic {
			return nil, fmt.Errorf("kind %q is not person, worker or automatic: %w", kind, ErrInvalidTaskNoteFilter)
		}
		set[kind] = true
	}
	return set, nil
}

func notesAfter(notes []TaskNote, after string) ([]TaskNote, error) {
	after = strings.TrimSpace(after)
	if after == "" {
		return notes, nil
	}
	for i, note := range notes {
		if note.ID == after {
			return notes[i+1:], nil
		}
	}
	moment, err := time.Parse(time.RFC3339, after)
	if err != nil {
		return nil, fmt.Errorf("after %q is neither a note of this issue nor an RFC 3339 time: %w", after, ErrInvalidTaskNoteFilter)
	}
	cut := moment.UTC().Format(noteTimeForm)
	for i, note := range notes {
		if note.CreatedAt > cut {
			return notes[i:], nil
		}
	}
	return nil, nil
}
