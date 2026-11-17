package routes_test

import (
	"net/http"
	"strings"
	"testing"
)

type batchIssue struct {
	code, backlog, step string
	rank                int
	tags                []string
	files               []string
}

func seedNextReadyBatchHub(t *testing.T, issues ...batchIssue) *accessHub {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	for _, name := range []string{"Software", "Hidden"} {
		h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": name, "steps": []string{"In coming", "To do", "In progress"}})
	}
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})
	for _, issue := range issues {
		made := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{
			"title": "Title " + issue.code, "code": issue.code, "rank": issue.rank, "tags": issue.tags,
			"files": issue.files, "effort": "low", "description": "a description of " + issue.code,
		})
		h.expect(http.StatusOK, "owner", "POST", "/tasks/"+made["id"].(string)+"/move-to-step", map[string]any{"backlog": issue.backlog, "step": issue.step})
	}
	return h
}

func ready(code string, rank int, files ...string) batchIssue {
	return batchIssue{code: code, backlog: "software", step: "to-do", rank: rank, tags: []string{"Ready"}, files: files}
}

func doing(code string, rank int, files ...string) batchIssue {
	return batchIssue{code: code, backlog: "software", step: "in-progress", rank: rank, files: files}
}

func batchCodes(t *testing.T, body map[string]any) string {
	t.Helper()
	codes := []string{}
	for _, item := range body["items"].([]any) {
		entry := item.(map[string]any)
		if _, has := entry["description"]; has {
			t.Errorf("an item carries a description: %v", entry)
		}
		codes = append(codes, entry["code"].(string))
	}
	return strings.Join(codes, ",")
}

func skippedPairs(body map[string]any) string {
	pairs := []string{}
	for _, raw := range body["skipped"].([]any) {
		entry := raw.(map[string]any)
		pairs = append(pairs, entry["code"].(string)+">"+entry["clashes_with"].(string))
	}
	return strings.Join(pairs, ",")
}

func wantBatch(t *testing.T, body map[string]any, items, skipped string) {
	t.Helper()
	if got := batchCodes(t, body); got != items {
		t.Errorf("items %q, want %q (%v)", got, items, body)
	}
	if got := skippedPairs(body); got != skipped {
		t.Errorf("skipped %q, want %q (%v)", got, skipped, body)
	}
}

const (
	batchMenu  = "hub-steward:code/app/src/menu.go"
	batchBrief = "hub-steward:code/wakala-api/modules/taskmanager/task_brief.go"
	batchDir   = "hub-steward:code/wakala-api/modules/taskmanager/"
	batchDocs  = "hub-steward:documentation/readme.md"
	batchOther = "hub-steward:code/app/src/other.go"
)

func TestNextReadyBatchReturnsDisjointIssuesLowestRankFirstUpToTheCount(t *testing.T) {
	h := seedNextReadyBatchHub(t,
		ready("SW3", 300, batchOther),
		ready("SW1", 100, batchMenu),
		ready("SW2", 200, batchDocs),
		ready("SW4", 400, "hub-steward:code/app/src/fourth.go"),
	)

	wantBatch(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch", nil), "SW1,SW2,SW3", "")
	wantBatch(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch?count=2", nil), "SW1,SW2", "")
	wantBatch(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch?count=10", nil), "SW1,SW2,SW3,SW4", "")
}

func TestNextReadyBatchSkipsTheLaterOfTwoClashingCandidatesAndNamesTheOtherOne(t *testing.T) {
	h := seedNextReadyBatchHub(t,
		ready("SW1", 100, batchMenu),
		ready("SW2", 200, batchMenu, batchDocs),
		ready("SW3", 300, batchOther),
		ready("SW4", 400, batchDocs),
	)

	wantBatch(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch?count=5", nil), "SW1,SW3,SW4", "SW2>SW1")
}

func TestNextReadyBatchSkipsAnIssueThatClashesWithOneInProgress(t *testing.T) {
	h := seedNextReadyBatchHub(t,
		doing("RUN1", 10, batchMenu),
		ready("SW1", 100, batchMenu),
		ready("SW2", 200, batchDocs),
	)

	body := h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch", nil)
	wantBatch(t, body, "SW2", "SW1>RUN1")
	skipped := body["skipped"].([]any)[0].(map[string]any)
	if skipped["title"] != "Title SW1" || skipped["id"] == "" {
		t.Errorf("the skipped entry is not compact: %v", skipped)
	}
}

func TestNextReadyBatchTreatsAFolderAsContainingItsFiles(t *testing.T) {
	h := seedNextReadyBatchHub(t,
		doing("RUN1", 10, batchDir),
		ready("SW1", 100, batchBrief),
		ready("SW2", 200, batchDir),
		ready("SW3", 300, batchDocs),
	)

	wantBatch(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch?count=5", nil), "SW3", "SW1>RUN1,SW2>RUN1")
}

func TestNextReadyBatchLeavesOutUnfiledIssuesUnlessAskedForAndCountsThemAsClashingWithNothing(t *testing.T) {
	h := seedNextReadyBatchHub(t,
		doing("RUN1", 10, batchMenu),
		ready("SW1", 100),
		ready("SW2", 200, batchDocs),
		ready("SW3", 300),
	)

	without := h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch?count=5", nil)
	wantBatch(t, without, "SW2", "")
	if without["unfiled_left_out"] != float64(2) {
		t.Errorf("unfiled_left_out is %v, want 2", without["unfiled_left_out"])
	}
	with := h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch?count=5&include_unfiled=true", nil)
	wantBatch(t, with, "SW1,SW2,SW3", "")
	if with["unfiled_left_out"] != float64(0) {
		t.Errorf("unfiled_left_out is %v, want 0", with["unfiled_left_out"])
	}
}

func TestNextReadyBatchSkipsSinbinBlockedWaitingAndIssuesOutsideToDo(t *testing.T) {
	h := seedNextReadyBatchHub(t,
		batchIssue{"SW1", "software", "to-do", 100, []string{"Ready", "Sinbin"}, []string{batchMenu}},
		batchIssue{"SW2", "software", "to-do", 200, []string{"Ready", "Blocked"}, []string{batchDocs}},
		batchIssue{"SW3", "software", "to-do", 300, []string{"Ready", "Waiting"}, []string{batchOther}},
		batchIssue{"SW4", "software", "in-coming", 400, []string{"Ready"}, []string{"hub-steward:a.go"}},
		batchIssue{"SW5", "software", "to-do", 500, []string{"app"}, []string{"hub-steward:b.go"}},
		ready("SW6", 600, "hub-steward:c.go"),
	)

	wantBatch(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch?count=5", nil), "SW6", "")
}

func TestNextReadyBatchNarrowsByBacklogAndTag(t *testing.T) {
	tagged := ready("SW1", 100, batchMenu)
	tagged.tags = []string{"Ready", "backend"}
	hidden := ready("HD1", 50, batchDocs)
	hidden.backlog = "hidden"
	h := seedNextReadyBatchHub(t, tagged, ready("SW2", 200, batchOther), hidden)

	wantBatch(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch?count=5", nil), "HD1,SW1,SW2", "")
	wantBatch(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch?count=5&backlog=Hidden", nil), "HD1", "")
	wantBatch(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch?count=5&tag=backend", nil), "SW1", "")
	wantBatch(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch?count=5&tag=nothing", nil), "", "")
}

func TestNextReadyBatchCountStaysBetweenOneAndTen(t *testing.T) {
	h := seedNextReadyBatchHub(t, ready("SW1", 100, batchMenu))

	for _, count := range []string{"0", "11", "-1", "many"} {
		if status, _ := h.call("owner", "GET", "/next-ready-batch?count="+count, nil); status != http.StatusBadRequest {
			t.Errorf("count %s answered %d, want 400", count, status)
		}
	}
	if status, _ := h.call("owner", "GET", "/next-ready-batch?count=10", nil); status != http.StatusOK {
		t.Errorf("count 10 answered %d, want 200", status)
	}
}

func TestNextReadyBatchRefusesAnUnknownBacklog(t *testing.T) {
	h := seedNextReadyBatchHub(t, ready("SW1", 100, batchMenu))

	if status, _ := h.call("owner", "GET", "/next-ready-batch?backlog=nowhere", nil); status != http.StatusNotFound && status != http.StatusBadRequest {
		t.Errorf("an unknown backlog answered %d, want a refusal", status)
	}
}

func TestNextReadyBatchShowsAPersonOnlyTheBacklogsTheirGroupsCanSee(t *testing.T) {
	hidden := ready("HD1", 50, batchDocs)
	hidden.backlog = "hidden"
	h := seedNextReadyBatchHub(t, hidden, ready("SW1", 100, batchMenu))

	wantBatch(t, h.expect(http.StatusOK, "dev", "GET", "/next-ready-batch?count=5", nil), "SW1", "")
	wantBatch(t, h.expect(http.StatusOK, "nobody", "GET", "/next-ready-batch?count=5", nil), "", "")
	if status, _ := h.call("dev", "GET", "/next-ready-batch?backlog=Hidden", nil); status == http.StatusOK {
		t.Errorf("a person who cannot see a backlog read it")
	}
}

func TestNextReadyBatchItemsAreCompactAndCarryTheirFiles(t *testing.T) {
	h := seedNextReadyBatchHub(t, ready("SW1", 100, batchMenu, batchDocs))

	item := h.expect(http.StatusOK, "owner", "GET", "/next-ready-batch", nil)["items"].([]any)[0].(map[string]any)
	for _, key := range []string{"id", "code", "title", "priority", "effort", "files"} {
		if _, has := item[key]; !has {
			t.Errorf("the item has no %s: %v", key, item)
		}
	}
	if files := item["files"].([]any); len(files) != 2 || files[0] != batchMenu {
		t.Errorf("files are %v", files)
	}
}
