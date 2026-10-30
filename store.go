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
	roleAssignment         = "assignment"
	roleCase               = "case"
	roleCaseTask           = "case_task"
	roleCaseVote           = "case_vote"
	roleRepeatingTask      = "repeating_task"
	roleRepeatingPeriod    = "repeating_task_period"
	roleSourcedTask        = "sourced_task"
	roleGeneratedTask      = "generated_task"
)

func roles() []string {
	return []string{
		roleTask, roleTaskTodo, roleAgent, roleProject, roleTag,
		roleDeliverable, roleAcceptanceCriteria, roleWorkflowRun, roleImportJob,
		roleBlocker, roleDependency, roleMembership, roleTagging, roleCodeSequence,
		roleAssignment,
		roleCase, roleCaseTask, roleCaseVote,
		roleRepeatingTask, roleRepeatingPeriod,
		roleSourcedTask, roleGeneratedTask,
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
		roleAssignment:         models.Assignment{},
		roleCase:               models.Case{},
		roleCaseTask:           models.CaseTask{},
		roleCaseVote:           models.CaseVote{},
		roleRepeatingTask:      models.RepeatingTaskRecord{},
		roleRepeatingPeriod:    models.RepeatingTaskPeriod{},
		roleSourcedTask:        models.SourcedTask{},
		roleGeneratedTask:      models.GeneratedTask{},
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

func (m *taskManager) onTable(db *gorm.DB, table string) *gorm.DB {
	q := db.Table(table)
	if id := m.store.RunID(); id != "" {
		q = q.Where(spec.RunIDColumn+" = ?", id)
	}
	return q
}

func (m *taskManager) within(db *gorm.DB, role string) *gorm.DB {
	return m.onTable(db, m.store.Table(role))
}

func (m *taskManager) stamped(row map[string]any) map[string]any {
	if id := m.store.RunID(); id != "" {
		row[spec.RunIDColumn] = id
	}
	return row
}

func (m *taskManager) inTx(tx *gorm.DB, publisher *heldEvents) (*taskManager, error) {
	txStore, err := newStore(tx, m.store.Spec(), carriers())
	if err != nil {
		return nil, err
	}
	if id := m.store.RunID(); id != "" {
		if txStore, err = txStore.ForRun(id); err != nil {
			return nil, err
		}
	}
	return &taskManager{db: tx, store: txStore, publisher: publisher, spec: m.spec, overdueGrace: m.overdueGrace}, nil
}
