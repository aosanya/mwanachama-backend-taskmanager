package models

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
