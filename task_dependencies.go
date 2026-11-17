package mwanachamataskmanager

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

type DependencyItem struct {
	ID    string `json:"id"`
	Code  string `json:"code"`
	Title string `json:"title"`
	Step  string `json:"step"`
	State string `json:"state"`
	Open  bool   `json:"open"`
}

type TaskDependencies struct {
	DependsOn       []DependencyItem `json:"depends_on"`
	NeededBy        []DependencyItem `json:"needed_by"`
	BlocksStart     bool             `json:"blocks_start"`
	HiddenDependsOn int              `json:"hidden_depends_on"`
	HiddenNeededBy  int              `json:"hidden_needed_by"`
}

type dependencySide struct {
	items  []DependencyItem
	hidden int
	open   bool
}

func (m *taskManager) TaskDependenciesFor(ctx context.Context, requesterID, taskID string) (TaskDependencies, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return TaskDependencies{}, err
	}
	scope, err := m.readScope(ctx, requesterID)
	if err != nil {
		return TaskDependencies{}, fmt.Errorf("TaskDependencies: %w", err)
	}
	waitsOn, err := m.dependencySide(ctx, scope, taskID, DirectionOutbound)
	if err != nil {
		return TaskDependencies{}, fmt.Errorf("TaskDependencies: %w", err)
	}
	waiting, err := m.dependencySide(ctx, scope, taskID, DirectionInbound)
	if err != nil {
		return TaskDependencies{}, fmt.Errorf("TaskDependencies: %w", err)
	}
	return TaskDependencies{
		DependsOn: waitsOn.items, NeededBy: waiting.items, BlocksStart: waitsOn.open,
		HiddenDependsOn: waitsOn.hidden, HiddenNeededBy: waiting.hidden,
	}, nil
}

func (m *taskManager) dependencySide(ctx context.Context, scope readScope, taskID string, dir Direction) (dependencySide, error) {
	side := dependencySide{items: []DependencyItem{}}
	edges, err := m.TraverseRelationships(ctx, taskID, RelLabelDependsOn, dir)
	if err != nil || len(edges) == 0 {
		return side, err
	}
	ids := make([]string, 0, len(edges))
	for _, e := range edges {
		ids = append(ids, otherEnd(e, taskID))
	}
	others, err := specstore.List[Task](m.store, m.store.Query(ctx, roleTask).Where("id IN ? AND deleted = ?", ids, false), roleTask)
	if err != nil {
		return side, err
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return side, err
	}
	states, err := m.taskStates(ctx)
	if err != nil {
		return side, err
	}
	slices.SortFunc(others, func(a, b Task) int {
		if a.Rank != b.Rank {
			return a.Rank - b.Rank
		}
		return strings.Compare(a.Code, b.Code)
	})
	for _, other := range others {
		open := !slices.Contains(closedStatuses, string(other.Status))
		side.open = side.open || open
		if !scope.sees(other) {
			side.hidden++
			continue
		}
		_, step := placeNames(backlogs, pendingRow{Backlog: other.Backlog, Step: other.Step, Status: string(other.Status)})
		side.items = append(side.items, DependencyItem{
			ID: other.ID, Code: other.Code, Title: other.Title, Step: step,
			State: models.StateNameFor(states, other.Status), Open: open,
		})
	}
	return side, nil
}

func otherEnd(e Relationship, taskID string) string {
	if e.FromID == taskID {
		return e.ToID
	}
	return e.FromID
}
