package gormstore

// This file holds the join-table rows for the Work edge labels that are
// genuinely many-to-many and so cannot collapse into a denormalised FK
// column on either endpoint (unlike assigned_to, subtask_of,
// started_task/started_todo, has_todo, and has_deliverable/
// has_acceptance_criteria, which are all one-to-(at most)one or
// one-to-many and live as plain columns on the "many" side's row — see
// task.go, tasktodo.go, deliverable.go, acceptancecriteria.go).
//
// blocks and depends_on are both Task→Task, many-to-many, and distinct
// labels (a blocks edge is a hard gate; a depends_on edge is informational
// only) so they get separate tables rather than sharing one with a label
// column — that would need a composite key including the label anyway.

// TaskBlockRow is the join row for the `blocks` label: FromTaskID must
// reach a terminal status before ToTaskID may transition to in_progress.
type TaskBlockRow struct {
	FromTaskID string `gorm:"primaryKey"`
	ToTaskID   string `gorm:"primaryKey;index"`
	CreatedAt  string
	Reason     string
}

// TaskDependencyRow is the join row for the `depends_on` label: FromTaskID
// depends on ToTaskID (soft dependency, no status gate by itself — see the
// blocked-status handling in task_impl.go/assignment.go).
type TaskDependencyRow struct {
	FromTaskID string `gorm:"primaryKey"`
	ToTaskID   string `gorm:"primaryKey;index"`
	CreatedAt  string
	Reason     string
}

// TaskProjectMembershipRow is the join row for the `member_of` label —
// many-to-many, a Task may belong to multiple Projects.
type TaskProjectMembershipRow struct {
	TaskID    string `gorm:"primaryKey"`
	ProjectID string `gorm:"primaryKey;index"`
	AddedAt   string
}

// TaskTagRow is the join row for the `has_tag` label — many-to-many.
type TaskTagRow struct {
	TaskID   string `gorm:"primaryKey"`
	TagID    string `gorm:"primaryKey;index"`
	TaggedAt string
}
