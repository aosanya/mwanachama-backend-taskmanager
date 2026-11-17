package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	releaseAsBlocked = "blocked"
	releaseAsSinbin  = "sinbin"
)

var ErrInvalidRelease = errors.New("invalid release")

var ErrNotInProgress = errors.New("the issue is not in progress")

type ReleaseTask struct {
	TaskID string   `json:"-"`
	Mode   string   `json:"mode"`
	Reason string   `json:"reason"`
	TodoID string   `json:"todo_id"`
	Links  []string `json:"links"`
}

type TaskReleased struct {
	ID      string     `json:"id"`
	Code    string     `json:"code"`
	Title   string     `json:"title"`
	Mode    string     `json:"mode"`
	Tags    []string   `json:"tags"`
	Backlog string     `json:"backlog"`
	Step    string     `json:"step"`
	Status  TaskStatus `json:"status"`
	NoteID  string     `json:"note_id"`
	TodoID  string     `json:"todo_id,omitempty"`
}

type releasePlan struct {
	task    Task
	mode    string
	mark    string
	reason  string
	todoID  string
	links   []string
	backlog string
}

func (m *taskManager) ReleaseTaskAs(ctx context.Context, requesterID string, req ReleaseTask) (TaskReleased, error) {
	plan, err := releaseRequest(req)
	if err != nil {
		return TaskReleased{}, err
	}
	if plan.task, err = m.releasableTask(ctx, requesterID, req.TaskID); err != nil {
		return TaskReleased{}, err
	}
	if err := m.requireAtInProgressStep(ctx, plan.task); err != nil {
		return TaskReleased{}, err
	}
	if err := m.requireOwnTodo(ctx, plan.task.ID, plan.todoID); err != nil {
		return TaskReleased{}, err
	}
	plan.backlog = plan.task.Backlog
	if plan.backlog == "" {
		plan.backlog = defaultBacklog
	}
	var held heldEvents
	var noteID string
	txErr := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txm, err := m.inTxKeepingAccess(tx, &held)
		if err != nil {
			return err
		}
		noteID, err = txm.writeRelease(ctx, requesterID, plan)
		return err
	})
	if txErr != nil {
		return TaskReleased{}, txErr
	}
	for _, e := range held {
		m.publish(ctx, e.topic, e.payload)
	}
	return m.releasedView(ctx, plan, noteID)
}

func (m *taskManager) inTxKeepingAccess(tx *gorm.DB, held *heldEvents) (*taskManager, error) {
	txm, err := m.inTx(tx, held)
	if err != nil {
		return nil, err
	}
	txm.groups, txm.groupExists, txm.instanceOwner = m.groups, m.groupExists, m.instanceOwner
	return txm, nil
}

func releaseRequest(req ReleaseTask) (releasePlan, error) {
	plan := releasePlan{
		mode:   strings.ToLower(strings.TrimSpace(req.Mode)),
		reason: strings.TrimSpace(req.Reason),
		todoID: strings.TrimSpace(req.TodoID),
		links:  req.Links,
	}
	switch plan.mode {
	case releaseAsBlocked:
		plan.mark = tagBlocked
	case releaseAsSinbin:
		plan.mark = tagSinbin
	default:
		return plan, fmt.Errorf("ReleaseTask: %w: mode is blocked or sinbin, not %q", ErrInvalidRelease, req.Mode)
	}
	if plan.reason == "" {
		return plan, fmt.Errorf("ReleaseTask: %w: a reason is required", ErrInvalidRelease)
	}
	if plan.todoID != "" && plan.mode != releaseAsBlocked {
		return plan, fmt.Errorf("ReleaseTask: %w: a todo is set Blocked only with mode blocked", ErrInvalidRelease)
	}
	return plan, nil
}

func (m *taskManager) releasableTask(ctx context.Context, requesterID, taskID string) (Task, error) {
	task, err := m.GetTask(ctx, taskID)
	if err != nil {
		return Task{}, err
	}
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil {
		return Task{}, err
	}
	if visible.all {
		return task, nil
	}
	if !visible.Sees(task.Backlog) {
		return Task{}, ErrTaskNotFound
	}
	if !visible.Moves(task.Backlog) {
		return Task{}, fmt.Errorf("%w: this needs Can move on the backlog the issue is in", ErrBacklogForbidden)
	}
	return task, nil
}

func (m *taskManager) requireOwnTodo(ctx context.Context, taskID, todoID string) error {
	if todoID == "" {
		return nil
	}
	todo, err := m.GetTaskTodo(ctx, todoID)
	if err != nil {
		return err
	}
	if todo.ParentTaskID != taskID {
		return fmt.Errorf("ReleaseTask: %w: %s belongs to another issue", ErrTaskTodoCrossIssue, todoID)
	}
	return nil
}

func (m *taskManager) requireAtInProgressStep(ctx context.Context, task Task) error {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return err
	}
	if at, ok := models.FindBacklog(backlogs, task.Backlog); ok {
		for _, step := range backlogs[at].Steps {
			if step.ID == task.Step && namesInProgress(step) {
				return nil
			}
		}
	}
	return fmt.Errorf("ReleaseTask: %w: it is at step %q", ErrNotInProgress, task.Step)
}

func namesInProgress(step models.BacklogStep) bool {
	slug := models.Slug(step.Name)
	return strings.EqualFold(strings.TrimSpace(step.Name), inProgressStepName) ||
		slug == inProgressStepID || slug == "doing" || models.StepNamesStatus(step, TaskStatusInProgress)
}

func releaseTags(tags []string, mark string) []string {
	kept := make([]string, 0, len(tags)+1)
	for _, tag := range tags {
		if !hasTagFold(markTags, tag) {
			kept = append(kept, tag)
		}
	}
	return append(kept, mark)
}

func releaseEntry(plan releasePlan) string {
	label := "Blocked"
	if plan.mode == releaseAsSinbin {
		label = "Sinbin"
	}
	return fmt.Sprintf("Released as %s: tagged %s without Ready and moved to To do. Reason: %s", label, plan.mark, plan.reason)
}

func (m *taskManager) writeRelease(ctx context.Context, requesterID string, plan releasePlan) (string, error) {
	note, err := m.AddTaskNote(ctx, requesterID, plan.task.ID, noteByWorker, releaseEntry(plan), plan.links)
	if err != nil {
		return "", fmt.Errorf("ReleaseTask: write the entry: %w", err)
	}
	tags := releaseTags(plan.task.Tags, plan.mark)
	if _, err := m.PatchTaskAs(ctx, requesterID, TaskPatch{ID: plan.task.ID, Tags: &tags}); err != nil {
		return "", fmt.Errorf("ReleaseTask: change the tags: %w", err)
	}
	if plan.todoID != "" {
		if _, err := m.SetTaskTodoStatus(ctx, plan.todoID, TodoStatusBlocked, plan.reason); err != nil {
			return "", fmt.Errorf("ReleaseTask: block the todo: %w", err)
		}
	}
	if _, err := m.MoveTaskToStep(ctx, plan.task.ID, plan.backlog, toDoStepID); err != nil {
		return "", fmt.Errorf("ReleaseTask: move to To do: %w", err)
	}
	if err := m.settleAsPending(ctx, plan.task.ID); err != nil {
		return "", fmt.Errorf("ReleaseTask: set the status back to pending: %w", err)
	}
	return note.ID, nil
}

func (m *taskManager) settleAsPending(ctx context.Context, taskID string) error {
	moved, err := m.GetTask(ctx, taskID)
	if err != nil || moved.Status == TaskStatusPending {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res := m.store.Query(ctx, roleTask).Where("id = ? AND deleted = ?", taskID, false).
		Updates(map[string]any{"status": string(TaskStatusPending), "updated_at": now, "held_by": ""})
	if res.Error != nil {
		return res.Error
	}
	m.logChange(ctx, taskID, changeLabelStatus, string(moved.Status), string(TaskStatusPending))
	pending := moved
	pending.Status = TaskStatusPending
	m.announceStatusChange(ctx, pending, moved.Status, now)
	return nil
}

func (m *taskManager) releasedView(ctx context.Context, plan releasePlan, noteID string) (TaskReleased, error) {
	task, err := m.GetTask(ctx, plan.task.ID)
	if err != nil {
		return TaskReleased{}, err
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return TaskReleased{}, err
	}
	backlog, step, err := locateStep(backlogs, plan.backlog, toDoStepID)
	if err != nil {
		return TaskReleased{}, err
	}
	return TaskReleased{
		ID: task.ID, Code: task.Code, Title: task.Title, Mode: plan.mode, Tags: task.Tags,
		Backlog: backlog.Name, Step: step.Name, Status: task.Status, NoteID: noteID, TodoID: plan.todoID,
	}, nil
}
