package mwanachamataskmanager_test

import (
	"encoding/json"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// W18: an edge with no Properties must not marshal a null object, which fails
// MCP structured-output schema validation.
func TestRelationship_UnsetPropertiesAreOmittedFromJSON(t *testing.T) {
	b, err := json.Marshal(mwanachamataskmanager.Relationship{ID: "e1", Label: mwanachamataskmanager.RelLabelDependsOn, FromID: "a", ToID: "b"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "Properties") {
		t.Fatalf("unset Properties should be omitted, got %s", b)
	}
	b, _ = json.Marshal(mwanachamataskmanager.Relationship{Properties: map[string]any{"k": "v"}})
	if !strings.Contains(string(b), `"Properties":{"k":"v"}`) {
		t.Fatalf("set Properties should be kept, got %s", b)
	}
}
