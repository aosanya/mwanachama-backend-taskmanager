package mwanachamataskmanager

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	defaultBriefLogLimit  = 10
	maxBriefLogLimit      = 50
	briefDependencyNotes  = 3
	briefPersonNotesLimit = maxNoteLimit
)

type TaskBriefFilter struct {
	LogLimit int `query:"log_limit,min=1"`

	RequesterID string `query:"-" json:"-"`
	TaskID      string `query:"-" json:"-"`
}

type BriefDependency struct {
	DependencyItem
	LastEntries []TaskNote `json:"last_entries"`
}

type TaskBrief struct {
	Issue           Task              `json:"issue"`
	Caps            TaskCaps          `json:"caps"`
	Log             []TaskNote        `json:"log"`
	Todos           IssueTodoList     `json:"todos"`
	DependsOn       []BriefDependency `json:"depends_on"`
	NeededBy        []DependencyItem  `json:"needed_by"`
	HiddenDependsOn int               `json:"hidden_depends_on"`
	HiddenNeededBy  int               `json:"hidden_needed_by"`
}

func (m *taskManager) TaskBriefFor(ctx context.Context, filter TaskBriefFilter) (TaskBrief, error) {
	limit := filter.LogLimit
	if limit == 0 {
		limit = defaultBriefLogLimit
	}
	if limit < 1 || limit > maxBriefLogLimit {
		return TaskBrief{}, fmt.Errorf("TaskBrief: %w: log_limit %d is not between 1 and %d", ErrInvalidFilter, limit, maxBriefLogLimit)
	}
	issue, err := m.GetTaskFor(ctx, filter.RequesterID, filter.TaskID)
	if err != nil {
		return TaskBrief{}, err
	}
	caps, err := m.capsReading(ctx, filter.TaskID)
	if err != nil {
		return TaskBrief{}, err
	}
	log, err := m.briefLog(ctx, filter, limit)
	if err != nil {
		return TaskBrief{}, err
	}
	todos, err := m.issueTodoList(ctx, filter.TaskID)
	if err != nil {
		return TaskBrief{}, err
	}
	dependencies, err := m.TaskDependenciesFor(ctx, filter.RequesterID, filter.TaskID)
	if err != nil {
		return TaskBrief{}, err
	}
	waitsOn, err := m.briefDependencies(ctx, filter.RequesterID, dependencies.DependsOn)
	if err != nil {
		return TaskBrief{}, err
	}
	return TaskBrief{
		Issue: issue, Caps: caps, Log: log, Todos: todos, DependsOn: waitsOn, NeededBy: dependencies.NeededBy,
		HiddenDependsOn: dependencies.HiddenDependsOn, HiddenNeededBy: dependencies.HiddenNeededBy,
	}, nil
}

func (m *taskManager) briefLog(ctx context.Context, filter TaskBriefFilter, limit int) ([]TaskNote, error) {
	newest, err := m.ListTaskNotesFiltered(ctx, TaskNoteFilter{RequesterID: filter.RequesterID, TaskID: filter.TaskID, Limit: limit})
	if err != nil {
		return nil, err
	}
	fromPeople, err := m.ListTaskNotesFiltered(ctx, TaskNoteFilter{
		RequesterID: filter.RequesterID, TaskID: filter.TaskID, Kind: models.TagList{noteByPerson}, Limit: briefPersonNotesLimit,
	})
	if err != nil {
		return nil, err
	}
	kept := map[string]bool{}
	for _, note := range newest {
		kept[note.ID] = true
	}
	for _, note := range fromPeople {
		if !kept[note.ID] {
			newest = append(newest, note)
		}
	}
	if newest == nil {
		newest = []TaskNote{}
	}
	slices.SortStableFunc(newest, func(a, b TaskNote) int {
		return strings.Compare(a.CreatedAt+a.ID, b.CreatedAt+b.ID)
	})
	return newest, nil
}

func (m *taskManager) briefDependencies(ctx context.Context, requesterID string, items []DependencyItem) ([]BriefDependency, error) {
	out := make([]BriefDependency, 0, len(items))
	for _, item := range items {
		entries, err := m.ListTaskNotesFiltered(ctx, TaskNoteFilter{RequesterID: requesterID, TaskID: item.ID, Limit: briefDependencyNotes})
		if err != nil {
			return nil, err
		}
		if entries == nil {
			entries = []TaskNote{}
		}
		out = append(out, BriefDependency{DependencyItem: item, LastEntries: entries})
	}
	return out, nil
}
