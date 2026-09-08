// relationship_impl.go — the generic CreateRelationship/DeleteRelationship/
// TraverseRelationships dispatcher.
//
// Every RelLabel* is either:
//   - a denormalised FK column on the FROM row (assigned_to, subtask_of,
//     todo_assigned_to) — CreateRelationship writes ToID into that column;
//   - a denormalised FK column on the TO row (has_todo, started_task,
//     started_todo, has_deliverable, has_acceptance_criteria) —
//     CreateRelationship writes FromID into that column;
//   - a join-table row (blocks, depends_on, member_of, has_tag) — the only
//     labels that are genuinely many-to-many on both ends.
//
// This mirrors relationshipEndpointTypes' whitelist from the pre-GORM
// implementation one-for-one; see models/relationship.go for the constants.
package mwanachamataskmanager

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// relFieldKind selects which side of the edge holds the FK column.
type relFieldKind int

const (
	relFieldOnFrom relFieldKind = iota
	relFieldOnTo
	relFieldJoinTable
)

// relSpec describes how one RelLabel* is stored.
type relSpec struct {
	kind relFieldKind

	// fromTable/toTable name the tables id-lookups for FromID/ToID go
	// against, for existence checks and not-found mapping.
	fromNotFound error
	toNotFound   error

	// For relFieldOnFrom/relFieldOnTo: the table + column holding the FK,
	// and the table's primary-key lookup used to validate the OTHER
	// endpoint exists.
	fkTable  string
	fkColumn string

	// For relFieldJoinTable: the join table name and its two columns.
	joinTable    string
	joinFromCol  string
	joinToCol    string
	joinExtraCol string // optional third column stamped with the current timestamp (created_at/added_at/tagged_at)
}

func (m *taskManager) relSpecs() map[string]relSpec {
	return map[string]relSpec{
		RelLabelAssignedTo: {
			kind: relFieldOnFrom, fromNotFound: ErrTaskNotFound, toNotFound: ErrAgentNotFound,
			fkTable: m.tables.Tasks, fkColumn: "assigned_agent_id",
		},
		RelLabelSubtaskOf: {
			kind: relFieldOnFrom, fromNotFound: ErrTaskNotFound, toNotFound: ErrTaskNotFound,
			fkTable: m.tables.Tasks, fkColumn: "parent_task_id",
		},
		RelLabelTodoAssignedTo: {
			kind: relFieldOnFrom, fromNotFound: ErrTaskTodoNotFound, toNotFound: ErrAgentNotFound,
			fkTable: m.tables.TaskTodos, fkColumn: "agent_id",
		},
		RelLabelHasTodo: {
			kind: relFieldOnTo, fromNotFound: ErrTaskNotFound, toNotFound: ErrTaskTodoNotFound,
			fkTable: m.tables.TaskTodos, fkColumn: "parent_task_id",
		},
		RelLabelStartedTask: {
			kind: relFieldOnTo, fromNotFound: ErrWorkflowRunNotFound, toNotFound: ErrTaskNotFound,
			fkTable: m.tables.Tasks, fkColumn: "workflow_run_id",
		},
		RelLabelStartedTodo: {
			kind: relFieldOnTo, fromNotFound: ErrWorkflowRunNotFound, toNotFound: ErrTaskTodoNotFound,
			fkTable: m.tables.TaskTodos, fkColumn: "workflow_run_id",
		},
		RelLabelHasDeliverable: {
			kind: relFieldOnTo, fromNotFound: ErrTaskNotFound, toNotFound: ErrDeliverableNotFound,
			fkTable: m.tables.Deliverables, fkColumn: "parent_id",
		},
		RelLabelHasAcceptanceCriteria: {
			kind: relFieldOnTo, fromNotFound: ErrTaskNotFound, toNotFound: ErrAcceptanceCriteriaNotFound,
			fkTable: m.tables.AcceptanceCriteria, fkColumn: "parent_id",
		},
		RelLabelBlocks: {
			kind: relFieldJoinTable, fromNotFound: ErrTaskNotFound, toNotFound: ErrTaskNotFound,
			joinTable: m.tables.TaskBlocks, joinFromCol: "from_task_id", joinToCol: "to_task_id", joinExtraCol: "created_at",
		},
		RelLabelDependsOn: {
			kind: relFieldJoinTable, fromNotFound: ErrTaskNotFound, toNotFound: ErrTaskNotFound,
			joinTable: m.tables.TaskDependencies, joinFromCol: "from_task_id", joinToCol: "to_task_id", joinExtraCol: "created_at",
		},
		RelLabelMemberOf: {
			kind: relFieldJoinTable, fromNotFound: ErrTaskNotFound, toNotFound: ErrProjectNotFound,
			joinTable: m.tables.TaskProjectMemberships, joinFromCol: "task_id", joinToCol: "project_id", joinExtraCol: "added_at",
		},
		RelLabelHasTag: {
			kind: relFieldJoinTable, fromNotFound: ErrTaskNotFound, toNotFound: ErrTagNotFound,
			joinTable: m.tables.TaskTags, joinFromCol: "task_id", joinToCol: "tag_id", joinExtraCol: "tagged_at",
		},
	}
}

// rowExists reports whether a row with the given id exists in table.
func (m *taskManager) rowExists(ctx context.Context, table, id string) (bool, error) {
	var count int64
	if err := m.db.WithContext(ctx).Table(table).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// vertexTables lists every table that holds a distinct relationship
// endpoint "type". Unlike the pre-GORM entitygraph implementation (one
// shared `entities` table keyed by id, so any id's TypeID was a single
// lookup away), each type now lives in its own physical table — so
// distinguishing "id does not exist at all" from "id exists, but as the
// wrong type" (see [taskManager.checkEndpoint]) means checking each table
// in turn.
func (m *taskManager) vertexTables() []string {
	return []string{
		m.tables.Tasks, m.tables.Agents, m.tables.Projects, m.tables.Tags,
		m.tables.TaskTodos, m.tables.WorkflowRuns, m.tables.Deliverables, m.tables.AcceptanceCriteria,
	}
}

// checkEndpoint verifies id exists in wantTable. Returns notFoundErr if id
// exists in no vertex table at all; returns ErrInvalidRelationship if id
// exists but in a different table than wantTable (the entitygraph-era
// "endpoint type does not match label" case).
func (m *taskManager) checkEndpoint(ctx context.Context, id, wantTable string, notFoundErr error) error {
	if ok, err := m.rowExists(ctx, wantTable, id); err != nil {
		return err
	} else if ok {
		return nil
	}
	for _, table := range m.vertexTables() {
		if table == wantTable {
			continue
		}
		if ok, err := m.rowExists(ctx, table, id); err != nil {
			return err
		} else if ok {
			return fmt.Errorf("%w: vertex %q is not the expected type for this label", ErrInvalidRelationship, id)
		}
	}
	return notFoundErr
}

// CreateRelationship validates the (label, FromID, ToID) triple and writes
// the edge. Re-creating an existing edge is idempotent.
func (m *taskManager) CreateRelationship(ctx context.Context, rel Relationship) (Relationship, error) {
	spec, ok := m.relSpecs()[rel.Label]
	if !ok {
		return Relationship{}, fmt.Errorf("%w: unknown label %q", ErrInvalidRelationship, rel.Label)
	}
	if rel.FromID == "" || rel.ToID == "" {
		return Relationship{}, fmt.Errorf("%w: FromID and ToID are required", ErrInvalidRelationship)
	}

	switch spec.kind {
	case relFieldOnFrom, relFieldOnTo:
		otherTable, ownerTable, ownerID, otherID, ownerNotFound, otherNotFound := m.fkEndpoints(spec, rel)
		if err := m.checkEndpoint(ctx, ownerID, ownerTable, ownerNotFound); err != nil {
			return Relationship{}, err
		}
		if err := m.checkEndpoint(ctx, otherID, otherTable, otherNotFound); err != nil {
			return Relationship{}, err
		}
		var current string
		if err := m.db.WithContext(ctx).Table(ownerTable).Where("id = ?", ownerID).
			Select(spec.fkColumn).Row().Scan(&current); err != nil {
			return Relationship{}, fmt.Errorf("CreateRelationship: %w", err)
		}
		wantValue := otherID
		if current != wantValue {
			now := time.Now().UTC().Format(time.RFC3339)
			if err := m.db.WithContext(ctx).Table(ownerTable).Where("id = ?", ownerID).
				UpdateColumn(spec.fkColumn, wantValue).Error; err != nil {
				return Relationship{}, fmt.Errorf("CreateRelationship: %w", err)
			}
			rel.CreatedAt = now
			rel.ID = syntheticEdgeID(rel.FromID, rel.Label, rel.ToID)
			m.publish(ctx, TopicRelationshipCreated, RelationshipCreatedPayload{FromID: rel.FromID, ToID: rel.ToID, Label: rel.Label})
			return rel, nil
		}
		rel.ID = syntheticEdgeID(rel.FromID, rel.Label, rel.ToID)
		return rel, nil

	case relFieldJoinTable:
		if err := m.checkEndpoint(ctx, rel.FromID, m.joinFromEndpointTable(rel.Label), spec.fromNotFound); err != nil {
			return Relationship{}, err
		}
		if err := m.checkEndpoint(ctx, rel.ToID, m.joinToEndpointTable(rel.Label), spec.toNotFound); err != nil {
			return Relationship{}, err
		}
		var count int64
		if err := m.db.WithContext(ctx).Table(spec.joinTable).
			Where(spec.joinFromCol+" = ? AND "+spec.joinToCol+" = ?", rel.FromID, rel.ToID).
			Count(&count).Error; err != nil {
			return Relationship{}, fmt.Errorf("CreateRelationship: %w", err)
		}
		if count == 0 {
			now := time.Now().UTC().Format(time.RFC3339)
			row := map[string]any{spec.joinFromCol: rel.FromID, spec.joinToCol: rel.ToID}
			if spec.joinExtraCol != "" {
				row[spec.joinExtraCol] = now
			}
			if err := m.db.WithContext(ctx).Table(spec.joinTable).Create(row).Error; err != nil {
				return Relationship{}, fmt.Errorf("CreateRelationship: %w", err)
			}
			rel.CreatedAt = now
			rel.ID = syntheticEdgeID(rel.FromID, rel.Label, rel.ToID)
			m.publish(ctx, TopicRelationshipCreated, RelationshipCreatedPayload{FromID: rel.FromID, ToID: rel.ToID, Label: rel.Label})
			return rel, nil
		}
		rel.ID = syntheticEdgeID(rel.FromID, rel.Label, rel.ToID)
		return rel, nil
	}
	return Relationship{}, fmt.Errorf("%w: unhandled label %q", ErrInvalidRelationship, rel.Label)
}

// DeleteRelationship removes the single edge identified by (fromID, toID, label).
func (m *taskManager) DeleteRelationship(ctx context.Context, fromID, toID, label string) error {
	spec, ok := m.relSpecs()[label]
	if !ok {
		return ErrRelationshipNotFound
	}
	switch spec.kind {
	case relFieldOnFrom, relFieldOnTo:
		ownerTable, ownerID, otherID := m.fkOwnerFor(spec, fromID, toID)
		var current string
		if err := m.db.WithContext(ctx).Table(ownerTable).Where("id = ?", ownerID).
			Select(spec.fkColumn).Row().Scan(&current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRelationshipNotFound
			}
			return fmt.Errorf("DeleteRelationship: %w", err)
		}
		if current != otherID {
			return ErrRelationshipNotFound
		}
		return m.db.WithContext(ctx).Table(ownerTable).Where("id = ?", ownerID).
			UpdateColumn(spec.fkColumn, "").Error

	case relFieldJoinTable:
		res := m.db.WithContext(ctx).Table(spec.joinTable).
			Where(spec.joinFromCol+" = ? AND "+spec.joinToCol+" = ?", fromID, toID).
			Delete(nil)
		if res.Error != nil {
			return fmt.Errorf("DeleteRelationship: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrRelationshipNotFound
		}
		return nil
	}
	return ErrRelationshipNotFound
}

// TraverseRelationships returns the single-hop edges incident on vertexID
// matching label and direction.
func (m *taskManager) TraverseRelationships(ctx context.Context, vertexID, label string, dir Direction) ([]Relationship, error) {
	spec, ok := m.relSpecs()[label]
	if !ok {
		return nil, nil
	}
	switch spec.kind {
	case relFieldOnFrom:
		return m.traverseFK(ctx, spec, vertexID, dir, label, true)
	case relFieldOnTo:
		return m.traverseFK(ctx, spec, vertexID, dir, label, false)
	case relFieldJoinTable:
		return m.traverseJoin(ctx, spec, vertexID, dir, label)
	}
	return nil, nil
}

// traverseFK handles both relFieldOnFrom (fkOnFrom=true) and relFieldOnTo
// (fkOnFrom=false) labels.
func (m *taskManager) traverseFK(ctx context.Context, spec relSpec, vertexID string, dir Direction, label string, fkOnFrom bool) ([]Relationship, error) {
	// outboundIsOwner reports whether traversing outbound from vertexID
	// means vertexID is the row that OWNS the fk column (true for
	// fkOnFrom labels) or the row the fk column POINTS AT (fkOnTo labels).
	if (dir == DirectionOutbound) == fkOnFrom {
		// vertexID is the owning row; read its fk column.
		ownerTable := spec.fkTable
		var value string
		err := m.db.WithContext(ctx).Table(ownerTable).Where("id = ?", vertexID).
			Select(spec.fkColumn).Row().Scan(&value)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil
			}
			return nil, fmt.Errorf("TraverseRelationships: %w", err)
		}
		if value == "" {
			return nil, nil
		}
		from, to := vertexID, value
		if !fkOnFrom {
			from, to = value, vertexID
		}
		return []Relationship{{ID: syntheticEdgeID(from, label, to), Label: label, FromID: from, ToID: to}}, nil
	}
	// vertexID is the value every matching row's fk column points at.
	var ids []string
	if err := m.db.WithContext(ctx).Table(spec.fkTable).Where(spec.fkColumn+" = ?", vertexID).
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("TraverseRelationships: %w", err)
	}
	out := make([]Relationship, 0, len(ids))
	for _, id := range ids {
		// vertexID is the value fkColumn points at, so it's the same side as
		// the FIRST branch's `value` above — mirror that branch's swap
		// exactly (swap when fkOnFrom, not when !fkOnFrom): the row owning
		// the fk column (spec.fkTable, i.e. this `id`) is the FROM side for
		// relFieldOnFrom labels and the TO side for relFieldOnTo labels.
		from, to := vertexID, id
		if fkOnFrom {
			from, to = id, vertexID
		}
		out = append(out, Relationship{ID: syntheticEdgeID(from, label, to), Label: label, FromID: from, ToID: to})
	}
	return out, nil
}

func (m *taskManager) traverseJoin(ctx context.Context, spec relSpec, vertexID string, dir Direction, label string) ([]Relationship, error) {
	matchCol, otherCol := spec.joinFromCol, spec.joinToCol
	if dir == DirectionInbound {
		matchCol, otherCol = spec.joinToCol, spec.joinFromCol
	}
	var others []string
	if err := m.db.WithContext(ctx).Table(spec.joinTable).Where(matchCol+" = ?", vertexID).
		Pluck(otherCol, &others).Error; err != nil {
		return nil, fmt.Errorf("TraverseRelationships: %w", err)
	}
	out := make([]Relationship, 0, len(others))
	for _, other := range others {
		from, to := vertexID, other
		if dir == DirectionInbound {
			from, to = other, vertexID
		}
		out = append(out, Relationship{ID: syntheticEdgeID(from, label, to), Label: label, FromID: from, ToID: to})
	}
	return out, nil
}

// fkEndpoints resolves, for a relFieldOnFrom/relFieldOnTo CreateRelationship
// call, which table/id owns the fk column and which table/id is just being
// existence-checked.
func (m *taskManager) fkEndpoints(spec relSpec, rel Relationship) (otherTable, ownerTable, ownerID, otherID string, ownerNotFound, otherNotFound error) {
	if spec.kind == relFieldOnFrom {
		return m.joinToEndpointTable(rel.Label), spec.fkTable, rel.FromID, rel.ToID, spec.fromNotFound, spec.toNotFound
	}
	return m.joinFromEndpointTable(rel.Label), spec.fkTable, rel.ToID, rel.FromID, spec.toNotFound, spec.fromNotFound
}

func (m *taskManager) fkOwnerFor(spec relSpec, fromID, toID string) (ownerTable, ownerID, otherID string) {
	if spec.kind == relFieldOnFrom {
		return spec.fkTable, fromID, toID
	}
	return spec.fkTable, toID, fromID
}

// joinFromEndpointTable/joinToEndpointTable name the table the FromID/ToID
// of a given label must exist in, for existence-check purposes. Every label
// here has exactly one meaning per side (unlike has_deliverable/
// has_acceptance_criteria's dual fromTypes in the entitygraph-era whitelist,
// which allowed Task OR TaskTodo — this package only ever creates those
// edges from a Task in practice, so the narrower check is intentional; see
// relationship_test.go for coverage).
func (m *taskManager) joinFromEndpointTable(label string) string {
	switch label {
	case RelLabelAssignedTo, RelLabelBlocks, RelLabelSubtaskOf, RelLabelDependsOn,
		RelLabelMemberOf, RelLabelHasTag, RelLabelHasTodo, RelLabelHasDeliverable, RelLabelHasAcceptanceCriteria:
		return m.tables.Tasks
	case RelLabelTodoAssignedTo:
		return m.tables.TaskTodos
	case RelLabelStartedTask, RelLabelStartedTodo:
		return m.tables.WorkflowRuns
	}
	return m.tables.Tasks
}

func (m *taskManager) joinToEndpointTable(label string) string {
	switch label {
	case RelLabelAssignedTo, RelLabelTodoAssignedTo:
		return m.tables.Agents
	case RelLabelBlocks, RelLabelSubtaskOf, RelLabelDependsOn:
		return m.tables.Tasks
	case RelLabelMemberOf:
		return m.tables.Projects
	case RelLabelHasTag:
		return m.tables.Tags
	case RelLabelHasTodo, RelLabelStartedTodo:
		return m.tables.TaskTodos
	case RelLabelStartedTask:
		return m.tables.Tasks
	case RelLabelHasDeliverable:
		return m.tables.Deliverables
	case RelLabelHasAcceptanceCriteria:
		return m.tables.AcceptanceCriteria
	}
	return m.tables.Tasks
}

// syntheticEdgeID builds a stable, opaque edge identifier — this package
// never stored a real edge-id table even before the GORM migration
// (Relationship.ID was already documented as opaque to callers).
func syntheticEdgeID(fromID, label, toID string) string {
	return fromID + "→" + label + "→" + toID
}
