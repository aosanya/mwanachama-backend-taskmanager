package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func tagOfGroup(group string) string { return "t-" + group }

func fixtureGroupTags() mwanachamataskmanager.GroupTags {
	byPerson := map[string][]string{
		"owner": {"all", tagOfGroup(ownerGroup), tagOfGroup(devGroup), tagOfGroup(docsGroup)},
		"dev":   {tagOfGroup(devGroup)},
		"both":  {tagOfGroup(devGroup), tagOfGroup(docsGroup)},
		"none":  nil,
	}
	return func(_ context.Context, id string) ([]string, error) { return byPerson[id], nil }
}

func withReach(mgr mwanachamataskmanager.TaskManager) mwanachamataskmanager.TaskManager {
	mgr.UseGroupTags(fixtureGroupTags())
	return mgr
}

func giveReach(t *testing.T, mgr mwanachamataskmanager.TaskManager, backlogID, group string) {
	t.Helper()
	if _, err := mgr.AddBacklogTagAs(context.Background(), "owner", backlogID, tagOfGroup(group)); err != nil {
		t.Fatalf("reach of %s on %s: %v", group, backlogID, err)
	}
}

func giveSee(mgr mwanachamataskmanager.TaskManager, ctx context.Context, backlogID, group string) (mwanachamataskmanager.BacklogAccess, error) {
	mgr.UseGroupTags(fixtureGroupTags())
	if _, err := mgr.AddBacklogTagAs(ctx, "owner", backlogID, tagOfGroup(group)); err != nil {
		return mwanachamataskmanager.BacklogAccess{}, err
	}
	return mgr.ListBacklogAccess(ctx, backlogID)
}

func giveAccess(mgr mwanachamataskmanager.TaskManager, ctx context.Context, backlogID, group, level string) (mwanachamataskmanager.BacklogAccess, error) {
	mgr.UseGroupTags(fixtureGroupTags())
	if group == devGroup || group == docsGroup || group == ownerGroup {
		if _, err := mgr.AddBacklogTagAs(ctx, "owner", backlogID, tagOfGroup(group)); err != nil && !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
			return mwanachamataskmanager.BacklogAccess{}, err
		}
	}
	return mgr.GiveBacklogAccess(ctx, backlogID, group, level)
}

func giveLevel(mgr mwanachamataskmanager.TaskManager, ctx context.Context, backlogID, group, level string) (mwanachamataskmanager.BacklogAccess, error) {
	if level == "see" {
		return giveSee(mgr, ctx, backlogID, group)
	}
	return giveAccess(mgr, ctx, backlogID, group, level)
}
