package routes_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func wk11Put(t *testing.T, path string, body map[string]any, serve func(*httptest.ResponseRecorder, *bytes.Reader, string)) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	serve(rec, bytes.NewReader(b), path)
	return rec
}

func TestARepeatingTaskIsUpdatedInPlaceOverTheMux(t *testing.T) {
	tm := wk11Manager(t)
	m := wk11Mux(tm)
	created := wk11Post(t, m, "/repeating-tasks", map[string]any{
		"title":         "Monthly street light check",
		"schedule":      map[string]any{"start": "2026-01-01T08:00:00Z", "zone": "Europe/Budapest", "rule": "FREQ=MONTHLY;BYMONTHDAY=1"},
		"assignee_kind": "role",
		"assignee_id":   "role-3",
	})
	var rec models.RepeatingTaskRecord
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &rec) != nil {
		t.Fatalf("create %d %s", created.Code, created.Body)
	}

	put := func(body map[string]any) *httptest.ResponseRecorder {
		return wk11Put(t, "/repeating-tasks/"+rec.ID, body, func(w *httptest.ResponseRecorder, r *bytes.Reader, p string) {
			req := httptest.NewRequest("PUT", p, r)
			m.ServeHTTP(w, req)
		})
	}

	ok := put(map[string]any{
		"title":         "Fortnightly street light check",
		"schedule":      map[string]any{"start": "2026-02-04T08:00:00Z", "zone": "Europe/Budapest", "rule": "FREQ=WEEKLY;INTERVAL=2;BYDAY=WE"},
		"assignee_kind": "role",
		"assignee_id":   "role-3",
	})
	var updated models.RepeatingTaskRecord
	if ok.Code != 200 || json.Unmarshal(ok.Body.Bytes(), &updated) != nil || updated.ID != rec.ID || updated.ScheduleRule != "FREQ=WEEKLY;INTERVAL=2;BYDAY=WE" || updated.Title != "Fortnightly street light check" {
		t.Fatalf("update %d %s", ok.Code, ok.Body)
	}

	bad := put(map[string]any{
		"title":         "x",
		"schedule":      map[string]any{"start": "2026-02-04T08:00:00Z", "zone": "Europe/Budapest", "rule": "BYDAY="},
		"assignee_kind": "role",
		"assignee_id":   "role-3",
	})
	if bad.Code != 400 {
		t.Fatalf("bad rule %d %s", bad.Code, bad.Body)
	}

	missing := wk11Put(t, "/repeating-tasks/nope", map[string]any{
		"title":         "x",
		"schedule":      map[string]any{"start": "2026-02-04T08:00:00Z", "zone": "UTC", "rule": "FREQ=DAILY"},
		"assignee_kind": "role",
		"assignee_id":   "role-3",
	}, func(w *httptest.ResponseRecorder, r *bytes.Reader, p string) { m.ServeHTTP(w, httptest.NewRequest("PUT", p, r)) })
	if missing.Code != 404 {
		t.Fatalf("missing %d %s", missing.Code, missing.Body)
	}

	wk11Post(t, m, "/repeating-tasks/"+rec.ID+"/stop", map[string]any{})
	stopped := put(map[string]any{
		"title":         "Revived",
		"schedule":      map[string]any{"start": "2026-02-04T08:00:00Z", "zone": "UTC", "rule": "FREQ=DAILY"},
		"assignee_kind": "role",
		"assignee_id":   "role-3",
	})
	if stopped.Code != 409 {
		t.Fatalf("stopped %d %s", stopped.Code, stopped.Body)
	}
}
