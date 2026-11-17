package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	pendingDefaultLimit = 25
	pendingMaxLimit     = 100
	noStepName          = "No step"
	likeEscape          = " ESCAPE '\\'"
	pendingColumns      = `id, code, title, status, priority, backlog, step, "rank"`
)

var priorityOrder = []TaskPriority{
	models.TaskPriorityCritical, models.TaskPriorityHigh, models.TaskPriorityMedium, models.TaskPriorityLow,
}

type pendingRow struct {
	ID       string
	Code     string
	Title    string
	Status   string
	Priority string
	Backlog  string
	Step     string
	Rank     int
}

type pendingTally struct {
	Backlog  string
	Step     string
	Status   string
	Priority string
	N        int
}

func (m *taskManager) PendingTasks(ctx context.Context, filter PendingFilter) (PendingTasks, error) {
	if filter.Offset < 0 {
		return PendingTasks{}, fmt.Errorf("PendingTasks: %w: offset %d", ErrInvalidFilter, filter.Offset)
	}
	limit := pageLimit(filter.Limit)
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return PendingTasks{}, fmt.Errorf("PendingTasks: %w", err)
	}
	tallies, err := m.pendingTallies(ctx, filter, false)
	if err != nil {
		return PendingTasks{}, fmt.Errorf("PendingTasks: %w", err)
	}
	var closedTallies []pendingTally
	if filter.IncludeClosed {
		if closedTallies, err = m.pendingTallies(ctx, filter, true); err != nil {
			return PendingTasks{}, fmt.Errorf("PendingTasks: %w", err)
		}
	}
	out := PendingTasks{Items: []PendingItem{}}
	out.ByStep, out.ByPriority, out.Total = tally(backlogs, tallies, closedTallies)

	rowQuery, err := m.pendingBase(ctx, filter, false)
	if err != nil {
		return PendingTasks{}, fmt.Errorf("PendingTasks: %w", err)
	}
	var rows []pendingRow
	if err := rowQuery.Select(pendingColumns).Order(sortedBy(filter.Sort, taskOrder)).Limit(limit).Offset(filter.Offset).Scan(&rows).Error; err != nil {
		return PendingTasks{}, fmt.Errorf("PendingTasks: %w", err)
	}
	if out.Items, err = m.compactItems(ctx, backlogs, rows); err != nil {
		return PendingTasks{}, fmt.Errorf("PendingTasks: %w", err)
	}
	return out, nil
}

func pageLimit(asked int) int {
	if asked <= 0 {
		return pendingDefaultLimit
	}
	return min(asked, pendingMaxLimit)
}

func (m *taskManager) compactItems(ctx context.Context, backlogs []models.Backlog, rows []pendingRow) ([]PendingItem, error) {
	states, err := m.taskStates(ctx)
	if err != nil {
		return nil, err
	}
	tags := m.tagNamesOf(ctx, rowIDs(rows))
	items := []PendingItem{}
	for _, r := range rows {
		backlogName, stepName := placeNames(backlogs, r)
		items = append(items, PendingItem{
			ID: r.ID, Code: r.Code, Title: r.Title, State: models.StateNameFor(states, TaskStatus(r.Status)),
			Backlog: backlogName, Step: stepName, Priority: TaskPriority(r.Priority),
			Tags: tags[r.ID], Rank: r.Rank, Blocked: isBlocked(tags[r.ID]), Waiting: isWaiting(tags[r.ID]), Sinbin: isSinbin(tags[r.ID]),
		})
	}
	return items, nil
}

func (m *taskManager) pendingTallies(ctx context.Context, filter PendingFilter, closed bool) ([]pendingTally, error) {
	query, err := m.pendingBase(ctx, filter, closed)
	if err != nil {
		return nil, err
	}
	var tallies []pendingTally
	err = query.Select("backlog, step, status, priority, COUNT(*) AS n").
		Group("backlog, step, status, priority").Scan(&tallies).Error
	return tallies, err
}

func (m *taskManager) pendingBase(ctx context.Context, filter PendingFilter, closed bool) (*gorm.DB, error) {
	q, err := m.scopedTasks(ctx, filter.Backlog, filter.Step, filter.Tag, filter.RequesterID)
	if err != nil {
		return nil, err
	}
	if closed {
		q = q.Where("status IN ?", closedStatuses)
	} else {
		q = q.Where("status NOT IN ?", closedStatuses)
	}
	if filter.Priority != "" {
		q = q.Where("priority = ?", string(filter.Priority))
	}
	for _, word := range strings.Fields(strings.ToLower(filter.Q)) {
		q = m.matchWord(ctx, q, word, false)
	}
	return m.narrowOrchestration(ctx, q, filter.Orchestration())
}

func (m *taskManager) scopedTasks(ctx context.Context, backlog, step string, tags []string, requesterID string) (*gorm.DB, error) {
	q := m.store.Query(ctx, roleTask).Where("deleted = ?", false)
	q, err := m.narrowToPlace(ctx, q, TaskFilter{Backlog: backlog, Step: step})
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

func (m *taskManager) matchWord(ctx context.Context, q *gorm.DB, word string, inDescription bool) *gorm.DB {
	pattern := "%" + likeLiteral(word) + "%"
	tagged := m.store.Query(ctx, roleTagging).Select("task_id").
		Where("tag_id IN (?)", m.store.Query(ctx, roleTag).Select("id").Where("LOWER(name) LIKE ?"+likeEscape, pattern))
	if inDescription {
		return q.Where("(LOWER(title) LIKE ?"+likeEscape+" OR LOWER(code) LIKE ?"+likeEscape+" OR LOWER(description) LIKE ?"+likeEscape+" OR id IN (?))", pattern, pattern, pattern, tagged)
	}
	return q.Where("(LOWER(title) LIKE ?"+likeEscape+" OR LOWER(code) LIKE ?"+likeEscape+" OR id IN (?))", pattern, pattern, tagged)
}

func likeLiteral(word string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(word)
}

func tally(backlogs []models.Backlog, tallies, closedTallies []pendingTally) (byStep, byPriority []PendingCount, total int) {
	stepCounts := map[string]int{}
	priorityCounts := map[string]int{}
	for _, t := range tallies {
		_, step := placeNames(backlogs, pendingRow{Status: t.Status, Backlog: t.Backlog, Step: t.Step})
		stepCounts[step] += t.N
		priorityCounts[t.Priority] += t.N
		total += t.N
	}
	for _, t := range closedTallies {
		_, step := placeNames(backlogs, pendingRow{Status: t.Status, Backlog: t.Backlog, Step: t.Step})
		stepCounts[step] += t.N
	}
	byStep = []PendingCount{}
	seen := map[string]bool{}
	for _, b := range backlogs {
		for _, s := range b.Steps {
			if n, ok := stepCounts[s.Name]; ok && !seen[s.Name] {
				seen[s.Name] = true
				byStep = append(byStep, PendingCount{Name: s.Name, Count: n})
			}
		}
	}
	if n, ok := stepCounts[noStepName]; ok {
		byStep = append(byStep, PendingCount{Name: noStepName, Count: n})
	}
	byPriority = []PendingCount{}
	for _, p := range priorityOrder {
		if n, ok := priorityCounts[string(p)]; ok {
			byPriority = append(byPriority, PendingCount{Name: string(p), Count: n})
			delete(priorityCounts, string(p))
		}
	}
	for name, n := range priorityCounts {
		byPriority = append(byPriority, PendingCount{Name: name, Count: n})
	}
	return byStep, byPriority, total
}

func placeNames(backlogs []models.Backlog, r pendingRow) (string, string) {
	backlogID, stepID := models.PlaceOf(backlogs, models.Task{Backlog: r.Backlog, Step: r.Step, Status: TaskStatus(r.Status)})
	bi, ok := models.FindBacklog(backlogs, backlogID)
	if !ok {
		return backlogID, noStepName
	}
	step := noStepName
	if si, found := models.FindStep(backlogs[bi], stepID); found {
		step = backlogs[bi].Steps[si].Name
	}
	return backlogs[bi].Name, step
}

func rowIDs(rows []pendingRow) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

func (m *taskManager) tagNamesOf(ctx context.Context, taskIDs []string) map[string][]string {
	out := map[string][]string{}
	if len(taskIDs) == 0 {
		return out
	}
	var links []struct{ TaskID, TagID string }
	if err := m.store.Query(ctx, roleTagging).Select("task_id, tag_id").Where("task_id IN ?", taskIDs).Scan(&links).Error; err != nil || len(links) == 0 {
		return out
	}
	tagIDs := make([]string, len(links))
	for i, l := range links {
		tagIDs[i] = l.TagID
	}
	var tags []struct{ ID, Name string }
	if err := m.store.Query(ctx, roleTag).Select("id, name").Where("id IN ?", tagIDs).Scan(&tags).Error; err != nil {
		return out
	}
	names := make(map[string]string, len(tags))
	for _, t := range tags {
		names[t.ID] = t.Name
	}
	for _, l := range links {
		if name, ok := names[l.TagID]; ok {
			out[l.TaskID] = append(out[l.TaskID], name)
		}
	}
	return out
}

func isBlocked(tags []string) bool {
	return hasTagFold(tags, tagBlocked)
}

func isSinbin(tags []string) bool {
	return hasTagFold(tags, tagSinbin) && !isBlocked(tags)
}

func isWaiting(tags []string) bool {
	return hasTagFold(tags, tagWaiting) && !isBlocked(tags) && !isSinbin(tags)
}

func markOfTags(tags []string) string {
	switch {
	case isBlocked(tags):
		return tagBlocked
	case isSinbin(tags):
		return tagSinbin
	case isWaiting(tags):
		return tagWaiting
	case hasTagFold(tags, tagReady):
		return tagReady
	}
	return ""
}

func hasTagFold(tags []string, name string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, name) {
			return true
		}
	}
	return false
}
