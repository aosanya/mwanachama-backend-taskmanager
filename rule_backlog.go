package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func (m *taskManager) backlogIssueTags(ctx context.Context, requesterID, backlogID string) ([]string, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return nil, err
	}
	if _, ok := models.FindBacklog(backlogs, backlogID); !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownBacklog, backlogID)
	}
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil {
		return nil, err
	}
	if !visible.Sees(backlogID) {
		return nil, fmt.Errorf("%w: reading the rules of a backlog needs Can see on it", ErrBacklogForbidden)
	}
	issues, err := m.ListTasks(ctx, TaskFilter{Backlog: backlogID})
	if err != nil {
		return nil, err
	}
	var tags []string
	for _, issue := range issues {
		for _, tag := range issue.Tags {
			if tag = strings.TrimSpace(tag); tag != "" && !containsFold(tags, tag) {
				tags = append(tags, tag)
			}
		}
	}
	return tags, nil
}

func (m *taskManager) listBacklogKind(ctx context.Context, requesterID, backlogID, kind string, filter RuleFilter) ([]RuleRecord, error) {
	if err := m.requireRuleAccess(ctx, requesterID, false); err != nil {
		return nil, err
	}
	tags, err := m.backlogIssueTags(ctx, requesterID, backlogID)
	if err != nil {
		return nil, err
	}
	filter.Kind, filter.Tags = kind, nil
	records, err := m.ListRules(ctx, requesterID, filter)
	if err != nil {
		return nil, err
	}
	out := []RuleRecord{}
	for _, record := range records {
		if sharesATag(tags, record.Tags) {
			out = append(out, record)
		}
	}
	return out, nil
}

func (m *taskManager) ListBacklogRules(ctx context.Context, requesterID, backlogID string, filter RuleFilter) ([]RuleRecord, error) {
	return m.listBacklogKind(ctx, requesterID, backlogID, ruleKindRule, filter)
}

func (m *taskManager) ListBacklogDecisions(ctx context.Context, requesterID, backlogID string, filter RuleFilter) ([]RuleRecord, error) {
	return m.listBacklogKind(ctx, requesterID, backlogID, ruleKindDecision, filter)
}

func (m *taskManager) ListBacklogQuestions(ctx context.Context, requesterID, backlogID string, filter RuleFilter) ([]RuleRecord, error) {
	return m.listBacklogKind(ctx, requesterID, backlogID, ruleKindQuestion, filter)
}
