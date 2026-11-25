package mwanachamataskmanager

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func (m *taskManager) visibleRuleSubjects(ctx context.Context, requesterID string, needsWaits bool) ([]RuleSubject, error) {
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
		return nil, fmt.Errorf("rule subjects: %w", err)
	}
	tagged := m.tagsOfUntagged(ctx, tasks)
	subjects := make([]RuleSubject, 0, len(tasks))
	for _, t := range tasks {
		if len(t.Tags) == 0 {
			t.Tags = tagged[t.ID]
		}
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
		subjects = append(subjects, subject)
	}
	return subjects, nil
}

const subjectTagBatch = 500

func (m *taskManager) tagsOfUntagged(ctx context.Context, tasks []Task) map[string][]string {
	ids := []string{}
	for _, t := range tasks {
		if len(t.Tags) == 0 {
			ids = append(ids, t.ID)
		}
	}
	out := map[string][]string{}
	for start := 0; start < len(ids); start += subjectTagBatch {
		end := min(start+subjectTagBatch, len(ids))
		for id, names := range m.tagNamesOf(ctx, ids[start:end]) {
			out[id] = names
		}
	}
	return out
}
