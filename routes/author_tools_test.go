package routes_test

import (
	"encoding/json"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func TestToolsDoNotAskForTheAuthorTheCallerSigns(t *testing.T) {
	asked := map[string]string{
		"taskmanager_workflow_run_cancel": "cancelled_by",
		"taskmanager_case_vote":           "member_id",
	}
	seen := map[string]bool{}
	for _, tool := range routes.ToolShape() {
		field, guarded := asked[tool.Name]
		if !guarded {
			continue
		}
		seen[tool.Name] = true
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s schema: %v", tool.Name, err)
		}
		if _, takes := schema.Properties[field]; takes {
			t.Errorf("%s still takes %s from the request; the caller signs it", tool.Name, field)
		}
	}
	for name := range asked {
		if !seen[name] {
			t.Errorf("%s is not a tool", name)
		}
	}
}
