package mwanachamataskmanager

import (
	"context"
	"fmt"
)

func priorityLevel(p TaskPriority) int {
	for i, known := range priorityOrder {
		if known == p {
			return i
		}
	}
	return len(priorityOrder)
}

func (m *taskManager) settleNewPriority(ctx context.Context, requesterID string, before Task, now TaskPriority) error {
	if err := m.rankAmongPriorities(ctx, before.ID, before.Backlog, before.Step, now); err != nil {
		return err
	}
	return m.logPriorityChange(ctx, requesterID, before.ID, before.Priority, now)
}

func (m *taskManager) rankAmongPriorities(ctx context.Context, taskID, backlogID, stepID string, now TaskPriority) error {
	if backlogID == "" {
		return nil
	}
	cards, err := m.ListTasks(ctx, TaskFilter{Backlog: backlogID, Step: stepID})
	if err != nil {
		return fmt.Errorf("rankAmongPriorities: %w", err)
	}
	others := make([]Task, 0, len(cards))
	for _, card := range cards {
		if card.ID != taskID {
			others = append(others, card)
		}
	}
	for _, card := range others {
		if priorityLevel(card.Priority) >= priorityLevel(now) {
			_, err := m.PlaceTask(ctx, taskID, "", card.ID)
			return err
		}
	}
	if len(others) == 0 || len(cards) >= maxListPage {
		return nil
	}
	_, err = m.PlaceTask(ctx, taskID, others[len(others)-1].ID, "")
	return err
}

func (m *taskManager) logPriorityChange(ctx context.Context, requesterID, taskID string, from, to TaskPriority) error {
	_, err := m.AddTaskNote(ctx, requesterID, taskID, noteByWorker, fmt.Sprintf("Priority %s to %s", from, to), nil)
	return err
}
