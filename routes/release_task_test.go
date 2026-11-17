package routes_test

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
)

var releaseGroups = map[string][]string{"owner": {"g-owner"}, "mover": {"g-move"}, "viewer": {"g-view"}, "nobody": nil}

func releaseHub(t *testing.T) *accessHub {
	t.Helper()
	h := newAccessHub(t, releaseGroups)
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"In coming", "To do", "In Progress", "Review"}})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-move", map[string]string{"level": "move"})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-view", map[string]string{"level": "see"})
	return h
}

func releaseIssueAt(t *testing.T, h *accessHub, backlog, step, status, title string, tags ...string) string {
	t.Helper()
	made := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": title, "tags": tags})
	id := made["id"].(string)
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": backlog, "step": step})
	if status != "pending" {
		h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"status": status})
	}
	return id
}

func releaseIssue(t *testing.T, h *accessHub, title string, tags ...string) string {
	t.Helper()
	return releaseIssueAt(t, h, "software", "in-progress", "in_progress", title, tags...)
}

func releaseTodo(t *testing.T, h *accessHub, issue, title string) string {
	t.Helper()
	return h.expect(http.StatusCreated, "owner", "POST", "/todos", map[string]any{"title": title, "instructions": "do " + title, "parent_task_id": issue})["id"].(string)
}

func sortedTags(raw any) string {
	var tags []string
	if list, ok := raw.([]any); ok {
		for _, tag := range list {
			tags = append(tags, tag.(string))
		}
	}
	sort.Strings(tags)
	return strings.Join(tags, ",")
}

func assertStillInProgress(t *testing.T, h *accessHub, id, tags string) {
	t.Helper()
	got := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
	if got["step"] != "in-progress" || got["status"] != "in_progress" || sortedTags(got["tags"]) != tags {
		t.Errorf("the issue is at step %v, status %v, tags %v; want in-progress, in_progress, %s", got["step"], got["status"], got["tags"], tags)
	}
	if notes := h.notesAs("owner", "/tasks/"+id+"/notes"); len(notes) != 0 {
		t.Errorf("a refused release left entries in the Log: %v", notes)
	}
}

func TestReleaseAsBlockedAddsBlockedDropsReadyWritesTheEntryAndMovesTheIssueToToDo(t *testing.T) {
	h := releaseHub(t)
	id := releaseIssue(t, h, "Stuck", "Ready", "backend")

	body := h.expect(http.StatusOK, "mover", "POST", "/tasks/"+id+"/release", map[string]any{
		"mode": "blocked", "reason": "needs a person to choose the format", "links": []string{"commit 3f9a1c2", " "},
	})

	if sortedTags(body["tags"]) != "Blocked,backend" || body["step"] != "To do" || body["status"] != "pending" || body["mode"] != "blocked" || body["id"] != id {
		t.Errorf("the reply is %v", body)
	}
	stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
	if sortedTags(stored["tags"]) != "Blocked,backend" || stored["step"] != "to-do" || stored["status"] != "pending" {
		t.Errorf("the stored issue is %v", stored)
	}
	notes := h.notesAs("owner", "/tasks/"+id+"/notes")
	if len(notes) != 1 || notes[0]["kind"] != "worker" || notes[0]["author_id"] != "mover" || notes[0]["id"] != body["note_id"] {
		t.Fatalf("the Log is %v, want one worker entry %v", notes, body["note_id"])
	}
	text := fmt.Sprint(notes[0]["text"])
	if !strings.Contains(text, "Blocked") || !strings.Contains(text, "needs a person to choose the format") {
		t.Errorf("the entry reads %q", text)
	}
	if links := fmt.Sprint(notes[0]["links"]); links != "[commit 3f9a1c2]" {
		t.Errorf("the entry links are %s", links)
	}
}

func TestReleaseAsSinbinAddsSinbinDropsReadyAndKeepsEveryOtherTag(t *testing.T) {
	h := releaseHub(t)
	id := releaseIssue(t, h, "Too big", "Ready", "backend", "hungarian-municipality")

	body := h.expect(http.StatusOK, "mover", "POST", "/tasks/"+id+"/release", map[string]any{"mode": "sinbin", "reason": "stopped at the input cap"})

	if sortedTags(body["tags"]) != "Sinbin,backend,hungarian-municipality" || body["step"] != "To do" || body["status"] != "pending" {
		t.Errorf("the reply is %v", body)
	}
	notes := h.notesAs("owner", "/tasks/"+id+"/notes")
	if len(notes) != 1 || !strings.Contains(fmt.Sprint(notes[0]["text"]), "Sinbin") || !strings.Contains(fmt.Sprint(notes[0]["text"]), "stopped at the input cap") {
		t.Errorf("the Log is %v", notes)
	}
}

func TestReleaseOfAnIssueCarryingReadyAndWaitingLeavesOnlyTheOneMark(t *testing.T) {
	h := releaseHub(t)
	for _, c := range []struct{ mode, mark string }{{"blocked", "Blocked"}, {"sinbin", "Sinbin"}} {
		id := releaseIssue(t, h, "Marked "+c.mode, "Ready", "Waiting", "Blocked", "Sinbin", "app")

		h.expect(http.StatusOK, "mover", "POST", "/tasks/"+id+"/release", map[string]any{"mode": c.mode, "reason": "stop"})

		stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
		if got, want := sortedTags(stored["tags"]), c.mark+",app"; got != want {
			t.Errorf("after %s the tags are %s, want %s", c.mode, got, want)
		}
	}
}

func TestReleaseAsBlockedSetsTheNamedTodoBlockedWithTheReason(t *testing.T) {
	h := releaseHub(t)
	id := releaseIssue(t, h, "With todos", "Ready")
	todo := releaseTodo(t, h, id, "the todo that cannot finish")
	other := releaseTodo(t, h, id, "the untouched todo")

	body := h.expect(http.StatusOK, "mover", "POST", "/tasks/"+id+"/release", map[string]any{"mode": "blocked", "reason": "the API is down", "todo_id": todo})

	if body["todo_id"] != todo {
		t.Errorf("the reply is %v", body)
	}
	list := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id+"/todos", nil)
	for _, raw := range list["items"].([]any) {
		item := raw.(map[string]any)
		switch item["id"] {
		case todo:
			if item["state"] != "Blocked" || item["blocked_reason"] != "the API is down" {
				t.Errorf("the named todo is %v", item)
			}
		case other:
			if item["state"] == "Blocked" {
				t.Errorf("the other todo was blocked: %v", item)
			}
		}
	}
}

func TestReleaseRefusesAnIssueThatIsNotAtTheInProgressStepWhateverItsStatusAndChangesNothing(t *testing.T) {
	h := releaseHub(t)
	for _, c := range []struct{ step, status string }{{"to-do", "pending"}, {"to-do", "in_progress"}, {"review", "pending"}, {"in-coming", "pending"}} {
		id := releaseIssueAt(t, h, "software", c.step, c.status, "At "+c.step+" with "+c.status, "Ready")

		for _, mode := range []string{"blocked", "sinbin"} {
			h.expect(http.StatusConflict, "mover", "POST", "/tasks/"+id+"/release", map[string]any{"mode": mode, "reason": "x"})
		}

		stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
		if sortedTags(stored["tags"]) != "Ready" || stored["step"] != c.step || stored["status"] != c.status {
			t.Errorf("the issue at %s with %s changed: %v", c.step, c.status, stored)
		}
		if notes := h.notesAs("owner", "/tasks/"+id+"/notes"); len(notes) != 0 {
			t.Errorf("a refused release wrote %v", notes)
		}
	}
}

func TestReleaseFromTheInProgressStepSetsTheStatusBackToPendingWhateverItWas(t *testing.T) {
	h := releaseHub(t)
	for _, status := range []string{"pending", "in_progress"} {
		id := releaseIssueAt(t, h, "software", "in-progress", status, "Status "+status, "Ready")

		h.expect(http.StatusOK, "mover", "POST", "/tasks/"+id+"/release", map[string]any{"mode": "blocked", "reason": "stuck"})

		stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
		if stored["step"] != "to-do" || stored["status"] != "pending" || sortedTags(stored["tags"]) != "Blocked" {
			t.Errorf("an issue left with status %s is %v", status, stored)
		}
	}
}

func TestReleaseFindsTheInProgressStepOfTheDefaultBacklogCalledDoing(t *testing.T) {
	h := releaseHub(t)
	id := releaseIssueAt(t, h, "backlog", "doing", "in_progress", "Default backlog", "Ready")

	body := h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/release", map[string]any{"mode": "sinbin", "reason": "cap"})

	if body["step"] != "To do" || body["status"] != "pending" || sortedTags(body["tags"]) != "Sinbin" {
		t.Errorf("the reply is %v", body)
	}
	if stored := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil); stored["step"] != "to-do" || stored["status"] != "pending" {
		t.Errorf("the stored issue is %v", stored)
	}
}

func TestReleaseNeedsCanMoveOnTheBacklogAndAnUnseenIssueIsNotFound(t *testing.T) {
	h := releaseHub(t)
	id := releaseIssue(t, h, "Guarded", "Ready")

	h.expect(http.StatusForbidden, "viewer", "POST", "/tasks/"+id+"/release", map[string]any{"mode": "sinbin", "reason": "x"})
	h.expect(http.StatusNotFound, "nobody", "POST", "/tasks/"+id+"/release", map[string]any{"mode": "sinbin", "reason": "x"})
	h.expect(http.StatusNotFound, "owner", "POST", "/tasks/no-such-issue/release", map[string]any{"mode": "sinbin", "reason": "x"})

	assertStillInProgress(t, h, id, "Ready")
}

func TestReleaseRefusesAnotherModeABlankReasonAndAMisplacedTodo(t *testing.T) {
	h := releaseHub(t)
	id := releaseIssue(t, h, "Bad calls", "Ready")
	own := releaseTodo(t, h, id, "own")
	elsewhere := releaseIssue(t, h, "Elsewhere", "Ready")
	foreign := releaseTodo(t, h, elsewhere, "foreign")

	for name, body := range map[string]map[string]any{
		"no mode":          {"reason": "x"},
		"another mode":     {"mode": "later", "reason": "x"},
		"a step name":      {"mode": "Done", "reason": "x"},
		"no reason":        {"mode": "blocked"},
		"blank reason":     {"mode": "sinbin", "reason": "   "},
		"todo with sinbin": {"mode": "sinbin", "reason": "x", "todo_id": own},
		"todo of another":  {"mode": "blocked", "reason": "x", "todo_id": foreign},
	} {
		if status, answer := h.call("mover", "POST", "/tasks/"+id+"/release", body); status != http.StatusBadRequest {
			t.Errorf("%s answered %d %v, want 400", name, status, answer)
		}
	}
	h.expect(http.StatusNotFound, "mover", "POST", "/tasks/"+id+"/release", map[string]any{"mode": "blocked", "reason": "x", "todo_id": "no-such-todo"})

	assertStillInProgress(t, h, id, "Ready")
	if list := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id+"/todos", nil); list["items"].([]any)[0].(map[string]any)["state"] == "Blocked" {
		t.Errorf("a refused release blocked a todo: %v", list)
	}
}
