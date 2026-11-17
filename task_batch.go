package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	batchMaxOperations = 50
	batchOpPatch       = "patch"
	batchOpMove        = "move_to_step"
	batchNotApplied    = "not applied: an earlier operation failed"
)

var ErrInvalidBatch = errors.New("invalid batch")

type BatchOperation struct {
	Op      string     `json:"op"`
	TaskID  string     `json:"task_id"`
	Patch   *TaskPatch `json:"patch,omitempty"`
	Backlog string     `json:"backlog,omitempty"`
	Step    string     `json:"step,omitempty"`
}

type BatchRequest struct {
	Operations   []BatchOperation `json:"operations"`
	AllOrNothing bool             `json:"all_or_nothing"`
}

type BatchResult struct {
	Index   int    `json:"index"`
	Op      string `json:"op"`
	TaskID  string `json:"task_id"`
	OK      bool   `json:"ok"`
	Applied bool   `json:"applied"`
	Error   string `json:"error,omitempty"`
}

type BatchOutcome struct {
	Results      []BatchResult `json:"results"`
	Applied      int           `json:"applied"`
	Failed       int           `json:"failed"`
	AllOrNothing bool          `json:"all_or_nothing"`
}

func (m *taskManager) Batch(ctx context.Context, requesterID string, req BatchRequest) (BatchOutcome, error) {
	if n := len(req.Operations); n == 0 || n > batchMaxOperations {
		return BatchOutcome{}, fmt.Errorf("Batch: %w: %d operations, send 1 to %d", ErrInvalidBatch, n, batchMaxOperations)
	}
	results := make([]BatchResult, len(req.Operations))
	for i, op := range req.Operations {
		results[i] = BatchResult{Index: i, Op: op.Op, TaskID: op.TaskID}
	}
	if req.AllOrNothing && m.refuseBatch(ctx, requesterID, req.Operations, results) {
		return tallyBatch(results, true), nil
	}
	m.applyBatch(ctx, requesterID, req, results)
	return tallyBatch(results, req.AllOrNothing), nil
}

func (m *taskManager) refuseBatch(ctx context.Context, requesterID string, ops []BatchOperation, results []BatchResult) bool {
	seen := make(map[string]Task, len(ops))
	refused := false
	for i, op := range ops {
		if err := m.checkBatchOperation(ctx, requesterID, op, seen); err != nil {
			results[i].Error = err.Error()
			refused = true
			continue
		}
		results[i].OK = true
	}
	return refused
}

func (m *taskManager) applyBatch(ctx context.Context, requesterID string, req BatchRequest, results []BatchResult) {
	stopped := false
	for i, op := range req.Operations {
		if stopped {
			results[i].Error = batchNotApplied
			continue
		}
		err := checkBatchShape(op)
		if err == nil {
			err = m.applyBatchOperation(ctx, requesterID, op)
		}
		if err != nil {
			results[i].Error = err.Error()
			stopped = req.AllOrNothing
			continue
		}
		results[i].OK = true
		results[i].Applied = true
	}
}

func (m *taskManager) applyBatchOperation(ctx context.Context, requesterID string, op BatchOperation) error {
	if op.Op == batchOpMove {
		_, err := m.MoveTaskToStepAs(ctx, requesterID, op.TaskID, op.Backlog, op.Step)
		return err
	}
	patch := *op.Patch
	patch.ID = op.TaskID
	_, err := m.PatchTaskAs(ctx, requesterID, patch)
	return err
}

func tallyBatch(results []BatchResult, allOrNothing bool) BatchOutcome {
	out := BatchOutcome{Results: results, AllOrNothing: allOrNothing}
	for _, r := range results {
		if r.Applied {
			out.Applied++
		}
		if !r.OK {
			out.Failed++
		}
	}
	return out
}

func checkBatchShape(op BatchOperation) error {
	if strings.TrimSpace(op.TaskID) == "" {
		return fmt.Errorf("%w: task_id is required", ErrInvalidBatch)
	}
	switch op.Op {
	case batchOpPatch:
		if op.Patch == nil {
			return fmt.Errorf("%w: a patch operation needs a patch", ErrInvalidBatch)
		}
	case batchOpMove:
		if strings.TrimSpace(op.Backlog) == "" || strings.TrimSpace(op.Step) == "" {
			return fmt.Errorf("%w: a move_to_step operation needs a backlog and a step", ErrInvalidBatch)
		}
	default:
		return fmt.Errorf("%w: op %q is not %s or %s", ErrInvalidBatch, op.Op, batchOpPatch, batchOpMove)
	}
	return nil
}

func (m *taskManager) checkBatchOperation(ctx context.Context, requesterID string, op BatchOperation, seen map[string]Task) error {
	if err := checkBatchShape(op); err != nil {
		return err
	}
	if err := m.requireMoveOnTask(ctx, requesterID, op.TaskID); err != nil {
		return err
	}
	current, ok := seen[op.TaskID]
	if !ok {
		var err error
		if current, err = m.GetTask(ctx, op.TaskID); err != nil {
			return err
		}
	}
	next, err := m.foreseeBatchOperation(ctx, requesterID, op, current)
	if err != nil {
		return err
	}
	seen[op.TaskID] = next
	return nil
}

func (m *taskManager) foreseeBatchOperation(ctx context.Context, requesterID string, op BatchOperation, current Task) (Task, error) {
	if op.Op == batchOpMove {
		return m.foreseeMove(ctx, requesterID, op, current)
	}
	return m.foreseePatch(ctx, *op.Patch, current)
}

func (m *taskManager) foreseePatch(ctx context.Context, patch TaskPatch, current Task) (Task, error) {
	patched := applyPatch(current, patch)
	if err := refuseDonePriorityChange(current, patched.Priority); err != nil {
		return Task{}, err
	}
	if patched.Effort == "" {
		patched.Effort = current.Effort
	}
	if err := checkEffort(patched.Effort); err != nil {
		return Task{}, err
	}
	files, err := checkFiles(patched.Files)
	if err != nil {
		return Task{}, err
	}
	patched.Files = files
	if current.Status != patched.Status && !current.Status.CanTransitionTo(patched.Status) {
		return Task{}, ErrInvalidStatusTransition
	}
	if current.Status == TaskStatusPending && patched.Status == TaskStatusInProgress {
		if err := m.refuseWhileBlocked(ctx, current, []TaskStatus{TaskStatusInProgress}); err != nil {
			return Task{}, err
		}
	}
	return patched, nil
}

func (m *taskManager) foreseeMove(ctx context.Context, requesterID string, op BatchOperation, current Task) (Task, error) {
	if err := m.requireMoveOn(ctx, requesterID, op.Backlog); err != nil {
		return Task{}, err
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return Task{}, err
	}
	_, step, err := locateStep(backlogs, op.Backlog, op.Step)
	if err != nil {
		return Task{}, err
	}
	path, err := movePath(current, step)
	if err != nil {
		return Task{}, err
	}
	if err := m.refuseWhileBlocked(ctx, current, path); err != nil {
		return Task{}, err
	}
	current.Backlog, current.Step = op.Backlog, op.Step
	if len(path) > 0 {
		current.Status = path[len(path)-1]
	}
	return current, nil
}
