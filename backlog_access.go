package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	accessSetting     = "backlog_access"
	ownerGroupSetting = "backlog_owner_group"
)

var (
	ErrBacklogForbidden = errors.New("backlog access forbidden")
	ErrOwnerGroupNotSet = errors.New("the Product owner group is not set: call set_backlog_owner_group first")
)

type Groups func(ctx context.Context, callerID string) ([]string, error)

type GroupExists func(ctx context.Context, groupID string) (bool, error)

func (m *taskManager) UseGroups(g Groups) { m.groups = g }

func (m *taskManager) UseGroupExists(g GroupExists) { m.groupExists = g }

func (m *taskManager) UseRoleSync(r RoleSync) { m.roleSync = r }

type backlogVisibility struct {
	all    bool
	levels map[string]models.AccessLevel
}

func (v backlogVisibility) Sees(backlogID string) bool {
	return v.all || v.levels[backlogID] != ""
}

func (v backlogVisibility) Moves(backlogID string) bool {
	return v.all || v.levels[backlogID] == models.AccessMove
}

func (m *taskManager) storedGrants(ctx context.Context) ([]models.BacklogGrant, error) {
	return m.grantsIn(m.db.WithContext(ctx))
}

func (m *taskManager) grantsIn(db *gorm.DB) ([]models.BacklogGrant, error) {
	if _, declared := m.spec.Setting(accessSetting); !declared {
		return nil, nil
	}
	effective, err := spec.ResolveStored(db, m.spec, accessSetting)
	if err != nil {
		return nil, fmt.Errorf("backlog access: %w", err)
	}
	return models.ParseGrants(effective.Value)
}

func (m *taskManager) storedOwnerGroup(ctx context.Context) (string, error) {
	return m.ownerGroupIn(m.db.WithContext(ctx))
}

func (m *taskManager) ownerGroupIn(db *gorm.DB) (string, error) {
	if _, declared := m.spec.Setting(ownerGroupSetting); !declared {
		return "", nil
	}
	effective, err := spec.ResolveStored(db, m.spec, ownerGroupSetting)
	if err != nil {
		return "", fmt.Errorf("backlog owner group: %w", err)
	}
	group, _ := effective.Value.(string)
	return group, nil
}

func (m *taskManager) storeSetting(ctx context.Context, name string, value any) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := spec.StoreOverride(m.db.WithContext(ctx), m.spec, name, value, "taskmanager", now); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func (m *taskManager) backlogVisibility(ctx context.Context, requesterID string) (backlogVisibility, error) {
	if requesterID == "" {
		return backlogVisibility{all: true}, nil
	}
	grants, err := m.storedGrants(ctx)
	if err != nil {
		return backlogVisibility{}, err
	}
	owner, err := m.storedOwnerGroup(ctx)
	if err != nil {
		return backlogVisibility{}, err
	}
	if owner == "" && len(grants) == 0 {
		return backlogVisibility{all: true}, nil
	}
	if m.groups == nil {
		return backlogVisibility{levels: map[string]models.AccessLevel{}}, nil
	}
	held, err := m.groups(ctx, requesterID)
	if err != nil {
		return backlogVisibility{}, fmt.Errorf("backlog access: groups: %w", err)
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return backlogVisibility{}, err
	}
	levels := map[string]models.AccessLevel{}
	for _, b := range backlogs {
		if level := models.AccessLevelOf(grants, owner, b.ID, held); level != "" {
			levels[b.ID] = level
		}
	}
	return backlogVisibility{levels: levels}, nil
}

func (m *taskManager) ListBacklogsFor(ctx context.Context, requesterID string) (models.BacklogList, error) {
	list, err := m.ListBacklogs(ctx)
	if err != nil {
		return models.BacklogList{}, err
	}
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil {
		return models.BacklogList{}, err
	}
	kept := make([]models.BacklogSummary, 0, len(list.Items))
	for _, item := range list.Items {
		if visible.Sees(item.ID) {
			kept = append(kept, item)
		}
	}
	return models.BacklogList{Items: kept}, nil
}

func (m *taskManager) MoveTaskToStepAs(ctx context.Context, requesterID, taskID, backlogID, stepID string) (Task, error) {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil {
		return Task{}, err
	}
	if !visible.all {
		task, err := m.GetTask(ctx, taskID)
		if err != nil {
			return Task{}, err
		}
		if !visible.Moves(task.Backlog) || !visible.Moves(backlogID) {
			return Task{}, fmt.Errorf("%w: moving needs Can move on both backlogs", ErrBacklogForbidden)
		}
	}
	return m.MoveTaskToStep(ctx, taskID, backlogID, stepID)
}

func (m *taskManager) accessOf(ctx context.Context, backlogID string) (models.BacklogAccess, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return models.BacklogAccess{}, err
	}
	if _, _, err := locateStep(backlogs, backlogID, ""); err != nil {
		return models.BacklogAccess{}, err
	}
	grants, err := m.storedGrants(ctx)
	if err != nil {
		return models.BacklogAccess{}, err
	}
	owner, err := m.storedOwnerGroup(ctx)
	if err != nil {
		return models.BacklogAccess{}, err
	}
	groups := models.GrantsFor(grants, backlogID)
	if m.groupExists != nil {
		for i := range groups {
			exists, err := m.groupExists(ctx, groups[i].GroupID)
			if err != nil {
				return models.BacklogAccess{}, fmt.Errorf("backlog access: group lookup: %w", err)
			}
			groups[i].Gone = !exists
		}
	}
	if err := m.markRoles(ctx, groups); err != nil {
		return models.BacklogAccess{}, err
	}
	return models.BacklogAccess{BacklogID: backlogID, OwnerGroup: owner, Groups: groups}, nil
}

func (m *taskManager) ListBacklogAccess(ctx context.Context, backlogID string) (models.BacklogAccess, error) {
	return m.accessOf(ctx, backlogID)
}

func (m *taskManager) changeGrants(ctx context.Context, change func(grants []models.BacklogGrant, owner string) ([]models.BacklogGrant, error)) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		grants, err := m.grantsIn(tx)
		if err != nil {
			return err
		}
		owner, err := m.ownerGroupIn(tx)
		if err != nil {
			return err
		}
		next, err := change(grants, owner)
		if err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := spec.StoreOverride(tx, m.spec, accessSetting, next, "taskmanager", now); err != nil {
			return fmt.Errorf("%s: %w", accessSetting, err)
		}
		return nil
	})
}

func (m *taskManager) GiveBacklogAccess(ctx context.Context, backlogID, groupID, level string) (models.BacklogAccess, error) {
	groupID = strings.TrimSpace(groupID)
	grant := models.BacklogGrant{BacklogID: backlogID, GroupID: groupID, Level: models.AccessLevel(level)}
	if err := models.CheckGrant(grant); err != nil {
		return models.BacklogAccess{}, fmt.Errorf("%w: %s", ErrInvalidBacklog, err)
	}
	if _, err := m.accessOf(ctx, backlogID); err != nil {
		return models.BacklogAccess{}, err
	}
	if err := m.requireGroup(ctx, groupID); err != nil {
		return models.BacklogAccess{}, err
	}
	err := m.changeGrants(ctx, func(grants []models.BacklogGrant, owner string) ([]models.BacklogGrant, error) {
		if owner == "" {
			return nil, ErrOwnerGroupNotSet
		}
		return models.WithGrant(grants, grant), nil
	})
	if err != nil {
		return models.BacklogAccess{}, err
	}
	if err := m.syncGroupRole(ctx, groupID); err != nil {
		return models.BacklogAccess{}, err
	}
	return m.accessOf(ctx, backlogID)
}

func (m *taskManager) TakeBacklogAccess(ctx context.Context, backlogID, groupID string) (models.BacklogAccess, error) {
	if _, err := m.accessOf(ctx, backlogID); err != nil {
		return models.BacklogAccess{}, err
	}
	err := m.changeGrants(ctx, func(grants []models.BacklogGrant, _ string) ([]models.BacklogGrant, error) {
		return models.WithoutGrant(grants, backlogID, groupID), nil
	})
	if err != nil {
		return models.BacklogAccess{}, err
	}
	if err := m.syncGroupRole(ctx, groupID); err != nil {
		return models.BacklogAccess{}, err
	}
	return m.accessOf(ctx, backlogID)
}

func (m *taskManager) requireGroup(ctx context.Context, groupID string) error {
	if groupID == "" {
		return fmt.Errorf("%w: a group is needed", ErrInvalidBacklog)
	}
	if m.groupExists == nil {
		return nil
	}
	exists, err := m.groupExists(ctx, groupID)
	if err != nil {
		return fmt.Errorf("backlog access: group lookup: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: group %q does not exist", ErrInvalidBacklog, groupID)
	}
	return nil
}

func (m *taskManager) SetBacklogOwnerGroup(ctx context.Context, groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if m.groupExists == nil {
		return fmt.Errorf("%w: the groups of this hub cannot be looked up, so the group cannot be checked", ErrInvalidBacklog)
	}
	if err := m.requireGroup(ctx, groupID); err != nil {
		return err
	}
	return m.storeSetting(ctx, ownerGroupSetting, groupID)
}

func (m *taskManager) pruneGrants(ctx context.Context, kept []models.Backlog) error {
	grants, err := m.storedGrants(ctx)
	if err != nil || len(grants) == 0 {
		return err
	}
	pruned := models.GrantsKept(grants, kept)
	if len(pruned) == len(grants) {
		return nil
	}
	if err := m.storeSetting(ctx, accessSetting, pruned); err != nil {
		return err
	}
	return m.syncGroupRoles(ctx, grants)
}
