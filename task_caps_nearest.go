package mwanachamataskmanager

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

func nearestCap(caps WorkerCaps, used *TaskUsage) *NearestCap {
	if used == nil {
		return nil
	}
	candidates := []NearestCap{
		{Key: "input_tokens", Used: used.InputTokens, Cap: caps.InputTokens},
		{Key: "output_tokens", Used: used.OutputTokens, Cap: caps.OutputTokens},
		{Key: "runtime_seconds", Used: used.SecondsUsed, Cap: caps.RuntimeSeconds},
	}
	var nearest *NearestCap
	for i := range candidates {
		candidates[i].Ratio = float64(candidates[i].Used) / float64(candidates[i].Cap)
		if nearest == nil || candidates[i].Ratio > nearest.Ratio {
			nearest = &candidates[i]
		}
	}
	return nearest
}

func (m *taskManager) nearestTaskIDs(ctx context.Context, filter CapsNearestFilter) ([]string, error) {
	q := m.store.Query(ctx, roleTask).Where("deleted = ?", false)
	if len(filter.TaskIDs) > 0 {
		q = q.Where("id IN ?", []string(filter.TaskIDs))
	}
	q, err := m.narrowToPlace(ctx, q, TaskFilter{Backlog: filter.Backlog})
	if err != nil {
		return nil, err
	}
	q, err = m.narrowToVisible(ctx, q, filter.RequesterID)
	if err != nil {
		return nil, err
	}
	var ids []string
	err = q.Order(taskOrder).Limit(maxListPage).Pluck("id", &ids).Error
	return ids, err
}

func (m *taskManager) NearestCaps(ctx context.Context, filter CapsNearestFilter) (CapsNearestList, error) {
	if len(filter.TaskIDs) == 0 && filter.Backlog == "" {
		return CapsNearestList{}, fmt.Errorf("NearestCaps: %w: send task_id or backlog", ErrInvalidFilter)
	}
	if len(filter.TaskIDs) > maxListPage {
		return CapsNearestList{}, fmt.Errorf("NearestCaps: %w: at most %d task_id", ErrInvalidFilter, maxListPage)
	}
	ids, err := m.nearestTaskIDs(ctx, filter)
	if err != nil {
		return CapsNearestList{}, fmt.Errorf("NearestCaps: %w", err)
	}
	out := CapsNearestList{Items: []CapsNearest{}}
	if len(ids) == 0 {
		return out, nil
	}
	defaults, err := m.GetWorkerCapsDefault(ctx)
	if err != nil {
		return CapsNearestList{}, err
	}
	owns, err := specstore.List[TaskCapsOverride](m.store, m.store.Query(ctx, roleTaskCaps).Where("id IN ?", ids), roleTaskCaps)
	if err != nil {
		return CapsNearestList{}, fmt.Errorf("NearestCaps: %w", err)
	}
	usages, err := specstore.List[TaskUsage](m.store, m.store.Query(ctx, roleTaskUsage).Where("id IN ?", ids), roleTaskUsage)
	if err != nil {
		return CapsNearestList{}, fmt.Errorf("NearestCaps: %w", err)
	}
	ownOf := map[string]TaskCapsOverride{}
	for _, own := range owns {
		ownOf[own.ID] = own
	}
	usedOf := map[string]*TaskUsage{}
	for i := range usages {
		usedOf[usages[i].ID] = &usages[i]
	}
	for _, id := range ids {
		inForce := withOwn(defaults, ownOf[id])
		out.Items = append(out.Items, CapsNearest{TaskID: id, InForce: inForce, Nearest: nearestCap(inForce, usedOf[id])})
	}
	return out, nil
}
