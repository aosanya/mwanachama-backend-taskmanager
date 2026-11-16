package mwanachamataskmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

var ErrDuplicateRules = errors.New("a smart backlog already has these rules")

type BacklogRules = models.BacklogRules

type RulesCount struct {
	Count int `json:"count"`
}

func duplicateRulesError(b models.Backlog) error {
	body, _ := json.Marshal(map[string]string{"existing_id": b.ID, "existing_name": b.Name})
	return fmt.Errorf("%w: %s", ErrDuplicateRules, body)
}

func ruleSetOf(r BacklogRules) RuleSet {
	set := RuleSet{Match: r.Match, Rules: make([]Rule, len(r.Rules))}
	for i, rule := range r.Rules {
		set.Rules[i] = Rule{Field: rule.Field, Operator: rule.Operator, Value: rule.Value}
	}
	return set
}

func backlogRulesOf(set RuleSet) *BacklogRules {
	out := &BacklogRules{Match: set.Match, Rules: make([]models.BacklogRule, len(set.Rules))}
	for i, rule := range set.Rules {
		out.Rules[i] = models.BacklogRule{Field: rule.Field, Operator: rule.Operator, Value: rule.Value}
	}
	return out
}

func normalisedRules(r *BacklogRules) (RuleSet, string, error) {
	if r == nil {
		return RuleSet{}, "", fmt.Errorf("%w: a smart backlog needs rules", ErrInvalidBacklog)
	}
	set, err := ruleSetOf(*r).Normalise()
	if err != nil {
		return RuleSet{}, "", fmt.Errorf("%w: %s", ErrInvalidBacklog, err)
	}
	key, err := set.Key()
	if err != nil {
		return RuleSet{}, "", fmt.Errorf("%w: %s", ErrInvalidBacklog, err)
	}
	return set, key, nil
}

func smartWithKey(current []models.Backlog, key, exceptID string) (models.Backlog, bool) {
	for _, b := range current {
		if !b.Smart() || b.ID == exceptID || b.Rules == nil {
			continue
		}
		if other, err := ruleSetOf(*b.Rules).Key(); err == nil && other == key {
			return b, true
		}
	}
	return models.Backlog{}, false
}

func (m *taskManager) CreateBacklogWithRules(ctx context.Context, name string, steps []string, rules *BacklogRules) (models.BacklogSummary, error) {
	if rules == nil {
		return m.CreateBacklog(ctx, name, steps)
	}
	if len(steps) > 0 {
		return models.BacklogSummary{}, fmt.Errorf("%w: a smart backlog has rules, not steps", ErrInvalidBacklog)
	}
	set, key, err := normalisedRules(rules)
	if err != nil {
		return models.BacklogSummary{}, err
	}
	var createdID string
	err = m.saveBacklogs(ctx, func(current []models.Backlog) ([]models.Backlog, error) {
		if existing, found := smartWithKey(current, key, ""); found {
			return nil, duplicateRulesError(existing)
		}
		taken := func(id string) bool { _, ok := models.FindBacklog(current, id); return ok }
		created := models.Backlog{
			ID: uniqueID(taken, name), Name: strings.TrimSpace(name), Steps: []models.BacklogStep{},
			Kind: models.BacklogKindSmart, Rules: backlogRulesOf(set),
		}
		createdID = created.ID
		return append(append([]models.Backlog{}, current...), created), nil
	})
	if err != nil {
		return models.BacklogSummary{}, err
	}
	return m.summaryOf(ctx, createdID)
}

func (m *taskManager) SetBacklogRules(ctx context.Context, backlogID string, rules *BacklogRules) (models.BacklogSummary, error) {
	set, key, err := normalisedRules(rules)
	if err != nil {
		return models.BacklogSummary{}, err
	}
	err = m.saveBacklogs(ctx, func(current []models.Backlog) ([]models.Backlog, error) {
		i, ok := models.FindBacklog(current, backlogID)
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownBacklog, backlogID)
		}
		if !current[i].Smart() {
			return nil, fmt.Errorf("%w: %q has steps, not rules", ErrInvalidBacklog, current[i].Name)
		}
		if existing, found := smartWithKey(current, key, backlogID); found {
			return nil, duplicateRulesError(existing)
		}
		next := append([]models.Backlog{}, current...)
		next[i].Rules = backlogRulesOf(set)
		return next, nil
	})
	if err != nil {
		return models.BacklogSummary{}, err
	}
	return m.summaryOf(ctx, backlogID)
}

func (m *taskManager) CountBacklogRules(ctx context.Context, requesterID string, rules *BacklogRules) (RulesCount, error) {
	set, _, err := normalisedRules(rules)
	if err != nil {
		return RulesCount{}, err
	}
	ids, err := m.smartTaskIDs(ctx, requesterID, set)
	return RulesCount{Count: len(ids)}, err
}

func hasRuleOn(set RuleSet, field string) bool {
	for _, r := range set.Rules {
		if r.Field == field {
			return true
		}
	}
	return false
}

func (m *taskManager) smartTaskIDs(ctx context.Context, requesterID string, set RuleSet) ([]string, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return nil, err
	}
	q, err := m.narrowToVisible(ctx, m.store.Query(ctx, roleTask).Where("deleted = ?", false), requesterID)
	if err != nil {
		return nil, err
	}
	tasks, err := specstore.List[Task](m.store, q, roleTask)
	if err != nil {
		return nil, fmt.Errorf("smart backlog: %w", err)
	}
	needsWaits := hasRuleOn(set, RuleFieldWaitsOn)
	now := time.Now().UTC()
	ids := []string{}
	for _, t := range tasks {
		bid, sid := models.PlaceOf(backlogs, t)
		subject := RuleSubject{Task: t}
		if bi, ok := models.FindBacklog(backlogs, bid); ok {
			subject.BacklogName = backlogs[bi].Name
			if si, ok := models.FindStep(backlogs[bi], sid); ok {
				subject.StepName = backlogs[bi].Steps[si].Name
			}
		}
		if needsWaits {
			edges, err := m.TraverseRelationships(ctx, t.ID, RelLabelDependsOn, DirectionOutbound)
			if err != nil {
				return nil, err
			}
			subject.WaitsOn = len(edges)
		}
		matched, err := set.Matches(subject, now)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidBacklog, err)
		}
		if matched {
			ids = append(ids, t.ID)
		}
	}
	return ids, nil
}

func (m *taskManager) countSmartIssues(ctx context.Context, requesterID string, list *models.BacklogList, backlogs []models.Backlog) error {
	for i, item := range list.Items {
		bi, ok := models.FindBacklog(backlogs, item.ID)
		if !ok || !backlogs[bi].Smart() || backlogs[bi].Rules == nil {
			continue
		}
		ids, err := m.smartTaskIDs(ctx, requesterID, ruleSetOf(*backlogs[bi].Rules))
		if err != nil {
			return err
		}
		list.Items[i].Issues = len(ids)
	}
	return nil
}

func (m *taskManager) refuseMoveOnSmart(ctx context.Context, backlogID string, level models.AccessLevel) error {
	if level != models.AccessMove {
		return nil
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return err
	}
	if i, ok := models.FindBacklog(backlogs, backlogID); ok && backlogs[i].Smart() {
		return fmt.Errorf("%w: a smart backlog holds no issues of its own, so only Can see and Can edit apply", ErrInvalidBacklog)
	}
	return nil
}

func (m *taskManager) narrowToSmart(ctx context.Context, q *gorm.DB, filter TaskFilter, b models.Backlog) (*gorm.DB, error) {
	if filter.Step != "" {
		return nil, fmt.Errorf("%w: %q is a smart backlog and has no steps", ErrUnknownBacklogStep, b.Name)
	}
	if b.Rules == nil {
		return q.Where("1 = 0"), nil
	}
	ids, err := m.smartTaskIDs(ctx, filter.RequesterID, ruleSetOf(*b.Rules))
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return q.Where("1 = 0"), nil
	}
	return q.Where("id IN ?", ids), nil
}
