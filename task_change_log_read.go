package mwanachamataskmanager

import (
	"context"
	"regexp"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const (
	changeLogged     = "log"
	changeMoved      = "moved"
	changeAssigned   = "assigned"
	changeUnassigned = "unassigned"
	changeStatus     = "status"
	changeTags       = "tags"
	changeDeleted    = "deleted"
)

var loggedChangePattern = regexp.MustCompile(`^(Moved|Assignee|Status|Tags) (.*?) to (.*)$`)

var loggedChangeTexts = []any{
	changeLabelMoved + " % to %",
	changeLabelAssignee + " % to %",
	changeLabelStatus + " % to %",
	changeLabelTags + " % to %",
	changeTextDeleted,
}

func (m *taskManager) loggedChanges(ctx context.Context, backlog string, filter ChangesSinceFilter, cursor changeCursor, take int) ([]Change, error) {
	visible, err := m.scopedTasksWithDeleted(ctx, backlog, filter.Tag, filter.RequesterID)
	if err != nil {
		return nil, err
	}
	after := time.Unix(0, cursor.moment).UTC().Format(noteTimeForm)
	query := m.store.Query(ctx, roleTaskNote).
		Where("automatic = ? AND (text LIKE ? OR text LIKE ? OR text LIKE ? OR text LIKE ? OR text = ?)", append([]any{true}, loggedChangeTexts...)...).
		Where("task_id IN (?)", visible.Select("id")).
		Where("(created_at > ? OR (created_at = ? AND ('"+changeLogged+":' || id) > ?))", after, after, cursor.key).
		Order("created_at").Order("id").Limit(take)
	notes, err := specstore.List[TaskNote](m.store, query, roleTaskNote)
	if err != nil {
		return nil, err
	}
	issues, err := m.issuesByID(ctx, notes)
	if err != nil {
		return nil, err
	}
	changes := make([]Change, 0, len(notes))
	for _, note := range notes {
		moment, err := time.Parse(time.RFC3339Nano, note.CreatedAt)
		if err != nil {
			continue
		}
		kind, from, to, ok := readLoggedChange(note.Text)
		if !ok {
			continue
		}
		issue := issues[note.TaskID]
		changes = append(changes, Change{
			ID: note.TaskID, Code: issue.Code, Title: issue.Title, Kind: kind,
			From: from, To: to, At: note.CreatedAt, moment: moment.UnixNano(), key: changeLogged + ":" + note.ID,
		})
	}
	return changes, nil
}

func (m *taskManager) scopedTasksWithDeleted(ctx context.Context, backlog string, tags []string, requesterID string) (*gorm.DB, error) {
	q, err := m.narrowToPlace(ctx, m.store.Query(ctx, roleTask), TaskFilter{Backlog: backlog})
	if err != nil {
		return nil, err
	}
	q, err = m.narrowToVisible(ctx, q, requesterID)
	if err != nil {
		return nil, err
	}
	for _, tag := range tags {
		if tag != "" {
			q = q.Where("id IN (?)", m.taskIDsWithTag(ctx, tag))
		}
	}
	return q, nil
}

func readLoggedChange(text string) (kind, from, to string, ok bool) {
	if text == changeTextDeleted {
		return changeDeleted, "", "", true
	}
	parts := loggedChangePattern.FindStringSubmatch(text)
	if parts == nil {
		return "", "", "", false
	}
	from, to = noneAsEmpty(parts[2]), noneAsEmpty(parts[3])
	switch parts[1] {
	case changeLabelMoved:
		return changeMoved, from, to, true
	case changeLabelStatus:
		return changeStatus, from, to, true
	case changeLabelTags:
		return changeTags, from, to, true
	}
	if to == "" {
		return changeUnassigned, from, to, true
	}
	return changeAssigned, from, to, true
}

func noneAsEmpty(value string) string {
	if value == changeNone {
		return ""
	}
	return value
}
