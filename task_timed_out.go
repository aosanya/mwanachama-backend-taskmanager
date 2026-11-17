package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const timedOutEntryRunes = 200

type TimedOutFilter struct {
	Backlog string `query:"backlog"`

	RequesterID string `query:"-" json:"-"`
}

type TimedOutEntry struct {
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

type TimedOutItem struct {
	ID                    string         `json:"id"`
	Code                  string         `json:"code"`
	Title                 string         `json:"title"`
	MinutesSinceFirst     *int64         `json:"minutes_since_first_entry"`
	CapMinutes            int64          `json:"cap_minutes"`
	MinutesSinceLastEntry *int64         `json:"minutes_since_last_entry"`
	ClockStopped          bool           `json:"clock_stopped"`
	LastEntry             *TimedOutEntry `json:"last_entry"`
}

type TimedOut struct {
	Items []TimedOutItem `json:"items"`
}

func (m *taskManager) TimedOut(ctx context.Context, filter TimedOutFilter) (TimedOut, error) {
	out := TimedOut{Items: []TimedOutItem{}}
	name := strings.TrimSpace(filter.Backlog)
	if name == "" {
		return out, fmt.Errorf("TimedOut: %w: backlog is required, send a backlog id or name", ErrInvalidFilter)
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return out, fmt.Errorf("TimedOut: %w", err)
	}
	if err := m.requireVisibleBacklog(ctx, backlogs, name, filter.RequesterID); err != nil {
		return out, fmt.Errorf("TimedOut: %w", err)
	}
	issues, err := m.inProgressIssues(ctx, PendingTodosFilter{Backlog: name, RequesterID: filter.RequesterID})
	if err != nil {
		return out, fmt.Errorf("TimedOut: %w", err)
	}
	var rows []pendingRow
	if err := issues().Select(pendingColumns).Order(taskOrder).Limit(maxListPage).Scan(&rows).Error; err != nil {
		return out, fmt.Errorf("TimedOut: %w", err)
	}
	if len(rows) == 0 {
		return out, nil
	}
	ids := rowIDs(rows)
	defaults, err := m.GetWorkerCapsDefault(ctx)
	if err != nil {
		return out, fmt.Errorf("TimedOut: %w", err)
	}
	owns, err := specstore.List[TaskCapsOverride](m.store, m.store.Query(ctx, roleTaskCaps).Where("id IN ?", ids), roleTaskCaps)
	if err != nil {
		return out, fmt.Errorf("TimedOut: %w", err)
	}
	startedOf, err := m.firstWorkerNoteTimes(ctx, ids)
	if err != nil {
		return out, fmt.Errorf("TimedOut: %w", err)
	}
	lastOf, err := m.lastNotes(ctx, ids)
	if err != nil {
		return out, fmt.Errorf("TimedOut: %w", err)
	}
	ownOf := map[string]TaskCapsOverride{}
	for _, own := range owns {
		ownOf[own.ID] = own
	}
	now := time.Now().UTC()
	for _, row := range rows {
		capSeconds := withOwn(defaults, ownOf[row.ID]).RuntimeSeconds
		item := TimedOutItem{ID: row.ID, Code: row.Code, Title: row.Title, CapMinutes: capSeconds / secondsInMinute}
		if started, ok := parsedNoteTime(startedOf[row.ID]); ok {
			if now.Sub(started) <= time.Duration(capSeconds)*time.Second {
				continue
			}
			item.MinutesSinceFirst = minutesBetween(started, now)
		}
		if last, has := lastOf[row.ID]; has {
			item.LastEntry = &TimedOutEntry{Kind: last.Kind, Text: firstRunes(last.Text, timedOutEntryRunes), CreatedAt: last.CreatedAt}
			if at, ok := parsedNoteTime(last.CreatedAt); ok {
				item.MinutesSinceLastEntry = minutesBetween(at, now)
			}
		}
		used, err := m.usageOf(ctx, row.ID)
		if err == nil {
			used, err = m.stopIfIdle(ctx, used)
		}
		if err != nil {
			return out, fmt.Errorf("TimedOut: %w", err)
		}
		item.ClockStopped = used != nil && used.ClockStopped
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (m *taskManager) lastNotes(ctx context.Context, ids []string) (map[string]TaskNote, error) {
	q := m.store.Query(ctx, roleTaskNote).Where("task_id IN ?", ids).Order("created_at").Order("id").Limit(maxWorkerNotesRead)
	notes, err := specstore.List[TaskNote](m.store, q, roleTaskNote)
	if err != nil {
		return nil, err
	}
	last := map[string]TaskNote{}
	for _, note := range notes {
		last[note.TaskID] = note
	}
	return last, nil
}

func parsedNoteTime(stamp string) (time.Time, bool) {
	if stamp == "" {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339Nano, stamp)
	return at, err == nil
}

func minutesBetween(from, to time.Time) *int64 {
	minutes := int64(to.Sub(from) / time.Minute)
	return &minutes
}

func firstRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
