package models

type OrchestrationFilter struct {
	Assignee     string
	Unassigned   bool
	Blocked      *bool
	Waiting      *bool
	Sinbin       *bool
	HasTodos     *bool
	TodosOpen    bool
	Effort       string
	CreatedSince string
	UpdatedSince string
	HasFiles     *bool
	LinkedRef    string
	IDs          []string
	Sort         string

	MinDaysInStep int
}

func (f TaskFilter) Orchestration() OrchestrationFilter {
	return OrchestrationFilter{
		Assignee: f.Assignee, Unassigned: f.Unassigned, Blocked: f.Blocked, Waiting: f.Waiting, Sinbin: f.Sinbin,
		HasTodos: f.HasTodos, TodosOpen: f.TodosOpen, Effort: f.Effort, CreatedSince: f.CreatedSince,
		UpdatedSince: f.UpdatedSince, HasFiles: f.HasFiles, LinkedRef: f.LinkedRef, IDs: f.IDs, Sort: f.Sort, MinDaysInStep: f.MinDaysInStep,
	}
}

func (f PendingFilter) Orchestration() OrchestrationFilter {
	return OrchestrationFilter{
		Assignee: f.Assignee, Unassigned: f.Unassigned, Blocked: f.Blocked, Waiting: f.Waiting, Sinbin: f.Sinbin,
		HasTodos: f.HasTodos, TodosOpen: f.TodosOpen, Effort: f.Effort, CreatedSince: f.CreatedSince,
		UpdatedSince: f.UpdatedSince, HasFiles: f.HasFiles, LinkedRef: f.LinkedRef, IDs: f.IDs, Sort: f.Sort, MinDaysInStep: f.MinDaysInStep,
	}
}
