package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type TaskState struct {
	Name     string       `json:"name"`
	Statuses []TaskStatus `json:"statuses"`
}

func DefaultTaskStates() []TaskState {
	return []TaskState{
		{Name: "To do", Statuses: []TaskStatus{TaskStatusPending}},
		{Name: "Doing", Statuses: []TaskStatus{TaskStatusInProgress}},
		{Name: "Waiting", Statuses: []TaskStatus{TaskStatusBlocked, TaskStatusAwaitingDirection}},
		{Name: "Done", Statuses: []TaskStatus{TaskStatusCompleted}},
		{Name: "Cancelled", Statuses: []TaskStatus{TaskStatusCancelled}},
	}
}

func StateNameFor(states []TaskState, status TaskStatus) string {
	for _, s := range states {
		for _, mapped := range s.Statuses {
			if mapped == status {
				return s.Name
			}
		}
	}
	return ""
}

var knownTaskStatuses = map[TaskStatus]bool{
	TaskStatusPending: true, TaskStatusInProgress: true, TaskStatusCompleted: true,
	TaskStatusFailed: true, TaskStatusCancelled: true, TaskStatusBlocked: true,
	TaskStatusAwaitingDirection: true, TaskStatusSplit: true,
}

func ParseTaskStates(value any) ([]TaskState, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("task states: %w", err)
	}
	var states []TaskState
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&states); err != nil {
		return nil, fmt.Errorf("task states: %w", err)
	}
	if len(states) == 0 {
		return nil, fmt.Errorf("task states: none declared")
	}
	names := map[string]bool{}
	held := map[TaskStatus]string{}
	for _, s := range states {
		if strings.TrimSpace(s.Name) == "" {
			return nil, fmt.Errorf("task states: a state has no name")
		}
		if names[s.Name] {
			return nil, fmt.Errorf("task states: %q is declared twice", s.Name)
		}
		names[s.Name] = true
		for _, status := range s.Statuses {
			if !knownTaskStatuses[status] {
				return nil, fmt.Errorf("task states: %q maps %q, which is not a task status", s.Name, status)
			}
			if other, ok := held[status]; ok {
				return nil, fmt.Errorf("task states: %q is mapped by both %q and %q", status, other, s.Name)
			}
			held[status] = s.Name
		}
	}
	return states, nil
}
