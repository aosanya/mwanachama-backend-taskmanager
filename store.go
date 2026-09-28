package mwanachamataskmanager

import (
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	roleTask               = "task"
	roleTaskTodo           = "task_todo"
	roleAgent              = "agent"
	roleProject            = "project"
	roleTag                = "tag"
	roleDeliverable        = "deliverable"
	roleAcceptanceCriteria = "acceptance_criteria"
	roleWorkflowRun        = "workflow_run"
	roleImportJob          = "import_job"
	roleBlocker            = "blocker"
	roleDependency         = "dependency"
	roleMembership         = "membership"
	roleTagging            = "tagging"
	roleCodeSequence       = "code_sequence"
)

func roles() []string {
	return []string{
		roleTask, roleTaskTodo, roleAgent, roleProject, roleTag,
		roleDeliverable, roleAcceptanceCriteria, roleWorkflowRun, roleImportJob,
		roleBlocker, roleDependency, roleMembership, roleTagging, roleCodeSequence,
	}
}

func carriers() map[string]any {
	return map[string]any{
		roleTask:               models.Task{},
		roleTaskTodo:           models.TaskTodo{},
		roleAgent:              models.Agent{},
		roleProject:            models.Project{},
		roleTag:                models.Tag{},
		roleDeliverable:        models.Deliverable{},
		roleAcceptanceCriteria: models.AcceptanceCriteria{},
		roleWorkflowRun:        models.WorkflowRun{},
		roleImportJob:          models.ImportProjectJob{},
		roleBlocker:            models.Blocker{},
		roleDependency:         models.Dependency{},
		roleMembership:         models.Membership{},
		roleTagging:            models.Tagging{},
		roleCodeSequence:       models.CodeSequence{},
	}
}

type store = specstore.Store

func newStore(db *gorm.DB, s *spec.Spec, carriers map[string]any) (*store, error) {
	return specstore.New(db, s, carriers)
}

func newSpecID() string { return specstore.NewID() }

func columnName(field string) string { return specstore.ColumnName(field) }

func encode(o spec.Object, v any) (map[string]any, error) { return specstore.Encode(o, v) }

func decode(o spec.Object, row map[string]any, out any) error {
	return specstore.Decode(o, row, out)
}
