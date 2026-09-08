package models

// Agent is the Work-domain projection of an AI agent. Uniqueness — at most
// one Agent per AgentID — is enforced at the storage level; UpsertAgent
// relies on this for find-or-create semantics.
type Agent struct {
	ID string `json:"id"`

	// AgentID is the external agent identifier. Required and unique.
	AgentID string `json:"agent_id"`

	// DisplayName is a human-readable label for the agent. Optional.
	DisplayName string `json:"display_name,omitempty"`

	// Capability is the agent's primary capability (e.g. "code", "research").
	Capability string `json:"capability,omitempty"`

	// RoleName is the role this agent fulfils, used as a stable filter key
	// in event payload_condition rules. Optional.
	RoleName string `json:"role_name,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
