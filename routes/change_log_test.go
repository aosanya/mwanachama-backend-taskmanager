package routes_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

func (h *changesHub) wantSet(t *testing.T, body map[string]any, want ...string) {
	t.Helper()
	got := changeRows(body)
	slices.Sort(got)
	want = slices.Clone(want)
	slices.Sort(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("changes %q, want %q (%v)", strings.Join(got, " "), strings.Join(want, " "), body)
	}
}

func onlyKinds(body map[string]any, kinds ...string) map[string]any {
	kept := []any{}
	for _, raw := range body["changes"].([]any) {
		if slices.Contains(kinds, raw.(map[string]any)["kind"].(string)) {
			kept = append(kept, raw)
		}
	}
	filtered := map[string]any{}
	for key, value := range body {
		filtered[key] = value
	}
	filtered["changes"] = kept
	return filtered
}

func (h *changesHub) quietly(t *testing.T, cursor string) {
	t.Helper()
	h.wantSet(t, h.since("owner", cursor, nil))
}

func TestChangesSinceReportsAMoveWithTheStepItLeftAndTheStepItEntered(t *testing.T) {
	h := newChangesHub(t)
	id := h.file("A", "software")
	cursor := h.settledCursor("owner")

	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": "software", "step": "in-progress"})

	h.wantSet(t, h.since("owner", cursor, nil), "A:moved:software/to-do>software/in-progress")
}

func TestMovingAnIssueToTheStepItIsAlreadyInLeavesNoEntry(t *testing.T) {
	h := newChangesHub(t)
	id := h.file("A", "software")
	cursor := h.settledCursor("owner")

	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": "software", "step": "to-do"})

	h.quietly(t, cursor)
}

func TestChangesSinceReportsAStatusChangeMadeByAPatchWithItsFromAndTo(t *testing.T) {
	h := newChangesHub(t)
	id := h.file("A", "software")
	cursor := h.settledCursor("owner")

	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"status": "in_progress"})

	statuses := 0
	for _, row := range changeRows(h.since("owner", cursor, nil)) {
		if row == "A:status:pending>in_progress" {
			statuses++
		}
	}
	if statuses != 1 {
		t.Errorf("the status change was reported %d times, want once", statuses)
	}
}

func TestChangesSinceReportsATagChangeWithTheOldAndNewTagLists(t *testing.T) {
	h := newChangesHub(t)
	id := h.file("A", "software")
	cursor := h.settledCursor("owner")

	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"tags": []string{"alpha"}})
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"tags": []string{"beta", "alpha"}})

	h.wantSet(t, h.since("owner", cursor, nil), "A:tags:>alpha", "A:tags:alpha>alpha,beta")
}

func TestAPatchThatChangesNothingTrackedLeavesNoEntry(t *testing.T) {
	h := newChangesHub(t)
	id := h.file("A", "software")
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"tags": []string{"alpha"}})
	cursor := h.settledCursor("owner")

	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"title": "Renamed", "tags": []string{"alpha"}})

	h.quietly(t, cursor)
}

func TestChangesSinceReportsAHandOverAReassignmentAndATakeBack(t *testing.T) {
	h := newChangesHub(t)
	id := h.file("A", "software")
	cursor := h.settledCursor("owner")

	h.expect(http.StatusNoContent, "owner", "POST", "/tasks/"+id+"/assign-holder", map[string]any{"assignee_kind": "actor", "assignee_id": "person-1"})
	h.expect(http.StatusNoContent, "owner", "POST", "/tasks/"+id+"/assign-holder", map[string]any{"assignee_kind": "actor", "assignee_id": "person-2"})
	h.expect(http.StatusNoContent, "owner", "POST", "/tasks/"+id+"/unassign", nil)

	h.wantSet(t, h.since("owner", cursor, nil), "A:assigned:>person-1", "A:assigned:person-1>person-2", "A:unassigned:person-2>")
}

func TestChangesSinceReportsADeleteEvenThoughTheIssueIsGoneFromEveryRead(t *testing.T) {
	h := newChangesHub(t)
	id := h.file("A", "software")
	cursor := h.settledCursor("owner")

	if status, body := h.call("owner", "DELETE", "/tasks/"+id, nil); status/100 != 2 {
		t.Fatalf("delete answered %d: %v", status, body)
	}

	h.wantSet(t, h.since("owner", cursor, nil), "A:deleted:>")
}

func TestAChangeToAnIssueOfAHiddenBacklogIsNotReportedToSomeoneWhoCannotSeeIt(t *testing.T) {
	h := newChangesHub(t)
	id := h.file("H", "hidden")
	cursor := h.settledCursor("dev")

	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"tags": []string{"alpha"}})
	h.call("owner", "DELETE", "/tasks/"+id, nil)

	h.wantSet(t, h.since("dev", cursor, nil))
}

func TestABatchLeavesOneEntryForEachChangeItMakes(t *testing.T) {
	h := newChangesHub(t)
	a := h.file("A", "software")
	b := h.file("B", "software")
	cursor := h.settledCursor("owner")

	h.expect(http.StatusOK, "owner", "POST", "/batch", map[string]any{"operations": []any{
		map[string]any{"op": "patch", "task_id": a, "patch": map[string]any{"tags": []string{"alpha"}}},
		map[string]any{"op": "move_to_step", "task_id": b, "backlog": "software", "step": "in-progress"},
	}})

	h.wantSet(t, h.since("owner", cursor, nil), "A:tags:>alpha", "B:moved:software/to-do>software/in-progress")
}

func TestClaimNextLeavesAnEntryForTheMoveTheStatusAndTheHolder(t *testing.T) {
	h := newClaimNextHub(t)
	first := h.ready("First", 10)
	cursor := h.expect(http.StatusOK, "mover", "GET", "/changes-since", nil)["next_cursor"].(string)
	cursor = h.expect(http.StatusOK, "mover", "GET", "/changes-since?cursor="+cursor, nil)["next_cursor"].(string)

	h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "worker-1", "backlog": "journey-book"})

	body := h.expect(http.StatusOK, "mover", "GET", "/changes-since?cursor="+cursor, nil)
	kinds := map[string]string{}
	for _, raw := range body["changes"].([]any) {
		entry := raw.(map[string]any)
		if entry["id"] == first {
			kinds[entry["kind"].(string)] = entry["to"].(string)
		}
	}
	if kinds["moved"] != "journey-book/in-progress" || kinds["status"] != "in_progress" || kinds["assigned"] != "worker-1" {
		t.Errorf("the claim left %v", kinds)
	}
}
