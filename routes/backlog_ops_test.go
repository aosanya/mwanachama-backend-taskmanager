package routes_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

type hub struct {
	t   *testing.T
	mux *http.ServeMux
}

func newHub(t *testing.T) *hub {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
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
	if err := spec.Migrate(db, s); err != nil {
		t.Fatal(err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	built, err := routes.Build(mgr)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	for _, r := range built {
		mux.HandleFunc(r.Pattern(""), r.Handler)
	}
	return &hub{t: t, mux: mux}
}

func (h *hub) call(method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		reader = strings.NewReader(string(raw))
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	var decoded map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	return rec.Code, decoded
}

func (h *hub) expect(want int, method, path string, body any) map[string]any {
	h.t.Helper()
	code, decoded := h.call(method, path, body)
	if code != want {
		h.t.Fatalf("%s %s = %d %v, want %d", method, path, code, decoded, want)
	}
	return decoded
}

func stepIDs(summary map[string]any) []string {
	var ids []string
	for _, raw := range summary["steps"].([]any) {
		ids = append(ids, raw.(map[string]any)["id"].(string))
	}
	return ids
}

func TestBacklogOperationsAreDeclaredWithTheirActions(t *testing.T) {
	built, err := routes.Build(mwanachamataskmanager.Unbound())
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, r := range built {
		have[r.Method+" "+r.Path+" "+r.Action] = true
	}
	want := []string{
		"GET /backlogs taskmanager.backlog.list",
		"POST /backlogs taskmanager.backlog.create",
		"PUT /backlogs taskmanager.backlog.manage",
		"PUT /backlogs/{backlogID} taskmanager.backlog.rename",
		"DELETE /backlogs/{backlogID} taskmanager.backlog.delete",
		"POST /backlogs/{backlogID}/steps taskmanager.backlog_step.add",
		"PUT /backlogs/{backlogID}/steps/{stepID} taskmanager.backlog_step.rename",
		"DELETE /backlogs/{backlogID}/steps/{stepID} taskmanager.backlog_step.remove",
		"PUT /backlogs/{backlogID}/step-order taskmanager.backlog_step.reorder",
		"POST /tasks/{taskID}/move-to-step taskmanager.task.move_to_step",
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("no route %q", w)
		}
	}
}

func TestAskingForBacklogsAnswersTheDefaultOne(t *testing.T) {
	h := newHub(t)
	list := h.expect(http.StatusOK, "GET", "/backlogs", nil)
	items := list["items"].([]any)
	if len(items) != 1 || len(stepIDs(items[0].(map[string]any))) != 5 {
		t.Fatalf("a new hub lists %v, want one backlog of five steps", items)
	}
}

func TestABacklogIsAddedRenamedAndItsStepsChangedThroughTheRoutes(t *testing.T) {
	h := newHub(t)
	made := h.expect(http.StatusCreated, "POST", "/backlogs", map[string]any{"name": "Support", "steps": []string{"New", "Answering", "Closed"}})
	id := made["id"].(string)

	h.expect(http.StatusOK, "PUT", "/backlogs/"+id, map[string]string{"name": "Help desk"})
	added := h.expect(http.StatusCreated, "POST", "/backlogs/"+id+"/steps", map[string]string{"name": "Review", "after": "answering"})
	if got := strings.Join(stepIDs(added), ","); got != "new,answering,review,closed" {
		t.Fatalf("steps after the add = %s", got)
	}
	renamed := h.expect(http.StatusOK, "PUT", "/backlogs/"+id+"/steps/review", map[string]string{"name": "Check"})
	if renamed["name"] != "Help desk" {
		t.Fatalf("renamed summary = %v", renamed)
	}
	reordered := h.expect(http.StatusOK, "PUT", "/backlogs/"+id+"/step-order", map[string]any{"steps": []string{"new", "review", "answering", "closed"}})
	if got := strings.Join(stepIDs(reordered), ","); got != "new,review,answering,closed" {
		t.Fatalf("steps after the reorder = %s", got)
	}
	removed := h.expect(http.StatusOK, "DELETE", "/backlogs/"+id+"/steps/review", nil)
	if got := strings.Join(stepIDs(removed), ","); got != "new,answering,closed" {
		t.Fatalf("steps after the removal = %s", got)
	}
	h.expect(http.StatusNoContent, "DELETE", "/backlogs/"+id, nil)
}

func TestAStepAndABacklogHoldingIssuesAreRefusedWithAConflict(t *testing.T) {
	h := newHub(t)
	made := h.expect(http.StatusCreated, "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"Ideas", "Build"}})
	id := made["id"].(string)
	issue := h.expect(http.StatusCreated, "POST", "/tasks", map[string]any{"title": "write the loader"})
	h.expect(http.StatusOK, "POST", "/tasks/"+issue["id"].(string)+"/move-to-step", map[string]string{"backlog": id, "step": "build"})

	h.expect(http.StatusConflict, "DELETE", "/backlogs/"+id+"/steps/build", nil)
	h.expect(http.StatusConflict, "DELETE", "/backlogs/"+id, nil)

	h.expect(http.StatusOK, "POST", "/tasks/"+issue["id"].(string)+"/move-to-step", map[string]string{"backlog": id, "step": "ideas"})
	h.expect(http.StatusOK, "DELETE", "/backlogs/"+id+"/steps/build", nil)
}

func TestUnknownBacklogsAndStepsAreRefusedAsBadRequests(t *testing.T) {
	h := newHub(t)
	h.expect(http.StatusBadRequest, "PUT", "/backlogs/nowhere", map[string]string{"name": "x"})
	h.expect(http.StatusBadRequest, "DELETE", "/backlogs/backlog/steps/nowhere", nil)
	h.expect(http.StatusBadRequest, "POST", "/backlogs", map[string]any{"name": "  ", "steps": []string{"New"}})
}

func TestReplacingTheBacklogsSetsTheWholeList(t *testing.T) {
	h := newHub(t)
	list := h.expect(http.StatusOK, "PUT", "/backlogs", map[string]any{"backlogs": []map[string]any{
		{"id": "backlog", "name": "Backlog", "steps": []map[string]any{{"id": "todo", "name": "To do", "statuses": []string{"pending", "in_progress", "blocked", "completed", "cancelled"}}}},
		{"id": "docs", "name": "Docs", "steps": []map[string]any{{"id": "write", "name": "Write"}, {"id": "done", "name": "Done"}}},
	}})
	if len(list["items"].([]any)) != 2 {
		t.Fatalf("replaced list = %v", list)
	}
}
