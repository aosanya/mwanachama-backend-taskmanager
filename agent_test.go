package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/entitygraph"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// ── UpsertAgent ──────────────────────────────────────────────────────────────

func TestUpsertAgent_NewAgent_Inserts(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	a, err := mgr.UpsertAgent(context.Background(), mwanachamataskmanager.Agent{
		AgentID: "agent-1", DisplayName: "Coder", Capability: "code",
	})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	if a.ID == "" {
		t.Error("inserted Agent missing ID")
	}
	if a.AgentID != "agent-1" || a.DisplayName != "Coder" || a.Capability != "code" {
		t.Errorf("unexpected Agent: %+v", a)
	}
}

func TestUpsertAgent_SameAgentID_MergesAndReturnsSameVertex(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := mwanachamataskmanager.NewTaskManager(fake, nil)
	ctx := context.Background()

	first, err := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{
		AgentID: "agent-1", DisplayName: "First", Capability: "code",
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second, err := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{
		AgentID: "agent-1", DisplayName: "Second", Capability: "review",
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("upsert created new vertex: first=%s second=%s", first.ID, second.ID)
	}
	if second.DisplayName != "Second" || second.Capability != "review" {
		t.Errorf("merge did not patch fields: %+v", second)
	}

	// Only one Agent vertex per (agentID).
	all, _ := fake.ListEntities(ctx, entitygraph.EntityFilter{TypeID: "Agent"})
	if len(all) != 1 {
		t.Errorf("want 1 Agent in store, got %d", len(all))
	}
}

func TestUpsertAgent_EmptyAgentID_ReturnsError(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.UpsertAgent(context.Background(), mwanachamataskmanager.Agent{})
	if err == nil {
		t.Fatal("want error for empty AgentID, got nil")
	}
}

// ── GetAgent / ListAgents ────────────────────────────────────────────────────

func TestGetAgent_NotFound(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.GetAgent(context.Background(), "missing")
	if !errors.Is(err, mwanachamataskmanager.ErrAgentNotFound) {
		t.Fatalf("got %v, want ErrAgentNotFound", err)
	}
}

func TestGetAgent_RoundTrip(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	created, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "agent-1", DisplayName: "X"})

	got, err := mgr.GetAgent(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if got.AgentID != "agent-1" || got.DisplayName != "X" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestGetAgent_AcceptsSlug(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	created, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "developer-01", DisplayName: "Dev"})

	got, err := mgr.GetAgent(ctx, "developer-01")
	if err != nil {
		t.Fatalf("GetAgent by slug: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("slug resolved to wrong entity: got %s want %s", got.ID, created.ID)
	}
}

func TestGetAgentByAgentID_NotFound(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.GetAgentByAgentID(context.Background(), "missing-slug")
	if !errors.Is(err, mwanachamataskmanager.ErrAgentNotFound) {
		t.Fatalf("got %v, want ErrAgentNotFound", err)
	}
}

func TestAssignTask_AcceptsAgentSlug_EdgeUsesUUID(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := mwanachamataskmanager.NewTaskManager(fake, nil)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	agent, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "developer-01"})

	if err := mgr.AssignTask(ctx, task.ID, "developer-01", ""); err != nil {
		t.Fatalf("AssignTask with slug: %v", err)
	}

	edges, err := mgr.TraverseRelationships(ctx, task.ID, mwanachamataskmanager.RelLabelAssignedTo, mwanachamataskmanager.DirectionOutbound)
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("want 1 assigned_to edge, got %d", len(edges))
	}
	if edges[0].ToID != agent.ID {
		t.Errorf("edge ToID = %q, want resolved UUID %q (not the slug)", edges[0].ToID, agent.ID)
	}
}

func TestListAgents_ReturnsAllAgents(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	_, _ = mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "a"})
	_, _ = mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "b"})
	_, _ = mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "c"})

	all, _ := mgr.ListAgents(ctx)
	if len(all) != 3 {
		t.Errorf("want 3 agents, got %d", len(all))
	}
}

// ── AssignTask ───────────────────────────────────────────────────────────────

func TestAssignTask_UnknownAgent_ReturnsErrAgentNotFound(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})

	err := mgr.AssignTask(ctx, task.ID, "no-such-agent", "")
	if !errors.Is(err, mwanachamataskmanager.ErrAgentNotFound) {
		t.Fatalf("got %v, want ErrAgentNotFound", err)
	}
}

func TestAssignTask_UnknownTask_ReturnsErrTaskNotFound(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	agent, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "a"})

	err := mgr.AssignTask(ctx, "no-such-task", agent.ID, "")
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("got %v, want ErrTaskNotFound", err)
	}
}

func TestAssignTask_HappyPath_CreatesEdge(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	agent, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "a"})

	if err := mgr.AssignTask(ctx, task.ID, agent.ID, ""); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	edges, _ := mgr.TraverseRelationships(ctx, task.ID, mwanachamataskmanager.RelLabelAssignedTo, mwanachamataskmanager.DirectionOutbound)
	if len(edges) != 1 {
		t.Fatalf("want 1 assigned_to edge, got %d", len(edges))
	}
	if edges[0].ToID != agent.ID {
		t.Errorf("edge points to %s, want %s", edges[0].ToID, agent.ID)
	}
}

func TestAssignTask_Reassign_ReplacesEdge(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	a1, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "a1"})
	a2, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "a2"})

	if err := mgr.AssignTask(ctx, task.ID, a1.ID, ""); err != nil {
		t.Fatalf("first AssignTask: %v", err)
	}
	if err := mgr.AssignTask(ctx, task.ID, a2.ID, ""); err != nil {
		t.Fatalf("second AssignTask: %v", err)
	}

	edges, _ := mgr.TraverseRelationships(ctx, task.ID, mwanachamataskmanager.RelLabelAssignedTo, mwanachamataskmanager.DirectionOutbound)
	if len(edges) != 1 {
		t.Fatalf("want exactly 1 outbound edge after reassign, got %d", len(edges))
	}
	if edges[0].ToID != a2.ID {
		t.Errorf("edge points to %s after reassign, want %s", edges[0].ToID, a2.ID)
	}
}

func TestAssignTask_PublishesEvent(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	agent, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "a"})

	if err := mgr.AssignTask(ctx, task.ID, agent.ID, ""); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	var found bool
	for _, ev := range pub.events {
		if ev == "task.assigned" {
			found = true
		}
	}
	if !found {
		t.Errorf("want task.assigned event, got %v", pub.events)
	}
}

// ── UnassignTask ─────────────────────────────────────────────────────────────

func TestUnassignTask_OnAssignedTask_RemovesEdge(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	agent, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "a"})
	_ = mgr.AssignTask(ctx, task.ID, agent.ID, "")

	if err := mgr.UnassignTask(ctx, task.ID); err != nil {
		t.Fatalf("UnassignTask: %v", err)
	}
	edges, _ := mgr.TraverseRelationships(ctx, task.ID, mwanachamataskmanager.RelLabelAssignedTo, mwanachamataskmanager.DirectionOutbound)
	if len(edges) != 0 {
		t.Errorf("want 0 edges after unassign, got %d", len(edges))
	}
}

func TestUnassignTask_OnUnassignedTask_IsIdempotent(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})

	if err := mgr.UnassignTask(ctx, task.ID); err != nil {
		t.Errorf("UnassignTask on unassigned task: got %v, want nil", err)
	}
}

func TestUnassignTask_UnknownTask_ReturnsErrTaskNotFound(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	err := mgr.UnassignTask(context.Background(), "no-such-task")
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("got %v, want ErrTaskNotFound", err)
	}
}

// ── Read path: Task does NOT carry AssignedTo ────────────────────────────────

// This test fails to compile if Task ever regrows an AssignedTo field —
// guarding the assigned_to-is-a-graph-edge schema decision against
// accidental regression.
func TestTask_HasNoAssignedToField(t *testing.T) {
	var task mwanachamataskmanager.Task
	// Compile-time evidence: list every field. If anyone adds AssignedTo
	// back, this struct literal fails.
	task = mwanachamataskmanager.Task{
		ID:             "",
		Description:    "",
		Status:         "",
		Priority:       "",
		DueAt:          "",
		Tags:           nil,
		EstimatedHours: 0,
		Context:        "",
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
		CompletedAt:    "",
	}
	_ = task
}
