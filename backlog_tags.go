package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

var ErrTagNotOfGroup = errors.New("the tag is not one of the assumed group's tags")

type GroupTags func(ctx context.Context, callerID string) ([]string, error)

func (m *taskManager) UseGroupTags(g GroupTags) {
	m.groupTags = g
}

func (m *taskManager) tagsOfCaller(ctx context.Context, requesterID string) ([]string, error) {
	if requesterID == "" || m.groupTags == nil {
		return nil, nil
	}
	tags, err := m.groupTags(ctx, requesterID)
	if err != nil {
		return nil, fmt.Errorf("backlog tags: %w", err)
	}
	return models.UnionTags(tags, nil), nil
}

func (m *taskManager) CreateBacklogAs(ctx context.Context, requesterID, name string, steps []string, rules *BacklogRules) (models.BacklogSummary, error) {
	first, err := m.tagsOfCaller(ctx, requesterID)
	if err != nil {
		return models.BacklogSummary{}, err
	}
	made, err := m.CreateBacklogWithRules(ctx, name, steps, rules)
	if err != nil || rules != nil || len(first) == 0 {
		return made, err
	}
	return m.changeBacklog(ctx, made.ID, func(b *models.Backlog) error {
		b.Tags = models.UnionTags(b.Tags, first)
		return nil
	})
}

func (m *taskManager) AddBacklogTagAs(ctx context.Context, requesterID, backlogID, tagID string) (models.BacklogSummary, error) {
	tagID = strings.TrimSpace(tagID)
	if err := m.requireEditOn(ctx, requesterID, backlogID); err != nil {
		return models.BacklogSummary{}, err
	}
	own, err := m.tagsOfCaller(ctx, requesterID)
	if err != nil {
		return models.BacklogSummary{}, err
	}
	if tagID == "" || !containsTag(own, tagID) {
		return models.BacklogSummary{}, fmt.Errorf("%w: %q", ErrTagNotOfGroup, tagID)
	}
	return m.changeBacklog(ctx, backlogID, func(b *models.Backlog) error {
		if b.Smart() {
			return fmt.Errorf("%w: a smart backlog carries no tag", ErrInvalidBacklog)
		}
		b.Tags = models.UnionTags(b.Tags, []string{tagID})
		return nil
	})
}

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}
