package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"

	"github.com/aosanya/mwanachama-backend-shared/identity"
)

const identityModule = "taskmanager"

var ErrNoStableIdentity = errors.New("taskmanager: this object has no stable identity")

var identityObjects = []string{
	roleTask, roleProject, roleAgent, roleCase, roleRepeatingTask,
	roleTaskTodo, roleWorkflowRun, roleAcceptanceCriteria,
}

var softDeletedObjects = map[string]bool{roleTask: true, roleProject: true, roleTaskTodo: true}

func hasStableIdentity(object string) bool {
	for _, o := range identityObjects {
		if o == object {
			return true
		}
	}
	return false
}

func (m *taskManager) identityRowExists(ctx context.Context, object, id string) (bool, error) {
	q := m.store.Query(ctx, object).Select("id").Where("id = ?", id)
	if softDeletedObjects[object] {
		q = q.Where("deleted = ?", false)
	}
	var rows []map[string]any
	if err := q.Limit(1).Find(&rows).Error; err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (m *taskManager) StableIdentity(ctx context.Context, object, id string) (string, error) {
	if !hasStableIdentity(object) {
		return "", fmt.Errorf("%s: %w", object, ErrNoStableIdentity)
	}
	found, err := m.identityRowExists(ctx, object, id)
	if err != nil {
		return "", fmt.Errorf("StableIdentity: %w", err)
	}
	if !found {
		return "", fmt.Errorf("%s %s: %w", object, id, identity.ErrNotFound)
	}
	raw, err := identity.New(identityModule, m.store.Spec().Instance, id)
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", object, id, err)
	}
	return raw.String(), nil
}

func (m *taskManager) Resolve(ctx context.Context, raw string) (string, string, error) {
	r, err := identity.Parse(raw)
	if err != nil {
		return "", "", err
	}
	if r.Module != identityModule || r.Instance != m.store.Spec().Instance {
		return "", "", fmt.Errorf("%s: %w", raw, identity.ErrNotOwned)
	}
	for _, object := range identityObjects {
		found, err := m.identityRowExists(ctx, object, r.Identity)
		if err != nil {
			return "", "", fmt.Errorf("Resolve: %w", err)
		}
		if found {
			return object, r.Identity, nil
		}
	}
	return "", "", fmt.Errorf("%s: %w", raw, identity.ErrNotFound)
}
