package routes_test

import (
	"net/http"
	"testing"
)

func writesOnABacklog(backlog, stepName string) map[string]probeCall {
	b := "/backlogs/" + backlog
	return map[string]probeCall{
		"rename_backlog":        {"PUT", b, map[string]string{"name": "Renamed"}},
		"delete_backlog":        {"DELETE", b, nil},
		"replace_backlogs":      {"PUT", "/backlogs", map[string]any{"backlogs": []map[string]any{}}},
		"add_backlog_step":      {"POST", b + "/steps", map[string]string{"name": stepName}},
		"rename_backlog_step":   {"PUT", b + "/steps/write", map[string]string{"name": "Drafting"}},
		"remove_backlog_step":   {"DELETE", b + "/steps/done", nil},
		"reorder_backlog_steps": {"PUT", b + "/step-order", map[string]any{"steps": []string{"done", "write"}}},
		"give_backlog_access":   {"PUT", b + "/access/contributors", map[string]string{"level": "move"}},
		"take_backlog_access":   {"DELETE", b + "/access/contributors", nil},
	}
}

func newEditHub(t *testing.T) (*accessHub, string, string) {
	t.Helper()
	h := newAccessHub(t, map[string][]string{
		"boss": {"owners"}, "ann": {"contributors"}, "mia": {"movers"}, "eli": {"editors"}, "nog": {},
	})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	docs := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Docs", "steps": []string{"Write", "Done"}})["id"].(string)
	other := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Other", "steps": []string{"Write", "Done"}})["id"].(string)
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+docs+"/access/contributors", map[string]string{"level": "see"})
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+docs+"/access/movers", map[string]string{"level": "move"})
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+docs+"/access/editors", map[string]string{"level": "edit"})
	return h, docs, other
}

func TestCanSeeAndCanMoveAreRefusedEveryEditOfABacklog(t *testing.T) {
	h, docs, _ := newEditHub(t)
	before := len(backlogIDs(h.expect(http.StatusOK, "boss", "GET", "/backlogs", nil)))
	for _, who := range []string{"ann", "mia"} {
		for name, call := range writesOnABacklog(docs, "Review") {
			if code, body := h.call(who, call.method, call.path, call.body); code != http.StatusForbidden {
				t.Errorf("%s as %s = %d %v, want 403", name, who, code, body)
			}
		}
	}
	got := h.expect(http.StatusOK, "boss", "GET", "/backlogs/"+docs+"/access", nil)
	if len(got["groups"].([]any)) != 3 {
		t.Fatalf("a refused edit changed the access list: %v", got)
	}
	listed := backlogIDs(h.expect(http.StatusOK, "boss", "GET", "/backlogs", nil))
	if len(listed) != before {
		t.Fatalf("a refused edit changed the backlogs: %v", listed)
	}
}

func TestCanEditAllowsEditingThatBacklogButNotOneWhereTheGroupOnlyMoves(t *testing.T) {
	h, docs, other := newEditHub(t)
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+other+"/access/editors", map[string]string{"level": "move"})
	h.expect(http.StatusOK, "eli", "PUT", "/backlogs/"+docs, map[string]string{"name": "Guides"})
	h.expect(http.StatusCreated, "eli", "POST", "/backlogs/"+docs+"/steps", map[string]string{"name": "Review"})
	h.expect(http.StatusOK, "eli", "PUT", "/backlogs/"+docs+"/access/contributors", map[string]string{"level": "move"})
	h.expect(http.StatusOK, "eli", "DELETE", "/backlogs/"+docs+"/access/contributors", nil)
	for name, call := range writesOnABacklog(other, "Review") {
		if code, body := h.call("eli", call.method, call.path, call.body); code != http.StatusForbidden {
			t.Errorf("%s on a backlog where the group can only move = %d %v, want 403", name, code, body)
		}
	}
}

func TestCanEditIncludesCanMoveAndCanSee(t *testing.T) {
	h, docs, _ := newEditHub(t)
	issue := h.expect(http.StatusCreated, "eli", "POST", "/tasks", map[string]any{"title": "a", "backlog": docs, "step": "write"})["id"].(string)
	h.expect(http.StatusOK, "eli", "POST", "/tasks/"+issue+"/move-to-step", map[string]string{"backlog": docs, "step": "done"})
	h.expect(http.StatusOK, "eli", "GET", "/tasks/"+issue, nil)
	if ids := backlogIDs(h.expect(http.StatusOK, "eli", "GET", "/backlogs", nil)); len(ids) != 1 || ids[0] != docs {
		t.Fatalf("a Can edit group lists %v", ids)
	}
}

func TestReplacingAllBacklogsIsRefusedToAGroupThatOnlyMovesOnOne(t *testing.T) {
	h, _, other := newEditHub(t)
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+other+"/access/editors", map[string]string{"level": "move"})
	h.expect(http.StatusForbidden, "eli", "PUT", "/backlogs", map[string]any{"backlogs": []map[string]any{}})
}

func TestTheOwnerGroupAlwaysHasCanEditAndTheAccessListShowsIt(t *testing.T) {
	h, docs, _ := newEditHub(t)
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+docs, map[string]string{"name": "Guides"})
	got := h.expect(http.StatusOK, "boss", "GET", "/backlogs/"+docs+"/access", nil)
	found := false
	for _, g := range got["groups"].([]any) {
		if g.(map[string]any)["level"] == "edit" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no group listed with level edit: %v", got)
	}
}
