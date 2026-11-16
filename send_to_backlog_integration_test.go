package mwanachamataskmanager_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

const sendPersonHeader = "X-Send-Person"

type sendPersonKey struct{}

type sendHub struct {
	t       *testing.T
	mux     *http.ServeMux
	mgr     mwanachamataskmanager.TaskManager
	backlog map[string]string
}

func newSendHub(t *testing.T) *sendHub {
	t.Helper()
	mgr := newTestManager(t)
	byPerson := map[string][]string{
		"owner":  {ownerGroup},
		"ann":    {devGroup},
		"bea":    {docsGroup},
		"nobody": nil,
	}
	mgr.UseGroups(func(_ context.Context, id string) ([]string, error) { return byPerson[id], nil })
	mgr.UseGroupExists(knownGroups(ownerGroup, devGroup, docsGroup))
	built, err := routes.BuildFor(mgr, routes.Mount{Caller: func(ctx context.Context) string {
		id, _ := ctx.Value(sendPersonKey{}).(string)
		return id
	}})
	if err != nil {
		t.Fatalf("BuildFor: %v", err)
	}
	mux := http.NewServeMux()
	for _, route := range built {
		mux.HandleFunc(route.Pattern(""), route.Handler)
	}
	hub := &sendHub{t: t, mux: mux, mgr: mgr, backlog: map[string]string{}}
	if err := mgr.SetBacklogOwnerGroup(context.Background(), ownerGroup); err != nil {
		t.Fatalf("SetBacklogOwnerGroup: %v", err)
	}
	hub.addBacklog("software", "Software", []string{"Open", "Shipped"})
	hub.addBacklog("documentation", "Documentation", []string{"Draft", "Final"})
	hub.addBacklog("private", "Private", []string{"Idea", "Done"})
	return hub
}

func (h *sendHub) addBacklog(key, name string, steps []string) {
	h.t.Helper()
	made := h.expect(http.StatusCreated, "owner", http.MethodPost, "/backlogs", map[string]any{"name": name, "steps": steps})
	h.backlog[key] = made["id"].(string)
}

func (h *sendHub) call(person, method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	raw := []byte(nil)
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			h.t.Fatalf("marshal %s %s: %v", method, path, err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), sendPersonKey{}, person))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

func (h *sendHub) expect(want int, person, method, path string, body any) map[string]any {
	h.t.Helper()
	code, out := h.call(person, method, path, body)
	if code != want {
		h.t.Fatalf("%s %s as %q = %d %v, want %d", method, path, person, code, out, want)
	}
	return out
}

func (h *sendHub) grant(key, group, level string) {
	h.t.Helper()
	h.expect(http.StatusOK, "owner", http.MethodPut, "/backlogs/"+h.backlog[key]+"/access/"+group, map[string]string{"level": level})
}

func (h *sendHub) issueAt(key, step, title string) string {
	h.t.Helper()
	made := h.expect(http.StatusCreated, "owner", http.MethodPost, "/tasks", map[string]any{"title": title, "backlog": h.backlog[key], "step": step})
	return made["id"].(string)
}

func (h *sendHub) firstStepOf(key string) string {
	h.t.Helper()
	list := h.expect(http.StatusOK, "owner", http.MethodGet, "/backlogs", nil)
	for _, item := range list["items"].([]any) {
		backlog := item.(map[string]any)
		if backlog["id"] == h.backlog[key] {
			return backlog["steps"].([]any)[0].(map[string]any)["id"].(string)
		}
	}
	h.t.Fatalf("backlog %q not listed", key)
	return ""
}

func (h *sendHub) send(person, issue, key string) int {
	h.t.Helper()
	code, _ := h.call(person, http.MethodPost, "/tasks/"+issue+"/move-to-step", map[string]any{"backlog": h.backlog[key], "step": h.firstStepOf(key)})
	return code
}

func (h *sendHub) placeOf(issue string) (string, string) {
	h.t.Helper()
	got := h.expect(http.StatusOK, "owner", http.MethodGet, "/tasks/"+issue, nil)
	backlog, _ := got["backlog"].(string)
	step, _ := got["step"].(string)
	return backlog, step
}

func (h *sendHub) holds(key, step string) int {
	h.t.Helper()
	list := h.expect(http.StatusOK, "owner", http.MethodGet, "/backlogs", nil)
	for _, item := range list["items"].([]any) {
		backlog := item.(map[string]any)
		if backlog["id"] != h.backlog[key] {
			continue
		}
		for _, s := range backlog["steps"].([]any) {
			stepRow := s.(map[string]any)
			if stepRow["id"] == step {
				return int(stepRow["issues"].(float64))
			}
		}
	}
	h.t.Fatalf("step %q of %q not listed", step, key)
	return 0
}

func TestSendToBacklog_TheIssueLeavesItsBacklogAndLandsAtTheFirstStepOfTheTarget(t *testing.T) {
	h := newSendHub(t)
	h.grant("software", devGroup, "move")
	h.grant("documentation", devGroup, "move")
	issue := h.issueAt("software", "open", "write the guide")

	if code := h.send("ann", issue, "documentation"); code != http.StatusOK {
		t.Fatalf("send with Can move on both = %d, want 200", code)
	}

	backlog, step := h.placeOf(issue)
	if backlog != h.backlog["documentation"] || step != h.firstStepOf("documentation") {
		t.Fatalf("issue is at %s/%s, want %s/%s", backlog, step, h.backlog["documentation"], h.firstStepOf("documentation"))
	}
	if n := h.holds("software", "open"); n != 0 {
		t.Fatalf("software/open still holds %d after the send", n)
	}
	if n := h.holds("documentation", h.firstStepOf("documentation")); n != 1 {
		t.Fatalf("documentation's first step holds %d, want 1", n)
	}
}

func TestSendToBacklog_CanSeeOnTheTargetIsRefusedAndTheIssueStaysPut(t *testing.T) {
	h := newSendHub(t)
	h.grant("software", devGroup, "move")
	h.grant("documentation", devGroup, "see")
	issue := h.issueAt("software", "open", "stay in software")

	if code := h.send("ann", issue, "documentation"); code != http.StatusForbidden {
		t.Fatalf("send with Can see on the target = %d, want 403", code)
	}
	if backlog, step := h.placeOf(issue); backlog != h.backlog["software"] || step != "open" {
		t.Fatalf("a refused send moved the issue to %s/%s", backlog, step)
	}

	h.grant("documentation", devGroup, "move")
	if code := h.send("ann", issue, "documentation"); code != http.StatusOK {
		t.Fatalf("the same send once Can move is given = %d, want 200", code)
	}
}

func TestSendToBacklog_CanMoveOnTheTargetAloneIsNotEnoughWhenTheSourceIsOutOfReach(t *testing.T) {
	h := newSendHub(t)
	h.grant("documentation", docsGroup, "move")
	issue := h.issueAt("software", "open", "not bea's to move")

	if code := h.send("bea", issue, "documentation"); code != http.StatusForbidden {
		t.Fatalf("send from a backlog the person has no access to = %d, want 403", code)
	}
	if backlog, _ := h.placeOf(issue); backlog != h.backlog["software"] {
		t.Fatalf("the issue left software for %s", backlog)
	}

	h.grant("software", docsGroup, "see")
	if code := h.send("bea", issue, "documentation"); code != http.StatusForbidden {
		t.Fatalf("send from a backlog the person can only see = %d, want 403", code)
	}

	h.grant("software", docsGroup, "move")
	if code := h.send("bea", issue, "documentation"); code != http.StatusOK {
		t.Fatalf("send once Can move is on both = %d, want 200", code)
	}
}

func TestSendToBacklog_ABacklogThePersonCannotSeeIsNotATarget(t *testing.T) {
	h := newSendHub(t)
	h.grant("software", devGroup, "move")
	issue := h.issueAt("software", "open", "aim at a hidden backlog")

	if code := h.send("ann", issue, "private"); code != http.StatusForbidden {
		t.Fatalf("send to a hidden backlog = %d, want 403", code)
	}
	if code := h.send("nobody", issue, "documentation"); code != http.StatusForbidden {
		t.Fatalf("send by a person in no group = %d, want 403", code)
	}
	if backlog, _ := h.placeOf(issue); backlog != h.backlog["software"] {
		t.Fatalf("a refused send left the issue at %s", backlog)
	}
}

func TestSendToBacklog_SeveralIssuesAreSentOneCallEachAndARefusedOneDoesNotUndoTheOthers(t *testing.T) {
	h := newSendHub(t)
	h.grant("software", devGroup, "move")
	h.grant("documentation", devGroup, "move")
	first := h.issueAt("software", "open", "first")
	hidden := h.issueAt("private", "idea", "in a backlog ann cannot reach")
	last := h.issueAt("software", "open", "last")

	codes := []int{h.send("ann", first, "documentation"), h.send("ann", hidden, "documentation"), h.send("ann", last, "documentation")}

	want := []int{http.StatusOK, http.StatusForbidden, http.StatusOK}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("send %d = %d, want %d (all: %v)", i+1, codes[i], want[i], codes)
		}
	}
	docs := h.backlog["documentation"]
	if backlog, _ := h.placeOf(first); backlog != docs {
		t.Fatalf("the first issue is at %s, want documentation", backlog)
	}
	if backlog, _ := h.placeOf(last); backlog != docs {
		t.Fatalf("the issue sent after the refused one is at %s, want documentation", backlog)
	}
	if backlog, step := h.placeOf(hidden); backlog != h.backlog["private"] || step != "idea" {
		t.Fatalf("the refused issue is at %s/%s, want private/idea", backlog, step)
	}
	if n := h.holds("documentation", h.firstStepOf("documentation")); n != 2 {
		t.Fatalf("documentation's first step holds %d, want 2", n)
	}
}

func TestSendToBacklog_AnUnknownTargetOrStepIsRefusedAndTheIssueStaysPut(t *testing.T) {
	h := newSendHub(t)
	issue := h.issueAt("software", "open", "stay put")

	unknownBacklog := map[string]any{"backlog": "no-such-backlog", "step": "draft"}
	unknownStep := map[string]any{"backlog": h.backlog["documentation"], "step": "no-such-step"}
	path := "/tasks/" + issue + "/move-to-step"

	if code, _ := h.call("owner", http.MethodPost, path, unknownBacklog); code != http.StatusForbidden {
		t.Fatalf("a person sending to a backlog that does not exist = %d, want 403 (it is not one they can move in)", code)
	}
	if code, _ := h.call("", http.MethodPost, path, unknownBacklog); code != http.StatusBadRequest {
		t.Fatalf("an unnarrowed call to a backlog that does not exist = %d, want 400", code)
	}
	if code, _ := h.call("owner", http.MethodPost, path, unknownStep); code != http.StatusBadRequest {
		t.Fatalf("send to a step that does not exist = %d, want 400", code)
	}
	if backlog, step := h.placeOf(issue); backlog != h.backlog["software"] || step != "open" {
		t.Fatalf("a refused send moved the issue to %s/%s", backlog, step)
	}
}
