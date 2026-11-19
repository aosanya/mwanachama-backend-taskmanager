package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"strings"
	"time"
)

type ClaimNextRequest struct {
	Agent        string         `json:"agent"`
	Backlog      string         `json:"backlog"`
	Tag          models.TagList `json:"tag"`
	LeaseMinutes int            `json:"lease_minutes"`
}
type NextClaim struct {
	TaskClaim
	Agent      string `json:"agent,omitempty"`
	LeaseUntil string `json:"lease_until,omitempty"`
}

func (m *taskManager) ClaimNext(ctx context.Context, requesterID string, req ClaimNextRequest) (NextClaim, error) {
	length, err := leaseLength(req.LeaseMinutes)
	if err != nil {
		return NextClaim{}, err
	}
	if _, err := m.releaseExpiredLeases(ctx); err != nil {
		return NextClaim{}, fmt.Errorf("ClaimNext: %w", err)
	}
	candidates, err := m.claimCandidates(ctx, requesterID, req)
	if err != nil {
		return NextClaim{}, fmt.Errorf("ClaimNext: %w", err)
	}
	running, err := m.inProgressWithFiles(ctx, FileOverlapsFilter{RequesterID: requesterID})
	if err != nil {
		return NextClaim{}, fmt.Errorf("ClaimNext: %w", err)
	}
	agent := agentOrCaller(req.Agent, requesterID)
	var forbidden error
	for _, candidate := range candidates {
		clash, err := m.clashesWithRunning(ctx, candidate.ID, running)
		if err != nil {
			return NextClaim{}, fmt.Errorf("ClaimNext: %w", err)
		}
		if clash {
			continue
		}
		hold := leaseHold{Agent: agent, Until: leaseEnd(time.Now(), length)}
		claim, err := m.claimIssue(ctx, requesterID, candidate.ID, hold)
		switch {
		case err == nil:
			return NextClaim{TaskClaim: claim, Agent: agent, LeaseUntil: hold.Until}, nil
		case errors.Is(err, ErrBacklogForbidden):
			forbidden = err
		case errors.Is(err, ErrTaskNotClaimable), errors.Is(err, ErrTaskMoveConflict), errors.Is(err, ErrTaskNotFound):
			if running, err = m.inProgressWithFiles(ctx, FileOverlapsFilter{RequesterID: requesterID}); err != nil {
				return NextClaim{}, fmt.Errorf("ClaimNext: %w", err)
			}
		default:
			return NextClaim{}, err
		}
	}
	if forbidden != nil {
		return NextClaim{}, forbidden
	}
	return NextClaim{}, ErrNoReadyIssue
}
func (m *taskManager) claimCandidates(ctx context.Context, requesterID string, req ClaimNextRequest) ([]nextReadyRow, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return nil, err
	}
	filter := NextReadyFilter{Tag: req.Tag, Backlog: req.Backlog, RequesterID: requesterID}
	if err := m.requireSeenBacklog(ctx, backlogs, filter); err != nil {
		return nil, err
	}
	query, err := m.scopedTasks(ctx, strings.TrimSpace(req.Backlog), "", append([]string{tagReady}, req.Tag...), requesterID)
	if err != nil {
		return nil, err
	}
	var rows []nextReadyRow
	if err := query.Where("status NOT IN ?", closedStatuses).Select(pendingColumns + ", effort").Order(taskOrder).Scan(&rows).Error; err != nil {
		return nil, err
	}
	tags := m.tagNamesOf(ctx, nextReadyIDs(rows))
	free := rows[:0]
	for _, row := range rows {
		if isToDoStep(backlogs, row.place()) && !isMarkedNotReady(tags[row.ID]) {
			free = append(free, row)
		}
	}
	return free, nil
}
func (m *taskManager) clashesWithRunning(ctx context.Context, taskID string, running []Task) (bool, error) {
	if len(running) == 0 {
		return false, nil
	}
	task, err := m.GetTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, other := range running {
		if len(overlappingPairs(task.Files, other.Files)) > 0 {
			return true, nil
		}
	}
	return false, nil
}
