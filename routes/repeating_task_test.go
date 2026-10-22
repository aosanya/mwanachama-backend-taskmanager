package routes_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/recur"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestARepeatingTaskIsReadListedAndStoppedOverTheMux(t *testing.T) {
	tm := wk11Manager(t)
	rule, err := recur.ParseRule("FREQ=WEEKLY")
	if err != nil {
		t.Fatal(err)
	}
	rec, err := tm.CreateRepeatingTask(context.Background(), mwanachamataskmanager.RepeatingTask{
		Title:        "Weekly count",
		Schedule:     recur.Schedule{Start: time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC), Zone: time.UTC, Rule: &rule},
		AssigneeKind: models.AssigneeRole,
		AssigneeID:   "role-9",
	})
	if err != nil {
		t.Fatal(err)
	}
	m := wk11Mux(tm)

	get := httptest.NewRecorder()
	m.ServeHTTP(get, httptest.NewRequest("GET", "/repeating-tasks/"+rec.ID, nil))
	var read models.RepeatingTaskRecord
	if get.Code != 200 || json.Unmarshal(get.Body.Bytes(), &read) != nil || read.ScheduleRule == "" || read.AssigneeID != "role-9" {
		t.Fatalf("get %d %s", get.Code, get.Body)
	}

	list := httptest.NewRecorder()
	m.ServeHTTP(list, httptest.NewRequest("GET", "/repeating-tasks?assignee_kind=role&assignee_id=role-9", nil))
	var listed []models.RepeatingTaskRecord
	if list.Code != 200 || json.Unmarshal(list.Body.Bytes(), &listed) != nil || len(listed) != 1 {
		t.Fatalf("list %d %s", list.Code, list.Body)
	}

	stop := wk11Post(t, m, "/repeating-tasks/"+rec.ID+"/stop", map[string]any{})
	var stopped models.RepeatingTaskRecord
	if stop.Code != 200 || json.Unmarshal(stop.Body.Bytes(), &stopped) != nil || stopped.StoppedAt == "" {
		t.Fatalf("stop %d %s", stop.Code, stop.Body)
	}

	missing := httptest.NewRecorder()
	m.ServeHTTP(missing, httptest.NewRequest("GET", "/repeating-tasks/nope", nil))
	if missing.Code != 404 {
		t.Fatalf("missing %d %s", missing.Code, missing.Body)
	}
}
