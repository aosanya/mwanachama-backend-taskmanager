package models

// Relationship is the Work-domain projection of an edge between two Work
// vertices (Task / Project / Agent / ...). Backed by either a denormalised
// FK column or a join table, depending on the label — see gormstore's
// relationship strategy table.
type Relationship struct {
	// ID is a storage-assigned or synthesised edge identifier.
	ID string

	// Label is the edge label — one of the RelLabel* constants.
	Label string

	FromID string
	ToID   string

	// Properties are caller-supplied edge metadata.
	Properties map[string]any

	CreatedAt string
}

// Direction selects edge orientation for TaskManager.TraverseRelationships.
type Direction int

const (
	// DirectionInbound returns edges pointing AT the start vertex.
	DirectionInbound Direction = iota

	// DirectionOutbound returns edges pointing AWAY from the start vertex.
	DirectionOutbound
)

// Edge-label constants — the closed set of allowed Work relationship labels.
const (
	// RelLabelAssignedTo connects a Task to the Agent currently responsible
	// for it (functional — at most one per Task).
	RelLabelAssignedTo = "assigned_to"

	// RelLabelBlocks indicates the source Task must reach a terminal status
	// before the target Task may transition to in_progress.
	RelLabelBlocks = "blocks"

	// RelLabelSubtaskOf marks the source Task as a child of the target Task
	// (functional — a subtask has at most one parent).
	RelLabelSubtaskOf = "subtask_of"

	// RelLabelDependsOn is a soft dependency — informational only, no status gate.
	RelLabelDependsOn = "depends_on"

	// RelLabelMemberOf links a Task to a Project. Project membership is many-to-many.
	RelLabelMemberOf = "member_of"

	// RelLabelHasTag links a Task to a Tag. Tagging is many-to-many.
	RelLabelHasTag = "has_tag"

	// RelLabelHasTodo links a Task to a TaskTodo produced by a decomposition
	// run. One-to-many.
	RelLabelHasTodo = "has_todo"

	// RelLabelTodoAssignedTo links a TaskTodo to the Agent responsible for
	// executing it.
	RelLabelTodoAssignedTo = "todo_assigned_to"

	// RelLabelStartedTask links a WorkflowRun to a Task it produced. One-to-many.
	RelLabelStartedTask = "started_task"

	// RelLabelStartedTodo links a WorkflowRun to a TaskTodo it produced.
	RelLabelStartedTodo = "started_todo"

	// RelLabelPartOfRun is the inverse of started_task / started_todo.
	RelLabelPartOfRun = "part_of_run"

	// RelLabelHasDeliverable links a Task or TaskTodo to a Deliverable it must produce.
	RelLabelHasDeliverable = "has_deliverable"

	// RelLabelHasAcceptanceCriteria links a Task or TaskTodo to a verifiable AcceptanceCriteria.
	RelLabelHasAcceptanceCriteria = "has_acceptance_criteria"
)
