package mwanachamataskmanager

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/events"
	"github.com/aosanya/mwanachama-backend-shared/identity"
	"github.com/aosanya/mwanachama-backend-shared/spec"
)

type TaskManager interface {
	identity.Owner

	ForRun(runID string) (TaskManager, error)

	CreateTask(ctx context.Context, task Task) (Task, error)

	GetTask(ctx context.Context, taskID string) (Task, error)

	UpdateTask(ctx context.Context, task Task) (Task, error)

	DeleteTask(ctx context.Context, taskID string) error

	ListTasks(ctx context.Context, filter TaskFilter) ([]Task, error)

	ListOverdueTasks(ctx context.Context, filter OverdueFilter) ([]OverdueTask, error)

	PendingTasks(ctx context.Context, filter PendingFilter) (PendingTasks, error)

	FindTasks(ctx context.Context, filter FindFilter) (FoundTasks, error)

	ListTaskStates(ctx context.Context) (TaskStateList, error)

	MoveTaskToState(ctx context.Context, taskID, state string) (Task, error)

	ListBacklogs(ctx context.Context) (BacklogList, error)
	CreateBacklog(ctx context.Context, name string, steps []string) (BacklogSummary, error)
	CreateBacklogWithRules(ctx context.Context, name string, steps []string, rules *BacklogRules) (BacklogSummary, error)
	SetBacklogRules(ctx context.Context, backlogID string, rules *BacklogRules) (BacklogSummary, error)
	CountBacklogRules(ctx context.Context, requesterID string, rules *BacklogRules) (RulesCount, error)
	RenameBacklog(ctx context.Context, backlogID, name string) (BacklogSummary, error)
	ArchiveBacklog(ctx context.Context, backlogID string) (BacklogSummary, error)
	UnarchiveBacklog(ctx context.Context, backlogID string) (BacklogSummary, error)
	DeleteBacklog(ctx context.Context, backlogID string) error
	AddBacklogStep(ctx context.Context, backlogID, name, after string) (BacklogSummary, error)
	RenameBacklogStep(ctx context.Context, backlogID, stepID, name string) (BacklogSummary, error)
	ReorderBacklogSteps(ctx context.Context, backlogID string, steps []string) (BacklogSummary, error)
	RemoveBacklogStep(ctx context.Context, backlogID, stepID string) (BacklogSummary, error)
	ReplaceBacklogs(ctx context.Context, backlogs []Backlog) (BacklogList, error)
	MoveTaskToStep(ctx context.Context, taskID, backlogID, stepID string) (Task, error)
	MoveTaskToStepAs(ctx context.Context, requesterID, taskID, backlogID, stepID string) (Task, error)
	RenameBacklogAs(ctx context.Context, requesterID, backlogID, name string) (BacklogSummary, error)
	ArchiveBacklogAs(ctx context.Context, requesterID, backlogID string) (BacklogSummary, error)
	UnarchiveBacklogAs(ctx context.Context, requesterID, backlogID string) (BacklogSummary, error)
	DeleteBacklogAs(ctx context.Context, requesterID, backlogID string) error
	AddBacklogStepAs(ctx context.Context, requesterID, backlogID, name, after string) (BacklogSummary, error)
	RenameBacklogStepAs(ctx context.Context, requesterID, backlogID, stepID, name string) (BacklogSummary, error)
	ReorderBacklogStepsAs(ctx context.Context, requesterID, backlogID string, steps []string) (BacklogSummary, error)
	RemoveBacklogStepAs(ctx context.Context, requesterID, backlogID, stepID string) (BacklogSummary, error)
	ReplaceBacklogsAs(ctx context.Context, requesterID string, backlogs []Backlog) (BacklogList, error)
	GiveBacklogAccessAs(ctx context.Context, requesterID, backlogID, groupID, level string) (BacklogAccess, error)
	TakeBacklogAccessAs(ctx context.Context, requesterID, backlogID, groupID string) (BacklogAccess, error)
	ListBacklogsFor(ctx context.Context, requesterID string) (BacklogList, error)
	GetTaskFor(ctx context.Context, requesterID, taskID string) (Task, error)
	GetTaskByNameFor(ctx context.Context, requesterID, projectName, taskName string) (Task, error)
	ListTaskAssignmentsFor(ctx context.Context, requesterID, taskID string) ([]Assignment, error)
	ListTasksInProjectFor(ctx context.Context, requesterID, projectID string) ([]Task, error)
	ListTasksForRunFor(ctx context.Context, requesterID, runID string) ([]Task, error)
	ListCaseTasksFor(ctx context.Context, requesterID, caseID string) ([]Task, error)
	TraverseRelationshipsFor(ctx context.Context, requesterID, vertexID, label string, dir Direction) ([]Relationship, error)
	ListBacklogAccess(ctx context.Context, backlogID string) (BacklogAccess, error)
	GiveBacklogAccess(ctx context.Context, backlogID, groupID, level string) (BacklogAccess, error)
	TakeBacklogAccess(ctx context.Context, backlogID, groupID string) (BacklogAccess, error)
	SetBacklogOwnerGroup(ctx context.Context, groupID string) error
	UseGroups(g Groups)
	UseGroupExists(g GroupExists)
	UseInstanceOwner(o InstanceOwner)
	UseRoleSync(r RoleSync)
	CreateTaskAs(ctx context.Context, requesterID string, task Task) (Task, error)
	UpdateTaskAs(ctx context.Context, requesterID string, task Task) (Task, error)
	PatchTaskAs(ctx context.Context, requesterID string, patch TaskPatch) (Task, error)
	MoveTaskToStateAs(ctx context.Context, requesterID, taskID, state string) (Task, error)
	PlaceTaskAs(ctx context.Context, requesterID, taskID, afterID, beforeID string) (Task, error)
	DeleteTaskAs(ctx context.Context, requesterID, taskID string) error
	AssignTaskAs(ctx context.Context, requesterID, taskID, agentID, workflowRunID string) error
	UnassignTaskAs(ctx context.Context, requesterID, taskID string) error
	AssignTaskToHolderAs(ctx context.Context, requesterID, taskID string, kind AssigneeKind, assigneeID string) error
	UnblockTaskAs(ctx context.Context, requesterID, taskID, note string) (Task, error)
	UnblockDependentsAs(ctx context.Context, requesterID, completedTaskID string) error
	AddTaskToProjectAs(ctx context.Context, requesterID, taskID, projectID string) error
	RemoveTaskFromProjectAs(ctx context.Context, requesterID, taskID, projectID string) error
	CreateTaskInProjectAs(ctx context.Context, requesterID, projectName string, task Task) (Task, error)
	LinkTaskToRunAs(ctx context.Context, requesterID, runID, taskID string) error
	LinkCaseTaskAs(ctx context.Context, requesterID, caseID, taskID string) error
	CancelWorkflowRunAs(ctx context.Context, requesterID, runID, reason, cancelledBy string, quiesceDeadline time.Time) (WorkflowRun, error)
	RollbackWorkflowRunAs(ctx context.Context, requesterID, runID, reason string) (WorkflowRun, error)
	EndCaseAs(ctx context.Context, requesterID, caseID string, status CaseStatus, reply string) (Case, error)
	CreateRelationshipAs(ctx context.Context, requesterID string, rel Relationship) (Relationship, error)
	DeleteRelationshipAs(ctx context.Context, requesterID, fromID, toID, label string) error

	PlaceTask(ctx context.Context, taskID, afterID, beforeID string) (Task, error)

	CreateRelationship(ctx context.Context, rel Relationship) (Relationship, error)

	DeleteRelationship(ctx context.Context, fromID, toID, label string) error

	TraverseRelationships(ctx context.Context, vertexID, label string, dir Direction) ([]Relationship, error)

	UpsertAgent(ctx context.Context, agent Agent) (Agent, error)

	GetAgent(ctx context.Context, idOrSlug string) (Agent, error)

	GetAgentByAgentID(ctx context.Context, agentIDSlug string) (Agent, error)

	ListAgents(ctx context.Context) ([]Agent, error)

	AssignTask(ctx context.Context, taskID, agentID, workflowRunID string) error

	UnassignTask(ctx context.Context, taskID string) error
	AssignTaskToHolder(ctx context.Context, taskID string, kind AssigneeKind, assigneeID string) error
	CreateRepeatingTask(ctx context.Context, r RepeatingTask) (RepeatingTaskRecord, error)
	StartRepeatingTask(ctx context.Context, n NewRepeatingTask) (RepeatingTaskRecord, error)
	GetRepeatingTask(ctx context.Context, id string) (RepeatingTaskRecord, error)
	ListRepeatingTasks(ctx context.Context, filter RepeatingTaskFilter) ([]RepeatingTaskRecord, error)
	ListRunningRepeatingTasks(ctx context.Context) ([]RepeatingTaskRecord, error)
	StopRepeatingTask(ctx context.Context, id string) (RepeatingTaskRecord, error)
	MakeRepeatingTaskDue(ctx context.Context, id string, at time.Time) (RepeatingTaskPeriod, bool, error)
	OpenSourcedTasks(ctx context.Context, req OpenSourcedTasks) (SourcedTasksOpened, error)
	OpenSourcedTasksAs(ctx context.Context, requesterID string, req OpenSourcedTasks) (SourcedTasksOpened, error)
	GenerateTasks(ctx context.Context, req GenerateTasks) (TasksGenerated, error)
	GenerateTasksAs(ctx context.Context, requesterID string, req GenerateTasks) (TasksGenerated, error)
	ListTaskAssignments(ctx context.Context, taskID string) ([]Assignment, error)
	ListHeldTasks(ctx context.Context, query HolderTasksQuery) ([]Task, error)

	UnblockDependents(ctx context.Context, completedTaskID string) error

	CreateProject(ctx context.Context, p Project) (Project, error)

	GetProject(ctx context.Context, projectID string) (Project, error)

	GetProjectByName(ctx context.Context, projectName string) (Project, error)

	UpdateProject(ctx context.Context, p Project) (Project, error)

	DeleteProject(ctx context.Context, projectID string) error

	ListProjects(ctx context.Context) ([]Project, error)

	AddTaskToProject(ctx context.Context, taskID, projectID string) error

	RemoveTaskFromProject(ctx context.Context, taskID, projectID string) error

	ListTasksInProject(ctx context.Context, projectID string) ([]Task, error)

	ListProjectsForTask(ctx context.Context, taskID string) ([]Project, error)

	OpenCase(ctx context.Context, requesterID string, c Case) (Case, error)
	GetCase(ctx context.Context, caseID string) (Case, error)
	ListCases(ctx context.Context, filter CaseFilter) ([]Case, error)
	LinkCaseTask(ctx context.Context, caseID, taskID string) error
	ListCaseTasks(ctx context.Context, caseID string) ([]Task, error)
	EndCase(ctx context.Context, caseID string, status CaseStatus, reply string) (Case, error)
	ListMyCases(ctx context.Context, requesterID string, filter CaseFilter) ([]Case, error)
	VoteCase(ctx context.Context, caseID, memberID, castBy, memberKind string) (RankedCase, error)
	TakeBackVote(ctx context.Context, caseID, memberID string) (RankedCase, error)
	ListCaseVotes(ctx context.Context, caseID string) ([]CaseVote, error)
	AddTaskNote(ctx context.Context, requesterID, taskID, kind, text string, links []string) (TaskNote, error)
	ListTaskNotes(ctx context.Context, requesterID, taskID string) ([]TaskNote, error)
	ListTodosOfIssue(ctx context.Context, requesterID, issueID string) (IssueTodoList, error)
	CountTodosOfIssues(ctx context.Context, filter TodoCountsFilter) (IssueTodoCounts, error)
	EditTodoFor(ctx context.Context, requesterID, todoID string, change TodoChange) (TodoView, error)
	ReorderTodosOfIssue(ctx context.Context, requesterID, issueID string, order TodoOrder) (IssueTodoList, error)
	DeleteTodoFor(ctx context.Context, requesterID, todoID string) error
	SetTodoStateFor(ctx context.Context, requesterID, todoID, state, blockedReason string) (TodoView, error)
	GetWorkerCapsDefault(ctx context.Context) (WorkerCaps, error)
	SetWorkerCapsDefault(ctx context.Context, requesterID string, change TaskCapsChange) (WorkerCaps, error)
	GetTaskCaps(ctx context.Context, requesterID, taskID string) (TaskCaps, error)
	SetTaskCaps(ctx context.Context, requesterID, taskID string, change TaskCapsChange) (TaskCaps, error)
	ClearTaskCaps(ctx context.Context, requesterID, taskID string) (TaskCaps, error)
	WriteTaskUsage(ctx context.Context, requesterID, taskID string, usage TaskUsageWrite) (TaskCaps, error)
	NearestCaps(ctx context.Context, filter CapsNearestFilter) (CapsNearestList, error)
	AddTaskReference(ctx context.Context, requesterID, taskID, kind, repo, target string) (TaskReference, error)
	ProposeTaskReference(ctx context.Context, requesterID, taskID, kind, repo, target string) (TaskReference, error)
	ConfirmTaskReference(ctx context.Context, requesterID, taskID, referenceID string) (TaskReference, error)
	ListTaskReferences(ctx context.Context, requesterID, taskID string) ([]TaskReference, error)
	RemoveTaskReference(ctx context.Context, requesterID, taskID, referenceID string) error
	ListCasesByVotes(ctx context.Context, filter CaseFilter) ([]RankedCase, error)

	GetTaskByName(ctx context.Context, projectName, taskName string) (Task, error)

	CreateTaskInProject(ctx context.Context, projectName string, task Task) (Task, error)

	ImportProject(ctx context.Context, document string) (ImportResult, error)

	StartImportProject(ctx context.Context, document string) (ImportProjectJob, error)

	GetImportProjectStatus(ctx context.Context, jobID string) (ImportProjectJob, error)

	CancelImportProject(ctx context.Context, jobID string) error

	UnblockTask(ctx context.Context, taskID, note string) (Task, error)

	CreateTaskTodo(ctx context.Context, todo TaskTodo) (TaskTodo, error)

	DispatchTaskTodo(ctx context.Context, todoID string) error

	GetTaskTodo(ctx context.Context, todoID string) (TaskTodo, error)

	UpdateTaskTodoStatus(ctx context.Context, todoID string, status TodoStatus) (TaskTodo, error)

	SetTaskTodoStatus(ctx context.Context, todoID string, status TodoStatus, blockedReason string) (TaskTodo, error)

	UpdateTaskTodo(ctx context.Context, todoID string, patch TaskTodoPatch) (TaskTodo, error)

	ReorderTaskTodos(ctx context.Context, issueID string, orderedIDs []string) ([]TaskTodo, error)

	DeleteTaskTodo(ctx context.Context, todoID string) error

	ListIssueTodos(ctx context.Context, issueID string) ([]TaskTodo, error)

	CountIssueTodos(ctx context.Context, issueIDs []string) (map[string]TodoCount, error)

	CheckTodoSameIssue(ctx context.Context, issueID string, dependsOn []string) error

	CheckTodoCycle(ctx context.Context, todoID string, dependsOn []string) error

	CreateWorkflowRun(ctx context.Context, name, triggerEvent, initiator string) (WorkflowRun, error)

	StartWorkflowRun(ctx context.Context, name, workflowCode string, workflowVersion int, triggerEvent, initiator string) (WorkflowRun, error)

	ListWorkflowRunsUnderWay(ctx context.Context, workflowCode string) ([]WorkflowRun, error)

	ListWorkflowRunsClosed(ctx context.Context, workflowCode string) ([]WorkflowRun, error)

	StopWorkflowRun(ctx context.Context, runID, stoppedBy, reason string, restartOnVersion int) (WorkflowRunStop, error)

	BeginWorkflowRunStep(ctx context.Context, runID, stepCode string, limitMinutes int, cancelWorkflowCode string) (WorkflowRun, error)

	AnswerWorkflowRunStep(ctx context.Context, runID string) (WorkflowRun, error)

	CloseWorkflowRunsPastLimit(ctx context.Context, asOf string) ([]WorkflowRun, error)

	CreateRecoveryWorkflowRun(ctx context.Context, name, triggerEvent, initiator, parentRunID, rootRunID string) (WorkflowRun, error)

	SetFailureBudget(ctx context.Context, runID string, budget int) (WorkflowRun, error)

	IncrementFailureBudget(ctx context.Context, rootRunID, childRunID string) (used, budget int, exhausted bool, err error)

	GetWorkflowRun(ctx context.Context, runID string) (WorkflowRun, error)

	GetWorkflowRunByName(ctx context.Context, name string) (WorkflowRun, error)

	ListWorkflowRuns(ctx context.Context, name string) ([]WorkflowRun, error)

	LinkTaskToRun(ctx context.Context, runID, taskID string) error

	LinkTodoToRun(ctx context.Context, runID, todoID string) error

	GetWorkflowRunClosure(ctx context.Context, runID string) (WorkflowRunClosure, error)

	UpdateWorkflowRunStatus(ctx context.Context, runID string, newStatus WorkflowRunStatus, reason string) (WorkflowRun, error)

	RollbackWorkflowRun(ctx context.Context, runID, reason string) (WorkflowRun, error)

	DeleteWorkflowRunArtifacts(ctx context.Context, runID string) error

	CancelWorkflowRun(ctx context.Context, runID, reason, cancelledBy string, quiesceDeadline time.Time) (WorkflowRun, error)

	FinalizeWorkflowRunCancellation(ctx context.Context, runID string) (WorkflowRun, error)

	TouchWorkflowRunLastEventAt(ctx context.Context, runID, ts string) error

	ListWorkflowRunsStaleSince(ctx context.Context, cutoff time.Time) ([]WorkflowRun, error)

	ListWorkflowRunsStepStaleSince(ctx context.Context, cutoff time.Time) ([]WorkflowRun, error)

	MarkTimeoutPublished(ctx context.Context, runID string) error

	HandleRunTimeout(ctx context.Context, runID string) error

	HandleTaskTimeout(ctx context.Context, taskOrTodoID string, runID string) error

	ListTasksForRun(ctx context.Context, runID string) ([]Task, error)

	ListTaskTodos(ctx context.Context, workflowRunID string) ([]TaskTodo, error)

	ListDeliverablesForTask(ctx context.Context, taskID string) ([]Deliverable, error)

	ListAcceptanceCriteriaForTask(ctx context.Context, taskID string) ([]AcceptanceCriteria, error)

	WriteAcceptanceCriteriaResult(ctx context.Context, criteriaID, result, notes string) error
}

type Publisher = events.Publisher

type taskManager struct {
	db            *gorm.DB
	store         *store
	publisher     events.Publisher
	spec          *spec.Spec
	overdueGrace  time.Duration
	groups        Groups
	groupExists   GroupExists
	instanceOwner InstanceOwner
	roleSync      RoleSync
}

func NewTaskManager(db *gorm.DB, s *spec.Spec, pub events.Publisher) (TaskManager, error) {
	if db == nil {
		return nil, fmt.Errorf("NewTaskManager: db must not be nil")
	}
	if s == nil {
		return nil, fmt.Errorf("NewTaskManager: spec must not be nil")
	}
	st, err := newStore(db, s, carriers())
	if err != nil {
		return nil, fmt.Errorf("NewTaskManager: %w", err)
	}
	if _, err := moduleTaskStates(s); err != nil {
		return nil, fmt.Errorf("NewTaskManager: %w", err)
	}
	grace, err := moduleOverdueGrace(s)
	if err != nil {
		return nil, fmt.Errorf("NewTaskManager: %w", err)
	}
	m := &taskManager{db: db, store: st, publisher: pub, spec: s, overdueGrace: grace}
	if err := m.migrateTodoDependencies(context.Background()); err != nil {
		return nil, fmt.Errorf("NewTaskManager: %w", err)
	}
	return m, nil
}

func Unbound() TaskManager { return (*taskManager)(nil) }
