package routes_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestTasksAreGeneratedOverTheMux(t *testing.T) {
	tm := wk11Manager(t)
	m := wk11Mux(tm)
	body := map[string]any{
		"run":      "agency/agy1/stations",
		"template": map[string]any{"title": "Check the station", "due_at": "2026-12-01T00:00:00Z"},
		"subjects": []string{"actor/agy1/st-1", "actor/agy1/st-2"},
	}
	first := wk11Post(t, m, "/generated-tasks", body)
	var got models.TasksGenerated
	if first.Code != 200 || json.Unmarshal(first.Body.Bytes(), &got) != nil || len(got.Items) != 2 || !got.Items[0].Created {
		t.Fatalf("generate %d %s", first.Code, first.Body)
	}
	replay := wk11Post(t, m, "/generated-tasks", body)
	var again models.TasksGenerated
	if replay.Code != 200 || json.Unmarshal(replay.Body.Bytes(), &again) != nil || again.Items[0].Created || again.Items[0].TaskID != got.Items[0].TaskID {
		t.Fatalf("replay %d %s", replay.Code, replay.Body)
	}

	bad := wk11Post(t, m, "/generated-tasks", map[string]any{"run": "r", "template": map[string]any{"title": "t"}, "subjects": []string{"a", "a"}})
	if bad.Code != 400 {
		t.Fatalf("repeated subject %d %s", bad.Code, bad.Body)
	}

	many := make([]string, 5001)
	for i := range many {
		many[i] = fmt.Sprintf("s-%d", i)
	}
	over := wk11Post(t, m, "/generated-tasks", map[string]any{"run": "r", "template": map[string]any{"title": "t"}, "subjects": many})
	if over.Code != 413 {
		t.Fatalf("over the cap %d %s", over.Code, over.Body)
	}
}
