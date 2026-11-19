package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"
)

const todoDoneState = "Done"

type TodoFinished struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

func (m *taskManager) FinishTodoFor(ctx context.Context, requesterID, todoID, text string, links []string) (TodoFinished, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return TodoFinished{}, ErrInvalidTaskNote
	}
	done, err := m.SetTodoStateFor(ctx, requesterID, todoID, todoDoneState, "")
	if err != nil {
		return TodoFinished{}, err
	}
	if done.State == todoDoneState {
		if _, err := m.AddTaskNote(ctx, requesterID, done.ParentTaskID, noteByWorker, text, links); err != nil {
			return TodoFinished{}, fmt.Errorf("FinishTodoFor: write the entry: %w", err)
		}
	}
	return TodoFinished{Code: done.Code, State: done.State}, nil
}
