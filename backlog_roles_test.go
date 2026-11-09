package mwanachamataskmanager_test

import (
	"context"
	"slices"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type recordedRoles struct {
	set   map[string]string
	state map[string]mwanachamataskmanager.RoleState
}

func (r *recordedRoles) SetBacklogRole(_ context.Context, groupID, roleCode string) error {
	r.set[groupID] = roleCode
	return nil
}

func (r *recordedRoles) GroupRoleState(_ context.Context, groupID string) (mwanachamataskmanager.RoleState, error) {
	return r.state[groupID], nil
}

func managerWithRoles(t *testing.T) (mwanachamataskmanager.TaskManager, *recordedRoles) {
	t.Helper()
	mgr := managerWithTwoBacklogs(t)
	roles := &recordedRoles{set: map[string]string{}, state: map[string]mwanachamataskmanager.RoleState{}}
	mgr.UseRoleSync(roles)
	if err := mgr.SetBacklogOwnerGroup(context.Background(), ownerGroup); err != nil {
		t.Fatal(err)
	}
	return mgr, roles
}

func TestBacklogRoles_AGroupHoldsTheRoleOfItsHighestLevelAcrossBacklogs(t *testing.T) {
	mgr, roles := managerWithRoles(t)
	ctx := context.Background()
	if _, err := mgr.GiveBacklogAccess(ctx, "backlog", devGroup, "see"); err != nil {
		t.Fatal(err)
	}
	if roles.set[devGroup] != mwanachamataskmanager.RoleCodeBacklogMember {
		t.Fatalf("after Can see the group's role is %q", roles.set[devGroup])
	}
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	if roles.set[devGroup] != mwanachamataskmanager.RoleCodeBacklogWorker {
		t.Fatalf("after Can move on another backlog the role is %q", roles.set[devGroup])
	}
	if _, err := mgr.TakeBacklogAccess(ctx, "software", devGroup); err != nil {
		t.Fatal(err)
	}
	if roles.set[devGroup] != mwanachamataskmanager.RoleCodeBacklogMember {
		t.Fatalf("after Can move is taken the role falls back to %q", roles.set[devGroup])
	}
	if _, err := mgr.TakeBacklogAccess(ctx, "backlog", devGroup); err != nil {
		t.Fatal(err)
	}
	if role, called := roles.set[devGroup]; !called || role != "" {
		t.Fatalf("after the last access is taken the role is %q (called %v), want none", role, called)
	}
}

func TestBacklogRoles_TheOwnerGroupIsNeverGivenABacklogRole(t *testing.T) {
	mgr, roles := managerWithRoles(t)
	if _, err := mgr.GiveBacklogAccess(context.Background(), "backlog", ownerGroup, "see"); err != nil {
		t.Logf("giving the owner group access: %v", err)
	}
	if _, called := roles.set[ownerGroup]; called {
		t.Fatalf("the owner group was given the role %q", roles.set[ownerGroup])
	}
}

func TestBacklogRoles_TheAccessListSaysWhenARoleIsMissingOrTooSmall(t *testing.T) {
	mgr, roles := managerWithRoles(t)
	ctx := context.Background()
	if _, err := mgr.GiveBacklogAccess(ctx, "backlog", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, "backlog", docsGroup, "see"); err != nil {
		t.Fatal(err)
	}
	roles.state[devGroup] = mwanachamataskmanager.RoleState{Codes: []string{"backlog_member"}, CanRead: true}
	access, err := mgr.ListBacklogAccess(ctx, "backlog")
	if err != nil {
		t.Fatal(err)
	}
	byGroup := map[string]bool{}
	for _, g := range access.Groups {
		switch g.GroupID {
		case devGroup:
			if !g.RoleTooSmall || g.NoRole || !slices.Equal(g.Roles, []string{"backlog_member"}) {
				t.Fatalf("dev entry = %+v, want a role that is too small", g)
			}
		case docsGroup:
			if !g.NoRole || g.RoleTooSmall {
				t.Fatalf("docs entry = %+v, want no role", g)
			}
		}
		byGroup[g.GroupID] = true
	}
	if !byGroup[devGroup] || !byGroup[docsGroup] {
		t.Fatalf("groups listed = %v", byGroup)
	}
}
