package routes_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type claimNextHub struct {
	*accessHub
	dsn string
}

func newClaimNextHub(t *testing.T) *claimNextHub {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "claim-next.db") + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	return &claimNextHub{accessHub: seedClaimHub(t, newAccessHubOn(t, dsn, claimGroups)), dsn: dsn}
}

func (h *claimNextHub) issueAt(step, title string, rank int, files []string, tags ...string) string {
	h.t.Helper()
	made := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": title, "tags": tags, "rank": rank, "files": files})
	id := made["id"].(string)
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": "journey-book", "step": step})
	return id
}

func (h *claimNextHub) ready(title string, rank int, files ...string) string {
	h.t.Helper()
	return h.issueAt("to-do", title, rank, files, "Ready")
}

func (h *claimNextHub) expireLease(id string) {
	h.t.Helper()
	db, err := gorm.Open(sqlite.Open(h.dsn), &gorm.Config{})
	if err != nil {
		h.t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := db.Exec("UPDATE "+claimNextTable(h.t)+" SET lease_until = ? WHERE id = ?", past, id).Error; err != nil {
		h.t.Fatal(err)
	}
}

func claimNextTable(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../spec/examples/agency.taskmanager.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["instance"] = "test"
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	s, err := mwanachamataskmanager.ParseSpec(out)
	if err != nil {
		t.Fatal(err)
	}
	object, ok := s.Object("work_item")
	if !ok {
		t.Fatal("spec declares no work_item object")
	}
	return s.TableFor(object)
}

func leaseMinutesAhead(t *testing.T, lease any) float64 {
	t.Helper()
	at, err := time.Parse(time.RFC3339, fmt.Sprint(lease))
	if err != nil {
		t.Fatalf("lease_until %v is not a time: %v", lease, err)
	}
	return time.Until(at).Minutes()
}

func wantLeaseNear(t *testing.T, lease any, minutes float64) {
	t.Helper()
	if got := leaseMinutesAhead(t, lease); got < minutes-1.5 || got > minutes+0.5 {
		t.Errorf("the lease runs out in %.1f minutes, want about %.0f", got, minutes)
	}
}

func TestClaimNextTakesTheFirstReadyIssueForTheAgentWithAThirtyMinuteLease(t *testing.T) {
	h := newClaimNextHub(t)
	first := h.ready("First", 10)
	second := h.ready("Second", 20)

	body := h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "worker-1", "backlog": "journey-book"})

	issue := body["issue"].(map[string]any)
	if issue["id"] != first || issue["status"] != "in_progress" || issue["step"] != "In Progress" {
		t.Fatalf("the claimed issue is %v, want %s in progress", issue, first)
	}
	if body["agent"] != "worker-1" {
		t.Errorf("agent %v, want worker-1", body["agent"])
	}
	wantLeaseNear(t, body["lease_until"], 30)
	stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+first, nil)
	if stored["assigned_to"] != "worker-1" || stored["assignee_kind"] != "agent" || stored["lease_until"] != body["lease_until"] || stored["step"] != "in-progress" {
		t.Errorf("the stored issue is %v", stored)
	}
	if untouched := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+second, nil); untouched["step"] != "to-do" {
		t.Errorf("the second issue moved: %v", untouched)
	}
	notes := h.notesAs("owner", "/tasks/"+first+"/notes")
	if len(notes) != 1 || notes[0]["id"] != body["note_id"] || !strings.HasPrefix(fmt.Sprint(notes[0]["text"]), "Taken by mover.") {
		t.Errorf("the Log is %v", notes)
	}
}

func TestClaimNextNamesTheCallerWhenNoAgentIsSentAndTakesTheLeaseLength(t *testing.T) {
	h := newClaimNextHub(t)
	h.ready("Only", 10)

	body := h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"lease_minutes": 5})

	if body["agent"] != "mover" {
		t.Errorf("agent %v, want the caller", body["agent"])
	}
	wantLeaseNear(t, body["lease_until"], 5)
}

func TestClaimNextSkipsAnIssueWhoseFilesClashWithOneInProgress(t *testing.T) {
	h := newClaimNextHub(t)
	h.issueAt("in-progress", "Running", 5, []string{"app/src/"})
	clashing := h.ready("Clashing", 10, "app/src/main.go")
	free := h.ready("Free", 20, "docs/")

	body := h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "w"})

	if got := body["issue"].(map[string]any)["id"]; got != free {
		t.Fatalf("claimed %v, want the issue that clashes with nothing (%s)", got, free)
	}
	status, refusal := h.call("mover", "POST", "/claim-next", map[string]any{"agent": "w"})
	if status != http.StatusConflict {
		t.Fatalf("claim with only a clashing issue left = %d %v, want 409", status, refusal)
	}
	if got := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+clashing, nil); got["step"] != "to-do" || got["status"] != "pending" {
		t.Errorf("the clashing issue changed: %v", got)
	}
}

func TestClaimNextAnswers409AndTakesNothingWhenNoIssueIsFree(t *testing.T) {
	h := newClaimNextHub(t)
	held := h.issueAt("to-do", "Sinbin", 10, nil, "Ready", "Sinbin")
	h.issueAt("to-do", "Not ready", 20, nil)

	status, body := h.call("mover", "POST", "/claim-next", map[string]any{})

	if status != http.StatusConflict || !strings.Contains(strings.ToLower(fmt.Sprint(body)), "no ready issue") {
		t.Fatalf("claim = %d %v, want 409 saying no ready issue is free", status, body)
	}
	if got := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+held, nil); got["step"] != "to-do" {
		t.Errorf("a Sinbin issue was taken: %v", got)
	}
}

func TestClaimNextByACallerWhoCannotMoveIsRefusedWith403(t *testing.T) {
	h := newClaimNextHub(t)
	id := h.ready("Visible", 10)

	status, body := h.call("viewer", "POST", "/claim-next", map[string]any{})

	if status != http.StatusForbidden {
		t.Fatalf("claim as a viewer = %d %v, want 403", status, body)
	}
	if got := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil); got["step"] != "to-do" {
		t.Errorf("a refused claim moved the issue: %v", got)
	}
}

func TestClaimNextRefusesALeaseOutsideOneMinuteToFourHours(t *testing.T) {
	h := newClaimNextHub(t)
	h.ready("Waiting", 10)
	for _, minutes := range []int{-1, 241, 100000} {
		if status, body := h.call("mover", "POST", "/claim-next", map[string]any{"lease_minutes": minutes}); status != http.StatusBadRequest {
			t.Errorf("lease_minutes %d = %d %v, want 400", minutes, status, body)
		}
	}
}

func TestClaimNextWithManyCallersAtOnceNeverGivesTwoAgentsTheSameIssue(t *testing.T) {
	h := newClaimNextHub(t)
	callers := []string{"owner", "mover", "owner", "mover"}

	for round := 0; round < 5; round++ {
		want := map[string]bool{
			h.ready(fmt.Sprintf("Race %d a", round), 10+round): true,
			h.ready(fmt.Sprintf("Race %d b", round), 10+round): true,
		}
		statuses := make([]int, len(callers))
		bodies := make([]map[string]any, len(callers))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, caller := range callers {
			wg.Add(1)
			go func(i int, caller string) {
				defer wg.Done()
				<-start
				statuses[i], bodies[i] = h.call(caller, "POST", "/claim-next", map[string]any{"agent": fmt.Sprintf("agent-%d", i)})
			}(i, caller)
		}
		close(start)
		wg.Wait()

		won := map[string]bool{}
		for i, status := range statuses {
			switch status {
			case http.StatusOK:
				id := bodies[i]["issue"].(map[string]any)["id"].(string)
				if won[id] {
					t.Fatalf("round %d: two agents took issue %s", round, id)
				}
				won[id] = true
			case http.StatusConflict:
			default:
				t.Fatalf("round %d: claim %d answered %d %v", round, i, status, bodies[i])
			}
		}
		if len(won) != 2 {
			t.Fatalf("round %d: %d issues taken from %v, want both of %v", round, len(won), statuses, want)
		}
		for id := range won {
			if !want[id] {
				t.Fatalf("round %d: took %s, which is not one of this round's", round, id)
			}
		}
	}
}

func TestAPlainClaimLeavesNoLeaseOnTheIssue(t *testing.T) {
	h := newClaimNextHub(t)
	id := h.ready("Plain", 10)

	h.expect(http.StatusOK, "mover", "POST", "/tasks/"+id+"/claim", nil)

	stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
	if lease, has := stored["lease_until"]; has && lease != "" {
		t.Errorf("a plain claim left a lease: %v", lease)
	}
}

func TestHeartbeatRenewsTheLeaseOfTheAgentThatHoldsIt(t *testing.T) {
	h := newClaimNextHub(t)
	id := h.ready("Held", 10)
	h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "w1", "lease_minutes": 5})

	body := h.expect(http.StatusOK, "mover", "POST", "/tasks/"+id+"/heartbeat", map[string]any{"agent": "w1", "lease_minutes": 60})

	wantLeaseNear(t, body["lease_until"], 60)
	stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
	if stored["lease_until"] != body["lease_until"] || stored["step"] != "in-progress" {
		t.Errorf("the stored issue is %v, want the renewed lease", stored)
	}
}

func TestHeartbeatByAnotherAgentOrOnAnIssueNobodyHoldsIsRefusedWith409(t *testing.T) {
	h := newClaimNextHub(t)
	held := h.ready("Held", 10)
	idle := h.ready("Idle", 20)
	claimed := h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "w1", "lease_minutes": 5})

	if status, body := h.call("mover", "POST", "/tasks/"+held+"/heartbeat", map[string]any{"agent": "w2", "lease_minutes": 60}); status != http.StatusConflict {
		t.Errorf("heartbeat by another agent = %d %v, want 409", status, body)
	}
	if status, body := h.call("mover", "POST", "/tasks/"+idle+"/heartbeat", map[string]any{"agent": "w1"}); status != http.StatusConflict {
		t.Errorf("heartbeat on an issue nobody holds = %d %v, want 409", status, body)
	}
	if stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+held, nil); stored["lease_until"] != claimed["lease_until"] {
		t.Errorf("a refused heartbeat changed the lease: %v", stored)
	}
}

func TestHeartbeatByACallerWhoCannotMoveIsRefusedWith403(t *testing.T) {
	h := newClaimNextHub(t)
	id := h.ready("Held", 10)
	h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "w1"})

	if status, body := h.call("viewer", "POST", "/tasks/"+id+"/heartbeat", map[string]any{"agent": "w1"}); status != http.StatusForbidden {
		t.Errorf("heartbeat as a viewer = %d %v, want 403", status, body)
	}
}

func TestHeartbeatAfterTheLeaseRanOutReleasesTheIssueToToDoAndLogsWhy(t *testing.T) {
	h := newClaimNextHub(t)
	id := h.ready("Abandoned", 10)
	h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "w1", "lease_minutes": 5})
	h.expireLease(id)

	status, body := h.call("mover", "POST", "/tasks/"+id+"/heartbeat", map[string]any{"agent": "w1"})

	if status != http.StatusConflict || !strings.Contains(strings.ToLower(fmt.Sprint(body)), "ran out") {
		t.Fatalf("heartbeat after expiry = %d %v, want 409 saying the lease ran out", status, body)
	}
	stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
	if stored["step"] != "to-do" || stored["status"] != "pending" || stored["assigned_to"] != nil && stored["assigned_to"] != "" {
		t.Errorf("the issue is %v, want it back at To do, pending and unassigned", stored)
	}
	logged := false
	for _, note := range h.notesAs("owner", "/tasks/"+id+"/notes") {
		if strings.Contains(fmt.Sprint(note["text"]), "lease of w1 ran out") {
			logged = true
		}
	}
	if !logged {
		t.Errorf("no Log entry says the lease ran out: %v", h.notesAs("owner", "/tasks/"+id+"/notes"))
	}
}

func TestAnIssueWhoseLeaseRanOutIsClaimableAgainByAnotherAgent(t *testing.T) {
	h := newClaimNextHub(t)
	id := h.ready("Abandoned", 10)
	h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "w1", "lease_minutes": 5})
	h.expireLease(id)

	body := h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "w2"})

	if got := body["issue"].(map[string]any)["id"]; got != id || body["agent"] != "w2" {
		t.Fatalf("claim after expiry = %v, want %s taken by w2", body, id)
	}
	if status, refusal := h.call("mover", "POST", "/tasks/"+id+"/heartbeat", map[string]any{"agent": "w1"}); status != http.StatusConflict {
		t.Errorf("the first agent's heartbeat after takeover = %d %v, want 409", status, refusal)
	}
	wantLeaseNear(t, h.expect(http.StatusOK, "mover", "POST", "/tasks/"+id+"/heartbeat", map[string]any{"agent": "w2", "lease_minutes": 10})["lease_until"], 10)
}

func TestAnExpiredLeaseOnAnIssueThatLeftInProgressIsNotReleased(t *testing.T) {
	h := newClaimNextHub(t)
	id := h.ready("Finished", 10)
	h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "w1", "lease_minutes": 5})
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": "journey-book", "step": "review"})
	h.expireLease(id)
	h.ready("Another", 20)

	h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "w2"})

	if got := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil); got["step"] != "review" {
		t.Errorf("an issue at Review was pulled back: %v", got)
	}
}
