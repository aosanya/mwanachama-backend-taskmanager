package routes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setStepLimit(h *accessHub, caller string, want int, backlog, step string, limit any) map[string]any {
	h.t.Helper()
	return h.expect(want, caller, "PUT", "/backlogs/"+backlog+"/steps/"+step+"/wip-limit", map[string]any{"wip_limit": limit})
}

func issueAtStep(h *accessHub, backlog, step, title string) string {
	h.t.Helper()
	return h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": title, "backlog": backlog, "step": step})["id"].(string)
}

func moveAs(h *accessHub, caller string, want int, id, backlog, step string, extra map[string]any) map[string]any {
	h.t.Helper()
	body := map[string]any{"backlog": backlog, "step": step}
	for k, v := range extra {
		body[k] = v
	}
	return h.expect(want, caller, "POST", "/tasks/"+id+"/move-to-step", body)
}

func stepOf(list map[string]any, backlog, step string) map[string]any {
	for _, raw := range list["items"].([]any) {
		b := raw.(map[string]any)
		if b["id"] != backlog {
			continue
		}
		for _, s := range b["steps"].([]any) {
			if s.(map[string]any)["id"] == step {
				return s.(map[string]any)
			}
		}
	}
	return nil
}

func TestOnlyCanEditSetsAStepLimitAndTheBacklogListShowsIt(t *testing.T) {
	h, docs, _ := newEditHub(t)
	for _, who := range []string{"ann", "mia", "nog"} {
		setStepLimit(h, who, http.StatusForbidden, docs, "write", 2)
	}
	summary := setStepLimit(h, "eli", http.StatusOK, docs, "write", 2)
	var seen any
	for _, s := range summary["steps"].([]any) {
		if s.(map[string]any)["id"] == "write" {
			seen = s.(map[string]any)["wip_limit"]
		}
	}
	if seen != float64(2) {
		t.Fatalf("the summary says the limit is %v, want 2: %v", seen, summary)
	}
	listed := stepOf(h.expect(http.StatusOK, "boss", "GET", "/backlogs", nil), docs, "write")
	if listed["wip_limit"] != float64(2) {
		t.Fatalf("the backlog list says %v, want a limit of 2", listed)
	}
	if other := stepOf(h.expect(http.StatusOK, "boss", "GET", "/backlogs", nil), docs, "done"); other["wip_limit"] != nil {
		t.Fatalf("a step nobody limited carries %v", other["wip_limit"])
	}
}

func TestAStepLimitIsAWholeNumberAndZeroClearsIt(t *testing.T) {
	h, docs, _ := newEditHub(t)
	setStepLimit(h, "boss", http.StatusBadRequest, docs, "write", -1)
	setStepLimit(h, "boss", http.StatusBadRequest, docs, "write", "many")
	setStepLimit(h, "boss", http.StatusBadRequest, docs, "write", nil)
	setStepLimit(h, "boss", http.StatusBadRequest, docs, "nowhere", 1)
	setStepLimit(h, "boss", http.StatusOK, docs, "write", 1)
	setStepLimit(h, "boss", http.StatusOK, docs, "write", 0)
	if listed := stepOf(h.expect(http.StatusOK, "boss", "GET", "/backlogs", nil), docs, "write"); listed["wip_limit"] != nil {
		t.Fatalf("zero left the limit %v in place", listed["wip_limit"])
	}
	first, second := issueAtStep(h, docs, "write", "a"), issueAtStep(h, docs, "write", "b")
	moveAs(h, "boss", http.StatusOK, first, docs, "done", nil)
	moveAs(h, "boss", http.StatusOK, second, docs, "done", nil)
}

func TestAMoveIntoAFullStepIsRefusedWithTheLimitNamed(t *testing.T) {
	h, docs, _ := newEditHub(t)
	first, second := issueAtStep(h, docs, "write", "a"), issueAtStep(h, docs, "write", "b")
	setStepLimit(h, "boss", http.StatusOK, docs, "done", 1)
	moveAs(h, "mia", http.StatusOK, first, docs, "done", nil)

	refused := moveAs(h, "mia", http.StatusConflict, second, docs, "done", nil)
	if text := strings.ToLower(fmt.Sprint(refused)); !strings.Contains(text, "limit") || !strings.Contains(text, "1") || !strings.Contains(text, "done") {
		t.Fatalf("the refusal %v does not name the step and its limit of 1", refused)
	}
	if got := h.expect(http.StatusOK, "boss", "GET", "/tasks/"+second, nil); got["step"] != "write" {
		t.Fatalf("a refused move left the issue at %v", got["step"])
	}
	moveAs(h, "mia", http.StatusOK, first, docs, "done", nil)
}

func TestOnlyCanEditOverridesAFullStep(t *testing.T) {
	h, docs, _ := newEditHub(t)
	first, second, third := issueAtStep(h, docs, "write", "a"), issueAtStep(h, docs, "write", "b"), issueAtStep(h, docs, "write", "c")
	setStepLimit(h, "boss", http.StatusOK, docs, "done", 1)
	moveAs(h, "mia", http.StatusOK, first, docs, "done", nil)

	moveAs(h, "mia", http.StatusForbidden, second, docs, "done", map[string]any{"override": true})
	moveAs(h, "ann", http.StatusForbidden, second, docs, "done", map[string]any{"override": true})
	moveAs(h, "eli", http.StatusOK, second, docs, "done", map[string]any{"override": true})
	if got := h.expect(http.StatusOK, "boss", "GET", "/tasks/"+second, nil); got["step"] != "done" {
		t.Fatalf("an override left the issue at %v", got["step"])
	}
	moveAs(h, "eli", http.StatusConflict, third, docs, "done", map[string]any{"override": false})
	moveAs(h, "mia", http.StatusOK, third, docs, "write", map[string]any{"override": true})
}

func TestLoweringALimitBelowTheCountRemovesNothingAndRefusesNewMoves(t *testing.T) {
	h, docs, _ := newEditHub(t)
	ids := []string{issueAtStep(h, docs, "write", "a"), issueAtStep(h, docs, "write", "b"), issueAtStep(h, docs, "write", "c"), issueAtStep(h, docs, "write", "d")}
	for _, id := range ids[:3] {
		moveAs(h, "boss", http.StatusOK, id, docs, "done", nil)
	}
	setStepLimit(h, "boss", http.StatusOK, docs, "done", 1)
	if held := stepOf(h.expect(http.StatusOK, "boss", "GET", "/backlogs", nil), docs, "done")["issues"]; held != float64(3) {
		t.Fatalf("lowering the limit changed the count to %v", held)
	}
	moveAs(h, "mia", http.StatusConflict, ids[3], docs, "done", nil)
	moveAs(h, "boss", http.StatusOK, ids[0], docs, "write", nil)
	moveAs(h, "boss", http.StatusOK, ids[1], docs, "write", nil)
	moveAs(h, "mia", http.StatusConflict, ids[3], docs, "done", nil)
	moveAs(h, "boss", http.StatusOK, ids[2], docs, "write", nil)
	moveAs(h, "mia", http.StatusOK, ids[3], docs, "done", nil)
}

func TestAnIssueAlreadyAtAFullStepCanBeMovedThereAgain(t *testing.T) {
	h, docs, _ := newEditHub(t)
	id := issueAtStep(h, docs, "write", "a")
	moveAs(h, "mia", http.StatusOK, id, docs, "done", nil)
	setStepLimit(h, "boss", http.StatusOK, docs, "done", 1)
	moveAs(h, "mia", http.StatusOK, id, docs, "done", nil)
}

func TestATakeIntoAFullInProgressStepIsRefusedTheSameWay(t *testing.T) {
	h := newClaimNextHub(t)
	first, second, third := h.ready("First", 10), h.ready("Second", 20), h.ready("Third", 30)
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/journey-book/steps/in-progress/wip-limit", map[string]any{"wip_limit": 1})

	h.expect(http.StatusOK, "mover", "POST", "/tasks/"+first+"/claim", nil)
	status, body := h.call("mover", "POST", "/tasks/"+second+"/claim", nil)
	if status != http.StatusConflict || !strings.Contains(strings.ToLower(fmt.Sprint(body)), "limit") {
		t.Fatalf("a take into a full step = %d %v, want 409 naming the limit", status, body)
	}
	status, body = h.call("mover", "POST", "/claim-next", map[string]any{"agent": "worker-1", "backlog": "journey-book"})
	if status != http.StatusConflict {
		t.Fatalf("claim_next into a full step = %d %v, want 409", status, body)
	}
	for _, id := range []string{second, third} {
		if got := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil); got["status"] != "pending" {
			t.Errorf("a refused take moved %s to %v", id, got["status"])
		}
	}
}

type stepClockHub struct {
	*accessHub
	db *gorm.DB
}

func newStepClockHub(t *testing.T) (*stepClockHub, string) {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "step-clock.db") + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	hub := newAccessHubOn(t, dsn, map[string][]string{"boss": {"owners"}, "mia": {"movers"}})
	hub.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	docs := hub.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Docs", "steps": []string{"Write", "Done"}})["id"].(string)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return &stepClockHub{accessHub: hub, db: db}, docs
}

func (h *stepClockHub) backdate(id string, entered, updated time.Time) {
	h.t.Helper()
	changes := map[string]any{"step_entered_at": "", "updated_at": updated.UTC().Format(time.RFC3339)}
	if !entered.IsZero() {
		changes["step_entered_at"] = entered.UTC().Format(time.RFC3339)
	}
	if err := h.db.Table(claimNextTable(h.t)).Where("id = ?", id).Updates(changes).Error; err != nil {
		h.t.Fatal(err)
	}
}

func (h *accessHub) listedIDs(caller, path string) []string {
	h.t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req = req.WithContext(context.WithValue(req.Context(), callerKey{}, caller))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body.String())
	}
	var tasks []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &tasks); err != nil {
		h.t.Fatalf("GET %s body %s: %v", path, rec.Body.String(), err)
	}
	var ids []string
	for _, task := range tasks {
		ids = append(ids, task["id"].(string))
	}
	return ids
}

func itemIDs(list map[string]any) []string {
	var ids []string
	for _, raw := range list["items"].([]any) {
		ids = append(ids, raw.(map[string]any)["id"].(string))
	}
	return ids
}

func TestAnIssueReadsWhenItEnteredItsStepAndHowManyDaysAgo(t *testing.T) {
	h, docs := newStepClockHub(t)
	id := issueAtStep(h.accessHub, docs, "write", "a")
	moveAs(h.accessHub, "boss", http.StatusOK, id, docs, "done", nil)

	fresh := h.expect(http.StatusOK, "boss", "GET", "/tasks/"+id, nil)
	entered, err := time.Parse(time.RFC3339, fmt.Sprint(fresh["entered_step_at"]))
	if err != nil || time.Since(entered) > time.Minute {
		t.Fatalf("entered_step_at %v is not the moment of the move: %v", fresh["entered_step_at"], err)
	}
	if fresh["days_in_step"] != float64(0) {
		t.Fatalf("days_in_step %v, want 0 for a move just made", fresh["days_in_step"])
	}

	now := time.Now()
	h.backdate(id, now.Add(-73*time.Hour), now)
	aged := h.expect(http.StatusOK, "boss", "GET", "/tasks/"+id, nil)
	if aged["days_in_step"] != float64(3) {
		t.Fatalf("days_in_step %v, want 3 after 73 hours", aged["days_in_step"])
	}
}

func TestDaysInStepCountFromTheLastMoveNotFromTheLastEdit(t *testing.T) {
	h, docs := newStepClockHub(t)
	id := issueAtStep(h.accessHub, docs, "write", "a")
	moveAs(h.accessHub, "boss", http.StatusOK, id, docs, "done", nil)
	now := time.Now()
	h.backdate(id, now.Add(-5*24*time.Hour), now.Add(-5*24*time.Hour))

	h.expect(http.StatusOK, "boss", "PATCH", "/tasks/"+id, map[string]any{"priority": "high"})
	if got := h.expect(http.StatusOK, "boss", "GET", "/tasks/"+id, nil); got["days_in_step"] != float64(5) {
		t.Fatalf("an edit changed days_in_step to %v, want 5", got["days_in_step"])
	}

	moveAs(h.accessHub, "boss", http.StatusOK, id, docs, "write", nil)
	if got := h.expect(http.StatusOK, "boss", "GET", "/tasks/"+id, nil); got["days_in_step"] != float64(0) {
		t.Fatalf("a move left days_in_step at %v, want 0", got["days_in_step"])
	}
}

func TestAnIssueWithNoRecordedMoveCountsFromItsLastChange(t *testing.T) {
	h, docs := newStepClockHub(t)
	id := issueAtStep(h.accessHub, docs, "write", "a")
	h.backdate(id, time.Time{}, time.Now().Add(-49*time.Hour))
	got := h.expect(http.StatusOK, "boss", "GET", "/tasks/"+id, nil)
	if got["days_in_step"] != float64(2) {
		t.Fatalf("days_in_step %v, want 2 from the last change", got["days_in_step"])
	}
	if got["entered_step_at"] == "" || got["entered_step_at"] == nil {
		t.Fatalf("entered_step_at is empty: %v", got)
	}
}

func TestMinDaysInStepNarrowsTheListAndThePendingAnswer(t *testing.T) {
	h, docs := newStepClockHub(t)
	young, old, older := issueAtStep(h.accessHub, docs, "write", "young"), issueAtStep(h.accessHub, docs, "write", "old"), issueAtStep(h.accessHub, docs, "write", "older")
	now := time.Now()
	h.backdate(young, now.Add(-24*time.Hour), now)
	h.backdate(old, now.Add(-72*time.Hour), now)
	h.backdate(older, now.Add(-240*time.Hour), now)

	cases := map[string][]string{
		"1":  {young, old, older},
		"3":  {old, older},
		"4":  {older},
		"11": nil,
	}
	for days, want := range cases {
		listed := h.listedIDs("boss", "/tasks?min_days_in_step="+days)
		pending := itemIDs(h.expect(http.StatusOK, "boss", "GET", "/pending-tasks?min_days_in_step="+days, nil))
		for name, got := range map[string][]string{"list": listed, "pending": pending} {
			if len(got) != len(want) {
				t.Errorf("%s with min_days_in_step=%s holds %d issues, want %d", name, days, len(got), len(want))
				continue
			}
			held := map[string]bool{}
			for _, id := range got {
				held[id] = true
			}
			for _, id := range want {
				if !held[id] {
					t.Errorf("%s with min_days_in_step=%s lacks %s", name, days, id)
				}
			}
		}
	}
	items := h.expect(http.StatusOK, "boss", "GET", "/pending-tasks?min_days_in_step=4", nil)["items"].([]any)
	if item := items[0].(map[string]any); item["days_in_step"] != float64(10) || item["entered_step_at"] == nil {
		t.Fatalf("a pending item carries %v, want 10 days and the time it entered", item)
	}
}

func TestAMinDaysInStepThatIsNotAWholeNumberIsRefused(t *testing.T) {
	h, _ := newStepClockHub(t)
	for _, bad := range []string{"-1", "abc", "1.5"} {
		if code, body := h.call("boss", "GET", "/tasks?min_days_in_step="+bad, nil); code != http.StatusBadRequest {
			t.Errorf("/tasks?min_days_in_step=%s = %d %v, want 400", bad, code, body)
		}
		h.expect(http.StatusBadRequest, "boss", "GET", "/pending-tasks?min_days_in_step="+bad, nil)
	}
	if ids := h.listedIDs("boss", "/tasks?min_days_in_step=0"); len(ids) != 0 {
		t.Errorf("an empty board lists %v", ids)
	}
}
