package mwanachamataskmanager

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const claimNoteFormat = "Taken by %s. Caps in force: %d input, %d output, %s"

type ClaimedIssue struct {
	ID       string       `json:"id"`
	Code     string       `json:"code"`
	Title    string       `json:"title"`
	Status   TaskStatus   `json:"status"`
	Priority TaskPriority `json:"priority"`
	Effort   TaskEffort   `json:"effort"`
	Tags     []string     `json:"tags"`
	Backlog  string       `json:"backlog"`
	Step     string       `json:"step"`
}

type TaskClaim struct {
	Issue  ClaimedIssue `json:"issue"`
	NoteID string       `json:"note_id"`
	Caps   WorkerCaps   `json:"caps"`
}

func (m *taskManager) ClaimTaskAs(ctx context.Context, requesterID, taskID string) (TaskClaim, error) {
	return m.claimIssue(ctx, requesterID, taskID, leaseHold{})
}

func (m *taskManager) claimIssue(ctx context.Context, requesterID, taskID string, hold leaseHold) (TaskClaim, error) {
	task, err := m.GetTaskFor(ctx, requesterID, taskID)
	if err != nil {
		return TaskClaim{}, err
	}
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return TaskClaim{}, err
	}
	backlog, target, err := m.claimTarget(ctx, task)
	if err != nil {
		return TaskClaim{}, err
	}
	if reason := claimRefusal(task, target); reason != "" {
		return TaskClaim{}, fmt.Errorf("%w: %s", ErrTaskNotClaimable, reason)
	}
	if err := m.refuseOpenDependency(ctx, requesterID, task); err != nil {
		return TaskClaim{}, err
	}
	claim, err := m.takeIssue(ctx, requesterID, task, target, hold)
	if err != nil {
		return TaskClaim{}, err
	}
	claimed, err := m.GetTask(ctx, taskID)
	if err != nil {
		return TaskClaim{}, err
	}
	claim.Issue = claimedIssue(claimed, backlog, target)
	return claim, nil
}

func (m *taskManager) claimTarget(ctx context.Context, task Task) (models.Backlog, models.BacklogStep, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return models.Backlog{}, models.BacklogStep{}, err
	}
	at, ok := models.FindBacklog(backlogs, task.Backlog)
	if !ok {
		return models.Backlog{}, models.BacklogStep{}, fmt.Errorf("%w: %q", ErrUnknownBacklog, task.Backlog)
	}
	step, ok := inProgressStepOf(backlogs[at])
	if !ok {
		return models.Backlog{}, models.BacklogStep{}, fmt.Errorf("%w: %q has no In Progress step", ErrUnknownBacklogStep, backlogs[at].Name)
	}
	return backlogs[at], step, nil
}

func inProgressStepOf(backlog models.Backlog) (models.BacklogStep, bool) {
	for _, step := range backlog.Steps {
		if models.StepNamesStatus(step, TaskStatusInProgress) {
			return step, true
		}
	}
	for _, step := range backlog.Steps {
		slug := models.Slug(step.Name)
		if slug == inProgressStepID || slug == "doing" {
			return step, true
		}
	}
	return models.BacklogStep{}, false
}

func claimRefusal(task Task, target models.BacklogStep) string {
	switch {
	case task.Status == TaskStatusInProgress || task.Step == target.ID:
		return "the issue is already in progress, so someone else holds it"
	case hasTagFold(task.Tags, tagSinbin):
		return "the issue is tagged Sinbin: a worker stopped at a cap, and the Triager splits it before anyone takes it"
	case hasTagFold(task.Tags, tagBlocked):
		return "the issue is tagged Blocked: a person must look at it first"
	case hasTagFold(task.Tags, tagWaiting):
		return "the issue is tagged Waiting: an issue it depends on is still open"
	case task.Status != TaskStatusPending:
		return fmt.Sprintf("the issue has status %s; only an issue that is waiting to be done can be taken", task.Status)
	}
	return ""
}

func (m *taskManager) refuseOpenDependency(ctx context.Context, requesterID string, task Task) error {
	deps, err := m.TaskDependenciesFor(ctx, requesterID, task.ID)
	if err != nil {
		return fmt.Errorf("ClaimTask: %w", err)
	}
	if deps.BlocksStart {
		return fmt.Errorf("%w: the issue waits on a dependency that is still open", ErrTaskNotClaimable)
	}
	return nil
}

func (m *taskManager) takeIssue(ctx context.Context, requesterID string, task Task, target models.BacklogStep, hold leaseHold) (TaskClaim, error) {
	var held heldEvents
	var claim TaskClaim
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txm, err := m.inTx(tx, &held)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := txm.winIssue(ctx, task, target, now.Format(time.RFC3339), hold); err != nil {
			return err
		}
		reading, err := txm.capsReading(ctx, task.ID)
		if err != nil {
			return err
		}
		note := TaskNote{
			ID: newSpecID(), TaskID: task.ID, AuthorID: requesterID, Kind: noteByWorker,
			Text: claimNoteText(requesterID, reading.InForce), CreatedAt: now.Format(noteTimeForm),
		}
		if err := txm.store.Insert(ctx, roleTaskNote, note); err != nil {
			return fmt.Errorf("ClaimTask: %w", err)
		}
		claim = TaskClaim{NoteID: note.ID, Caps: reading.InForce}
		taken := task
		taken.Status = TaskStatusInProgress
		txm.announceStatusChange(ctx, taken, task.Status, now.Format(time.RFC3339))
		return nil
	})
	if err != nil {
		return TaskClaim{}, err
	}
	for _, e := range held {
		m.publish(ctx, e.topic, e.payload)
	}
	return claim, nil
}

func (m *taskManager) winIssue(ctx context.Context, task Task, target models.BacklogStep, now string, hold leaseHold) error {
	changes := map[string]any{
		"backlog": task.Backlog, "step": target.ID, "status": string(TaskStatusInProgress),
		"updated_at": now, "held_by": "",
	}
	maps.Copy(changes, hold.fields())
	res := m.store.Query(ctx, roleTask).
		Where("id = ? AND deleted = ? AND status = ? AND COALESCE(step, '') <> ?", task.ID, false, string(task.Status), target.ID).
		Updates(changes)
	if res.Error != nil {
		return fmt.Errorf("ClaimTask: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		return nil
	}
	current, err := m.GetTask(ctx, task.ID)
	if err != nil {
		return err
	}
	if reason := claimRefusal(current, target); reason != "" {
		return fmt.Errorf("%w: %s", ErrTaskNotClaimable, reason)
	}
	return ErrTaskMoveConflict
}

func claimNoteText(requesterID string, caps WorkerCaps) string {
	return fmt.Sprintf(claimNoteFormat, requesterID, caps.InputTokens, caps.OutputTokens, runtimeText(caps.RuntimeSeconds))
}

func runtimeText(seconds int64) string {
	if seconds%60 == 0 {
		return fmt.Sprintf("%d minutes", seconds/60)
	}
	return fmt.Sprintf("%d seconds", seconds)
}

func claimedIssue(task Task, backlog models.Backlog, step models.BacklogStep) ClaimedIssue {
	return ClaimedIssue{
		ID: task.ID, Code: task.Code, Title: strings.TrimSpace(task.Title), Status: task.Status,
		Priority: task.Priority, Effort: task.Effort, Tags: task.Tags, Backlog: backlog.Name, Step: step.Name,
	}
}
