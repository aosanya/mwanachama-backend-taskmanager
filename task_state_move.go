package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

var ErrUnknownTaskState = errors.New("unknown task state")

var ErrNoPathToState = errors.New("no legal path to that state")

type TaskStateList struct {
	Items []models.TaskState `json:"items"`
}

var allTaskStatuses = []TaskStatus{
	TaskStatusPending, TaskStatusInProgress, TaskStatusBlocked, TaskStatusAwaitingDirection,
	TaskStatusFailed, TaskStatusSplit, TaskStatusCompleted, TaskStatusCancelled,
}

func (m *taskManager) ListTaskStates(ctx context.Context) (TaskStateList, error) {
	states, err := m.taskStates(ctx)
	if err != nil {
		return TaskStateList{}, err
	}
	return TaskStateList{Items: states}, nil
}

func (m *taskManager) MoveTaskToState(ctx context.Context, taskID, state string) (Task, error) {
	states, err := m.taskStates(ctx)
	if err != nil {
		return Task{}, err
	}
	target, ok := findTaskState(states, state)
	if !ok {
		return Task{}, fmt.Errorf("%w: %q", ErrUnknownTaskState, state)
	}
	task, err := m.GetTask(ctx, taskID)
	if err != nil {
		return Task{}, err
	}
	path, ok := StatusPathToState(task.Status, target)
	if !ok {
		return Task{}, fmt.Errorf("%w: a task in %s cannot reach %q", ErrNoPathToState, task.Status, target.Name)
	}
	for _, next := range path {
		task.Status = next
		task, err = m.UpdateTask(ctx, task)
		if err != nil {
			return Task{}, err
		}
	}
	return task, nil
}

func findTaskState(states []models.TaskState, name string) (models.TaskState, bool) {
	for _, s := range states {
		if s.Name == name {
			return s, true
		}
	}
	return models.TaskState{}, false
}

func StatusPathToState(from TaskStatus, target models.TaskState) ([]TaskStatus, bool) {
	rank := map[TaskStatus]int{}
	for i, st := range target.Statuses {
		rank[st] = i
	}
	if _, in := rank[from]; in {
		return nil, true
	}
	previous := map[TaskStatus]TaskStatus{from: from}
	frontier := []TaskStatus{from}
	for len(frontier) > 0 {
		var reached []TaskStatus
		var next []TaskStatus
		for _, cur := range frontier {
			for _, cand := range allTaskStatuses {
				if _, seen := previous[cand]; seen || !cur.CanTransitionTo(cand) {
					continue
				}
				previous[cand] = cur
				next = append(next, cand)
				if _, in := rank[cand]; in {
					reached = append(reached, cand)
				}
			}
		}
		if len(reached) > 0 {
			best := reached[0]
			for _, r := range reached[1:] {
				if rank[r] < rank[best] {
					best = r
				}
			}
			return walkBack(previous, from, best), true
		}
		frontier = next
	}
	return nil, false
}

func walkBack(previous map[TaskStatus]TaskStatus, from, to TaskStatus) []TaskStatus {
	var path []TaskStatus
	for cur := to; cur != from; cur = previous[cur] {
		path = append([]TaskStatus{cur}, path...)
	}
	return path
}
