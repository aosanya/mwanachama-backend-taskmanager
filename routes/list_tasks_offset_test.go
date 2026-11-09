package routes_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func (h *hub) list(path string) (int, []map[string]any) {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, strings.NewReader(""))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	var tasks []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &tasks)
	return rec.Code, tasks
}

func TestListTasksRoute_OffsetPagesAndANegativeOffsetIs400(t *testing.T) {
	h := newHub(t)
	for i := 0; i < 5; i++ {
		h.expect(http.StatusCreated, "POST", "/tasks", map[string]any{"title": fmt.Sprintf("issue %d", i)})
	}
	code, all := h.list("/tasks")
	if code != http.StatusOK || len(all) != 5 {
		t.Fatalf("GET /tasks = %d with %d issues", code, len(all))
	}
	code, page := h.list("/tasks?offset=3&limit=10")
	if code != http.StatusOK || len(page) != 2 {
		t.Fatalf("GET /tasks?offset=3 = %d with %d issues", code, len(page))
	}
	if page[0]["id"] != all[3]["id"] || page[1]["id"] != all[4]["id"] {
		t.Fatalf("offset page is not the tail of the whole list")
	}
	if code, _ := h.list("/tasks?offset=-1"); code != http.StatusBadRequest {
		t.Fatalf("GET /tasks?offset=-1 = %d, want 400", code)
	}
}
