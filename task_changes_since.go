package mwanachamataskmanager

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	changesSinceDefaultLimit = 100
	changesSinceMaxLimit     = 500
	changeCreated            = "created"
	changePriority           = "priority"
)

var priorityNotePattern = regexp.MustCompile(`^Priority (\w+) to (\w+)$`)

type ChangesSinceFilter struct {
	Cursor  string         `query:"cursor"`
	Limit   int            `query:"limit,min=1"`
	Tag     models.TagList `query:"tag"`
	Backlog string         `query:"backlog"`

	RequesterID string `query:"-" json:"-"`
}

type Change struct {
	ID    string `json:"id"`
	Code  string `json:"code"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
	At    string `json:"at"`

	moment int64
	key    string
}

type ChangesSince struct {
	Changes    []Change `json:"changes"`
	NextCursor string   `json:"next_cursor"`
	HasMore    bool     `json:"has_more"`
}

type changeCursor struct {
	moment int64
	key    string
}

func (m *taskManager) ChangesSince(ctx context.Context, filter ChangesSinceFilter) (ChangesSince, error) {
	out := ChangesSince{Changes: []Change{}}
	limit := filter.Limit
	if limit > changesSinceMaxLimit {
		return out, fmt.Errorf("ChangesSince: %w: limit %d is above %d", ErrInvalidFilter, limit, changesSinceMaxLimit)
	}
	if limit == 0 {
		limit = changesSinceDefaultLimit
	}
	cursor, fresh, err := parseChangeCursor(filter.Cursor)
	if err != nil {
		return out, fmt.Errorf("ChangesSince: %w", err)
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return out, fmt.Errorf("ChangesSince: %w", err)
	}
	backlog := strings.TrimSpace(filter.Backlog)
	if err := m.requireSeenBacklog(ctx, backlogs, NextReadyFilter{Backlog: backlog, RequesterID: filter.RequesterID}); err != nil {
		return out, fmt.Errorf("ChangesSince: %w", err)
	}
	if fresh {
		out.NextCursor = encodeChangeCursor(changeCursor{moment: time.Now().UTC().Truncate(time.Second).UnixNano()})
		return out, nil
	}
	created, err := m.createdChanges(ctx, backlog, filter, cursor, limit+1)
	if err != nil {
		return out, fmt.Errorf("ChangesSince: %w", err)
	}
	reprioritised, err := m.priorityChanges(ctx, backlog, filter, cursor, limit+1)
	if err != nil {
		return out, fmt.Errorf("ChangesSince: %w", err)
	}
	logged, err := m.loggedChanges(ctx, backlog, filter, cursor, limit+1)
	if err != nil {
		return out, fmt.Errorf("ChangesSince: %w", err)
	}
	merged := append(append(created, reprioritised...), logged...)
	slices.SortFunc(merged, compareChanges)
	if len(merged) > limit {
		merged = merged[:limit]
		out.HasMore = true
	}
	out.Changes = merged
	out.NextCursor = filter.Cursor
	if len(merged) > 0 {
		last := merged[len(merged)-1]
		out.NextCursor = encodeChangeCursor(changeCursor{moment: last.moment, key: last.key})
	}
	return out, nil
}

func (m *taskManager) createdChanges(ctx context.Context, backlog string, filter ChangesSinceFilter, cursor changeCursor, take int) ([]Change, error) {
	query, err := m.scopedTasks(ctx, backlog, "", filter.Tag, filter.RequesterID)
	if err != nil {
		return nil, err
	}
	floor := time.Unix(0, cursor.moment).UTC().Truncate(time.Second)
	after := floor.Format(time.RFC3339)
	if whole := floor.UnixNano() == cursor.moment; whole {
		query = query.Where("(created_at > ? OR (created_at = ? AND ('"+changeCreated+":' || id) > ?))", after, after, cursor.key)
	} else {
		query = query.Where("created_at > ?", after)
	}
	tasks, err := specstore.List[Task](m.store, query.Order("created_at").Order("id").Limit(take), roleTask)
	if err != nil {
		return nil, err
	}
	changes := make([]Change, 0, len(tasks))
	for _, task := range tasks {
		moment, err := time.Parse(time.RFC3339, task.CreatedAt)
		if err != nil {
			continue
		}
		changes = append(changes, Change{
			ID: task.ID, Code: task.Code, Title: task.Title, Kind: changeCreated,
			At: task.CreatedAt, moment: moment.UnixNano(), key: changeCreated + ":" + task.ID,
		})
	}
	return changes, nil
}

func (m *taskManager) priorityChanges(ctx context.Context, backlog string, filter ChangesSinceFilter, cursor changeCursor, take int) ([]Change, error) {
	visible, err := m.scopedTasks(ctx, backlog, "", filter.Tag, filter.RequesterID)
	if err != nil {
		return nil, err
	}
	after := time.Unix(0, cursor.moment).UTC().Format(noteTimeForm)
	query := m.store.Query(ctx, roleTaskNote).
		Where("kind = ? AND automatic = ? AND text LIKE ?", noteByWorker, false, "Priority % to %").
		Where("task_id IN (?)", visible.Select("id")).
		Where("(created_at > ? OR (created_at = ? AND ('"+changePriority+":' || id) > ?))", after, after, cursor.key).
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
		parts := priorityNotePattern.FindStringSubmatch(note.Text)
		moment, err := time.Parse(time.RFC3339Nano, note.CreatedAt)
		if parts == nil || err != nil {
			continue
		}
		issue := issues[note.TaskID]
		changes = append(changes, Change{
			ID: note.TaskID, Code: issue.Code, Title: issue.Title, Kind: changePriority,
			From: parts[1], To: parts[2], At: note.CreatedAt, moment: moment.UnixNano(), key: changePriority + ":" + note.ID,
		})
	}
	return changes, nil
}

func (m *taskManager) issuesByID(ctx context.Context, notes []TaskNote) (map[string]Task, error) {
	ids := make([]string, 0, len(notes))
	for _, note := range notes {
		ids = append(ids, note.TaskID)
	}
	found := map[string]Task{}
	if len(ids) == 0 {
		return found, nil
	}
	tasks, err := specstore.List[Task](m.store, m.store.Query(ctx, roleTask).Where("id IN ?", ids), roleTask)
	if err != nil {
		return nil, err
	}
	for _, task := range tasks {
		found[task.ID] = task
	}
	return found, nil
}

func compareChanges(a, b Change) int {
	switch {
	case a.moment < b.moment:
		return -1
	case a.moment > b.moment:
		return 1
	}
	return strings.Compare(a.key, b.key)
}

func encodeChangeCursor(cursor changeCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(cursor.moment, 10) + "|" + cursor.key))
}

func parseChangeCursor(raw string) (changeCursor, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return changeCursor{}, true, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	moment, key, found := strings.Cut(string(decoded), "|")
	nanos, parseErr := strconv.ParseInt(moment, 10, 64)
	if err != nil || !found || parseErr != nil || nanos < 0 {
		return changeCursor{}, false, fmt.Errorf("%w: the cursor %q is not one this call returned", ErrInvalidFilter, raw)
	}
	return changeCursor{moment: nanos, key: key}, false, nil
}
