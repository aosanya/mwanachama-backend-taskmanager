package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

// UpsertAgent creates or merges an Agent row keyed by (agent_id).
//
// On the merge branch, display_name and capability are updated to the
// request values; agent_id is treated as immutable (the natural key cannot
// change).
func (m *taskManager) UpsertAgent(ctx context.Context, agent Agent) (Agent, error) {
	if agent.AgentID == "" {
		return Agent{}, fmt.Errorf("%w: AgentID is required", ErrInvalidTask)
	}
	now := time.Now().UTC().Format(time.RFC3339)

	existing, err := m.GetAgentByAgentID(ctx, agent.AgentID)
	switch {
	case err == nil:
		existing.DisplayName = agent.DisplayName
		existing.Capability = agent.Capability
		existing.RoleName = agent.RoleName
		existing.UpdatedAt = now
		row, err := encode(m.store.Object(roleAgent), existing)
		if err != nil {
			return Agent{}, fmt.Errorf("UpsertAgent: %w", err)
		}
		if err := m.store.Query(ctx, roleAgent).Where("id = ?", existing.ID).Updates(row).Error; err != nil {
			return Agent{}, fmt.Errorf("UpsertAgent: %w", err)
		}
		return existing, nil
	case errors.Is(err, ErrAgentNotFound):
		agent.ID = newSpecID()
		agent.CreatedAt = now
		agent.UpdatedAt = now
		if err := m.store.Insert(ctx, roleAgent, agent); err != nil {
			return Agent{}, fmt.Errorf("UpsertAgent: %w", err)
		}
		return agent, nil
	default:
		return Agent{}, fmt.Errorf("UpsertAgent: %w", err)
	}
}

// GetAgent reads an Agent row by either its entity ID (the storage UUID) or
// its external AgentID slug (e.g. "developer-01"). ID lookup is tried
// first; on NotFound it falls back to a slug match. Returns
// [ErrAgentNotFound] if no match is found.
func (m *taskManager) GetAgent(ctx context.Context, idOrSlug string) (Agent, error) {
	var a Agent
	q := m.store.Query(ctx, roleAgent).Where("id = ?", idOrSlug)
	err := m.store.Take(q, roleAgent, &a, ErrAgentNotFound)
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, ErrAgentNotFound) {
		return Agent{}, fmt.Errorf("GetAgent: %w", err)
	}
	return m.GetAgentByAgentID(ctx, idOrSlug)
}

// GetAgentByAgentID reads an Agent row by its external AgentID slug (e.g.
// "developer-01"). Returns [ErrAgentNotFound] if no Agent has that slug.
func (m *taskManager) GetAgentByAgentID(ctx context.Context, agentIDSlug string) (Agent, error) {
	if agentIDSlug == "" {
		return Agent{}, ErrAgentNotFound
	}
	var a Agent
	q := m.store.Query(ctx, roleAgent).Where("agent_id = ?", agentIDSlug)
	if err := m.store.Take(q, roleAgent, &a, ErrAgentNotFound); err != nil {
		if errors.Is(err, ErrAgentNotFound) {
			return Agent{}, err
		}
		return Agent{}, fmt.Errorf("GetAgentByAgentID: %w", err)
	}
	return a, nil
}

// ListAgents returns all Agents.
func (m *taskManager) ListAgents(ctx context.Context) ([]Agent, error) {
	q := m.store.Query(ctx, roleAgent).Limit(maxListPage)
	out, err := specstore.List[Agent](m.store, q, roleAgent)
	if err != nil {
		return nil, fmt.Errorf("ListAgents: %w", err)
	}
	return out, nil
}
