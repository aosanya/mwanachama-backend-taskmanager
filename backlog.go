package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const backlogsSetting = "backlogs"

var (
	ErrUnknownBacklogStep = errors.New("unknown backlog step")
	ErrInvalidBacklog     = errors.New("invalid backlog")
	ErrBacklogHoldsIssues = errors.New("backlog holds issues")
)

func moduleBacklogs(s *spec.Spec) ([]models.Backlog, error) {
	if _, declared := s.Setting(backlogsSetting); !declared {
		return models.DefaultBacklogs(), nil
	}
	effective, err := spec.Resolve(s.Settings, nil, nil, backlogsSetting)
	if err != nil {
		return nil, err
	}
	return models.ParseBacklogs(effective.Value)
}

func StoredBacklogs(db *gorm.DB, s *spec.Spec) ([]models.Backlog, error) {
	if _, declared := s.Setting(backlogsSetting); !declared {
		return models.DefaultBacklogs(), nil
	}
	effective, err := spec.ResolveStored(db, s, backlogsSetting)
	if err != nil {
		return nil, err
	}
	return models.ParseBacklogs(effective.Value)
}

func (m *taskManager) backlogs(ctx context.Context) ([]models.Backlog, error) {
	if m == nil || m.spec == nil {
		return models.DefaultBacklogs(), nil
	}
	backlogs, err := StoredBacklogs(m.db.WithContext(ctx), m.spec)
	if err != nil {
		return nil, fmt.Errorf("backlogs: %w", err)
	}
	return backlogs, nil
}

func (m *taskManager) placedTasks(ctx context.Context) ([]Task, error) {
	return specstore.List[Task](m.store, m.store.Query(ctx, roleTask).Where("deleted = ?", false), roleTask)
}

func (m *taskManager) fillPlaces(ctx context.Context, tasks []Task) error {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return err
	}
	for i := range tasks {
		tasks[i].Backlog, tasks[i].Step = models.PlaceOf(backlogs, tasks[i])
	}
	return nil
}

func (m *taskManager) storedPlace(ctx context.Context, taskID string) (string, string) {
	var place struct{ Backlog, Step string }
	m.store.Query(ctx, roleTask).Where("id = ?", taskID).Select("backlog", "step").Scan(&place)
	return place.Backlog, place.Step
}

func (m *taskManager) checkPlace(ctx context.Context, backlogID, stepID string) error {
	if backlogID == "" && stepID == "" {
		return nil
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return err
	}
	_, _, err = locateStep(backlogs, backlogID, stepID)
	return err
}

func locateStep(backlogs []models.Backlog, backlogID, stepID string) (models.Backlog, models.BacklogStep, error) {
	bi, ok := models.FindBacklog(backlogs, backlogID)
	if !ok {
		return models.Backlog{}, models.BacklogStep{}, fmt.Errorf("%w: %q", ErrUnknownBacklog, backlogID)
	}
	b := backlogs[bi]
	if stepID == "" {
		return b, models.BacklogStep{}, nil
	}
	si, ok := models.FindStep(b, stepID)
	if !ok {
		return models.Backlog{}, models.BacklogStep{}, fmt.Errorf("%w: %q in %q", ErrUnknownBacklogStep, stepID, b.Name)
	}
	return b, b.Steps[si], nil
}

func summarize(backlogs []models.Backlog, tasks []Task) models.BacklogList {
	counts := map[string]map[string]int{}
	totals := map[string]int{}
	for _, t := range tasks {
		bid, sid := models.PlaceOf(backlogs, t)
		if counts[bid] == nil {
			counts[bid] = map[string]int{}
		}
		counts[bid][sid]++
		totals[bid]++
	}
	out := models.BacklogList{Items: make([]models.BacklogSummary, len(backlogs))}
	for i, b := range backlogs {
		item := models.BacklogSummary{ID: b.ID, Name: b.Name, Issues: totals[b.ID], Steps: make([]models.StepSummary, len(b.Steps))}
		for j, s := range b.Steps {
			item.Steps[j] = models.StepSummary{ID: s.ID, Name: s.Name, Issues: counts[b.ID][s.ID]}
		}
		out.Items[i] = item
	}
	return out
}

func (m *taskManager) ListBacklogs(ctx context.Context) (models.BacklogList, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return models.BacklogList{}, err
	}
	tasks, err := m.placedTasks(ctx)
	if err != nil {
		return models.BacklogList{}, fmt.Errorf("ListBacklogs: %w", err)
	}
	return summarize(backlogs, tasks), nil
}

func (m *taskManager) summaryOf(ctx context.Context, backlogID string) (models.BacklogSummary, error) {
	list, err := m.ListBacklogs(ctx)
	if err != nil {
		return models.BacklogSummary{}, err
	}
	for _, item := range list.Items {
		if item.ID == backlogID {
			return item, nil
		}
	}
	return models.BacklogSummary{}, fmt.Errorf("%w: %q", ErrUnknownBacklog, backlogID)
}

func (m *taskManager) saveBacklogs(ctx context.Context, current, next []models.Backlog) error {
	if err := models.CheckBacklogs(next); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidBacklog, err)
	}
	if err := m.refuseRemovalsHoldingIssues(ctx, current, next); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := spec.StoreOverride(m.db.WithContext(ctx), m.spec, backlogsSetting, next, "taskmanager", now); err != nil {
		return fmt.Errorf("backlogs: %w", err)
	}
	return nil
}

func (m *taskManager) refuseRemovalsHoldingIssues(ctx context.Context, current, next []models.Backlog) error {
	kept := map[string]map[string]bool{}
	for _, b := range next {
		kept[b.ID] = map[string]bool{}
		for _, s := range b.Steps {
			kept[b.ID][s.ID] = true
		}
	}
	tasks, err := m.placedTasks(ctx)
	if err != nil {
		return fmt.Errorf("backlogs: %w", err)
	}
	names := map[string]string{}
	for _, b := range current {
		names[b.ID] = b.Name
		for _, s := range b.Steps {
			names[b.ID+"/"+s.ID] = s.Name
		}
	}
	for _, t := range tasks {
		bid, sid := models.PlaceOf(current, t)
		steps, backlogKept := kept[bid]
		switch {
		case !backlogKept:
			return fmt.Errorf("%w: %q still holds issues", ErrBacklogHoldsIssues, names[bid])
		case sid != "" && !steps[sid]:
			return fmt.Errorf("%w: step %q of %q still holds issues", ErrBacklogHoldsIssues, names[bid+"/"+sid], names[bid])
		}
	}
	return nil
}

func (m *taskManager) ReplaceBacklogs(ctx context.Context, backlogs []models.Backlog) (models.BacklogList, error) {
	current, err := m.backlogs(ctx)
	if err != nil {
		return models.BacklogList{}, err
	}
	if err := m.saveBacklogs(ctx, current, backlogs); err != nil {
		return models.BacklogList{}, err
	}
	return m.ListBacklogs(ctx)
}

func uniqueID(taken func(string) bool, name string) string {
	base := models.Slug(name)
	if base == "" {
		base = "item"
	}
	id := base
	for n := 2; taken(id); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

func newStep(b models.Backlog, name string) models.BacklogStep {
	taken := func(id string) bool { _, ok := models.FindStep(b, id); return ok }
	return models.BacklogStep{ID: uniqueID(taken, name), Name: strings.TrimSpace(name)}
}

func (m *taskManager) CreateBacklog(ctx context.Context, name string, steps []string) (models.BacklogSummary, error) {
	current, err := m.backlogs(ctx)
	if err != nil {
		return models.BacklogSummary{}, err
	}
	if len(steps) == 0 {
		steps = []string{"To do", "Done"}
	}
	taken := func(id string) bool { _, ok := models.FindBacklog(current, id); return ok }
	created := models.Backlog{ID: uniqueID(taken, name), Name: strings.TrimSpace(name)}
	for _, step := range steps {
		created.Steps = append(created.Steps, newStep(created, step))
	}
	next := append(append([]models.Backlog{}, current...), created)
	if err := m.saveBacklogs(ctx, current, next); err != nil {
		return models.BacklogSummary{}, err
	}
	return m.summaryOf(ctx, created.ID)
}

func (m *taskManager) changeBacklog(ctx context.Context, backlogID string, change func(*models.Backlog) error) (models.BacklogSummary, error) {
	current, err := m.backlogs(ctx)
	if err != nil {
		return models.BacklogSummary{}, err
	}
	next := make([]models.Backlog, len(current))
	for i, b := range current {
		next[i] = b
		next[i].Steps = append([]models.BacklogStep{}, b.Steps...)
	}
	i, ok := models.FindBacklog(next, backlogID)
	if !ok {
		return models.BacklogSummary{}, fmt.Errorf("%w: %q", ErrUnknownBacklog, backlogID)
	}
	if err := change(&next[i]); err != nil {
		return models.BacklogSummary{}, err
	}
	if err := m.saveBacklogs(ctx, current, next); err != nil {
		return models.BacklogSummary{}, err
	}
	return m.summaryOf(ctx, backlogID)
}

func (m *taskManager) RenameBacklog(ctx context.Context, backlogID, name string) (models.BacklogSummary, error) {
	return m.changeBacklog(ctx, backlogID, func(b *models.Backlog) error {
		b.Name = strings.TrimSpace(name)
		return nil
	})
}

func (m *taskManager) DeleteBacklog(ctx context.Context, backlogID string) error {
	current, err := m.backlogs(ctx)
	if err != nil {
		return err
	}
	i, ok := models.FindBacklog(current, backlogID)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownBacklog, backlogID)
	}
	next := append(append([]models.Backlog{}, current[:i]...), current[i+1:]...)
	return m.saveBacklogs(ctx, current, next)
}

func (m *taskManager) AddBacklogStep(ctx context.Context, backlogID, name, after string) (models.BacklogSummary, error) {
	return m.changeBacklog(ctx, backlogID, func(b *models.Backlog) error {
		at := len(b.Steps)
		if after != "" {
			i, ok := models.FindStep(*b, after)
			if !ok {
				return fmt.Errorf("%w: %q in %q", ErrUnknownBacklogStep, after, b.Name)
			}
			at = i + 1
		}
		added := newStep(*b, name)
		steps := append([]models.BacklogStep{}, b.Steps[:at]...)
		steps = append(steps, added)
		b.Steps = append(steps, b.Steps[at:]...)
		return nil
	})
}

func (m *taskManager) RenameBacklogStep(ctx context.Context, backlogID, stepID, name string) (models.BacklogSummary, error) {
	return m.changeBacklog(ctx, backlogID, func(b *models.Backlog) error {
		i, ok := models.FindStep(*b, stepID)
		if !ok {
			return fmt.Errorf("%w: %q in %q", ErrUnknownBacklogStep, stepID, b.Name)
		}
		b.Steps[i].Name = strings.TrimSpace(name)
		return nil
	})
}

func (m *taskManager) RemoveBacklogStep(ctx context.Context, backlogID, stepID string) (models.BacklogSummary, error) {
	return m.changeBacklog(ctx, backlogID, func(b *models.Backlog) error {
		i, ok := models.FindStep(*b, stepID)
		if !ok {
			return fmt.Errorf("%w: %q in %q", ErrUnknownBacklogStep, stepID, b.Name)
		}
		b.Steps = append(b.Steps[:i], b.Steps[i+1:]...)
		return nil
	})
}

func (m *taskManager) ReorderBacklogSteps(ctx context.Context, backlogID string, steps []string) (models.BacklogSummary, error) {
	return m.changeBacklog(ctx, backlogID, func(b *models.Backlog) error {
		if len(steps) != len(b.Steps) {
			return fmt.Errorf("%w: name every step of %q once", ErrInvalidBacklog, b.Name)
		}
		ordered := make([]models.BacklogStep, 0, len(steps))
		seen := map[string]bool{}
		for _, id := range steps {
			i, ok := models.FindStep(*b, id)
			if !ok {
				return fmt.Errorf("%w: %q in %q", ErrUnknownBacklogStep, id, b.Name)
			}
			if seen[id] {
				return fmt.Errorf("%w: step %q named twice", ErrInvalidBacklog, id)
			}
			seen[id] = true
			ordered = append(ordered, b.Steps[i])
		}
		b.Steps = ordered
		return nil
	})
}

func (m *taskManager) MoveTaskToStep(ctx context.Context, taskID, backlogID, stepID string) (Task, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return Task{}, err
	}
	if stepID == "" {
		return Task{}, fmt.Errorf("%w: name the step to move to", ErrUnknownBacklogStep)
	}
	_, step, err := locateStep(backlogs, backlogID, stepID)
	if err != nil {
		return Task{}, err
	}
	task, err := m.GetTask(ctx, taskID)
	if err != nil {
		return Task{}, err
	}
	if len(step.Statuses) > 0 && !coversStatus(step, task.Status) {
		path, ok := StatusPathToState(task.Status, models.TaskState{Name: step.Name, Statuses: step.Statuses})
		if !ok {
			return Task{}, fmt.Errorf("%w: a task in %s cannot reach %q", ErrNoPathToState, task.Status, step.Name)
		}
		for _, next := range path {
			task.Status = next
			if task, err = m.UpdateTask(ctx, task); err != nil {
				return Task{}, err
			}
		}
	}
	res := m.store.Query(ctx, roleTask).Where("id = ? AND deleted = ?", taskID, false).
		Updates(map[string]any{"backlog": backlogID, "step": stepID})
	if res.Error != nil {
		return Task{}, fmt.Errorf("MoveTaskToStep: %w", res.Error)
	}
	return m.GetTask(ctx, taskID)
}

func coversStatus(step models.BacklogStep, status TaskStatus) bool {
	for _, s := range step.Statuses {
		if s == status {
			return true
		}
	}
	return false
}

func (m *taskManager) placeOne(ctx context.Context, t *Task) error {
	tasks := []Task{*t}
	if err := m.fillPlaces(ctx, tasks); err != nil {
		return err
	}
	*t = tasks[0]
	return nil
}
