package mwanachamataskmanager

import "time"

func timesForStatus(before TaskTodo, status TodoStatus, now string) map[string]any {
	switch status {
	case TodoStatusPending:
		return map[string]any{"started_at": "", "done_at": ""}
	case TodoStatusDispatched:
		started := before.StartedAt
		if started == "" {
			started = now
		}
		return map[string]any{"started_at": started, "done_at": ""}
	case TodoStatusCompleted:
		return map[string]any{"done_at": now}
	case TodoStatusBlocked:
		return map[string]any{"done_at": ""}
	}
	return map[string]any{}
}

func withElapsed(todo TaskTodo, now time.Time) TaskTodo {
	todo.ElapsedSeconds = nil
	started, err := time.Parse(time.RFC3339, todo.StartedAt)
	if err != nil {
		return todo
	}
	end := now
	if done, err := time.Parse(time.RFC3339, todo.DoneAt); err == nil {
		end = done
	}
	seconds := int64(end.Sub(started) / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	todo.ElapsedSeconds = &seconds
	return todo
}

func withElapsedAll(todos []TaskTodo) []TaskTodo {
	now := time.Now().UTC()
	for at := range todos {
		todos[at] = withElapsed(todos[at], now)
	}
	return todos
}
