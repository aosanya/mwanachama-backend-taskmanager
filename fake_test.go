package mwanachamataskmanager_test

import (
	"context"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/entitygraph"
	"github.com/google/uuid"
)

// ── Fake DataManager ─────────────────────────────────────────────────────────

// fakeDataManager is an in-memory entitygraph.DataManager used for unit tests.
// mwanachama-backend-shared ships a schema-aware memory.Backend (S8), but its
// UpsertEntity requires an active published schema (GetActive) — this
// lighter fake keeps unit tests that only care about TaskManager behavior
// (not schema plumbing) free of that setup, mirroring the original
// CodeValdWork test suite's own fake.
type fakeDataManager struct {
	entities      map[string]entitygraph.Entity       // key: entityID
	relationships map[string]entitygraph.Relationship // key: relID
}

func newFakeDataManager() *fakeDataManager {
	return &fakeDataManager{
		entities:      make(map[string]entitygraph.Entity),
		relationships: make(map[string]entitygraph.Relationship),
	}
}

func (f *fakeDataManager) key(entityID string) string {
	return entityID
}

func (f *fakeDataManager) CreateEntity(_ context.Context, req entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	props := make(map[string]any, len(req.Properties))
	for k, v := range req.Properties {
		props[k] = v
	}
	e := entitygraph.Entity{
		ID:         id,
		TypeID:     req.TypeID,
		Properties: props,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	f.entities[f.key(id)] = e
	return e, nil
}

func (f *fakeDataManager) GetEntity(_ context.Context, entityID string) (entitygraph.Entity, error) {
	e, ok := f.entities[f.key(entityID)]
	if !ok || e.Deleted {
		return entitygraph.Entity{}, entitygraph.ErrEntityNotFound
	}
	return e, nil
}

func (f *fakeDataManager) UpdateEntity(_ context.Context, entityID string, req entitygraph.UpdateEntityRequest) (entitygraph.Entity, error) {
	k := f.key(entityID)
	e, ok := f.entities[k]
	if !ok || e.Deleted {
		return entitygraph.Entity{}, entitygraph.ErrEntityNotFound
	}
	if e.Properties == nil {
		e.Properties = map[string]any{}
	}
	for k2, v := range req.Properties {
		e.Properties[k2] = v
	}
	e.UpdatedAt = time.Now().UTC()
	f.entities[k] = e
	return e, nil
}

func (f *fakeDataManager) DeleteEntity(_ context.Context, entityID string) error {
	k := f.key(entityID)
	e, ok := f.entities[k]
	if !ok || e.Deleted {
		return entitygraph.ErrEntityNotFound
	}
	now := time.Now().UTC()
	e.Deleted = true
	e.DeletedAt = &now
	f.entities[k] = e
	return nil
}

func (f *fakeDataManager) ListEntities(_ context.Context, filter entitygraph.EntityFilter) ([]entitygraph.Entity, error) {
	var out []entitygraph.Entity
	for _, e := range f.entities {
		if e.Deleted {
			continue
		}
		if filter.TypeID != "" && e.TypeID != filter.TypeID {
			continue
		}
		match := true
		for k, want := range filter.Properties {
			got, ok := e.Properties[k]
			if !ok || got != want {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		out = append(out, e)
	}
	if out == nil {
		out = []entitygraph.Entity{}
	}
	return out, nil
}

// UpsertEntity in the fake matches a non-deleted entity by every property
// listed in the type's UniqueKey. The fake doesn't carry the schema, so
// callers pass UniqueKey-relevant property values via req.Properties and
// the fake key-matches against those — sufficient for unit tests where the
// caller knows which keys are unique.
func (f *fakeDataManager) UpsertEntity(ctx context.Context, req entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	uniqueKey := uniqueKeyFor(req.TypeID)
	if len(uniqueKey) == 0 {
		return entitygraph.Entity{}, entitygraph.ErrUniqueKeyNotDefined
	}
	for _, e := range f.entities {
		if e.Deleted || e.TypeID != req.TypeID {
			continue
		}
		match := true
		for _, k := range uniqueKey {
			if e.Properties[k] != req.Properties[k] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		// Merge: patch properties onto the existing entity.
		if e.Properties == nil {
			e.Properties = map[string]any{}
		}
		for k, v := range req.Properties {
			e.Properties[k] = v
		}
		e.UpdatedAt = time.Now().UTC()
		f.entities[f.key(e.ID)] = e
		return e, nil
	}
	// Insert path.
	return f.CreateEntity(ctx, req)
}

// uniqueKeyFor returns the property names that form the natural key for
// the given Work type. Mirrors [DefaultWorkSchema]'s UniqueKey declarations.
func uniqueKeyFor(typeID string) []string {
	switch typeID {
	case "Agent":
		return []string{"agent_id"}
	default:
		return nil
	}
}

func (f *fakeDataManager) CreateRelationship(_ context.Context, req entitygraph.CreateRelationshipRequest) (entitygraph.Relationship, error) {
	if _, ok := f.entities[f.key(req.FromID)]; !ok {
		return entitygraph.Relationship{}, entitygraph.ErrEntityNotFound
	}
	if _, ok := f.entities[f.key(req.ToID)]; !ok {
		return entitygraph.Relationship{}, entitygraph.ErrEntityNotFound
	}
	id := uuid.NewString()
	props := make(map[string]any, len(req.Properties))
	for k, v := range req.Properties {
		props[k] = v
	}
	r := entitygraph.Relationship{
		ID:         id,
		Name:       req.Name,
		FromID:     req.FromID,
		ToID:       req.ToID,
		Properties: props,
		CreatedAt:  time.Now().UTC(),
	}
	f.relationships[f.key(id)] = r
	return r, nil
}

func (f *fakeDataManager) GetRelationship(_ context.Context, relID string) (entitygraph.Relationship, error) {
	r, ok := f.relationships[f.key(relID)]
	if !ok {
		return entitygraph.Relationship{}, entitygraph.ErrRelationshipNotFound
	}
	return r, nil
}

func (f *fakeDataManager) DeleteRelationship(_ context.Context, relID string) error {
	k := f.key(relID)
	if _, ok := f.relationships[k]; !ok {
		return entitygraph.ErrRelationshipNotFound
	}
	delete(f.relationships, k)
	return nil
}

func (f *fakeDataManager) ListRelationships(_ context.Context, filter entitygraph.RelationshipFilter) ([]entitygraph.Relationship, error) {
	out := make([]entitygraph.Relationship, 0)
	for _, r := range f.relationships {
		if filter.FromID != "" && r.FromID != filter.FromID {
			continue
		}
		if filter.ToID != "" && r.ToID != filter.ToID {
			continue
		}
		if filter.Name != "" && r.Name != filter.Name {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// ── recordingPublisher ───────────────────────────────────────────────────────

// publishedEvent is one recorded call to [recordingPublisher.Publish].
type publishedEvent struct {
	Topic   string
	Payload any
}

// recordingPublisher is the test-side implementation of [events.Publisher].
// It records every call for assertions and also derives a topic-only
// projection (events) for the legacy assertion form. Unlike the original
// CodeValdWork test suite's recordingPublisher, there is no AgencyID to
// record — mwanachama-backend-shared's simplified Publisher contract
// (Publish(ctx, topic, payload)) carries no separate agency envelope.
type recordingPublisher struct {
	full   []publishedEvent
	events []string // topic-only projection of full
}

func (p *recordingPublisher) Publish(_ context.Context, topic string, payload any) error {
	p.full = append(p.full, publishedEvent{Topic: topic, Payload: payload})
	p.events = append(p.events, topic)
	return nil
}

// findEvent returns the first recorded event matching topic.
func findEvent(events []publishedEvent, topic string) (publishedEvent, bool) {
	for _, e := range events {
		if e.Topic == topic {
			return e, true
		}
	}
	return publishedEvent{}, false
}
