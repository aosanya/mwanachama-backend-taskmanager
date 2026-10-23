package mwanachamataskmanager_test

import (
	"reflect"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestTheBlueprintDeclaresTaskStatesDefaultingToTheFive(t *testing.T) {
	s, err := mwanachamataskmanager.SpecFor("w31")
	if err != nil {
		t.Fatal(err)
	}
	st, ok := s.Setting("task_states")
	if !ok {
		t.Fatal("the blueprint declares no task_states setting")
	}
	got, err := models.ParseTaskStates(st.Default)
	if err != nil {
		t.Fatalf("the declared default does not parse: %v", err)
	}
	if !reflect.DeepEqual(got, models.DefaultTaskStates()) {
		t.Errorf("declared default %+v, want %+v", got, models.DefaultTaskStates())
	}
}

func TestParseTaskStatesRefusesWhatCannotBeRead(t *testing.T) {
	cases := map[string]any{
		"none":           []any{},
		"unnamed":        []any{map[string]any{"name": " ", "statuses": []any{"pending"}}},
		"twice":          []any{map[string]any{"name": "A"}, map[string]any{"name": "A"}},
		"unknown status": []any{map[string]any{"name": "A", "statuses": []any{"review"}}},
		"status in two":  []any{map[string]any{"name": "A", "statuses": []any{"pending"}}, map[string]any{"name": "B", "statuses": []any{"pending"}}},
		"unknown field":  []any{map[string]any{"name": "A", "colour": "red"}},
		"not a list":     "To do",
	}
	for name, value := range cases {
		if _, err := models.ParseTaskStates(value); err == nil || !strings.HasPrefix(err.Error(), "task states:") {
			t.Errorf("%s: got %v, want a task states refusal", name, err)
		}
	}
}

func TestParseTaskStatesKeepsAnAgencysOwnOrderAndNames(t *testing.T) {
	got, err := models.ParseTaskStates([]any{
		map[string]any{"name": "Backlog", "statuses": []any{"pending"}},
		map[string]any{"name": "Review", "statuses": []any{"in_progress", "awaiting-direction"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if models.StateNameFor(got, models.TaskStatusAwaitingDirection) != "Review" || got[0].Name != "Backlog" {
		t.Errorf("got %+v", got)
	}
	if models.StateNameFor(got, models.TaskStatusCompleted) != "" {
		t.Error("a status no state covers should read with no state")
	}
}
