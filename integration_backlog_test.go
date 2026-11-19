package mwanachamataskmanager_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/aosanya/mwanachama-backend-shared/dispatch"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
	"net/http"
	"net/http/httptest"
	"testing"
)

type board struct {
	t   *testing.T
	mux *http.ServeMux
	mgr mwanachamataskmanager.TaskManager
}

func newBoard(t *testing.T) *board {
	t.Helper()
	return newBoardAs(t, nil)
}
func newBoardAs(t *testing.T, authorize dispatch.Authorizer) *board {
	t.Helper()
	mgr := newTestManager(t)
	built, err := routes.BuildWith(mgr, authorize)
	if err != nil {
		t.Fatalf("BuildWith: %v", err)
	}
	mux := http.NewServeMux()
	for _, route := range built {
		mux.HandleFunc(route.Pattern(""), route.Handler)
	}
	return &board{t: t, mux: mux, mgr: mgr}
}
func (b *board) call(method, path string, body any) (int, []byte) {
	b.t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			b.t.Fatalf("marshal %s %s: %v", method, path, err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	b.mux.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}
func (b *board) create(task map[string]any) map[string]any {
	b.t.Helper()
	code, raw := b.call(http.MethodPost, "/tasks", task)
	if code != http.StatusCreated {
		b.t.Fatalf("POST /tasks %v = %d %s", task, code, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		b.t.Fatalf("decode created task: %v", err)
	}
	return out
}
func (b *board) titles(query string) []string {
	b.t.Helper()
	code, raw := b.call(http.MethodGet, "/tasks"+query, nil)
	if code != http.StatusOK {
		b.t.Fatalf("GET /tasks%s = %d %s", query, code, raw)
	}
	var page struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		b.t.Fatalf("GET /tasks%s: body is not an items object: %s", query, raw)
	}
	out := make([]string, 0, len(page.Items))
	for _, task := range page.Items {
		out = append(out, task["title"].(string))
	}
	return out
}
func sameSet(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, g := range got {
		seen[g]++
	}
	for _, w := range want {
		seen[w]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
func (b *board) wantTitles(query string, want ...string) {
	b.t.Helper()
	if got := b.titles(query); !sameSet(got, want...) {
		b.t.Fatalf("GET /tasks%s = %v, want %v", query, got, want)
	}
}
func seededBoard(t *testing.T) *board {
	t.Helper()
	b := newBoard(t)
	b.create(map[string]any{"title": "docs hub", "tags": []string{"Docs"}, "project_name": "hub-steward", "priority": "high"})
	b.create(map[string]any{"title": "docs swe", "tags": []string{"Docs", "Blocked"}, "project_name": "hub-steward-swe", "priority": "low"})
	b.create(map[string]any{"title": "plain hub", "project_name": "hub-steward", "priority": "high"})
	return b
}
func TestListTasksHTTP_TagAndRepoCombine(t *testing.T) {
	b := seededBoard(t)
	b.wantTitles("?tag=Docs", "docs hub", "docs swe")
	b.wantTitles("?repo=hub-steward", "docs hub", "plain hub")
	b.wantTitles("?tag=Docs&repo=hub-steward", "docs hub")
	b.wantTitles("?tag=Docs&tag=Blocked&repo=hub-steward-swe", "docs swe")
	b.wantTitles("?tag=Docs&priority=high&repo=hub-steward", "docs hub")
	b.wantTitles("?tag=Blocked&priority=high")
}
func TestListTasksHTTP_UnknownValuesReturnNothingNotEverything(t *testing.T) {
	b := seededBoard(t)
	b.wantTitles("?tag=Nonexistent")
	b.wantTitles("?repo=no-such-repo")
	b.wantTitles("?tag=docs", "docs hub", "docs swe")
	b.wantTitles("?tag=Docs&tag=Blocked&tag=Missing")
	b.wantTitles("?repo=hub")
}
func TestListTasksHTTP_AgeFilter(t *testing.T) {
	b := seededBoard(t)
	b.wantTitles("?untouched_days=1")
	b.wantTitles("?untouched_days=0", "docs hub", "docs swe", "plain hub")
	code, _ := b.call(http.MethodGet, "/tasks?untouched_days=-3", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("negative untouched_days = %d, want 400", code)
	}
	code, _ = b.call(http.MethodGet, "/tasks?untouched_days=abc", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("untouched_days=abc = %d, want 400", code)
	}
}
func TestListTasksHTTP_UnknownBacklogIsA400(t *testing.T) {
	b := seededBoard(t)
	code, raw := b.call(http.MethodGet, "/tasks?backlog=no-such-backlog", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("unknown backlog = %d %s, want 400", code, raw)
	}
}
func TestListTasksHTTP_StepNarrowsByTaskState(t *testing.T) {
	b := seededBoard(t)
	b.wantTitles("?step=To+do", "docs hub", "docs swe", "plain hub")
	b.wantTitles("?step=Doing")
}
func TestListTasksHTTP_LimitStillAppliesWithFilters(t *testing.T) {
	b := seededBoard(t)
	got := b.titles("?tag=Docs&limit=1")
	if len(got) != 1 {
		t.Fatalf("tag=Docs&limit=1 = %v, want one task", got)
	}
}
func dependsOnEdge(from, to string) map[string]any {
	return map[string]any{"label": mwanachamataskmanager.RelLabelDependsOn, "from_id": from, "to_id": to}
}
func (b *board) edges(vertex, direction string) []map[string]any {
	b.t.Helper()
	query := "?vertex_id=" + vertex + "&label=depends_on"
	if direction != "" {
		query += "&direction=" + direction
	}
	code, raw := b.call(http.MethodGet, "/relationships/traverse"+query, nil)
	if code != http.StatusOK {
		b.t.Fatalf("traverse %s = %d %s", query, code, raw)
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		b.t.Fatalf("traverse %s: body is not a list: %s", query, raw)
	}
	return out
}
func TestTraverseHTTP_DependsOnComesBackAsAList(t *testing.T) {
	b := newBoard(t)
	a := b.create(map[string]any{"title": "needs both"})["id"].(string)
	x := b.create(map[string]any{"title": "first dep"})["id"].(string)
	y := b.create(map[string]any{"title": "second dep"})["id"].(string)
	for _, to := range []string{x, y} {
		if code, raw := b.call(http.MethodPost, "/relationships", dependsOnEdge(a, to)); code != http.StatusCreated {
			t.Fatalf("create depends_on %s->%s = %d %s", a, to, code, raw)
		}
	}
	out := b.edges(a, "")
	if len(out) != 2 {
		t.Fatalf("outbound depends_on of a = %v, want two edges", out)
	}
	gotTo := map[string]bool{}
	for _, e := range out {
		if e["from_id"] != a || e["label"] != "depends_on" {
			t.Fatalf("edge %v is not a depends_on edge leaving a", e)
		}
		gotTo[e["to_id"].(string)] = true
	}
	if !gotTo[x] || !gotTo[y] {
		t.Fatalf("outbound targets %v, want %s and %s", gotTo, x, y)
	}
	in := b.edges(x, "inbound")
	if len(in) != 1 || in[0]["from_id"] != a {
		t.Fatalf("inbound depends_on of x = %v, want one edge from a", in)
	}
}
func TestTraverseHTTP_NoEdgesIsAnEmptyListNotNull(t *testing.T) {
	b := newBoard(t)
	a := b.create(map[string]any{"title": "alone"})["id"].(string)
	code, raw := b.call(http.MethodGet, "/relationships/traverse?vertex_id="+a+"&label=depends_on", nil)
	if code != http.StatusOK {
		t.Fatalf("traverse = %d %s", code, raw)
	}
	if string(bytes.TrimSpace(raw)) != "[]" {
		t.Fatalf("traverse with no edges = %s, want []", raw)
	}
}
func TestTraverseHTTP_RewritingAnEdgeDoesNotDuplicateIt(t *testing.T) {
	b := newBoard(t)
	a := b.create(map[string]any{"title": "a"})["id"].(string)
	x := b.create(map[string]any{"title": "x"})["id"].(string)
	for i := 0; i < 2; i++ {
		b.call(http.MethodPost, "/relationships", dependsOnEdge(a, x))
	}
	if out := b.edges(a, ""); len(out) != 1 {
		t.Fatalf("after writing the same edge twice: %v, want one", out)
	}
}
func TestTraverseHTTP_DeletingATaskLeavesNoDanglingList(t *testing.T) {
	b := newBoard(t)
	a := b.create(map[string]any{"title": "a"})["id"].(string)
	x := b.create(map[string]any{"title": "x"})["id"].(string)
	b.call(http.MethodPost, "/relationships", dependsOnEdge(a, x))
	if err := b.mgr.DeleteTask(context.Background(), x); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	code, raw := b.call(http.MethodGet, "/relationships/traverse?vertex_id="+a+"&label=depends_on", nil)
	if code != http.StatusOK {
		t.Fatalf("traverse after delete = %d %s, want 200", code, raw)
	}
}
