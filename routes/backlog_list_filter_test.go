package routes_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func (h *hub) listBacklogIDs(query string) []string {
	h.t.Helper()
	req := httptest.NewRequest("GET", "/backlogs"+query, nil)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("GET /backlogs%s = %d %s", query, rec.Code, rec.Body.String())
	}
	var body struct {
		Items []struct {
			ID    string           `json:"id"`
			Steps []map[string]any `json:"steps"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		h.t.Fatal(err)
	}
	ids := []string{}
	for _, item := range body.Items {
		ids = append(ids, item.ID)
	}
	return ids
}
func TestListBacklogsTakesAnOptionalBacklogFilter(t *testing.T) {
	h := newHub(t)
	h.expect(http.StatusCreated, "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"Open", "Shipped"}})
	if got := h.listBacklogIDs(""); len(got) != 2 {
		t.Fatalf("unfiltered %v", got)
	}
	if got := h.listBacklogIDs("?backlog=software"); len(got) != 1 || got[0] != "software" {
		t.Fatalf("by id %v", got)
	}
	if got := h.listBacklogIDs("?backlog=SOFTWARE"); len(got) != 1 || got[0] != "software" {
		t.Fatalf("by name %v", got)
	}
	if got := h.listBacklogIDs("?backlog=nothing"); len(got) != 0 {
		t.Fatalf("unknown %v", got)
	}
}
