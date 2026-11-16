package mwanachamataskmanager

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

var (
	issueIDShape   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	issueCodeShape = regexp.MustCompile(`^[a-z]+-?[0-9]+$`)
)

func (m *taskManager) FindTasks(ctx context.Context, filter FindFilter) (FoundTasks, error) {
	query := strings.ToLower(strings.TrimSpace(filter.Q))
	if query == "" {
		return FoundTasks{}, fmt.Errorf("FindTasks: %w: q is required, send an issue id, an issue code or words", ErrInvalidFilter)
	}
	if filter.Offset < 0 {
		return FoundTasks{}, fmt.Errorf("FindTasks: %w: offset %d", ErrInvalidFilter, filter.Offset)
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return FoundTasks{}, fmt.Errorf("FindTasks: %w", err)
	}
	countQuery, err := m.findBase(ctx, filter, query)
	if err != nil {
		return FoundTasks{}, fmt.Errorf("FindTasks: %w", err)
	}
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return FoundTasks{}, fmt.Errorf("FindTasks: %w", err)
	}
	rowQuery, err := m.findBase(ctx, filter, query)
	if err != nil {
		return FoundTasks{}, fmt.Errorf("FindTasks: %w", err)
	}
	var rows []pendingRow
	if err := rowQuery.Select(pendingColumns).
		Order(gorm.Expr("CASE WHEN LOWER(code) = ? THEN 0 ELSE 1 END", query)).Order(taskOrder).
		Limit(pageLimit(filter.Limit)).Offset(filter.Offset).Scan(&rows).Error; err != nil {
		return FoundTasks{}, fmt.Errorf("FindTasks: %w", err)
	}
	items, err := m.compactItems(ctx, backlogs, rows)
	if err != nil {
		return FoundTasks{}, fmt.Errorf("FindTasks: %w", err)
	}
	return FoundTasks{Total: int(total), Items: items}, nil
}

func (m *taskManager) findBase(ctx context.Context, filter FindFilter, query string) (*gorm.DB, error) {
	q, err := m.scopedTasks(ctx, filter.Backlog, filter.Step, filter.Tag, filter.RequesterID)
	if err != nil {
		return nil, err
	}
	if !filter.IncludeClosed {
		q = q.Where("status NOT IN ?", closedStatuses)
	}
	switch {
	case issueIDShape.MatchString(query):
		return q.Where("id = ?", query), nil
	case issueCodeShape.MatchString(query):
		return q.Where("LOWER(code) LIKE ?"+likeEscape, likeLiteral(query)+"%"), nil
	}
	for _, word := range strings.Fields(query) {
		q = m.matchWord(ctx, q, word, true)
	}
	return q, nil
}
