package routes_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func callTool(h *accessHub, name string, args map[string]any) error {
	h.t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		h.t.Fatal(err)
	}
	_, err = h.tool(name).Invoke(context.WithValue(context.Background(), callerKey{}, "owner"), raw)
	return err
}
func issueTools(h *accessHub) []string {
	var names []string
	for _, tool := range h.tools {
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			h.t.Fatal(err)
		}
		if _, has := schema.Properties["taskID"]; has {
			names = append(names, tool.Name)
		}
	}
	return names
}
func TestIssueTools_EveryToolThatTakesOneIssueAcceptsIDAsWellAsTaskID(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"owners"}})
	names := issueTools(h)
	if len(names) < 10 {
		t.Fatalf("only %d tools take a taskID, want the whole family", len(names))
	}
	for _, name := range names {
		for _, key := range []string{"taskID", "id"} {
			args := map[string]any{key: "no-such-issue"}
			err := callTool(h, name, args)
			if err != nil && strings.Contains(err.Error(), "takes no argument") {
				t.Errorf("%s refuses %q: %v", name, key, err)
			}
		}
	}
}
func TestIssueTools_IDAndTaskIDAreOneArgumentAndTheyMustAgree(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"owners"}})
	err := callTool(h, "taskmanager_task_read", map[string]any{"id": "a", "taskID": "b"})
	if err == nil || !strings.Contains(err.Error(), "taskID") || !strings.Contains(err.Error(), "id") {
		t.Fatalf("conflicting id and taskID: err = %v, want a refusal naming both", err)
	}
	err = callTool(h, "taskmanager_task_read", map[string]any{"id": "a", "taskID": "a"})
	if err != nil && strings.Contains(err.Error(), "takes no argument") {
		t.Fatalf("the same value under both names was refused: %v", err)
	}
}
func TestIssueTools_AnUnknownArgumentNamesTheAliasOfTheIssue(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"owners"}})
	err := callTool(h, "taskmanager_task_read", map[string]any{"issue": "x"})
	if err == nil || !strings.Contains(err.Error(), "taskID") || !strings.Contains(err.Error(), `"id"`) {
		t.Fatalf("an unknown argument: err = %v, want a refusal that names taskID and its alias id", err)
	}
}
