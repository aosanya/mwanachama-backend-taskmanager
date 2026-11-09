package routes_test

import (
	"context"
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

type accessHub struct {
	t   *testing.T
	mux *http.ServeMux
}

func newAccessHub(t *testing.T, groupsOf map[string][]string) *accessHub {
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
	mgr.UseGroups(func(_ context.Context, callerID string) ([]string, error) { return groupsOf[callerID], nil })
	known := map[string]bool{"owners": true, "contributors": true, "g-owner": true, "g-dev": true}
	for _, held := range groupsOf {
		for _, g := range held {
			known[g] = true
		}
	}
	mgr.UseGroupExists(func(_ context.Context, groupID string) (bool, error) { return known[groupID], nil })
	built, err := routes.BuildFor(mgr, routes.Mount{Caller: func(ctx context.Context) string {
		id, _ := ctx.Value(callerKey{}).(string)
		return id
	}})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	for _, r := range built {
		mux.HandleFunc(r.Pattern(""), r.Handler)
	}
	return &accessHub{t: t, mux: mux}
}

func (h *accessHub) call(caller, method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	payload := ""
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
		payload = string(raw)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), callerKey{}, caller))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	var decoded map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	return rec.Code, decoded
}

func (h *accessHub) expect(want int, caller, method, path string, body any) map[string]any {
	h.t.Helper()
	code, decoded := h.call(caller, method, path, body)
	if code != want {
		h.t.Fatalf("%s %s as %q = %d %v, want %d", method, path, caller, code, decoded, want)
	}
	return decoded
}

func backlogIDs(list map[string]any) []string {
	var ids []string
	for _, raw := range list["items"].([]any) {
		ids = append(ids, raw.(map[string]any)["id"].(string))
	}
	return ids
}

func TestAccessOperationsAreDeclaredWithTheirActions(t *testing.T) {
	built, err := routes.Build(mwanachamataskmanager.Unbound())
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, r := range built {
		have[r.Method+" "+r.Path+" "+r.Action] = true
	}
	want := []string{
		"GET /backlogs/{backlogID}/access taskmanager.backlog_access.list",
		"PUT /backlogs/{backlogID}/access/{groupID} taskmanager.backlog_access.give",
		"DELETE /backlogs/{backlogID}/access/{groupID} taskmanager.backlog_access.take",
		"PUT /backlog-owner-group taskmanager.backlog.set_owner_group",
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("no route %q", w)
		}
	}
}

func TestAGroupIsGivenAndLosesAccessThroughTheRoutes(t *testing.T) {
	h := newAccessHub(t, nil)
	h.expect(http.StatusConflict, "", "PUT", "/backlogs/backlog/access/contributors", map[string]string{"level": "see"})
	h.expect(http.StatusBadRequest, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-nowhere"})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	h.expect(http.StatusOK, "", "PUT", "/backlogs/backlog/access/contributors", map[string]string{"level": "see"})
	access := h.expect(http.StatusOK, "", "GET", "/backlogs/backlog/access", nil)
	if groups := access["groups"].([]any); len(groups) != 1 || groups[0].(map[string]any)["level"] != "see" {
		t.Fatalf("access after the grant = %v", access)
	}
	h.expect(http.StatusOK, "", "PUT", "/backlogs/backlog/access/contributors", map[string]string{"level": "move"})
	h.expect(http.StatusOK, "", "DELETE", "/backlogs/backlog/access/contributors", nil)
	access = h.expect(http.StatusOK, "", "GET", "/backlogs/backlog/access", nil)
	if groups := access["groups"].([]any); len(groups) != 0 {
		t.Fatalf("access after taking it away = %v", access)
	}
	h.expect(http.StatusBadRequest, "", "PUT", "/backlogs/backlog/access/contributors", map[string]string{"level": "own"})
	h.expect(http.StatusBadRequest, "", "PUT", "/backlogs/nowhere/access/contributors", map[string]string{"level": "see"})
}

func TestAPersonListsAndMovesOnlyWhereTheirGroupsAllow(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"ann": {"contributors"}, "boss": {"owners"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	docs := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Docs", "steps": []string{"Write", "Done"}})["id"].(string)
	issue := h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "write the guide"})["id"].(string)
	h.expect(http.StatusOK, "boss", "POST", "/tasks/"+issue+"/move-to-step", map[string]string{"backlog": docs, "step": "write"})

	if ids := backlogIDs(h.expect(http.StatusOK, "ann", "GET", "/backlogs", nil)); len(ids) != 0 {
		t.Fatalf("a group with no access lists %v", ids)
	}
	if ids := backlogIDs(h.expect(http.StatusOK, "boss", "GET", "/backlogs", nil)); len(ids) != 2 {
		t.Fatalf("the Product owner group lists %v, want both backlogs", ids)
	}

	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+docs+"/access/contributors", map[string]string{"level": "see"})
	if ids := backlogIDs(h.expect(http.StatusOK, "ann", "GET", "/backlogs", nil)); len(ids) != 1 || ids[0] != docs {
		t.Fatalf("a group that can see Docs lists %v", ids)
	}
	h.expect(http.StatusForbidden, "ann", "POST", "/tasks/"+issue+"/move-to-step", map[string]string{"backlog": docs, "step": "done"})

	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+docs+"/access/contributors", map[string]string{"level": "move"})
	h.expect(http.StatusOK, "ann", "POST", "/tasks/"+issue+"/move-to-step", map[string]string{"backlog": docs, "step": "done"})
}
