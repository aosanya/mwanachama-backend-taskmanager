package mwanachamataskmanager

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	RoleCodeBacklogMember = "backlog_member"
	RoleCodeBacklogWorker = "backlog_worker"
	PermissionTaskRead    = "task.read"
	PermissionTaskEdit    = "task.edit"
)

type RoleState struct {
	Codes   []string
	CanRead bool
	CanEdit bool
}

type RoleSync interface {
	SetBacklogRole(ctx context.Context, groupID, roleCode string) error
	GroupRoleState(ctx context.Context, groupID string) (RoleState, error)
}

func backlogRoleFor(level models.AccessLevel) string {
	switch level {
	case models.AccessMove:
		return RoleCodeBacklogWorker
	case models.AccessSee:
		return RoleCodeBacklogMember
	}
	return ""
}

func (m *taskManager) syncGroupRole(ctx context.Context, groupID string) error {
	if m.roleSync == nil {
		return nil
	}
	owner, err := m.storedOwnerGroup(ctx)
	if err != nil {
		return err
	}
	if groupID == owner {
		return nil
	}
	grants, err := m.storedGrants(ctx)
	if err != nil {
		return err
	}
	if err := m.roleSync.SetBacklogRole(ctx, groupID, backlogRoleFor(models.HighestLevelOf(grants, groupID))); err != nil {
		return fmt.Errorf("backlog access: the group's role: %w", err)
	}
	return nil
}

func (m *taskManager) syncGroupRoles(ctx context.Context, before []models.BacklogGrant) error {
	seen := map[string]bool{}
	for _, g := range before {
		if seen[g.GroupID] {
			continue
		}
		seen[g.GroupID] = true
		if err := m.syncGroupRole(ctx, g.GroupID); err != nil {
			return err
		}
	}
	return nil
}

func (m *taskManager) markRoles(ctx context.Context, groups []models.GroupAccess) error {
	if m.roleSync == nil {
		return nil
	}
	for i := range groups {
		state, err := m.roleSync.GroupRoleState(ctx, groups[i].GroupID)
		if err != nil {
			return fmt.Errorf("backlog access: the group's roles: %w", err)
		}
		groups[i].Roles = state.Codes
		groups[i].NoRole = !state.CanRead
		groups[i].RoleTooSmall = groups[i].Level == models.AccessMove && !state.CanEdit
	}
	return nil
}
