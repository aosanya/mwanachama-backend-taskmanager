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

func TestARepeatingTaskIsCreatedOverTheMuxWithTheFormsScheduleShape(t *testing.T) {
	tm := wk11Manager(t)
	m := wk11Mux(tm)

	created := wk11Post(t, m, "/repeating-tasks", map[string]any{
		"title":       "Monthly margin review",
		"description": "Review last month's margin.",
		"schedule": map[string]any{
			"start": "2026-01-01T08:00:00Z",
			"end":   "2027-01-01T08:00:00Z",
			"zone":  "Africa/Nairobi",
			"rule":  "FREQ=MONTHLY",
		},
		"assignee_kind": "role",
		"assignee_id":   "role-3",
	})
	var rec models.RepeatingTaskRecord
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &rec) != nil {
		t.Fatalf("create %d %s", created.Code, created.Body)
	}
	if rec.ID == "" || rec.ScheduleRule == "" || rec.ScheduleZone != "Africa/Nairobi" || rec.ScheduleEnd != "2027-01-01T08:00:00Z" || rec.AssigneeID != "role-3" {
		t.Fatalf("stored %+v", rec)
	}
	if _, err := tm.GetRepeatingTask(context.Background(), rec.ID); err != nil {
		t.Fatal(err)
	}

	dated := wk11Post(t, m, "/repeating-tasks", map[string]any{
		"title":         "Term report",
		"schedule":      map[string]any{"start": "2026-01-05T00:00:00Z", "zone": "UTC", "end": "2026-12-18T00:00:00Z", "dates": []string{"2026-01-05", "2026-04-20", "2026-09-07"}},
		"assignee_kind": "actor",
		"assignee_id":   "actor-1",
	})
	if dated.Code != 201 {
		t.Fatalf("dates %d %s", dated.Code, dated.Body)
	}

	for name, body := range map[string]map[string]any{
		"unknown zone": {"title": "x", "schedule": map[string]any{"start": "2026-01-01T00:00:00Z", "zone": "Mars/Base", "rule": "FREQ=DAILY"}, "assignee_kind": "role", "assignee_id": "r"},
		"bad start":    {"title": "x", "schedule": map[string]any{"start": "tomorrow", "zone": "UTC", "rule": "FREQ=DAILY"}, "assignee_kind": "role", "assignee_id": "r"},
		"agent":        {"title": "x", "schedule": map[string]any{"start": "2026-01-01T00:00:00Z", "zone": "UTC", "rule": "FREQ=DAILY"}, "assignee_kind": "agent", "assignee_id": "a"},
	} {
		got := wk11Post(t, m, "/repeating-tasks", body)
		if got.Code != 400 {
			t.Fatalf("%s: %d %s", name, got.Code, got.Body)
		}
	}
}
