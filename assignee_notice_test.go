package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

type handOverRecorder struct {
	got []mwanachamataskmanager.TaskHandedOverPayload
}

func (r *handOverRecorder) Publish(_ context.Context, topic string, payload any) error {
	if p, ok := payload.(mwanachamataskmanager.TaskHandedOverPayload); ok && topic == mwanachamataskmanager.TopicTaskHandedOver {
		r.got = append(r.got, p)
	}
	return nil
}

func TestAssignTaskToHolder_PublishesTheHandOverToItsHolder(t *testing.T) {
	rec := &handOverRecorder{}
	mgr := newTestManagerWithPublisher(t, rec)
	ctx := context.Background()
	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Fix the gate"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := mgr.AssignTaskToHolder(ctx, task.ID, models.AssigneeActor, "actor-1"); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if err := mgr.AssignTaskToHolder(ctx, task.ID, models.AssigneeRole, "role-desk"); err != nil {
		t.Fatalf("reassign: %v", err)
	}
	if len(rec.got) != 2 {
		t.Fatalf("hand-overs published = %d, want 2", len(rec.got))
	}
	first, second := rec.got[0], rec.got[1]
	if first.TaskID != task.ID || first.AssigneeKind != models.AssigneeActor || first.AssigneeID != "actor-1" || first.Title != "Fix the gate" {
		t.Fatalf("first hand-over = %+v", first)
	}
	if second.AssigneeKind != models.AssigneeRole || second.AssigneeID != "role-desk" {
		t.Fatalf("second hand-over = %+v", second)
	}
}

func TestAssignTaskToHolder_RefusedAssignmentPublishesNothing(t *testing.T) {
	rec := &handOverRecorder{}
	mgr := newTestManagerWithPublisher(t, rec)
	ctx := context.Background()
	if err := mgr.AssignTaskToHolder(ctx, "no-such-task", models.AssigneeActor, "actor-1"); err == nil {
		t.Fatal("assigning a missing task was accepted")
	}
	if len(rec.got) != 0 {
		t.Fatalf("published %d hand-overs for a refused assignment", len(rec.got))
	}
}
