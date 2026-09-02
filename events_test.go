package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-taskmanager"
)

// findEvent is defined in fake_test.go (shared across test files) — it takes
// []publishedEvent, the local stand-in for the original's []eventbus.Event.

func TestCreateTask_PublishesTypedTaskCreatedPayload(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	created, _ := mgr.CreateTask(context.Background(), "ag", mwanachamataskmanager.Task{
		Priority: mwanachamataskmanager.TaskPriorityHigh,
	})

	ev, ok := findEvent(pub.full, mwanachamataskmanager.TopicTaskCreated)
	if !ok {
		t.Fatal("no task.created event published")
	}
	// No AgencyID/Timestamp assertions here — unlike the original's
	// eventbus.Event, events.Publisher.Publish(ctx, topic, payload) carries
	// neither a separate agency envelope nor a stamped timestamp; that was
	// eventbus.SafePublish's job, and mwanachamataskmanager's publish
	// helper calls Publisher.Publish directly (see task_impl_task.go).
	p, ok := ev.Payload.(mwanachamataskmanager.TaskCreatedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want TaskCreatedPayload", ev.Payload)
	}
	if p.TaskID != created.ID || p.Priority != mwanachamataskmanager.TaskPriorityHigh {
		t.Errorf("payload = %+v", p)
	}
}

func TestUpdateTask_NoStatusChange_PublishesUpdatedNotStatusChanged(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	created, _ := mgr.CreateTask(context.Background(), "ag", mwanachamataskmanager.Task{})

	created.Description = "patched"
	if _, err := mgr.UpdateTask(context.Background(), "ag", created); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	if _, ok := findEvent(pub.full, mwanachamataskmanager.TopicTaskStatusChanged); ok {
		t.Error("status.changed fired when no status change occurred")
	}
	ev, ok := findEvent(pub.full, mwanachamataskmanager.TopicTaskUpdated)
	if !ok {
		t.Fatal("no task.updated event")
	}
	p, _ := ev.Payload.(mwanachamataskmanager.TaskUpdatedPayload)
	if len(p.ChangedFields) != 1 || p.ChangedFields[0] != "description" {
		t.Errorf("ChangedFields = %v, want [description]", p.ChangedFields)
	}
}

func TestUpdateTask_StatusChange_FiresStatusChangedWithFromTo(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	created, _ := mgr.CreateTask(context.Background(), "ag", mwanachamataskmanager.Task{})

	created.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), "ag", created); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	ev, ok := findEvent(pub.full, mwanachamataskmanager.TopicTaskStatusChanged)
	if !ok {
		t.Fatal("no status.changed event")
	}
	p, _ := ev.Payload.(mwanachamataskmanager.TaskStatusChangedPayload)
	if p.From != mwanachamataskmanager.TaskStatusPending || p.To != mwanachamataskmanager.TaskStatusInProgress {
		t.Errorf("from/to = %s→%s, want pending→in_progress", p.From, p.To)
	}
	// In_progress is not terminal; completed must NOT have fired.
	if _, ok := findEvent(pub.full, mwanachamataskmanager.TopicTaskCompleted); ok {
		t.Error("completed event fired on non-terminal transition")
	}
}

func TestUpdateTask_StatusChangeOnly_DoesNotFireUpdated(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	created, _ := mgr.CreateTask(context.Background(), "ag", mwanachamataskmanager.Task{})

	// Only the status field differs.
	created.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), "ag", created); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if _, ok := findEvent(pub.full, mwanachamataskmanager.TopicTaskUpdated); ok {
		t.Error("updated event fired when only status changed")
	}
}

func TestAssignTask_Replacement_FiresAssignedOnce(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, "ag", mwanachamataskmanager.Task{})
	a1, _ := mgr.UpsertAgent(ctx, "ag", mwanachamataskmanager.Agent{AgentID: "a1"})
	a2, _ := mgr.UpsertAgent(ctx, "ag", mwanachamataskmanager.Agent{AgentID: "a2"})

	// Reset captured events so we count only the reassign hits.
	if err := mgr.AssignTask(ctx, "ag", task.ID, a1.ID, ""); err != nil {
		t.Fatalf("AssignTask a1: %v", err)
	}
	pub.full = nil
	pub.events = nil

	if err := mgr.AssignTask(ctx, "ag", task.ID, a2.ID, ""); err != nil {
		t.Fatalf("AssignTask a2: %v", err)
	}

	count := 0
	for _, e := range pub.full {
		if e.Topic == mwanachamataskmanager.TopicTaskAssigned {
			count++
		}
	}
	if count != 1 {
		t.Errorf("reassignment fired %d assigned events, want 1", count)
	}
	ev, _ := findEvent(pub.full, mwanachamataskmanager.TopicTaskAssigned)
	p, _ := ev.Payload.(mwanachamataskmanager.TaskAssignedPayload)
	if p.AgentID != a2.ID {
		t.Errorf("AgentID = %s, want %s (the new assignee)", p.AgentID, a2.ID)
	}
}

func TestAssignTask_PayloadHydrated_IncludesTaskCodeAndTitle(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()

	task, _ := mgr.CreateTask(ctx, "ag", mwanachamataskmanager.Task{
		TaskName:    "UTIL-001",
		Title:       "Implement app version display widget",
		Description: "Add a read-only widget to the settings screen showing the current app version.",
	})
	agent, _ := mgr.UpsertAgent(ctx, "ag", mwanachamataskmanager.Agent{
		AgentID:  "dev-01",
		RoleName: "Developer",
	})

	if err := mgr.AssignTask(ctx, "ag", task.ID, agent.ID, ""); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	ev, ok := findEvent(pub.full, mwanachamataskmanager.TopicTaskAssigned)
	if !ok {
		t.Fatal("no task.assigned event published")
	}
	p, ok := ev.Payload.(mwanachamataskmanager.TaskAssignedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want TaskAssignedPayload", ev.Payload)
	}
	if p.TaskCode != "UTIL-001" {
		t.Errorf("TaskCode = %q, want %q", p.TaskCode, "UTIL-001")
	}
	if p.Title != "Implement app version display widget" {
		t.Errorf("Title = %q, want %q", p.Title, "Implement app version display widget")
	}
	if p.Description == "" {
		t.Error("Description is empty; LLM will not have task context")
	}
	if p.RoleName != "Developer" {
		t.Errorf("RoleName = %q, want Developer", p.RoleName)
	}
}

func TestCreateRelationship_PublishesTypedRelationshipCreatedPayload(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, "ag", mwanachamataskmanager.Task{})
	b, _ := mgr.CreateTask(ctx, "ag", mwanachamataskmanager.Task{})

	if _, err := mgr.CreateRelationship(ctx, "ag", mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelBlocks, FromID: a.ID, ToID: b.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}

	ev, ok := findEvent(pub.full, mwanachamataskmanager.TopicRelationshipCreated)
	if !ok {
		t.Fatal("no relationship.created event")
	}
	p, ok := ev.Payload.(mwanachamataskmanager.RelationshipCreatedPayload)
	if !ok {
		t.Fatalf("payload type = %T", ev.Payload)
	}
	if p.FromID != a.ID || p.ToID != b.ID || p.Label != mwanachamataskmanager.RelLabelBlocks {
		t.Errorf("payload = %+v", p)
	}
}

// TestEventSequence_FullPhase2Flow_EmitsExactOrderedTopics drives the
// canonical lifecycle (create → update → assign → status changes to
// completed → create blocks edge) past a recordingPublisher and asserts
// the *exact* list of topics emitted, in order. Single-event tests above
// cover payload shape; this test pins the cross-event ordering — in
// particular, that status.changed precedes completed on a terminal
// transition, and that an update with no status change does not surface
// a status.changed event.
func TestEventSequence_FullPhase2Flow_EmitsExactOrderedTopics(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	const agency = "ag"

	// Step 1 — create a Task.
	task, err := mgr.CreateTask(ctx, agency, mwanachamataskmanager.Task{
		Priority: mwanachamataskmanager.TaskPriorityHigh,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Step 2 — non-status update (description only).
	task.Description = "patched"
	if _, err := mgr.UpdateTask(ctx, agency, task); err != nil {
		t.Fatalf("UpdateTask description: %v", err)
	}

	// Step 3 — assign to a fresh agent.
	agent, err := mgr.UpsertAgent(ctx, agency, mwanachamataskmanager.Agent{AgentID: "a1"})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	if err := mgr.AssignTask(ctx, agency, task.ID, agent.ID, ""); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	// Step 4 — drive task to completed via in_progress.
	cur, err := mgr.GetTask(ctx, agency, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	cur.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, agency, cur); err != nil {
		t.Fatalf("→ in_progress: %v", err)
	}
	cur.Status = mwanachamataskmanager.TaskStatusCompleted
	if _, err := mgr.UpdateTask(ctx, agency, cur); err != nil {
		t.Fatalf("→ completed: %v", err)
	}

	// Step 5 — create a blocks edge to a sibling Task.
	other, err := mgr.CreateTask(ctx, agency, mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatalf("CreateTask other: %v", err)
	}
	if _, err := mgr.CreateRelationship(ctx, agency, mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelBlocks, FromID: task.ID, ToID: other.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}

	wantTopics := []string{
		mwanachamataskmanager.TopicTaskCreated,         // step 1
		mwanachamataskmanager.TopicTaskUpdated,         // step 2 (no status change)
		mwanachamataskmanager.TopicTaskAssigned,        // step 3
		mwanachamataskmanager.TopicTaskStatusChanged,   // step 4a pending → in_progress
		mwanachamataskmanager.TopicTaskStatusChanged,   // step 4b in_progress → completed
		mwanachamataskmanager.TopicTaskCompleted,       // step 4b terminal hook
		mwanachamataskmanager.TopicTaskCreated,         // step 5 sibling
		mwanachamataskmanager.TopicRelationshipCreated, // step 5 edge
	}
	gotTopics := make([]string, len(pub.full))
	for i, e := range pub.full {
		gotTopics[i] = e.Topic
	}
	if len(gotTopics) != len(wantTopics) {
		t.Fatalf("event count: got %d, want %d\n got=%v\nwant=%v",
			len(gotTopics), len(wantTopics), gotTopics, wantTopics)
	}
	for i := range wantTopics {
		if gotTopics[i] != wantTopics[i] {
			t.Errorf("event[%d] topic: got %q, want %q (full sequence: %v)",
				i, gotTopics[i], wantTopics[i], gotTopics)
		}
	}

	// Spot-check key payloads — the full sequence is locked above; here we
	// confirm the typed payloads are intact at the load-bearing positions.
	if p, ok := pub.full[0].Payload.(mwanachamataskmanager.TaskCreatedPayload); !ok ||
		p.TaskID != task.ID || p.Priority != mwanachamataskmanager.TaskPriorityHigh {
		t.Errorf("event[0] TaskCreatedPayload = %+v", pub.full[0].Payload)
	}
	if p, ok := pub.full[2].Payload.(mwanachamataskmanager.TaskAssignedPayload); !ok ||
		p.TaskID != task.ID || p.AgentID != agent.ID {
		t.Errorf("event[2] TaskAssignedPayload = %+v", pub.full[2].Payload)
	}
	if p, ok := pub.full[3].Payload.(mwanachamataskmanager.TaskStatusChangedPayload); !ok ||
		p.From != mwanachamataskmanager.TaskStatusPending || p.To != mwanachamataskmanager.TaskStatusInProgress {
		t.Errorf("event[3] TaskStatusChangedPayload = %+v", pub.full[3].Payload)
	}
	if p, ok := pub.full[4].Payload.(mwanachamataskmanager.TaskStatusChangedPayload); !ok ||
		p.From != mwanachamataskmanager.TaskStatusInProgress || p.To != mwanachamataskmanager.TaskStatusCompleted {
		t.Errorf("event[4] TaskStatusChangedPayload = %+v", pub.full[4].Payload)
	}
	if p, ok := pub.full[5].Payload.(mwanachamataskmanager.TaskCompletedPayload); !ok ||
		p.TaskID != task.ID || p.TerminalStatus != mwanachamataskmanager.TaskStatusCompleted {
		t.Errorf("event[5] TaskCompletedPayload = %+v", pub.full[5].Payload)
	}
	if p, ok := pub.full[7].Payload.(mwanachamataskmanager.RelationshipCreatedPayload); !ok ||
		p.FromID != task.ID || p.ToID != other.ID || p.Label != mwanachamataskmanager.RelLabelBlocks {
		t.Errorf("event[7] RelationshipCreatedPayload = %+v", pub.full[7].Payload)
	}
}

// Note: the original CodeValdWork test suite also had a
// TestAllTopics_StableSurface guarding AllTopics()'s schema-derived +
// business-extra topic set. That function isn't ported (see events.go's
// doc comment — it only ever fed the dropped Cross registrar, and its
// schema-derived half has no replacement now that mwanachama-go-shared's
// schema package dropped TopicsFromSchema). Nothing here to guard.
