package mwanachamataskmanager

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

const (
	tagReady   = "Ready"
	tagWaiting = "Waiting"
	tagBlocked = "Blocked"
	tagSinbin  = "Sinbin"
)

func finishesDependency(s TaskStatus) bool {
	return s == TaskStatusCompleted || s == TaskStatusCancelled
}

func (m *taskManager) openDependencies(ctx context.Context, taskID string) ([]string, error) {
	edges, err := m.TraverseRelationships(ctx, taskID, RelLabelDependsOn, DirectionOutbound)
	if err != nil {
		return nil, err
	}
	var open []string
	for _, e := range edges {
		dep, err := m.GetTask(ctx, e.ToID)
		if err != nil || !finishesDependency(dep.Status) {
			open = append(open, e.ToID)
		}
	}
	return open, nil
}

func (m *taskManager) freeDependents(ctx context.Context, finished Task) {
	edges, err := m.TraverseRelationships(ctx, finished.ID, RelLabelDependsOn, DirectionInbound)
	if err != nil {
		log.Printf("mwanachamataskmanager: freeDependents: traverse %s: %v", finished.ID, err)
		return
	}
	reason := fmt.Sprintf("%s finished (%s): no open dependency is left, Waiting dropped and Ready added.", labelOf(finished), finished.Status)
	for _, e := range edges {
		m.dropWaitingIfClear(ctx, e.FromID, reason)
	}
}

func (m *taskManager) dropWaitingIfClear(ctx context.Context, taskID, reason string) {
	task, err := m.GetTask(ctx, taskID)
	if err != nil || !hasTagFold(task.Tags, tagWaiting) || finishesDependency(task.Status) {
		return
	}
	open, err := m.openDependencies(ctx, taskID)
	if err != nil || len(open) > 0 {
		return
	}
	if err := m.setTaskTags(ctx, taskID, swapMark(task.Tags, tagWaiting, tagReady)); err != nil {
		log.Printf("mwanachamataskmanager: dropWaitingIfClear: setTaskTags %s: %v", taskID, err)
		return
	}
	m.logAsHub(ctx, taskID, reason)
}

func (m *taskManager) markWaitingIfOpen(ctx context.Context, taskID string) {
	task, err := m.GetTask(ctx, taskID)
	if err != nil || hasTagFold(task.Tags, tagWaiting) || hasTagFold(task.Tags, tagBlocked) || finishesDependency(task.Status) {
		return
	}
	open, err := m.openDependencies(ctx, taskID)
	if err != nil || len(open) == 0 {
		return
	}
	if err := m.setTaskTags(ctx, taskID, swapMark(task.Tags, tagReady, tagWaiting)); err != nil {
		log.Printf("mwanachamataskmanager: markWaitingIfOpen: setTaskTags %s: %v", taskID, err)
		return
	}
	m.logAsHub(ctx, taskID, "A dependency is open: Waiting added, Ready dropped.")
}

func swapMark(tags []string, from, to string) []string {
	out := make([]string, 0, len(tags)+1)
	for _, tag := range tags {
		if !strings.EqualFold(tag, from) && !strings.EqualFold(tag, to) {
			out = append(out, tag)
		}
	}
	return append(out, to)
}

func labelOf(t Task) string {
	if t.Code != "" {
		return t.Code
	}
	return t.Title
}

func (m *taskManager) logAsHub(ctx context.Context, taskID, text string) {
	note := TaskNote{
		ID: newSpecID(), TaskID: taskID, Kind: noteByWorker,
		Text: text, CreatedAt: time.Now().UTC().Format(noteTimeForm),
	}
	if err := m.store.Insert(ctx, roleTaskNote, note); err != nil {
		log.Printf("mwanachamataskmanager: logAsHub: %s: %v", taskID, err)
	}
}
