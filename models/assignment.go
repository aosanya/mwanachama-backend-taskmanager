package models

type AssigneeKind string

const (
	AssigneeAgent AssigneeKind = "agent"
	AssigneeActor AssigneeKind = "actor"
	AssigneeRole  AssigneeKind = "role"
)

type Assignment struct {
	ID           string       `json:"id"`
	TaskID       string       `json:"task_id"`
	AssigneeKind AssigneeKind `json:"assignee_kind"`
	AssigneeID   string       `json:"assignee_id"`
	AssignedAt   string       `json:"assigned_at"`
	UnassignedAt string       `json:"unassigned_at,omitempty"`
}

type HolderTasksQuery struct {
	ActorID     string   `json:"actor_id"`
	RoleKindIDs []string `json:"role_kind_ids"`
}
