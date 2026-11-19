package routes_test

import (
	"context"
	"encoding/json"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func (h *accessHub) rawGet(caller, path string) (int, []byte) {
	h.t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req = req.WithContext(context.WithValue(req.Context(), callerKey{}, caller))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}
func (h *accessHub) rawItems(caller, path, envelope string) []map[string]any {
	h.t.Helper()
	code, body := h.rawGet(caller, path)
	if code != http.StatusOK {
		h.t.Fatalf("GET %s = %d %s", path, code, body)
	}
	if envelope == "" {
		var items []map[string]any
		if err := json.Unmarshal(body, &items); err != nil {
			h.t.Fatalf("GET %s body %s: %v", path, body, err)
		}
		return items
	}
	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(body, &wrapped); err != nil {
		h.t.Fatalf("GET %s body %s: %v", path, body, err)
	}
	var items []map[string]any
	if err := json.Unmarshal(wrapped[envelope], &items); err != nil {
		h.t.Fatalf("GET %s items %s: %v", path, wrapped[envelope], err)
	}
	return items
}
func keysOf(item map[string]any) []string {
	keys := make([]string, 0, len(item))
	for key := range item {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func fieldsHub(t *testing.T) *accessHub {
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "one", "effort": "low", "tags": []string{"Ready"}, "description": "long text"})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "two", "effort": "high", "tags": []string{"Ready"}, "description": "more text"})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "three", "tags": []string{"Ready"}})
	return h
}
func TestFieldsKeepsOnlyTheNamedKeysOnEachOfTheThreeTools(t *testing.T) {
	h := fieldsHub(t)
	for _, c := range []struct{ path, envelope string }{{"/tasks?fields=id,code,title", "items"}, {"/pending-tasks?fields=id,code,title", "items"}, {"/next-ready?limit=5&fields=id,code,title", "items"}} {
		items := h.rawItems("owner", c.path, c.envelope)
		if len(items) != 3 {
			t.Fatalf("%s: %d items, want 3", c.path, len(items))
		}
		for _, item := range items {
			if got := keysOf(item); !reflect.DeepEqual(got, []string{"code", "id", "title"}) {
				t.Errorf("%s: keys %v, want code id title", c.path, got)
			}
		}
	}
}
func TestFieldsAcceptsARepeatedParameterAndACommaList(t *testing.T) {
	h := fieldsHub(t)
	repeated := h.rawItems("owner", "/pending-tasks?fields=id&fields=title", "items")
	for _, item := range repeated {
		if got := keysOf(item); !reflect.DeepEqual(got, []string{"id", "title"}) {
			t.Errorf("repeated fields: keys %v, want id title", got)
		}
	}
	spaced := h.rawItems("owner", "/pending-tasks?fields=id,%20title", "items")
	for _, item := range spaced {
		if got := keysOf(item); !reflect.DeepEqual(got, []string{"id", "title"}) {
			t.Errorf("comma list with a space: keys %v, want id title", got)
		}
	}
}
func TestAnUnknownFieldIsRefusedWithAFilterErrorNamingIt(t *testing.T) {
	h := fieldsHub(t)
	for _, path := range []string{"/tasks", "/pending-tasks", "/next-ready"} {
		code, body := h.rawGet("owner", path+"?fields=id,gigantic")
		if code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", path, code, body)
		}
		if !strings.Contains(string(body), "gigantic") {
			t.Errorf("%s: the refusal does not name the field: %s", path, body)
		}
	}
}
func TestWithoutFieldsEachToolAnswersAsBefore(t *testing.T) {
	h := fieldsHub(t)
	withAll := h.rawItems("owner", "/pending-tasks", "items")
	for _, key := range []string{"id", "code", "title", "backlog", "step", "priority", "tags", "rank", "blocked", "waiting", "sinbin"} {
		if _, ok := withAll[0][key]; !ok {
			t.Errorf("pending without fields lacks %s: %v", key, withAll[0])
		}
	}
	listed := h.rawItems("owner", "/tasks", "items")
	for _, key := range []string{"id", "title", "status", "priority", "effort", "tags", "files"} {
		if _, ok := listed[0][key]; !ok {
			t.Errorf("list without fields lacks %s: %v", key, listed[0])
		}
	}
	if len(h.rawItems("owner", "/next-ready", "items")[0]) < 9 {
		t.Errorf("next_ready without fields lost keys")
	}
}
func TestFieldsLeavesTotalsPagingAndByStepUnchanged(t *testing.T) {
	h := fieldsHub(t)
	plain := h.expect(http.StatusOK, "owner", "GET", "/pending-tasks?limit=2&offset=1", nil)
	narrow := h.expect(http.StatusOK, "owner", "GET", "/pending-tasks?limit=2&offset=1&fields=id", nil)
	for _, key := range []string{"total", "by_step", "by_priority"} {
		if !reflect.DeepEqual(plain[key], narrow[key]) {
			t.Errorf("%s changed with fields: %v vs %v", key, plain[key], narrow[key])
		}
	}
	if !reflect.DeepEqual(itemIDs(plain), itemIDs(narrow)) {
		t.Errorf("paging changed with fields: %v vs %v", itemIDs(plain), itemIDs(narrow))
	}
	ready := h.expect(http.StatusOK, "owner", "GET", "/next-ready?limit=2", nil)
	readyNarrow := h.expect(http.StatusOK, "owner", "GET", "/next-ready?limit=2&fields=code", nil)
	if ready["total_ready"] != readyNarrow["total_ready"] {
		t.Errorf("total_ready changed with fields: %v vs %v", ready["total_ready"], readyNarrow["total_ready"])
	}
}
func TestFieldsIsInTheInputSchemaOfTheThreeTools(t *testing.T) {
	wanted := map[string]bool{"taskmanager.task.list": false, "taskmanager.task.pending": false, "taskmanager.task.next_ready": false}
	for _, tool := range routes.ToolShape() {
		if _, ok := wanted[tool.Action]; !ok {
			continue
		}
		wanted[tool.Action] = true
		if !strings.Contains(string(tool.InputSchema), `"fields"`) {
			t.Errorf("%s: the input schema has no fields: %s", tool.Action, tool.InputSchema)
		}
	}
	for action, found := range wanted {
		if !found {
			t.Errorf("tool %s not found", action)
		}
	}
}
