package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// seedTask creates a real Task via the manager. Returns the task ID for use
// as a relationship endpoint.
func seedTask(t *testing.T, mgr mwanachamataskmanager.TaskManager, title string) string {
	t.Helper()
	task, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatalf("seedTask(%q): %v", title, err)
	}
	return task.ID
}

// seedAgent creates a real Agent via the manager, with a unique AgentID
// slug derived from name.
func seedAgent(t *testing.T, mgr mwanachamataskmanager.TaskManager, name string) string {
	t.Helper()
	a, err := mgr.UpsertAgent(context.Background(), mwanachamataskmanager.Agent{AgentID: name})
	if err != nil {
		t.Fatalf("seedAgent(%q): %v", name, err)
	}
	return a.ID
}

// seedProject creates a real Project via the manager.
func seedProject(t *testing.T, mgr mwanachamataskmanager.TaskManager, name string) string {
	t.Helper()
	p, err := mgr.CreateProject(context.Background(), mwanachamataskmanager.Project{Name: name})
	if err != nil {
		t.Fatalf("seedProject(%q): %v", name, err)
	}
	return p.ID
}

// ── CreateRelationship ───────────────────────────────────────────────────────

func TestCreateRelationship_AllWhitelistedLabels(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	agent := seedAgent(t, mgr, "agent-all-labels")
	project := seedProject(t, mgr, "project-all-labels")

	cases := []struct {
		label string
		toID  string
	}{
		{mwanachamataskmanager.RelLabelAssignedTo, agent},
		{mwanachamataskmanager.RelLabelBlocks, ""},
		{mwanachamataskmanager.RelLabelSubtaskOf, ""},
		{mwanachamataskmanager.RelLabelDependsOn, ""},
		{mwanachamataskmanager.RelLabelMemberOf, project},
	}
	for _, tc := range cases {
		// Use a fresh source-target pair per label to avoid cardinality
		// constraints (assigned_to / subtask_of are functional).
		from := seedTask(t, mgr, "from-"+tc.label)
		to := tc.toID
		if to == "" {
			to = seedTask(t, mgr, "to-"+tc.label)
		}
		out, err := mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{
			Label:  tc.label,
			FromID: from,
			ToID:   to,
		})
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.label, err)
			continue
		}
		if out.ID == "" {
			t.Errorf("%s: edge missing ID", tc.label)
		}
		if out.Label != tc.label || out.FromID != from || out.ToID != to {
			t.Errorf("%s: round-trip mismatch: %+v", tc.label, out)
		}
	}
}

func TestCreateRelationship_UnknownLabel_ReturnsErrInvalidRelationship(t *testing.T) {
	mgr := newTestManager(t)
	a := seedTask(t, mgr, "a")
	b := seedTask(t, mgr, "b")

	_, err := mgr.CreateRelationship(context.Background(), mwanachamataskmanager.Relationship{
		Label: "not_a_real_label", FromID: a, ToID: b,
	})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRelationship) {
		t.Fatalf("got %v, want ErrInvalidRelationship", err)
	}
}

func TestCreateRelationship_WrongVertexType_ReturnsErrInvalidRelationship(t *testing.T) {
	mgr := newTestManager(t)
	taskID := seedTask(t, mgr, "task")
	projectID := seedProject(t, mgr, "project-wrong-type")

	// blocks must point Task→Task; using Project as the target is invalid.
	_, err := mgr.CreateRelationship(context.Background(), mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelBlocks, FromID: taskID, ToID: projectID,
	})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRelationship) {
		t.Fatalf("got %v, want ErrInvalidRelationship", err)
	}
}

func TestCreateRelationship_MissingTaskEndpoint_ReturnsErrTaskNotFound(t *testing.T) {
	mgr := newTestManager(t)
	taskID := seedTask(t, mgr, "a")

	_, err := mgr.CreateRelationship(context.Background(), mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelBlocks, FromID: taskID, ToID: "no-such-task",
	})
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("got %v, want ErrTaskNotFound", err)
	}
}

func TestCreateRelationship_MissingAgentEndpoint_ReturnsErrAgentNotFound(t *testing.T) {
	mgr := newTestManager(t)
	taskID := seedTask(t, mgr, "a")

	_, err := mgr.CreateRelationship(context.Background(), mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelAssignedTo, FromID: taskID, ToID: "no-such-agent",
	})
	if !errors.Is(err, mwanachamataskmanager.ErrAgentNotFound) {
		t.Fatalf("got %v, want ErrAgentNotFound", err)
	}
}

func TestCreateRelationship_MissingProjectEndpoint_ReturnsErrProjectNotFound(t *testing.T) {
	mgr := newTestManager(t)
	taskID := seedTask(t, mgr, "a")

	_, err := mgr.CreateRelationship(context.Background(), mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelMemberOf, FromID: taskID, ToID: "no-such-project",
	})
	if !errors.Is(err, mwanachamataskmanager.ErrProjectNotFound) {
		t.Fatalf("got %v, want ErrProjectNotFound", err)
	}
}

func TestCreateRelationship_RecreateExisting_IsIdempotent(t *testing.T) {
	mgr := newTestManager(t)
	a := seedTask(t, mgr, "a")
	b := seedTask(t, mgr, "b")
	ctx := context.Background()

	first, err := mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelBlocks, FromID: a, ToID: b,
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelBlocks, FromID: a, ToID: b,
	})
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("idempotent re-create returned new edge: first=%s second=%s", first.ID, second.ID)
	}

	all, _ := mgr.TraverseRelationships(ctx, a, mwanachamataskmanager.RelLabelBlocks, mwanachamataskmanager.DirectionOutbound)
	if len(all) != 1 {
		t.Errorf("want exactly 1 edge in store, got %d", len(all))
	}
}

func TestCreateRelationship_PublishesEvent(t *testing.T) {
	pub := &recordingPublisher{}
	mgr := newTestManagerWithPublisher(t, pub)
	a := seedTask(t, mgr, "a")
	b := seedTask(t, mgr, "b")

	if _, err := mgr.CreateRelationship(context.Background(), mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelBlocks, FromID: a, ToID: b,
	}); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}

	var found bool
	for _, ev := range pub.events {
		if ev == "relationship.created" {
			found = true
		}
	}
	if !found {
		t.Errorf("want relationship.created event, got %v", pub.events)
	}
}

// ── DeleteRelationship ───────────────────────────────────────────────────────

func TestDeleteRelationship_Existing_RemovesEdge(t *testing.T) {
	mgr := newTestManager(t)
	a := seedTask(t, mgr, "a")
	b := seedTask(t, mgr, "b")
	ctx := context.Background()

	if _, err := mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelBlocks, FromID: a, ToID: b,
	}); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}
	if err := mgr.DeleteRelationship(ctx, a, b, mwanachamataskmanager.RelLabelBlocks); err != nil {
		t.Fatalf("DeleteRelationship: %v", err)
	}
	edges, _ := mgr.TraverseRelationships(ctx, a, mwanachamataskmanager.RelLabelBlocks, mwanachamataskmanager.DirectionOutbound)
	if len(edges) != 0 {
		t.Errorf("want 0 edges after delete, got %d", len(edges))
	}
}

func TestDeleteRelationship_Missing_ReturnsErrRelationshipNotFound(t *testing.T) {
	mgr := newTestManager(t)
	a := seedTask(t, mgr, "a")
	b := seedTask(t, mgr, "b")

	err := mgr.DeleteRelationship(context.Background(), a, b, mwanachamataskmanager.RelLabelBlocks)
	if !errors.Is(err, mwanachamataskmanager.ErrRelationshipNotFound) {
		t.Fatalf("got %v, want ErrRelationshipNotFound", err)
	}
}

// ── TraverseRelationships ────────────────────────────────────────────────────

func TestTraverseRelationships_Outbound_ReturnsAllMatchingEdges(t *testing.T) {
	mgr := newTestManager(t)
	source := seedTask(t, mgr, "source")
	target1 := seedTask(t, mgr, "t1")
	target2 := seedTask(t, mgr, "t2")
	ctx := context.Background()

	for _, to := range []string{target1, target2} {
		if _, err := mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{
			Label: mwanachamataskmanager.RelLabelBlocks, FromID: source, ToID: to,
		}); err != nil {
			t.Fatalf("CreateRelationship: %v", err)
		}
	}

	edges, err := mgr.TraverseRelationships(ctx, source, mwanachamataskmanager.RelLabelBlocks, mwanachamataskmanager.DirectionOutbound)
	if err != nil {
		t.Fatalf("TraverseRelationships: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("want 2 edges, got %d", len(edges))
	}
	gotTargets := []string{edges[0].ToID, edges[1].ToID}
	sort.Strings(gotTargets)
	wantTargets := []string{target1, target2}
	sort.Strings(wantTargets)
	for i := range gotTargets {
		if gotTargets[i] != wantTargets[i] {
			t.Errorf("targets mismatch: got %v, want %v", gotTargets, wantTargets)
			break
		}
	}
}

func TestTraverseRelationships_Inbound_ReturnsOnlyEdgesPointingAtVertex(t *testing.T) {
	mgr := newTestManager(t)
	a := seedTask(t, mgr, "a")
	b := seedTask(t, mgr, "b") // a blocks b
	c := seedTask(t, mgr, "c") // b blocks c (b is on the OUTBOUND side here)
	ctx := context.Background()

	if _, err := mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelBlocks, FromID: a, ToID: b,
	}); err != nil {
		t.Fatalf("a→b: %v", err)
	}
	if _, err := mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelBlocks, FromID: b, ToID: c,
	}); err != nil {
		t.Fatalf("b→c: %v", err)
	}

	// Inbound on b → should return only the a→b edge.
	edges, err := mgr.TraverseRelationships(ctx, b, mwanachamataskmanager.RelLabelBlocks, mwanachamataskmanager.DirectionInbound)
	if err != nil {
		t.Fatalf("TraverseRelationships: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("want 1 inbound edge on b, got %d", len(edges))
	}
	if edges[0].FromID != a || edges[0].ToID != b {
		t.Errorf("wrong edge: %+v", edges[0])
	}
}

func TestTraverseRelationships_NoMatches_ReturnsEmpty(t *testing.T) {
	mgr := newTestManager(t)
	a := seedTask(t, mgr, "a")

	edges, err := mgr.TraverseRelationships(context.Background(), a, mwanachamataskmanager.RelLabelBlocks, mwanachamataskmanager.DirectionOutbound)
	if err != nil {
		t.Fatalf("TraverseRelationships: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("want 0 edges, got %d", len(edges))
	}
}
