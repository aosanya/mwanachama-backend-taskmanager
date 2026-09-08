package gormstore

import "gorm.io/gorm"

// TableNames configures which physical tables one mounted instance of this
// package reads and writes.
type TableNames struct {
	Tasks                  string
	TaskTodos              string
	Agents                 string
	Projects               string
	Tags                   string
	Deliverables           string
	AcceptanceCriteria     string
	WorkflowRuns           string
	ImportProjectJobs      string
	TaskBlocks             string
	TaskDependencies       string
	TaskProjectMemberships string
	TaskTags               string
}

// DefaultTableNames builds the conventional table set for one mounted
// instance of this package.
func DefaultTableNames(instance string) TableNames {
	return TableNames{
		Tasks:                  instance + "_tasks",
		TaskTodos:              instance + "_task_todos",
		Agents:                 instance + "_agents",
		Projects:               instance + "_projects",
		Tags:                   instance + "_tags",
		Deliverables:           instance + "_deliverables",
		AcceptanceCriteria:     instance + "_acceptance_criteria",
		WorkflowRuns:           instance + "_workflow_runs",
		ImportProjectJobs:      instance + "_import_project_jobs",
		TaskBlocks:             instance + "_task_blocks",
		TaskDependencies:       instance + "_task_dependencies",
		TaskProjectMemberships: instance + "_task_project_memberships",
		TaskTags:               instance + "_task_tags",
	}
}

// Migrate creates or updates every table named by t.
func Migrate(db *gorm.DB, t TableNames) error {
	migrations := []struct {
		table string
		model any
	}{
		{t.Tasks, &TaskRow{}},
		{t.TaskTodos, &TaskTodoRow{}},
		{t.Agents, &AgentRow{}},
		{t.Projects, &ProjectRow{}},
		{t.Tags, &TagRow{}},
		{t.Deliverables, &DeliverableRow{}},
		{t.AcceptanceCriteria, &AcceptanceCriteriaRow{}},
		{t.WorkflowRuns, &WorkflowRunRow{}},
		{t.ImportProjectJobs, &ImportProjectJobRow{}},
		{t.TaskBlocks, &TaskBlockRow{}},
		{t.TaskDependencies, &TaskDependencyRow{}},
		{t.TaskProjectMemberships, &TaskProjectMembershipRow{}},
		{t.TaskTags, &TaskTagRow{}},
	}
	for _, m := range migrations {
		if err := db.Table(m.table).AutoMigrate(m.model); err != nil {
			return err
		}
	}
	return nil
}
