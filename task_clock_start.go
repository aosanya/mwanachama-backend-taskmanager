package mwanachamataskmanager

import (
	"context"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"strings"
	"time"
)

const (
	claimWriteWindow = 5 * time.Second
	doingStepID      = "doing"
)

func movedIntoDoing(note TaskNote) bool {
	if !note.Automatic {
		return false
	}
	match := loggedChangePattern.FindStringSubmatch(note.Text)
	if match == nil || match[1] != changeLabelMoved {
		return false
	}
	step := match[3][strings.LastIndex(match[3], "/")+1:]
	return step == inProgressStepID || step == doingStepID
}
func isWorkerEntry(note TaskNote) bool {
	return note.Kind == noteByWorker && !note.Automatic
}
func (m *taskManager) clockStartTimes(ctx context.Context, ids []string) (map[string]string, error) {
	q := m.store.Query(ctx, roleTaskNote).Where("task_id IN ?", ids).Where("(kind = ? AND automatic = ?) OR (automatic = ? AND text LIKE ?)", noteByWorker, false, true, changeLabelMoved+" % to %").Order("created_at").Limit(maxWorkerNotesRead)
	notes, err := specstore.List[TaskNote](m.store, q, roleTaskNote)
	if err != nil {
		return nil, err
	}
	byTask := map[string][]TaskNote{}
	for _, note := range notes {
		byTask[note.TaskID] = append(byTask[note.TaskID], note)
	}
	started := map[string]string{}
	for id, own := range byTask {
		if at, found := clockStartOf(own); found {
			started[id] = at
		}
	}
	return started, nil
}
func clockStartOf(notes []TaskNote) (string, bool) {
	var claimedAt time.Time
	for _, note := range notes {
		if at, ok := parsedNoteTime(note.CreatedAt); ok && movedIntoDoing(note) && at.After(claimedAt) {
			claimedAt = at
		}
	}
	earliest := claimedAt.Add(-claimWriteWindow)
	for _, note := range notes {
		at, ok := parsedNoteTime(note.CreatedAt)
		if ok && isWorkerEntry(note) && !at.Before(earliest) {
			return note.CreatedAt, true
		}
	}
	return "", false
}
