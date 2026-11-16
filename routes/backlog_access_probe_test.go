package routes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

type probeCall struct {
	method string
	path   string
	body   any
}

var operationsNotBoundToABacklog = map[string]bool{
	"open_case":               true,
	"vote_case":               true,
	"take_back_vote":          true,
	"add_task_note":           true,
	"add_task_reference":      true,
	"propose_task_reference":  true,
	"confirm_task_reference":  true,
	"remove_task_reference":   true,
	"set_worker_caps_default": true,
}

var readsSentAsPost = map[string]bool{
	"count_backlog_rules": true,
}

func operationsTakingTheCallerThatWrite(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../taskmanager.operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Operations map[string]struct {
			Method string `json:"method"`
			Args   []struct {
				From string `json:"from"`
				As   string `json:"as"`
			} `json:"args"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	var names []string
	for name, op := range file.Operations {
		if op.Method == http.MethodGet || readsSentAsPost[name] || operationsNotBoundToABacklog[name] {
			continue
		}
		for _, arg := range op.Args {
			if arg.From == "caller" && arg.As == "requester_id" {
				names = append(names, name)
				break
			}
		}
	}
	sort.Strings(names)
	return names
}

func writesOnAnIssue(f *moreWritesFixture, other string) map[string]probeCall {
	deadline := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	issue := "/tasks/" + f.issue
	return map[string]probeCall{
		"create_task":              {"POST", "/tasks", map[string]any{"title": "new", "backlog": f.docs, "step": "write"}},
		"update_task":              {"PUT", issue, map[string]any{"title": "renamed", "status": "pending", "backlog": f.docs, "step": "write"}},
		"patch_task":               {"PATCH", issue, map[string]any{"title": "patched"}},
		"move_task_to_state":       {"POST", issue + "/move", map[string]string{"state": "Done"}},
		"move_task_to_step":        {"POST", issue + "/move-to-step", map[string]string{"backlog": f.docs, "step": "done"}},
		"place_task":               {"POST", issue + "/place", map[string]string{"after": other}},
		"delete_task":              {"DELETE", issue, nil},
		"unblock_task":             {"POST", issue + "/unblock", map[string]any{"note": "go"}},
		"unblock_dependents":       {"POST", issue + "/unblock-dependents", nil},
		"assign_task":              {"POST", issue + "/assign", map[string]any{"agent_id": "worker"}},
		"unassign_task":            {"POST", issue + "/unassign", nil},
		"assign_task_to_holder":    {"POST", issue + "/assign-holder", map[string]any{"assignee_kind": "actor", "assignee_id": "someone"}},
		"create_task_in_project":   {"POST", "/projects/guide/tasks", map[string]any{"title": "new", "backlog": f.docs, "step": "write"}},
		"add_task_to_project":      {"POST", "/projects/" + f.project + "/tasks/" + f.issue, nil},
		"remove_task_from_project": {"DELETE", "/projects/" + f.project + "/tasks/" + f.issue, nil},
		"create_relationship":      {"POST", "/relationships", map[string]string{"label": "blocks", "from_id": f.issue, "to_id": other}},
		"delete_relationship":      {"DELETE", "/relationships?label=blocks&from_id=" + f.issue + "&to_id=" + other, nil},
		"link_task_to_run":         {"POST", "/workflow-runs/" + f.run + "/tasks/" + f.issue + "/link", nil},
		"cancel_workflow_run":      {"POST", "/workflow-runs/" + f.run + "/cancel", map[string]any{"reason": "no", "quiesce_deadline": deadline}},
		"rollback_workflow_run":    {"POST", "/workflow-runs/" + f.run + "/rollback", map[string]any{"reason": "no"}},
		"link_case_task":           {"PUT", "/cases/" + f.kase + "/tasks/" + f.issue, nil},
		"end_case":                 {"POST", "/cases/" + f.kase + "/end", map[string]any{"status": "cancelled"}},
		"open_sourced_tasks":       {"POST", "/sourced-tasks", map[string]any{"source": "feed", "items": []map[string]any{{"key": "k1", "title": "from a feed", "due_at": "2030-01-01T00:00:00Z"}}}},
		"set_task_caps":            {"PUT", issue + "/caps", map[string]any{"input_tokens": 5}},
		"clear_task_caps":          {"DELETE", issue + "/caps", nil},
		"write_task_usage":         {"PUT", issue + "/usage", map[string]any{"input_tokens": 5}},
		"generate_tasks":           {"POST", "/generated-tasks", map[string]any{"run": "r1", "template": map[string]any{"title": "per subject"}, "subjects": []string{"s1"}}},
	}
}

func TestEveryWriteThatTakesTheCallerIsProbedForCanSee(t *testing.T) {
	f := newMoreWritesFixture(t)
	other := f.h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "second", "backlog": f.docs, "step": "write"})["id"].(string)
	probed := writesOnAnIssue(f, other)
	for name, call := range writesOnABacklog(f.docs, "steps") {
		probed[name] = call
	}
	var have []string
	for name := range probed {
		have = append(have, name)
	}
	sort.Strings(have)
	declared := operationsTakingTheCallerThatWrite(t)
	if fmt.Sprint(have) != fmt.Sprint(declared) {
		t.Fatalf("operations that write and take the caller:\n declared %v\n probed   %v", declared, have)
	}
}

func TestACanSeeGroupAndAPersonInNoGroupAreRefusedEveryWriteOnAnIssue(t *testing.T) {
	f := newMoreWritesFixture(t)
	other := f.h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "second", "backlog": f.docs, "step": "write"})["id"].(string)
	for name, call := range writesOnAnIssue(f, other) {
		if code, body := f.h.call("ann", call.method, call.path, call.body); code != http.StatusForbidden {
			t.Errorf("%s as a Can see group = %d %v, want 403", name, code, body)
		}
		if code, body := f.h.call("nog", call.method, call.path, call.body); code != http.StatusForbidden && code != http.StatusNotFound {
			t.Errorf("%s as a person in no group = %d %v, want 403 or 404", name, code, body)
		}
	}
	got := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue, nil)
	if got["title"] != "write the guide" || got["status"] != "pending" || got["step"] != "write" || got["backlog"] != f.docs || got["deleted"] != false {
		t.Fatalf("a refused write changed the issue: %v", got)
	}
	if got := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+other, nil); got["title"] != "second" {
		t.Fatalf("a refused write changed the second issue: %v", got)
	}
}

func TestAPersonInTwoGroupsGetsTheHigherLevelInEitherOrder(t *testing.T) {
	for name, groups := range map[string][]string{
		"see first":  {"contributors", "g-dev"},
		"move first": {"g-dev", "contributors"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newAccessHub(t, map[string][]string{"boss": {"owners"}, "two": groups})
			h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
			docs := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Docs", "steps": []string{"Write", "Done"}})["id"].(string)
			issue := h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "a", "backlog": docs, "step": "write"})["id"].(string)
			h.expect(http.StatusOK, "", "PUT", "/backlogs/"+docs+"/access/contributors", map[string]string{"level": "see"})
			h.expect(http.StatusOK, "", "PUT", "/backlogs/"+docs+"/access/g-dev", map[string]string{"level": "move"})
			h.expect(http.StatusOK, "two", "PUT", "/tasks/"+issue, map[string]any{"title": "b", "status": "pending", "backlog": docs, "step": "write"})
			h.expect(http.StatusOK, "two", "POST", "/tasks/"+issue+"/move-to-step", map[string]string{"backlog": docs, "step": "done"})
			h.expect(http.StatusCreated, "two", "POST", "/tasks", map[string]any{"title": "c", "backlog": docs, "step": "write"})
			h.expect(http.StatusNoContent, "two", "DELETE", "/tasks/"+issue, nil)
		})
	}
}

func TestMovingAnIssueNeedsCanMoveOnBothBacklogsAndNothingMovesWhenRefused(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"boss": {"owners"}, "two": {"contributors", "g-dev"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	from := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "From", "steps": []string{"A", "B"}})["id"].(string)
	to := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "To", "steps": []string{"A", "B"}})["id"].(string)
	issue := h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "a", "backlog": from, "step": "a"})["id"].(string)
	h.expect(http.StatusOK, "", "PUT", "/backlogs/"+from+"/access/g-dev", map[string]string{"level": "move"})
	h.expect(http.StatusOK, "", "PUT", "/backlogs/"+to+"/access/contributors", map[string]string{"level": "see"})
	h.expect(http.StatusForbidden, "two", "POST", "/tasks/"+issue+"/move-to-step", map[string]string{"backlog": to, "step": "a"})
	h.expect(http.StatusForbidden, "two", "POST", "/tasks", map[string]any{"title": "x", "backlog": to, "step": "a"})
	got := h.expect(http.StatusOK, "boss", "GET", "/tasks/"+issue, nil)
	if got["backlog"] != from || got["step"] != "a" {
		t.Fatalf("a refused move changed the issue: %v", got)
	}
	h.expect(http.StatusOK, "", "PUT", "/backlogs/"+to+"/access/g-dev", map[string]string{"level": "move"})
	h.expect(http.StatusOK, "two", "POST", "/tasks/"+issue+"/move-to-step", map[string]string{"backlog": to, "step": "a"})
}

func TestAHubWithNoOwnerGroupAndNoGrantsIsOpenAndClosesWhenTheOwnerGroupIsSet(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"boss": {"owners"}})
	issue := h.expect(http.StatusCreated, "nog", "POST", "/tasks", map[string]any{"title": "open hub"})["id"].(string)
	h.expect(http.StatusOK, "nog", "GET", "/tasks/"+issue, nil)
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	h.expect(http.StatusNotFound, "nog", "GET", "/tasks/"+issue, nil)
	h.expect(http.StatusForbidden, "nog", "POST", "/tasks", map[string]any{"title": "closed hub"})
	if ids := backlogIDs(h.expect(http.StatusOK, "nog", "GET", "/backlogs", nil)); len(ids) != 0 {
		t.Fatalf("a person in no group lists %v once the owner group is set", ids)
	}
	if titles := h.titlesAs("nog"); len(titles) != 0 {
		t.Fatalf("a person in no group lists %v once the owner group is set", titles)
	}
	h.expect(http.StatusOK, "boss", "GET", "/tasks/"+issue, nil)
}

func TestChangingTheOwnerGroupMovesCanMoveToTheNewGroupAndKeepsTheOtherGrants(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"boss": {"owners"}, "ann": {"contributors"}, "dev": {"g-dev"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	docs := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Docs", "steps": []string{"Write", "Done"}})["id"].(string)
	issue := h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "a", "backlog": docs, "step": "write"})["id"].(string)
	h.expect(http.StatusOK, "", "PUT", "/backlogs/"+docs+"/access/contributors", map[string]string{"level": "see"})
	h.expect(http.StatusOK, "", "PUT", "/backlogs/"+docs+"/access/g-dev", map[string]string{"level": "move"})
	update := map[string]any{"title": "b", "status": "pending", "backlog": docs, "step": "write"}

	h.expect(http.StatusForbidden, "ann", "PUT", "/tasks/"+issue, update)
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "contributors"})

	h.expect(http.StatusOK, "ann", "PUT", "/tasks/"+issue, update)
	h.expect(http.StatusNotFound, "boss", "GET", "/tasks/"+issue, nil)
	h.expect(http.StatusForbidden, "boss", "PUT", "/tasks/"+issue, update)
	h.expect(http.StatusOK, "dev", "PUT", "/tasks/"+issue, update)
	access := h.expect(http.StatusOK, "", "GET", "/backlogs/"+docs+"/access", nil)
	if access["owner_group"] != "contributors" {
		t.Fatalf("owner group after the change = %v", access["owner_group"])
	}
	if ids := backlogIDs(h.expect(http.StatusOK, "boss", "GET", "/backlogs", nil)); len(ids) != 0 {
		t.Fatalf("the old owner group still lists %v", ids)
	}
}

func TestAPersonWhoLeavesTheirGroupLosesTheBacklogAndTheCallWithNoPersonCanSetANewOwnerGroup(t *testing.T) {
	groups := map[string][]string{"boss": {"owners"}, "two": {"g-dev"}}
	h := newAccessHub(t, groups)
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	docs := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Docs", "steps": []string{"Write"}})["id"].(string)
	issue := h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "a", "backlog": docs, "step": "write"})["id"].(string)
	h.expect(http.StatusOK, "", "PUT", "/backlogs/"+docs+"/access/g-dev", map[string]string{"level": "move"})
	h.expect(http.StatusOK, "two", "GET", "/tasks/"+issue, nil)

	groups["two"] = nil
	h.expect(http.StatusNotFound, "two", "GET", "/tasks/"+issue, nil)
	h.expect(http.StatusForbidden, "two", "DELETE", "/tasks/"+issue, nil)

	groups["boss"] = nil
	h.expect(http.StatusNotFound, "boss", "GET", "/tasks/"+issue, nil)
	groups["boss"] = []string{"owners"}
	h.expect(http.StatusOK, "boss", "GET", "/tasks/"+issue, nil)
	h.expect(http.StatusOK, "", "GET", "/backlogs/"+docs+"/access", nil)
}

func newFileBackedAccessHub(t *testing.T, groupsOf map[string][]string) *accessHub {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "hub.db")+"?_pragma=busy_timeout(5000)"), &gorm.Config{})
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
	mgr.UseGroups(func(_ context.Context, id string) ([]string, error) { return groupsOf[id], nil })
	mgr.UseGroupExists(func(context.Context, string) (bool, error) { return true, nil })
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

func TestEveryGrantTheHubAcknowledgesDuringSimultaneousGrantsSurvives(t *testing.T) {
	for round := 0; round < 10; round++ {
		h := newFileBackedAccessHub(t, map[string][]string{"boss": {"owners"}})
		h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
		docs := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Docs", "steps": []string{"Write"}})["id"].(string)
		var wg sync.WaitGroup
		var mu sync.Mutex
		acknowledged := map[string]bool{}
		for i := 0; i < 8; i++ {
			group := fmt.Sprintf("g-%d", i)
			wg.Add(1)
			go func() {
				defer wg.Done()
				code, _ := h.call("", "PUT", "/backlogs/"+docs+"/access/"+group, map[string]string{"level": "see"})
				if code == http.StatusOK {
					mu.Lock()
					acknowledged[group] = true
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if len(acknowledged) == 0 {
			t.Fatalf("round %d: not one of eight simultaneous grants was accepted", round)
		}
		stored := map[string]bool{}
		for _, raw := range h.expect(http.StatusOK, "", "GET", "/backlogs/"+docs+"/access", nil)["groups"].([]any) {
			stored[raw.(map[string]any)["group_id"].(string)] = true
		}
		for group := range acknowledged {
			if !stored[group] {
				t.Fatalf("round %d: grant to %s was acknowledged and is gone", round, group)
			}
		}
	}
}

func TestSourcedTasksAreRefusedToAPersonWithoutCanMoveOnTheFirstBacklog(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"boss": {"owners"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	body := map[string]any{
		"source": "feed",
		"items":  []map[string]any{{"key": "k1", "title": "from a feed", "due_at": "2030-01-01T00:00:00Z"}},
	}
	h.expect(http.StatusForbidden, "nog", "POST", "/sourced-tasks", body)
	opened := h.expect(http.StatusOK, "boss", "POST", "/sourced-tasks", body)
	if items := opened["items"].([]any); len(items) != 1 {
		t.Fatalf("sourced tasks opened %v", opened)
	}
}
