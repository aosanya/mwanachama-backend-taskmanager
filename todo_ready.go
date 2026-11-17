package mwanachamataskmanager

import "context"

type ReadyTodo struct {
	ID             string `json:"id"`
	Code           string `json:"code"`
	Title          string `json:"title"`
	Instructions   string `json:"instructions"`
	Ordinality     int    `json:"ordinality"`
	State          string `json:"state"`
	CanRunParallel bool   `json:"can_run_parallel"`

	ElapsedSeconds *int64 `json:"elapsed_seconds,omitempty"`
}

type HeldTodo struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	Title         string `json:"title"`
	Ordinality    int    `json:"ordinality"`
	BlockedReason string `json:"blocked_reason"`
}

type ReadyTodos struct {
	Items   []ReadyTodo `json:"items"`
	Blocked []HeldTodo  `json:"blocked"`
	Done    int         `json:"done"`
	Total   int         `json:"total"`
}

func (m *taskManager) ReadyTodosFor(ctx context.Context, requesterID, issueID string) (ReadyTodos, error) {
	list, err := m.ListTodosOfIssue(ctx, requesterID, issueID)
	if err != nil {
		return ReadyTodos{}, err
	}
	items, held := readyAndHeld(list.Items)
	return ReadyTodos{Items: items, Blocked: held, Done: list.Done, Total: list.Total}, nil
}

func readyAndHeld(views []TodoView) ([]ReadyTodo, []HeldTodo) {
	items := []ReadyTodo{}
	held := []HeldTodo{}
	for _, view := range views {
		switch view.Status {
		case TodoStatusPending, TodoStatusDispatched:
			items = append(items, ReadyTodo{
				ID: view.ID, Code: view.Code, Title: view.Title, Instructions: view.Instructions,
				Ordinality: view.Ordinality, State: view.State, CanRunParallel: view.CanRunParallel,
				ElapsedSeconds: view.ElapsedSeconds,
			})
		case TodoStatusBlocked, TodoStatusFailed:
			if view.BlockedReason != "" {
				held = append(held, HeldTodo{
					ID: view.ID, Code: view.Code, Title: view.Title,
					Ordinality: view.Ordinality, BlockedReason: view.BlockedReason,
				})
			}
		}
	}
	return items, held
}
