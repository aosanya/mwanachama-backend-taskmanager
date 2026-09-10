package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
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

	var existing gormstore.AgentRow
	err := m.db.WithContext(ctx).Table(m.tables.Agents).Where("agent_id = ?", agent.AgentID).First(&existing).Error
	switch {
	case err == nil:
		existing.DisplayName = agent.DisplayName
		existing.Capability = agent.Capability
		existing.RoleName = agent.RoleName
		existing.UpdatedAt = now
		if err := m.db.WithContext(ctx).Table(m.tables.Agents).Where("id = ?", existing.ID).Save(&existing).Error; err != nil {
			return Agent{}, fmt.Errorf("UpsertAgent: %w", err)
		}
		return gormstore.AgentFromRow(existing), nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		agent.CreatedAt = now
		agent.UpdatedAt = now
		row := gormstore.AgentToRow(agent)
		if err := m.db.WithContext(ctx).Table(m.tables.Agents).Create(&row).Error; err != nil {
			return Agent{}, fmt.Errorf("UpsertAgent: %w", err)
		}
		return gormstore.AgentFromRow(row), nil
	default:
		return Agent{}, fmt.Errorf("UpsertAgent: %w", err)
	}
}

// GetAgent reads an Agent row by either its entity ID (the storage UUID) or
// its external AgentID slug (e.g. "developer-01"). ID lookup is tried
// first; on NotFound it falls back to a slug match. Returns
// [ErrAgentNotFound] if no match is found.
func (m *taskManager) GetAgent(ctx context.Context, idOrSlug string) (Agent, error) {
	var row gormstore.AgentRow
	err := m.db.WithContext(ctx).Table(m.tables.Agents).Where("id = ?", idOrSlug).First(&row).Error
	if err == nil {
		return gormstore.AgentFromRow(row), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
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
	var row gormstore.AgentRow
	err := m.db.WithContext(ctx).Table(m.tables.Agents).Where("agent_id = ?", agentIDSlug).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Agent{}, ErrAgentNotFound
		}
		return Agent{}, fmt.Errorf("GetAgentByAgentID: %w", err)
	}
	return gormstore.AgentFromRow(row), nil
}

// ListAgents returns all Agents.
func (m *taskManager) ListAgents(ctx context.Context) ([]Agent, error) {
	var rows []gormstore.AgentRow
	if err := m.db.WithContext(ctx).Table(m.tables.Agents).Limit(maxListPage).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListAgents: %w", err)
	}
	out := make([]Agent, 0, len(rows))
	for _, r := range rows {
		out = append(out, gormstore.AgentFromRow(r))
	}
	return out, nil
}
