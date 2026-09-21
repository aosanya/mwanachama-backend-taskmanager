// workflow_run.go — WorkflowRun lifecycle and closure traversal.
//
// A WorkflowRun anchors the constellation of Tasks, TaskTodos, edges, and
// cross-service references produced by a single orchestrated execution.
// Its closure is what [TaskManager.RollbackWorkflowRun] compensates.
package mwanachamataskmanager

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
)

// runNameSuffixBytes is the number of random bytes that feed the
// 6-hex-char suffix in [generateRunName].
const runNameSuffixBytes = 3

// CreateWorkflowRun anchors a new orchestrated execution.
//
// If name is empty, the server generates one of the form
// `pipeline-YYYY-MM-DD-HHMMSS-<6hex>`. If name is set, leading/trailing
// whitespace is rejected (returns [ErrInvalidTask]); case is preserved.
//
// Returns [ErrWorkflowRunNameExists] when a run with the same name already
// exists — names are immutable, so the caller should append a
// discriminator and retry.
func (m *taskManager) CreateWorkflowRun(ctx context.Context, name, triggerEvent, initiator string) (WorkflowRun, error) {
	if name != "" && strings.TrimSpace(name) != name {
		return WorkflowRun{}, fmt.Errorf("%w: WorkflowRun.Name must not have leading/trailing whitespace", ErrInvalidTask)
	}
	now := time.Now().UTC()
	if name == "" {
		name = generateRunName(now)
	}
	if existing, err := m.GetWorkflowRunByName(ctx, name); err == nil && existing.ID != "" {
		return WorkflowRun{}, ErrWorkflowRunNameExists
	} else if err != nil && !errors.Is(err, ErrWorkflowRunNotFound) {
		return WorkflowRun{}, fmt.Errorf("CreateWorkflowRun: name precheck: %w", err)
	}
	run := WorkflowRun{
		Name:         name,
		Status:       WorkflowRunStatusPending,
		TriggerEvent: triggerEvent,
		Initiator:    initiator,
		StartedAt:    now.Format(time.RFC3339),
		CreatedAt:    now.Format(time.RFC3339),
		UpdatedAt:    now.Format(time.RFC3339),
		LastEventAt:  now.Format(time.RFC3339),
	}
	row := gormstore.WorkflowRunToRow(run)
	if err := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).Create(&row).Error; err != nil {
		return WorkflowRun{}, fmt.Errorf("CreateWorkflowRun: %w", err)
	}
	return gormstore.WorkflowRunFromRow(row), nil
}

// generateRunName builds a deterministic-looking but collision-resistant
// label of the form `pipeline-YYYY-MM-DD-HHMMSS-<6hex>`. The 6 hex chars
// come from 3 random bytes — ~16M possibilities, so collisions across
// 1k runs/day are negligible. Falls back to a UTC seconds-based suffix
// if the OS RNG is unavailable.
func generateRunName(now time.Time) string {
	var buf [runNameSuffixBytes]byte
	suffix := ""
	if _, err := rand.Read(buf[:]); err == nil {
		suffix = hex.EncodeToString(buf[:])
	} else {
		suffix = fmt.Sprintf("%06x", now.UnixNano()&0xFFFFFF)
	}
	return fmt.Sprintf("pipeline-%s-%s", now.UTC().Format("2006-01-02-150405"), suffix)
}

// GetWorkflowRun reads a single WorkflowRun row.
func (m *taskManager) GetWorkflowRun(ctx context.Context, runID string) (WorkflowRun, error) {
	var row gormstore.WorkflowRunRow
	err := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).Where("id = ?", runID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return WorkflowRun{}, ErrWorkflowRunNotFound
		}
		return WorkflowRun{}, fmt.Errorf("GetWorkflowRun: %w", err)
	}
	return gormstore.WorkflowRunFromRow(row), nil
}

// ListWorkflowRuns returns every WorkflowRun, sorted newest first by
// created_at. Returns an empty slice (not an error) when none exist.
//
// When name is non-empty, the result is filtered to runs whose Name field
// matches exactly — at most one row given Name's uniqueness.
func (m *taskManager) ListWorkflowRuns(ctx context.Context, name string) ([]WorkflowRun, error) {
	q := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns)
	if name != "" {
		q = q.Where("name = ?", name)
	}
	var rows []gormstore.WorkflowRunRow
	if err := q.Order("created_at DESC").Limit(maxListPage).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListWorkflowRuns: %w", err)
	}
	out := make([]WorkflowRun, 0, len(rows))
	for _, r := range rows {
		out = append(out, gormstore.WorkflowRunFromRow(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// GetWorkflowRunByName looks up a single run by its unique name.
// Returns [ErrWorkflowRunNotFound] when no match exists.
func (m *taskManager) GetWorkflowRunByName(ctx context.Context, name string) (WorkflowRun, error) {
	if name == "" {
		return WorkflowRun{}, fmt.Errorf("%w: WorkflowRun.Name is required", ErrInvalidTask)
	}
	var row gormstore.WorkflowRunRow
	err := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).Where("name = ?", name).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return WorkflowRun{}, ErrWorkflowRunNotFound
		}
		return WorkflowRun{}, fmt.Errorf("GetWorkflowRunByName: %w", err)
	}
	return gormstore.WorkflowRunFromRow(row), nil
}

// LinkTaskToRun writes the started_task edge from the run to a task.
func (m *taskManager) LinkTaskToRun(ctx context.Context, runID, taskID string) error {
	if _, err := m.GetWorkflowRun(ctx, runID); err != nil {
		return err
	}
	if _, err := m.GetTask(ctx, taskID); err != nil {
		return err
	}
	_, err := m.CreateRelationship(ctx, Relationship{Label: RelLabelStartedTask, FromID: runID, ToID: taskID})
	return err
}

// LinkTodoToRun writes the started_todo edge from the run to a todo.
func (m *taskManager) LinkTodoToRun(ctx context.Context, runID, todoID string) error {
	if _, err := m.GetWorkflowRun(ctx, runID); err != nil {
		return err
	}
	if _, err := m.GetTaskTodo(ctx, todoID); err != nil {
		return err
	}
	_, err := m.CreateRelationship(ctx, Relationship{Label: RelLabelStartedTodo, FromID: runID, ToID: todoID})
	return err
}

// GetWorkflowRunClosure walks the started_task / has_todo / assigned_to /
// depends_on edges reachable from the run and returns the full set of
// entities and edges encountered. Edges whose endpoints land outside the
// closure are still included — rollback needs them to plan compensating
// actions on neighbours.
func (m *taskManager) GetWorkflowRunClosure(ctx context.Context, runID string) (WorkflowRunClosure, error) {
	run, err := m.GetWorkflowRun(ctx, runID)
	if err != nil {
		return WorkflowRunClosure{}, err
	}

	closure := WorkflowRunClosure{
		Run:            run,
		AgentRunIDs:    append([]string(nil), run.AgentRunIDs...),
		FunctionJobIDs: append([]string(nil), run.FunctionJobIDs...),
		BranchNames:    append([]string(nil), run.BranchNames...),
	}

	taskIDs := map[string]struct{}{}
	todoIDs := map[string]struct{}{}
	edgeKeys := map[string]struct{}{}

	addEdge := func(rel Relationship) {
		if _, seen := edgeKeys[rel.ID]; seen {
			return
		}
		edgeKeys[rel.ID] = struct{}{}
		closure.Edges = append(closure.Edges, rel)
	}

	// Step 1 — started_task edges from the run.
	startedTaskEdges, err := m.TraverseRelationships(ctx, runID, RelLabelStartedTask, DirectionOutbound)
	if err != nil {
		return WorkflowRunClosure{}, fmt.Errorf("GetWorkflowRunClosure: started_task: %w", err)
	}
	for _, e := range startedTaskEdges {
		addEdge(e)
		taskIDs[e.ToID] = struct{}{}
	}

	// Step 2 — started_todo edges from the run (some producers link todos
	// directly without going through their parent task).
	startedTodoEdges, err := m.TraverseRelationships(ctx, runID, RelLabelStartedTodo, DirectionOutbound)
	if err != nil {
		return WorkflowRunClosure{}, fmt.Errorf("GetWorkflowRunClosure: started_todo: %w", err)
	}
	for _, e := range startedTodoEdges {
		addEdge(e)
		todoIDs[e.ToID] = struct{}{}
	}

	// Step 3 — for each task, walk has_todo, assigned_to, and depends_on
	// (both directions). Tag and member_of edges are not part of the
	// rollback closure.
	for taskID := range taskIDs {
		if todoEdges, err := m.TraverseRelationships(ctx, taskID, RelLabelHasTodo, DirectionOutbound); err == nil {
			for _, e := range todoEdges {
				addEdge(e)
				todoIDs[e.ToID] = struct{}{}
			}
		}
		if assignedEdges, err := m.TraverseRelationships(ctx, taskID, RelLabelAssignedTo, DirectionOutbound); err == nil {
			for _, e := range assignedEdges {
				addEdge(e)
			}
		}
		if dependsOut, err := m.TraverseRelationships(ctx, taskID, RelLabelDependsOn, DirectionOutbound); err == nil {
			for _, e := range dependsOut {
				addEdge(e)
			}
		}
		if dependsIn, err := m.TraverseRelationships(ctx, taskID, RelLabelDependsOn, DirectionInbound); err == nil {
			for _, e := range dependsIn {
				addEdge(e)
			}
		}
	}

	// Step 4 — resolve entities. Tasks first, sorted by created_at for
	// stable output, then todos. Missing entities are skipped (defensive).
	tasks := make([]Task, 0, len(taskIDs))
	for id := range taskIDs {
		if t, err := m.GetTask(ctx, id); err == nil {
			tasks = append(tasks, t)
		}
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt < tasks[j].CreatedAt })
	closure.Tasks = tasks

	todos := make([]TaskTodo, 0, len(todoIDs))
	for id := range todoIDs {
		if td, err := m.GetTaskTodo(ctx, id); err == nil {
			todos = append(todos, td)
		}
	}
	sort.Slice(todos, func(i, j int) bool {
		if todos[i].ParentTaskID != todos[j].ParentTaskID {
			return todos[i].ParentTaskID < todos[j].ParentTaskID
		}
		return todos[i].Ordinality < todos[j].Ordinality
	})
	closure.Todos = todos

	return closure, nil
}

// UpdateWorkflowRunStatus transitions a WorkflowRun to a new lifecycle status.
// Valid transitions are defined by [WorkflowRunStatus.CanTransitionTo]; any
// other request returns [ErrInvalidRunStatusTransition].
func (m *taskManager) UpdateWorkflowRunStatus(ctx context.Context, runID string, newStatus WorkflowRunStatus, reason string) (WorkflowRun, error) {
	run, err := m.GetWorkflowRun(ctx, runID)
	if err != nil {
		return WorkflowRun{}, err
	}
	if !run.Status.CanTransitionTo(newStatus) {
		return WorkflowRun{}, fmt.Errorf("%w: %s → %s", ErrInvalidRunStatusTransition, run.Status, newStatus)
	}
	now := time.Now().UTC()
	run.Status = newStatus
	run.UpdatedAt = now.Format(time.RFC3339)
	if newStatus == WorkflowRunStatusInProgress && run.StartedAt == "" {
		run.StartedAt = run.UpdatedAt
	}
	if newStatus == WorkflowRunStatusCompleted || newStatus == WorkflowRunStatusFailed ||
		newStatus == WorkflowRunStatusRolledBack || newStatus == WorkflowRunStatusRollbackFailed {
		run.CompletedAt = run.UpdatedAt
	}

	row := gormstore.WorkflowRunToRow(run)
	if reason != "" {
		row.FailureReason = reason
	}
	if err := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).Where("id = ?", runID).Save(&row).Error; err != nil {
		return WorkflowRun{}, fmt.Errorf("UpdateWorkflowRunStatus: %w", err)
	}
	result := gormstore.WorkflowRunFromRow(row)

	m.publishRunStatusEvent(ctx, result, now, reason)
	return result, nil
}

// publishRunStatusEvent fires the appropriate work.run.* event after a status transition.
func (m *taskManager) publishRunStatusEvent(ctx context.Context, run WorkflowRun, now time.Time, reason string) {
	var topic string
	var payload any
	switch run.Status {
	case WorkflowRunStatusInProgress:
		topic = TopicRunInProgress
		payload = WorkflowRunInProgressPayload{WorkflowRunID: run.ID, StartedAt: run.StartedAt}
	case WorkflowRunStatusCompleted:
		durationMs := int64(0)
		if run.StartedAt != "" {
			if started, err := time.Parse(time.RFC3339, run.StartedAt); err == nil {
				durationMs = now.UnixMilli() - started.UnixMilli()
			}
		}
		topic = TopicRunCompleted
		payload = WorkflowRunCompletedPayload{WorkflowRunID: run.ID, CompletedAt: run.CompletedAt, DurationMs: durationMs}
	case WorkflowRunStatusFailed:
		topic = TopicRunFailed
		payload = WorkflowRunFailedPayload{WorkflowRunID: run.ID, FailedAt: run.CompletedAt, FailureReason: reason}
	case WorkflowRunStatusRolledBack:
		topic = TopicRunRolledBack
		payload = WorkflowRunRolledBackPayload{WorkflowRunID: run.ID, RolledBackAt: run.CompletedAt, Reason: reason}
	case WorkflowRunStatusRollingBack:
		topic = TopicRunRollingBack
		payload = WorkflowRunRollingBackPayload{WorkflowRunID: run.ID, Reason: reason}
	case WorkflowRunStatusRollbackFailed:
		topic = TopicRunRollbackFailed
		payload = WorkflowRunRollbackFailedPayload{WorkflowRunID: run.ID, FailedAt: run.UpdatedAt, FailureReason: reason}
	case WorkflowRunStatusCancelling:
		topic = TopicRunCancelling
		payload = WorkflowRunCancellingPayload{
			WorkflowRunID:   run.ID,
			Reason:          run.CancelReason,
			CancelledBy:     run.CancelledBy,
			QuiesceDeadline: run.CancellingUntil,
		}
	case WorkflowRunStatusCancelled:
		topic = TopicRunCancelled
		payload = WorkflowRunCancelledPayload{
			WorkflowRunID: run.ID,
			CancelledAt:   run.CompletedAt,
			Reason:        run.CancelReason,
			CancelledBy:   run.CancelledBy,
		}
	default:
		return
	}
	m.publish(ctx, topic, payload)
}

// TouchWorkflowRunLastEventAt bumps last_event_at to ts for the given run.
// Best-effort: returns nil on NotFound (the run may have been deleted or
// rolled back concurrently with the event that triggered this call).
// ts must parse as RFC 3339 and is stored as canonical UTC, since the
// stale-run watchdog compares last_event_at as a string; anything else
// returns [ErrInvalidTask].
func (m *taskManager) TouchWorkflowRunLastEventAt(ctx context.Context, runID, ts string) error {
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return fmt.Errorf("%w: last_event_at must be an RFC 3339 timestamp", ErrInvalidTask)
	}
	return m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).Where("id = ?", runID).
		UpdateColumn("last_event_at", parsed.UTC().Format(time.RFC3339)).Error
}
