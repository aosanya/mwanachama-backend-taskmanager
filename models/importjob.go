package models

// ImportResult is returned by TaskManager.ImportProject.
type ImportResult struct {
	// Project is the newly created Project.
	Project Project

	// Tasks are the Task entities created in document order.
	Tasks []Task

	// DepsCreated is the number of depends_on edges written between tasks.
	DepsCreated int

	// TasksCreated is the number of Task entities created (len(Tasks)).
	TasksCreated int
}

// ImportProjectJob tracks an async project-import operation started by
// TaskManager.StartImportProject. Status transitions:
//
//	pending → running → completed | failed | cancelled
type ImportProjectJob struct {
	ID string `json:"id"`

	// Status is the current lifecycle state ("pending", "running",
	// "completed", "failed", "cancelled").
	Status string `json:"status"`

	// ErrorMessage is set when Status is "failed".
	ErrorMessage string `json:"error_message,omitempty"`

	// ProgressSteps are in-memory log lines captured while the job
	// goroutine is running. Not persisted.
	ProgressSteps []string `json:"progress_steps,omitempty"`

	// TasksCreated is populated once the job reaches "completed".
	TasksCreated int `json:"tasks_created,omitempty"`

	// DepsCreated is populated once the job reaches "completed".
	DepsCreated int `json:"deps_created,omitempty"`

	// ProjectName is populated once the job reaches "completed".
	ProjectName string `json:"project_name,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
