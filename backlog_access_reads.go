package mwanachamataskmanager

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

type readScope struct {
	visible  backlogVisibility
	backlogs []models.Backlog
}

func (m *taskManager) readScope(ctx context.Context, requesterID string) (readScope, error) {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil || visible.all {
		return readScope{visible: visible}, err
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return readScope{}, err
	}
	return readScope{visible: visible, backlogs: backlogs}, nil
}

func (s readScope) open() bool { return s.visible.all }

func (s readScope) sees(t Task) bool {
	if s.visible.all {
		return true
	}
	backlog, _ := models.PlaceOf(s.backlogs, t)
	return s.visible.Sees(backlog)
}

func (s readScope) keep(tasks []Task) []Task {
	if s.visible.all {
		return tasks
	}
	kept := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		if s.sees(t) {
			kept = append(kept, t)
		}
	}
	return kept
}

func (m *taskManager) GetTaskFor(ctx context.Context, requesterID, taskID string) (Task, error) {
	task, err := m.GetTask(ctx, taskID)
	if err != nil {
		return Task{}, err
	}
	scope, err := m.readScope(ctx, requesterID)
	if err != nil {
		return Task{}, fmt.Errorf("GetTask: %w", err)
	}
	if !scope.sees(task) {
		return Task{}, ErrTaskNotFound
	}
	return task, nil
}

func (m *taskManager) GetTaskByNameFor(ctx context.Context, requesterID, projectName, taskName string) (Task, error) {
	task, err := m.GetTaskByName(ctx, projectName, taskName)
	if err != nil {
		return Task{}, err
	}
	scope, err := m.readScope(ctx, requesterID)
	if err != nil {
		return Task{}, fmt.Errorf("GetTaskByName: %w", err)
	}
	if !scope.sees(task) {
		return Task{}, ErrTaskNotFound
	}
	return task, nil
}

func (m *taskManager) ListTaskAssignmentsFor(ctx context.Context, requesterID, taskID string) ([]Assignment, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return nil, err
	}
	return m.ListTaskAssignments(ctx, taskID)
}

func (m *taskManager) ListTasksInProjectFor(ctx context.Context, requesterID, projectID string) ([]Task, error) {
	tasks, err := m.ListTasksInProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	scope, err := m.readScope(ctx, requesterID)
	if err != nil {
		return nil, fmt.Errorf("ListTasksInProject: %w", err)
	}
	return scope.keep(tasks), nil
}

func (m *taskManager) ListCaseTasksFor(ctx context.Context, requesterID, caseID string) ([]Task, error) {
	tasks, err := m.ListCaseTasks(ctx, caseID)
	if err != nil {
		return nil, err
	}
	scope, err := m.readScope(ctx, requesterID)
	if err != nil {
		return nil, fmt.Errorf("ListCaseTasks: %w", err)
	}
	return scope.keep(tasks), nil
}

func (m *taskManager) ListTasksForRunFor(ctx context.Context, requesterID, runID string) ([]Task, error) {
	return m.ListTasks(ctx, TaskFilter{WorkflowRunID: runID, RequesterID: requesterID})
}

func (m *taskManager) TraverseRelationshipsFor(ctx context.Context, requesterID, vertexID, label string, dir Direction) ([]Relationship, error) {
	edges, err := m.TraverseRelationships(ctx, vertexID, label, dir)
	if err != nil {
		return nil, err
	}
	scope, err := m.readScope(ctx, requesterID)
	if err != nil {
		return nil, fmt.Errorf("TraverseRelationships: %w", err)
	}
	if scope.open() || len(edges) == 0 {
		return edges, nil
	}
	ends := make([]string, 0, len(edges)+1)
	ends = append(ends, vertexID)
	for _, e := range edges {
		ends = append(ends, e.FromID, e.ToID)
	}
	hidden, err := m.hiddenTaskIDs(ctx, scope, ends)
	if err != nil {
		return nil, fmt.Errorf("TraverseRelationships: %w", err)
	}
	if hidden[vertexID] {
		return []Relationship{}, nil
	}
	kept := make([]Relationship, 0, len(edges))
	for _, e := range edges {
		if !hidden[e.FromID] && !hidden[e.ToID] {
			kept = append(kept, e)
		}
	}
	return kept, nil
}

func (m *taskManager) hiddenTaskIDs(ctx context.Context, scope readScope, ids []string) (map[string]bool, error) {
	q := m.store.Query(ctx, roleTask).Where("id IN ? AND deleted = ?", ids, false)
	tasks, err := specstore.List[Task](m.store, q, roleTask)
	if err != nil {
		return nil, err
	}
	hidden := map[string]bool{}
	for _, t := range tasks {
		if !scope.sees(t) {
			hidden[t.ID] = true
		}
	}
	return hidden, nil
}
