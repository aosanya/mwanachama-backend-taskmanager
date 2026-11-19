package routes_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const takeUpHolder = "take-up-2"

func (h *stuckHub) claimAs(id string, body map[string]any) map[string]any {
	h.t.Helper()
	return h.expect(http.StatusOK, "mover", "POST", "/tasks/"+id+"/claim", body)
}
func (h *stuckHub) startedTodo(id string) {
	h.t.Helper()
	h.setTodo(h.todo(id, "Build it"), "In progress", nil)
}
func (h *stuckHub) displayed(id, title string) string {
	h.t.Helper()
	if code, _ := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)["code"].(string); code != "" {
		return code
	}
	return title
}
func assertNoHolder(t *testing.T, h *stuckHub, id string, answer map[string]any) {
	t.Helper()
	if agent, has := answer["agent"]; has && agent != "" {
		t.Errorf("the claim answer names agent %v, want none", agent)
	}
	if lease, has := answer["lease_until"]; has && lease != "" {
		t.Errorf("the claim answer carries lease_until %v, want none", lease)
	}
	stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
	if assignee, has := stored["assigned_to"]; has && assignee != "" {
		t.Errorf("the stored issue is assigned to %v, want nobody", assignee)
	}
	if lease, has := stored["lease_until"]; has && lease != "" {
		t.Errorf("the stored issue carries lease_until %v, want none", lease)
	}
}
func TestClaimWithAHolderAnswersWithTheAgentAndAThirtyMinuteLease(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	body := h.claimAs(id, map[string]any{"holder": takeUpHolder})
	if body["agent"] != takeUpHolder {
		t.Errorf("agent %v, want %s", body["agent"], takeUpHolder)
	}
	wantLeaseNear(t, body["lease_until"], 30)
	issue := body["issue"].(map[string]any)
	if issue["id"] != id || issue["status"] != "in_progress" || issue["step"] != "In Progress" {
		t.Errorf("the claimed issue is %v, want %s in progress", issue, id)
	}
	if body["note_id"] == "" || body["note_id"] == nil || body["caps"] == nil {
		t.Errorf("the answer lost the note_id or caps a plain claim returns: %v", body)
	}
}
func TestClaimWithAHolderStoresTheHolderAsAgentAssigneeAndTheLeaseOnTheIssue(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	body := h.claimAs(id, map[string]any{"holder": takeUpHolder})
	stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
	if stored["assigned_to"] != takeUpHolder || stored["assignee_kind"] != "agent" || stored["lease_until"] != body["lease_until"] || stored["step"] != "in-progress" {
		t.Errorf("the stored issue is %v, want assigned_to %s, kind agent and the answered lease", stored, takeUpHolder)
	}
}
func TestClaimWithAHolderStillWritesTheTakenByTheCallerEntry(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	body := h.claimAs(id, map[string]any{"holder": takeUpHolder})
	notes := h.notesAs("owner", "/tasks/"+id+"/notes")
	if len(notes) != 1 || notes[0]["id"] != body["note_id"] || !strings.HasPrefix(fmt.Sprint(notes[0]["text"]), "Taken by mover.") {
		t.Errorf("the Log is %v, want one entry %v written by the caller", notes, body["note_id"])
	}
}
func TestClaimTrimsTheHolderLabelBeforeRecordingIt(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	body := h.claimAs(id, map[string]any{"holder": "  " + takeUpHolder + "  "})
	if body["agent"] != takeUpHolder {
		t.Errorf("agent %q, want %q", body["agent"], takeUpHolder)
	}
	if stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil); stored["assigned_to"] != takeUpHolder {
		t.Errorf("assigned_to %q, want %q", stored["assigned_to"], takeUpHolder)
	}
}
func TestClaimWithLeaseMinutesSetsTheLeaseLength(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	body := h.claimAs(id, map[string]any{"holder": takeUpHolder, "lease_minutes": 90})
	wantLeaseNear(t, body["lease_until"], 90)
	if stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil); stored["lease_until"] != body["lease_until"] {
		t.Errorf("the stored lease is %v, want %v", stored["lease_until"], body["lease_until"])
	}
}
func TestClaimAcceptsTheLeaseBoundsOneAndTwoHundredFortyMinutes(t *testing.T) {
	h := newStuckHub(t)
	short := h.ready("Short lease", 10)
	long := h.ready("Long lease", 20)
	wantLeaseNear(t, h.claimAs(short, map[string]any{"holder": "w-short", "lease_minutes": 1})["lease_until"], 1)
	wantLeaseNear(t, h.claimAs(long, map[string]any{"holder": "w-long", "lease_minutes": 240})["lease_until"], 240)
}
func TestClaimRefusesALeaseMinutesOutsideOneToTwoHundredFortyWith400AndLeavesTheIssueAtToDo(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	for _, minutes := range []int{-5, -1, 241, 1000} {
		if status, body := h.call("mover", "POST", "/tasks/"+id+"/claim", map[string]any{"holder": takeUpHolder, "lease_minutes": minutes}); status != http.StatusBadRequest {
			t.Errorf("lease_minutes %d = %d %v, want 400", minutes, status, body)
		}
	}
	assertUntouched(t, h.accessHub, id, "to-do")
}
func TestClaimWithABlankHolderIsAPlainClaimWithNoAssigneeAndNoLease(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	body := h.claimAs(id, map[string]any{"holder": "   "})
	assertNoHolder(t, h, id, body)
	if issue := body["issue"].(map[string]any); issue["status"] != "in_progress" {
		t.Errorf("the issue is %v, want it in progress", issue)
	}
}
func TestClaimWithAHolderLongerThanASensibleLabelIsAPlainClaimWithNoAssigneeAndNoLease(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	body := h.claimAs(id, map[string]any{"holder": strings.Repeat("x", 200)})
	assertNoHolder(t, h, id, body)
	if issue := body["issue"].(map[string]any); issue["status"] != "in_progress" {
		t.Errorf("the issue is %v, want it in progress", issue)
	}
}
func TestClaimWithAnEmptyJSONObjectBehavesAsTheOldBodylessCall(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	body := h.claimAs(id, map[string]any{})
	assertNoHolder(t, h, id, body)
}
func TestClaimWithoutABodyStillWorksAndRecordsNoAssigneeAndNoLease(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	body := h.claimAs(id, nil)
	assertNoHolder(t, h, id, body)
	if issue := body["issue"].(map[string]any); issue["status"] != "in_progress" || issue["step"] != "In Progress" {
		t.Errorf("the issue is %v, want it in progress", issue)
	}
}
func TestBoardSnapshotAgentsListsTheHolderOfAClaimWithTheIssueCode(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	h.claimAs(id, map[string]any{"holder": takeUpHolder})
	agents := h.expect(http.StatusOK, "owner", "GET", "/board-snapshot", nil)["agents"].([]any)
	if len(agents) != 1 {
		t.Fatalf("agents %v, want only %s", agents, takeUpHolder)
	}
	agent := agents[0].(map[string]any)
	if agent["agent"] != takeUpHolder || agent["in_progress"] != float64(1) {
		t.Errorf("agent %v, want %s with one issue in progress", agent, takeUpHolder)
	}
	codes := fmt.Sprint(agent["codes"])
	if want := h.displayed(id, "Held issue"); !strings.Contains(codes, want) {
		t.Errorf("codes %s do not name the issue %q", codes, want)
	}
}
func TestBoardSnapshotAgentsSeparatesTwoHoldersByTheirOwnClaims(t *testing.T) {
	h := newStuckHub(t)
	first := h.ready("First held", 10)
	second := h.ready("Second held", 20)
	h.claimAs(first, map[string]any{"holder": "take-up-1"})
	h.claimAs(second, map[string]any{"holder": "take-up-2"})
	agents := h.expect(http.StatusOK, "owner", "GET", "/board-snapshot", nil)["agents"].([]any)
	names := []string{}
	for _, raw := range agents {
		names = append(names, fmt.Sprint(raw.(map[string]any)["agent"]))
	}
	if strings.Join(names, ",") != "take-up-1,take-up-2" {
		t.Errorf("agents %v, want take-up-1 and take-up-2", names)
	}
}
func TestStuckShowsTheHolderAsTheAssigneeAndDoesNotListUnassignedInProgress(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	h.claimAs(id, map[string]any{"holder": takeUpHolder})
	h.todo(id, "Build it")
	h.todo(id, "Other part")
	item := stuckItem(t, h.stuck(""), id)
	if item == nil {
		t.Fatal("the claimed issue with no todo started is not listed")
	}
	if item["assignee"] != takeUpHolder {
		t.Errorf("assignee %v, want %s", item["assignee"], takeUpHolder)
	}
	if got := stuckReasons(item); got != "no_todo_started" {
		t.Errorf("reasons %q, want only no_todo_started", got)
	}
}
func TestStuckLeavesOutAHeldIssueWithAStartedTodo(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	h.claimAs(id, map[string]any{"holder": takeUpHolder})
	h.startedTodo(id)
	body := h.stuck("")
	if item := stuckItem(t, body, id); item != nil {
		t.Errorf("a held issue with a started todo and a live lease is listed: %v", item)
	}
}
func TestStuckListsUnassignedInProgressForAClaimWithoutAHolder(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Plain issue", 10)
	h.claimAs(id, nil)
	h.startedTodo(id)
	item := stuckItem(t, h.stuck(""), id)
	if item == nil || stuckReasons(item) != "unassigned_in_progress" {
		t.Fatalf("item %v, want only unassigned_in_progress", item)
	}
	if item["assignee"] != "" && item["assignee"] != nil {
		t.Errorf("assignee %v, want none", item["assignee"])
	}
}
func TestStuckListsLeaseExpiredWhenTheLeaseOfAHolderClaimHasPassed(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	h.claimAs(id, map[string]any{"holder": takeUpHolder, "lease_minutes": 5})
	h.startedTodo(id)
	if item := stuckItem(t, h.stuck(""), id); item != nil {
		t.Fatalf("an issue under a live lease is listed: %v", item)
	}
	h.expireLease(id)
	item := stuckItem(t, h.stuck(""), id)
	if item == nil || stuckReasons(item) != "lease_expired" {
		t.Fatalf("item %v, want only lease_expired", item)
	}
	if item["assignee"] != takeUpHolder {
		t.Errorf("assignee %v, want %s", item["assignee"], takeUpHolder)
	}
}
func TestHeartbeatByTheHolderOfAClaimRenewsTheLease(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	claimed := h.claimAs(id, map[string]any{"holder": takeUpHolder, "lease_minutes": 5})
	body := h.expect(http.StatusOK, "mover", "POST", "/tasks/"+id+"/heartbeat", map[string]any{"agent": takeUpHolder, "lease_minutes": 60})
	wantLeaseNear(t, body["lease_until"], 60)
	if body["agent"] != takeUpHolder || body["lease_until"] == claimed["lease_until"] {
		t.Errorf("heartbeat answered %v after the claim %v, want a renewed lease for %s", body, claimed["lease_until"], takeUpHolder)
	}
	stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
	if stored["lease_until"] != body["lease_until"] || stored["assigned_to"] != takeUpHolder {
		t.Errorf("the stored issue is %v, want the renewed lease kept by %s", stored, takeUpHolder)
	}
}
func TestHeartbeatByAnotherAgentOnAHolderClaimIsRefusedWith409(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Held issue", 10)
	claimed := h.claimAs(id, map[string]any{"holder": takeUpHolder})
	status, body := h.call("mover", "POST", "/tasks/"+id+"/heartbeat", map[string]any{"agent": "take-up-3"})
	if status != http.StatusConflict {
		t.Errorf("heartbeat by another agent = %d %v, want 409", status, body)
	}
	if stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil); stored["lease_until"] != claimed["lease_until"] {
		t.Errorf("a refused heartbeat changed the lease: %v", stored)
	}
}
func TestHeartbeatOnAClaimWithoutAHolderIsRefusedWith409BecauseItHoldsNoLease(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Plain issue", 10)
	h.claimAs(id, nil)
	status, body := h.call("mover", "POST", "/tasks/"+id+"/heartbeat", map[string]any{"agent": takeUpHolder})
	if status != http.StatusConflict {
		t.Errorf("heartbeat on a claim with no holder = %d %v, want 409", status, body)
	}
}
