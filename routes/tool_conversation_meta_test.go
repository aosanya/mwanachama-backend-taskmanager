package routes_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/dispatch"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

type conversationRecorder struct {
	mwanachamataskmanager.TaskManager
	mu   sync.Mutex
	seen []string
}

func (r *conversationRecorder) GetTask(ctx context.Context, taskID string) (mwanachamataskmanager.Task, error) {
	r.mu.Lock()
	r.seen = append(r.seen, dispatch.ConversationOf(ctx))
	r.mu.Unlock()
	return r.TaskManager.GetTask(ctx, taskID)
}

func getTaskTool(t *testing.T, recorder *conversationRecorder) routes.Tool {
	t.Helper()
	tools, err := routes.BuildToolsFor(recorder, routes.Mount{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.Name == "taskmanager_task_read" {
			return tool
		}
	}
	t.Fatal("no taskmanager_task_read tool")
	return routes.Tool{}
}

func conversationsSeenBy(t *testing.T, meta map[string]any) ([]string, error) {
	t.Helper()
	recorder := &conversationRecorder{TaskManager: declaredManager(t)}
	args, _ := json.Marshal(map[string]any{"taskID": "any"})
	_, err := getTaskTool(t, recorder).Handle(context.Background(), dispatch.Request{Arguments: args, Meta: meta})
	return recorder.seen, err
}

func TestAToolCallCarryingMetaConversationIdReachesTheManagerOnItsContext(t *testing.T) {
	seen, _ := conversationsSeenBy(t, map[string]any{dispatch.ConversationMetaKey: "chat-42"})
	if len(seen) != 1 || seen[0] != "chat-42" {
		t.Fatalf("the manager saw conversations %q, want exactly [chat-42]", seen)
	}
}

func TestAToolCallCarryingNoMetaReachesTheManagerWithNoConversation(t *testing.T) {
	seen, _ := conversationsSeenBy(t, nil)
	if len(seen) != 1 || seen[0] != "" {
		t.Fatalf("the manager saw conversations %q, want exactly an empty one", seen)
	}
}

func TestAToolCallCarryingAMalformedConversationIdNeverReachesTheManager(t *testing.T) {
	seen, err := conversationsSeenBy(t, map[string]any{dispatch.ConversationMetaKey: "not a conversation"})
	if !errors.Is(err, dispatch.ErrInvalidConversation) {
		t.Fatalf("want ErrInvalidConversation, got %v", err)
	}
	if len(seen) != 0 {
		t.Fatalf("the manager was called %d times", len(seen))
	}
}
