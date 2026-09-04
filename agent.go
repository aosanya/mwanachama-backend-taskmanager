package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"

	"github.com/aosanya/mwanachama-backend-shared/entitygraph"
)

// UpsertAgent creates or merges an Agent vertex keyed by (agent_id).
//
// On the merge branch, display_name and capability are updated to the request
// values; agent_id is treated as immutable (the natural key cannot change).
func (m *taskManager) UpsertAgent(ctx context.Context, agent Agent) (Agent, error) {
	if agent.AgentID == "" {
		return Agent{}, fmt.Errorf("%w: AgentID is required", ErrInvalidTask)
	}
	upserted, err := m.dm.UpsertEntity(ctx, entitygraph.CreateEntityRequest{
		TypeID:     agentTypeID,
		Properties: agentToProperties(agent),
	})
	if err != nil {
		return Agent{}, fmt.Errorf("UpsertAgent: %w", err)
	}
	return agentFromEntity(upserted), nil
}

// GetAgent reads an Agent vertex by either its entity ID (the storage UUID) or
// its external AgentID slug (e.g. "developer-01"). UUID lookup is tried first;
// on NotFound it falls back to a slug match against all Agents. This mirrors
// UpsertAgent's slug-first semantics so HTTP routes that bind {agentId} can
// be passed either form without the caller knowing which.
// Returns [ErrAgentNotFound] if no match is found.
func (m *taskManager) GetAgent(ctx context.Context, idOrSlug string) (Agent, error) {
	e, err := m.dm.GetEntity(ctx, idOrSlug)
	if err == nil {
		if e.TypeID != agentTypeID {
			return Agent{}, ErrAgentNotFound
		}
		return agentFromEntity(e), nil
	}
	if !errors.Is(err, entitygraph.ErrEntityNotFound) {
		return Agent{}, fmt.Errorf("GetAgent: %w", err)
	}
	// Fallback: treat the argument as the AgentID slug.
	return m.GetAgentByAgentID(ctx, idOrSlug)
}

// GetAgentByAgentID reads an Agent vertex by its external AgentID slug
// (e.g. "developer-01") — the same field UpsertAgent uses as the natural key.
// Returns [ErrAgentNotFound] if no Agent has that slug.
func (m *taskManager) GetAgentByAgentID(ctx context.Context, agentIDSlug string) (Agent, error) {
	if agentIDSlug == "" {
		return Agent{}, ErrAgentNotFound
	}
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		TypeID:     agentTypeID,
		Properties: map[string]any{"agent_id": agentIDSlug},
	})
	if err != nil {
		return Agent{}, fmt.Errorf("GetAgentByAgentID: %w", err)
	}
	if len(entities) == 0 {
		return Agent{}, ErrAgentNotFound
	}
	return agentFromEntity(entities[0]), nil
}

// ListAgents returns all non-deleted Agents.
func (m *taskManager) ListAgents(ctx context.Context) ([]Agent, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		TypeID: agentTypeID,
	})
	if err != nil {
		return nil, fmt.Errorf("ListAgents: %w", err)
	}
	out := make([]Agent, 0, len(entities))
	for _, e := range entities {
		out = append(out, agentFromEntity(e))
	}
	return out, nil
}
