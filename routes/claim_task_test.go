package routes_test

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

var claimGroups = map[string][]string{"owner": {"g-owner"}, "mover": {"g-move"}, "viewer": {"g-view"}, "nobody": nil}

func seedClaimHub(t *testing.T, hub *accessHub) *accessHub {
	t.Helper()
	hub.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	hub.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Journey Book", "steps": []string{"In coming", "To do", "In Progress", "Review", "Done"}})
	hub.expect(http.StatusOK, "owner", "PUT", "/backlogs/journey-book/access/g-move", map[string]string{"level": "move"})
	hub.expect(http.StatusOK, "owner", "PUT", "/backlogs/journey-book/access/g-view", map[string]string{"level": "see"})
	return hub
}

func claimHub(t *testing.T) *accessHub {
	t.Helper()
	return seedClaimHub(t, newAccessHub(t, claimGroups))
}

func claimIssue(t *testing.T, h *accessHub, backlog, step, title string, tags ...string) string {
	t.Helper()
	made := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": title, "tags": tags})
	id := made["id"].(string)
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": backlog, "step": step})
	return id
}

func claimRefusal(t *testing.T, h *accessHub, caller, id string, want int, reason string) {
	t.Helper()
	status, body := h.call(caller, "POST", "/tasks/"+id+"/claim", nil)
	if status != want {
		t.Fatalf("claim as %q = %d %v, want %d", caller, status, body, want)
	}
	if reason != "" && !strings.Contains(strings.ToLower(fmt.Sprint(body)), strings.ToLower(reason)) {
		t.Errorf("the refusal %v does not name %q", body, reason)
	}
}

func assertUntouched(t *testing.T, h *accessHub, id, step string) {
	t.Helper()
	got := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
	if got["step"] != step || got["status"] != "pending" {
		t.Errorf("the issue is at step %v with status %v, want %s and pending", got["step"], got["status"], step)
	}
	for _, note := range h.notesAs("owner", "/tasks/"+id+"/notes") {
		if strings.HasPrefix(fmt.Sprint(note["text"]), "Taken by") {
			t.Errorf("a refused claim left an entry: %v", note)
		}
	}
}

func TestClaimMovesTheIssueToInProgressWritesTheTakenEntryAndReturnsTheCaps(t *testing.T) {
	h := claimHub(t)
	id := claimIssue(t, h, "journey-book", "to-do", "Take me", "Ready", "backend")

	body := h.expect(http.StatusOK, "mover", "POST", "/tasks/"+id+"/claim", nil)

	issue := body["issue"].(map[string]any)
	for key, want := range map[string]any{"id": id, "title": "Take me", "status": "in_progress", "backlog": "Journey Book", "step": "In Progress"} {
		if issue[key] != want {
			t.Errorf("issue %s is %v, want %v (%v)", key, issue[key], want, issue)
		}
	}
	if _, has := issue["description"]; has {
		t.Errorf("the issue is not compact: %v", issue)
	}
	caps := body["caps"].(map[string]any)
	if caps["input_tokens"] != float64(500000) || caps["output_tokens"] != float64(100000) || caps["runtime_seconds"] != float64(900) {
		t.Errorf("caps %v, want the default", caps)
	}
	stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
	if stored["step"] != "in-progress" || stored["status"] != "in_progress" {
		t.Errorf("the stored issue is %v", stored)
	}
	notes := h.notesAs("owner", "/tasks/"+id+"/notes")
	if len(notes) != 1 || notes[0]["id"] != body["note_id"] || notes[0]["kind"] != "worker" || notes[0]["author_id"] != "mover" {
		t.Fatalf("the Log is %v, want one worker entry %v", notes, body["note_id"])
	}
	if text := notes[0]["text"]; text != "Taken by mover. Caps in force: 500000 input, 100000 output, 15 minutes" {
		t.Errorf("the entry reads %q", text)
	}
}

func TestClaimWritesTheIssuesOwnCapsWhenItHasThem(t *testing.T) {
	h := claimHub(t)
	id := claimIssue(t, h, "journey-book", "to-do", "Own caps", "Ready")
	h.expect(http.StatusOK, "owner", "PUT", "/tasks/"+id+"/caps", map[string]any{"input_tokens": 2000, "runtime_seconds": 120})

	body := h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/claim", nil)

	caps := body["caps"].(map[string]any)
	if caps["input_tokens"] != float64(2000) || caps["output_tokens"] != float64(100000) || caps["runtime_seconds"] != float64(120) {
		t.Errorf("caps %v", caps)
	}
	notes := h.notesAs("owner", "/tasks/"+id+"/notes")
	if text := notes[0]["text"]; text != "Taken by owner. Caps in force: 2000 input, 100000 output, 2 minutes" {
		t.Errorf("the entry reads %q", text)
	}
}

func TestClaimFindsTheInProgressStepOfABacklogWhoseStepIdIsDoing(t *testing.T) {
	h := claimHub(t)
	id := claimIssue(t, h, "backlog", "to-do", "Default backlog", "Ready")

	body := h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/claim", nil)

	issue := body["issue"].(map[string]any)
	if issue["step"] != "In Progress" || issue["status"] != "in_progress" {
		t.Errorf("the issue is %v, want it at In Progress", issue)
	}
	if stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil); stored["step"] != "doing" {
		t.Errorf("the stored step is %v", stored["step"])
	}
}

func TestClaimRefusesAnIssueTaggedSinbinWaitingOrBlockedWithTheReason(t *testing.T) {
	h := claimHub(t)
	for _, mark := range []string{"Sinbin", "Waiting", "Blocked"} {
		id := claimIssue(t, h, "journey-book", "to-do", "Marked "+mark, mark)

		claimRefusal(t, h, "owner", id, http.StatusConflict, mark)

		assertUntouched(t, h, id, "to-do")
	}
}

func TestClaimRefusesAnIssueThatIsAlreadyInProgress(t *testing.T) {
	h := claimHub(t)
	id := claimIssue(t, h, "journey-book", "to-do", "Taken twice", "Ready")
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/claim", nil)

	claimRefusal(t, h, "mover", id, http.StatusConflict, "already in progress")

	if notes := h.notesAs("owner", "/tasks/"+id+"/notes"); len(notes) != 1 {
		t.Errorf("the Log holds %d entries, want the first claim's one", len(notes))
	}
}

func TestClaimRefusesAnIssueThatSitsInProgressWithoutHavingBeenClaimed(t *testing.T) {
	h := claimHub(t)
	id := claimIssue(t, h, "journey-book", "in-progress", "Moved by hand", "Ready")

	claimRefusal(t, h, "owner", id, http.StatusConflict, "already in progress")
}

func TestClaimNeedsCanMoveOnTheBacklogAndAnUnseenIssueIsNotFound(t *testing.T) {
	h := claimHub(t)
	id := claimIssue(t, h, "journey-book", "to-do", "Guarded", "Ready")

	claimRefusal(t, h, "viewer", id, http.StatusForbidden, "")
	claimRefusal(t, h, "nobody", id, http.StatusNotFound, "")
	claimRefusal(t, h, "owner", "no-such-issue", http.StatusNotFound, "")

	assertUntouched(t, h, id, "to-do")
}

func TestClaimIsAnMCPToolNamedByItsAction(t *testing.T) {
	for _, tool := range routes.ToolShape() {
		if tool.Action != "taskmanager.task.claim" {
			continue
		}
		if tool.Name != "taskmanager_task_claim" {
			t.Fatalf("the tool is named %q", tool.Name)
		}
		if strings.Contains(string(tool.InputSchema), "requester_id") {
			t.Errorf("the caller is asked for in the input: %s", tool.InputSchema)
		}
		return
	}
	t.Fatal("no tool for taskmanager.task.claim")
}

func TestTwoClaimsAtOnceGiveExactlyOneWinnerAndTheOtherTheAlreadyInProgressRefusal(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "claims.db") + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	h := seedClaimHub(t, newAccessHubOn(t, dsn, claimGroups))
	claimers := []string{"owner", "mover"}

	for round := 0; round < 8; round++ {
		id := claimIssue(t, h, "journey-book", "to-do", fmt.Sprintf("Race %d", round), "Ready")
		statuses := make([]int, len(claimers))
		bodies := make([]map[string]any, len(claimers))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, caller := range claimers {
			wg.Add(1)
			go func(i int, caller string) {
				defer wg.Done()
				<-start
				statuses[i], bodies[i] = h.call(caller, "POST", "/tasks/"+id+"/claim", nil)
			}(i, caller)
		}
		close(start)
		wg.Wait()

		winners, refused := 0, 0
		for i, status := range statuses {
			switch status {
			case http.StatusOK:
				winners++
			case http.StatusConflict:
				refused++
				if !strings.Contains(strings.ToLower(fmt.Sprint(bodies[i])), "already in progress") {
					t.Errorf("round %d: the loser was told %v", round, bodies[i])
				}
			default:
				t.Errorf("round %d: claim %d answered %d %v", round, i, status, bodies[i])
			}
		}
		if winners != 1 || refused != 1 {
			t.Fatalf("round %d: %d winners and %d refused from %v, want one each", round, winners, refused, statuses)
		}
		taken := 0
		for _, note := range h.notesAs("owner", "/tasks/"+id+"/notes") {
			if strings.HasPrefix(fmt.Sprint(note["text"]), "Taken by") {
				taken++
			}
		}
		if taken != 1 {
			t.Errorf("round %d: the Log holds %d taken entries, want 1", round, taken)
		}
	}
}
