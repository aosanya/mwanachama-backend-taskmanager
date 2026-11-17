package routes_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type changesHub struct {
	*accessHub
	ids map[string]string
}

func newChangesHub(t *testing.T) *changesHub {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	for _, name := range []string{"Software", "Hidden"} {
		h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": name, "steps": []string{"To do", "In progress", "Done"}})
	}
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})
	return &changesHub{accessHub: h, ids: map[string]string{}}
}

func (h *changesHub) file(code, backlog string) string {
	made := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{
		"title": "Title " + code, "code": code, "effort": "low", "description": "a description of " + code,
	})
	id := made["id"].(string)
	h.ids[code] = id
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": backlog, "step": "to-do"})
	return id
}

func (h *changesHub) since(requester, cursor string, extra url.Values) map[string]any {
	query := url.Values{}
	for key, values := range extra {
		query[key] = values
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	path := "/changes-since"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return h.expect(http.StatusOK, requester, "GET", path, nil)
}

func (h *changesHub) settledCursor(requester string) string {
	cursor := h.since(requester, "", nil)["next_cursor"].(string)
	return h.since(requester, cursor, nil)["next_cursor"].(string)
}

func changeRows(body map[string]any) []string {
	rows := []string{}
	for _, raw := range body["changes"].([]any) {
		entry := raw.(map[string]any)
		from, _ := entry["from"].(string)
		to, _ := entry["to"].(string)
		rows = append(rows, entry["code"].(string)+":"+entry["kind"].(string)+":"+from+">"+to)
	}
	return rows
}

func wantChanges(t *testing.T, body map[string]any, want ...string) {
	t.Helper()
	got := strings.Join(changeRows(body), " ")
	if got != strings.Join(want, " ") {
		t.Errorf("changes %q, want %q (%v)", got, strings.Join(want, " "), body)
	}
}

func TestChangesSinceWithoutACursorAnswersOnlyACursor(t *testing.T) {
	h := newChangesHub(t)
	h.file("A", "software")

	body := h.since("owner", "", nil)

	wantChanges(t, body)
	if cursor, _ := body["next_cursor"].(string); cursor == "" {
		t.Errorf("no cursor came back: %v", body)
	}
	if body["has_more"] != false {
		t.Errorf("has_more is %v, want false", body["has_more"])
	}
}

func TestChangesSinceReportsACreatedIssueOnceAndNotAgain(t *testing.T) {
	h := newChangesHub(t)
	cursor := h.since("owner", "", nil)["next_cursor"].(string)
	h.file("A", "software")

	first := h.since("owner", cursor, nil)
	second := h.since("owner", first["next_cursor"].(string), nil)

	wantChanges(t, first, "A:created:>")
	entry := first["changes"].([]any)[0].(map[string]any)
	for _, field := range []string{"id", "code", "title", "kind", "at"} {
		if entry[field] == nil || entry[field] == "" {
			t.Errorf("a change lacks %q: %v", field, entry)
		}
	}
	if entry["id"] != h.ids["A"] || entry["title"] != "Title A" {
		t.Errorf("the change names the wrong issue: %v", entry)
	}
	wantChanges(t, second)
}

func TestChangesSinceReportsAPriorityChangeWithItsFromAndTo(t *testing.T) {
	h := newChangesHub(t)
	id := h.file("A", "software")
	cursor := h.settledCursor("owner")

	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"priority": "high"})

	wantChanges(t, h.since("owner", cursor, nil), "A:priority:medium>high")
}

func TestChangesSincePagesWithoutLossOrRepeat(t *testing.T) {
	h := newChangesHub(t)
	cursor := h.since("owner", "", nil)["next_cursor"].(string)
	wanted := []string{"A", "B", "C", "D", "E"}
	for _, code := range wanted {
		h.file(code, "software")
	}
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+h.ids["C"], map[string]any{"priority": "low"})

	seen := map[string]int{}
	pages := 0
	for {
		body := h.since("owner", cursor, url.Values{"limit": {"2"}})
		pages++
		for _, row := range changeRows(body) {
			seen[row]++
		}
		cursor = body["next_cursor"].(string)
		if body["has_more"] != true {
			break
		}
		if pages > 10 {
			t.Fatal("the pages never end")
		}
	}

	for _, code := range wanted {
		if seen[code+":created:>"] != 1 {
			t.Errorf("%s created appeared %d times, want 1 (%v)", code, seen[code+":created:>"], seen)
		}
	}
	if seen["C:priority:medium>low"] != 1 {
		t.Errorf("the priority change appeared %d times, want 1 (%v)", seen["C:priority:medium>low"], seen)
	}
	if pages != 3 {
		t.Errorf("%d pages for six changes at limit 2, want 3", pages)
	}
}

func TestChangesSinceListsChangesInTimeOrder(t *testing.T) {
	h := newChangesHub(t)
	cursor := h.since("owner", "", nil)["next_cursor"].(string)
	first := h.file("A", "software")
	h.file("B", "software")
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+first, map[string]any{"priority": "high"})

	body := h.since("owner", cursor, nil)

	var previous time.Time
	for _, raw := range body["changes"].([]any) {
		at, err := time.Parse(time.RFC3339Nano, raw.(map[string]any)["at"].(string))
		if err != nil || at.Before(previous) {
			t.Errorf("%v comes after %v (%v): %v", at, previous, err, body)
		}
		previous = at
	}
	if len(body["changes"].([]any)) != 3 {
		t.Errorf("want three changes: %v", body)
	}
}

func TestChangesSinceDoesNotReportAMove(t *testing.T) {
	h := newChangesHub(t)
	id := h.file("A", "software")
	cursor := h.settledCursor("owner")

	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": "software", "step": "in-progress"})

	wantChanges(t, h.since("owner", cursor, nil))
}

func TestChangesSinceKeepsTheCursorWhenNothingChanged(t *testing.T) {
	h := newChangesHub(t)
	h.file("A", "software")
	cursor := h.settledCursor("owner")

	body := h.since("owner", cursor, nil)

	if body["next_cursor"] != cursor {
		t.Errorf("cursor changed from %q to %v with nothing new", cursor, body["next_cursor"])
	}
}

func TestChangesSinceRefusesABadCursor(t *testing.T) {
	h := newChangesHub(t)
	for _, cursor := range []string{"not a cursor", "bm9wZQ", "LTV8"} {
		h.expect(http.StatusBadRequest, "owner", "GET", "/changes-since?cursor="+url.QueryEscape(cursor), nil)
	}
}

func TestChangesSinceLimitIsBetweenOneAndFiveHundred(t *testing.T) {
	h := newChangesHub(t)
	h.expect(http.StatusOK, "owner", "GET", "/changes-since?limit=500", nil)
	h.expect(http.StatusBadRequest, "owner", "GET", "/changes-since?limit=501", nil)
}

func TestChangesSinceShowsOnlyWhatTheCallerCanSee(t *testing.T) {
	h := newChangesHub(t)
	cursor := h.since("owner", "", nil)["next_cursor"].(string)
	h.file("VISIBLE", "software")
	hiddenID := h.file("SECRET", "hidden")
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+hiddenID, map[string]any{"priority": "high"})

	wantChanges(t, h.since("dev", cursor, nil), "VISIBLE:created:>")
	wantChanges(t, h.since("nobody", cursor, nil))
	if got := len(h.since("owner", cursor, nil)["changes"].([]any)); got != 3 {
		t.Errorf("the owner sees %d changes, want 3", got)
	}
}

func TestChangesSinceNarrowsToABacklog(t *testing.T) {
	h := newChangesHub(t)
	cursor := h.since("owner", "", nil)["next_cursor"].(string)
	h.file("A", "software")
	h.file("B", "hidden")

	wantChanges(t, h.since("owner", cursor, url.Values{"backlog": {"hidden"}}), "B:created:>")
	h.expect(http.StatusBadRequest, "dev", "GET", "/changes-since?backlog=hidden", nil)
}

func TestChangesSinceNarrowsToATag(t *testing.T) {
	h := newChangesHub(t)
	cursor := h.since("owner", "", nil)["next_cursor"].(string)
	tagged := h.file("A", "software")
	h.file("B", "software")
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+tagged, map[string]any{"tags": []string{"Ready"}})

	wantChanges(t, h.since("owner", cursor, url.Values{"tag": {"Ready"}}), "A:created:>")
}

func TestChangesSinceMayNameAnIssueFiledJustBeforeTheFirstCursorButNeverLosesOne(t *testing.T) {
	h := newChangesHub(t)
	h.file("BEFORE", "software")
	cursor := h.since("owner", "", nil)["next_cursor"].(string)
	h.file("AFTER", "software")

	rows := changeRows(h.since("owner", cursor, nil))

	if !strings.Contains(strings.Join(rows, " "), "AFTER:created:>") {
		t.Errorf("an issue filed after the cursor was lost: %v", rows)
	}
}
