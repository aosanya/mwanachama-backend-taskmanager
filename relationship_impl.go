package mwanachamataskmanager

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type relFieldKind int

const (
	relFieldOnFrom relFieldKind = iota
	relFieldOnTo
	relFieldJoinTable
)

type relSpec struct {
	kind relFieldKind

	fromNotFound error
	toNotFound   error

	fkTable  string
	fkColumn string

	joinTable    string
	joinFromCol  string
	joinToCol    string
	joinExtraCol string

	joinKindCol   string
	joinKindValue string
}

func (m *taskManager) relSpecs() map[string]relSpec {
	return map[string]relSpec{
		RelLabelAssignedTo: {
			kind: relFieldOnFrom, fromNotFound: ErrTaskNotFound, toNotFound: ErrAgentNotFound,
			fkTable: m.store.Table(roleTask), fkColumn: "assigned_to",
		},
		RelLabelSubtaskOf: {
			kind: relFieldOnFrom, fromNotFound: ErrTaskNotFound, toNotFound: ErrTaskNotFound,
			fkTable: m.store.Table(roleTask), fkColumn: "parent_task_id",
		},
		RelLabelTodoAssignedTo: {
			kind: relFieldOnFrom, fromNotFound: ErrTaskTodoNotFound, toNotFound: ErrAgentNotFound,
			fkTable: m.store.Table(roleTaskTodo), fkColumn: "agent_id",
		},
		RelLabelHasTodo: {
			kind: relFieldOnTo, fromNotFound: ErrTaskNotFound, toNotFound: ErrTaskTodoNotFound,
			fkTable: m.store.Table(roleTaskTodo), fkColumn: "parent_task_id",
		},
		RelLabelStartedTask: {
			kind: relFieldOnTo, fromNotFound: ErrWorkflowRunNotFound, toNotFound: ErrTaskNotFound,
			fkTable: m.store.Table(roleTask), fkColumn: "workflow_run_id",
		},
		RelLabelStartedTodo: {
			kind: relFieldOnTo, fromNotFound: ErrWorkflowRunNotFound, toNotFound: ErrTaskTodoNotFound,
			fkTable: m.store.Table(roleTaskTodo), fkColumn: "workflow_run_id",
		},
		RelLabelHasDeliverable: {
			kind: relFieldOnTo, fromNotFound: ErrTaskNotFound, toNotFound: ErrDeliverableNotFound,
			fkTable: m.store.Table(roleDeliverable), fkColumn: "parent_id",
		},
		RelLabelHasAcceptanceCriteria: {
			kind: relFieldOnTo, fromNotFound: ErrTaskNotFound, toNotFound: ErrAcceptanceCriteriaNotFound,
			fkTable: m.store.Table(roleAcceptanceCriteria), fkColumn: "parent_id",
		},
		RelLabelBlocks: {
			kind: relFieldJoinTable, fromNotFound: ErrTaskNotFound, toNotFound: ErrTaskNotFound,
			joinTable: m.store.Table(roleBlocker), joinFromCol: "from_task_id", joinToCol: "to_task_id", joinExtraCol: "created_at",
		},
		RelLabelDependsOn: {
			kind: relFieldJoinTable, fromNotFound: ErrTaskNotFound, toNotFound: ErrTaskNotFound,
			joinTable: m.store.Table(roleDependency), joinFromCol: "from_task_id", joinToCol: "to_task_id", joinExtraCol: "created_at",
		},
		RelLabelMemberOf: {
			kind: relFieldJoinTable, fromNotFound: ErrTaskNotFound, toNotFound: ErrProjectNotFound,
			joinTable: m.store.Table(roleMembership), joinFromCol: "task_id", joinToCol: "project_id", joinExtraCol: "added_at",
		},
		RelLabelAppliesTo: m.ruleLinkSpec(RelLabelAppliesTo),
		RelLabelDecidedIn: m.ruleLinkSpec(RelLabelDecidedIn),
		RelLabelBrokenBy:  m.ruleLinkSpec(RelLabelBrokenBy),
		RelLabelHasTag: {
			kind: relFieldJoinTable, fromNotFound: ErrTaskNotFound, toNotFound: ErrTagNotFound,
			joinTable: m.store.Table(roleTagging), joinFromCol: "task_id", joinToCol: "tag_id", joinExtraCol: "tagged_at",
		},
	}
}

func (m *taskManager) ruleLinkSpec(label string) relSpec {
	return relSpec{
		kind: relFieldJoinTable, fromNotFound: ErrRuleNotFound, toNotFound: ErrTaskNotFound,
		joinTable: m.store.Table(roleRuleLink), joinFromCol: "rule_id", joinToCol: "task_id", joinExtraCol: "linked_at",
		joinKindCol: "relation", joinKindValue: label,
	}
}

func (m *taskManager) joinWhere(db *gorm.DB, spec relSpec) *gorm.DB {
	if spec.joinKindCol == "" {
		return db
	}
	return db.Where(spec.joinKindCol+" = ?", spec.joinKindValue)
}

func (m *taskManager) rowExists(ctx context.Context, table, id string) (bool, error) {
	var count int64
	if err := m.onTable(m.db.WithContext(ctx), table).Where("id = ?", id).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (m *taskManager) vertexTables() []string {
	return []string{
		m.store.Table(roleTask), m.store.Table(roleAgent), m.store.Table(roleProject), m.store.Table(roleTag),
		m.store.Table(roleTaskTodo), m.store.Table(roleWorkflowRun), m.store.Table(roleDeliverable), m.store.Table(roleAcceptanceCriteria),
		m.store.Table(roleRule),
	}
}

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
		if err := m.onTable(m.db.WithContext(ctx), ownerTable).Where("id = ?", ownerID).
			Select(spec.fkColumn).Row().Scan(&current); err != nil {
			return Relationship{}, fmt.Errorf("CreateRelationship: %w", err)
		}
		wantValue := otherID
		if current != wantValue {
			now := time.Now().UTC().Format(time.RFC3339)
			if err := m.onTable(m.db.WithContext(ctx), ownerTable).Where("id = ?", ownerID).
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
		if err := m.joinWhere(m.onTable(m.db.WithContext(ctx), spec.joinTable), spec).
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
			if spec.joinKindCol != "" {
				row[spec.joinKindCol] = spec.joinKindValue
			}
			if err := m.db.WithContext(ctx).Table(spec.joinTable).Create(m.stamped(row)).Error; err != nil {
				return Relationship{}, fmt.Errorf("CreateRelationship: %w", err)
			}
			rel.CreatedAt = now
			rel.ID = syntheticEdgeID(rel.FromID, rel.Label, rel.ToID)
			m.publish(ctx, TopicRelationshipCreated, RelationshipCreatedPayload{FromID: rel.FromID, ToID: rel.ToID, Label: rel.Label})
			if rel.Label == RelLabelDependsOn {
				m.markWaitingIfOpen(ctx, rel.FromID)
			}
			return rel, nil
		}
		rel.ID = syntheticEdgeID(rel.FromID, rel.Label, rel.ToID)
		return rel, nil
	}
	return Relationship{}, fmt.Errorf("%w: unhandled label %q", ErrInvalidRelationship, rel.Label)
}

func (m *taskManager) DeleteRelationship(ctx context.Context, fromID, toID, label string) error {
	spec, ok := m.relSpecs()[label]
	if !ok {
		return ErrRelationshipNotFound
	}
	switch spec.kind {
	case relFieldOnFrom, relFieldOnTo:
		ownerTable, ownerID, otherID := m.fkOwnerFor(spec, fromID, toID)
		var current string
		if err := m.onTable(m.db.WithContext(ctx), ownerTable).Where("id = ?", ownerID).
			Select(spec.fkColumn).Row().Scan(&current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrRelationshipNotFound
			}
			return fmt.Errorf("DeleteRelationship: %w", err)
		}
		if current != otherID {
			return ErrRelationshipNotFound
		}
		return m.onTable(m.db.WithContext(ctx), ownerTable).Where("id = ?", ownerID).
			UpdateColumn(spec.fkColumn, "").Error

	case relFieldJoinTable:
		res := m.joinWhere(m.onTable(m.db.WithContext(ctx), spec.joinTable), spec).
			Where(spec.joinFromCol+" = ? AND "+spec.joinToCol+" = ?", fromID, toID).
			Delete(nil)
		if res.Error != nil {
			return fmt.Errorf("DeleteRelationship: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrRelationshipNotFound
		}
		if label == RelLabelDependsOn {
			m.dropWaitingIfClear(ctx, fromID, "The last open dependency edge was removed: Waiting dropped and Ready added.")
		}
		return nil
	}
	return ErrRelationshipNotFound
}

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

func (m *taskManager) traverseFK(ctx context.Context, spec relSpec, vertexID string, dir Direction, label string, fkOnFrom bool) ([]Relationship, error) {

	if (dir == DirectionOutbound) == fkOnFrom {

		ownerTable := spec.fkTable
		var value string
		err := m.onTable(m.db.WithContext(ctx), ownerTable).Where("id = ?", vertexID).
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

	var ids []string
	if err := m.onTable(m.db.WithContext(ctx), spec.fkTable).Where(spec.fkColumn+" = ?", vertexID).
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("TraverseRelationships: %w", err)
	}
	out := make([]Relationship, 0, len(ids))
	for _, id := range ids {

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
	if err := m.joinWhere(m.onTable(m.db.WithContext(ctx), spec.joinTable), spec).Where(matchCol+" = ?", vertexID).
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

func (m *taskManager) joinFromEndpointTable(label string) string {
	switch label {
	case RelLabelAppliesTo, RelLabelDecidedIn, RelLabelBrokenBy:
		return m.store.Table(roleRule)
	case RelLabelAssignedTo, RelLabelBlocks, RelLabelSubtaskOf, RelLabelDependsOn,
		RelLabelMemberOf, RelLabelHasTag, RelLabelHasTodo, RelLabelHasDeliverable, RelLabelHasAcceptanceCriteria:
		return m.store.Table(roleTask)
	case RelLabelTodoAssignedTo:
		return m.store.Table(roleTaskTodo)
	case RelLabelStartedTask, RelLabelStartedTodo:
		return m.store.Table(roleWorkflowRun)
	}
	return m.store.Table(roleTask)
}

func (m *taskManager) joinToEndpointTable(label string) string {
	switch label {
	case RelLabelAssignedTo, RelLabelTodoAssignedTo:
		return m.store.Table(roleAgent)
	case RelLabelBlocks, RelLabelSubtaskOf, RelLabelDependsOn, RelLabelAppliesTo, RelLabelDecidedIn, RelLabelBrokenBy:
		return m.store.Table(roleTask)
	case RelLabelMemberOf:
		return m.store.Table(roleProject)
	case RelLabelHasTag:
		return m.store.Table(roleTag)
	case RelLabelHasTodo, RelLabelStartedTodo:
		return m.store.Table(roleTaskTodo)
	case RelLabelStartedTask:
		return m.store.Table(roleTask)
	case RelLabelHasDeliverable:
		return m.store.Table(roleDeliverable)
	case RelLabelHasAcceptanceCriteria:
		return m.store.Table(roleAcceptanceCriteria)
	}
	return m.store.Table(roleTask)
}

func syntheticEdgeID(fromID, label, toID string) string {
	return fromID + "→" + label + "→" + toID
}
