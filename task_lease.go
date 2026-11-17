package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	leaseDefaultMinutes = 30
	leaseMinMinutes     = 1
	leaseMaxMinutes     = 240
	leaseTimeForm       = time.RFC3339
	leaseRanOutFormat   = "The lease of %s ran out at %s: the issue is back at To do."
)

var ErrNoReadyIssue = errors.New("no ready issue is free to take")
var ErrLeaseNotHeld = errors.New("the issue is not held under a lease by that agent")
var ErrInvalidLease = errors.New("invalid lease")

type leaseHold struct {
	Agent string
	Until string
}

func (h leaseHold) fields() map[string]any {
	changes := map[string]any{"lease_until": h.Until}
	if h.Agent != "" {
		changes["assigned_to"] = h.Agent
		changes["assignee_kind"] = string(models.AssigneeAgent)
	}
	return changes
}

type leaseRow struct {
	ID         string
	LeaseUntil string
	AssignedTo string
}

func leaseLength(minutes int) (time.Duration, error) {
	if minutes == 0 {
		minutes = leaseDefaultMinutes
	}
	if minutes < leaseMinMinutes || minutes > leaseMaxMinutes {
		return 0, fmt.Errorf("%w: lease_minutes is %d to %d, not %d", ErrInvalidLease, leaseMinMinutes, leaseMaxMinutes, minutes)
	}
	return time.Duration(minutes) * time.Minute, nil
}

func leaseEnd(now time.Time, length time.Duration) string {
	return now.UTC().Add(length).Format(leaseTimeForm)
}

func agentOrCaller(agent, requesterID string) string {
	if agent = strings.TrimSpace(agent); agent != "" {
		return agent
	}
	return requesterID
}

func (m *taskManager) releaseExpiredLeases(ctx context.Context) (map[string]leaseRow, error) {
	released := map[string]leaseRow{}
	var rows []leaseRow
	err := m.store.Query(ctx, roleTask).
		Where("deleted = ? AND status = ? AND lease_until <> ''", false, string(TaskStatusInProgress)).
		Select("id, lease_until, assigned_to").Scan(&rows).Error
	if err != nil {
		return released, fmt.Errorf("release expired leases: %w", err)
	}
	now := time.Now().UTC()
	for _, row := range rows {
		until, err := time.Parse(leaseTimeForm, row.LeaseUntil)
		if err == nil && until.After(now) {
			continue
		}
		won, err := m.releaseLease(ctx, row, now)
		if err != nil {
			return released, err
		}
		if won {
			released[row.ID] = row
		}
	}
	return released, nil
}

func (m *taskManager) releaseLease(ctx context.Context, row leaseRow, now time.Time) (bool, error) {
	won := m.store.Query(ctx, roleTask).
		Where("id = ? AND deleted = ? AND lease_until = ?", row.ID, false, row.LeaseUntil).
		Updates(map[string]any{"lease_until": "", "assigned_to": "", "assignee_kind": "", "updated_at": now.Format(time.RFC3339)})
	if won.Error != nil {
		return false, fmt.Errorf("release expired leases: %w", won.Error)
	}
	if won.RowsAffected == 0 {
		return false, nil
	}
	task, err := m.GetTask(ctx, row.ID)
	if err != nil {
		return false, err
	}
	if err := m.requireAtInProgressStep(ctx, task); err != nil {
		return false, nil
	}
	note := TaskNote{
		ID: newSpecID(), TaskID: task.ID, Kind: noteAutomatic,
		Text: fmt.Sprintf(leaseRanOutFormat, row.AssignedTo, row.LeaseUntil), CreatedAt: now.Format(noteTimeForm),
	}
	if err := m.store.Insert(ctx, roleTaskNote, note); err != nil {
		return false, fmt.Errorf("release expired leases: write the entry: %w", err)
	}
	backlog := task.Backlog
	if backlog == "" {
		backlog = defaultBacklog
	}
	if _, err := m.MoveTaskToStep(ctx, task.ID, backlog, toDoStepID); err != nil {
		return false, fmt.Errorf("release expired leases: move to To do: %w", err)
	}
	if err := m.settleAsPending(ctx, task.ID); err != nil {
		return false, fmt.Errorf("release expired leases: set the status back to pending: %w", err)
	}
	return true, nil
}
