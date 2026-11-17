package routes_test

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
)

const (
	menuFile   = "hub-steward:code/app/src/menu.go"
	managerDir = "hub-steward:code/wakala-api/modules/taskmanager/"
	briefFile  = "hub-steward:code/wakala-api/modules/taskmanager/task_brief.go"
)

type overlapFixture struct {
	h      *accessHub
	issues map[string]string
}

func newOverlapFixture(t *testing.T) *overlapFixture {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	f := &overlapFixture{h: h, issues: map[string]string{}}
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	for _, name := range []string{"Book", "Elsewhere", "Hidden"} {
		h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": name, "steps": []string{"Open", "In progress", "Done"}})
	}
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/book/access/g-dev", map[string]string{"level": "see"})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/elsewhere/access/g-dev", map[string]string{"level": "see"})
	return f
}

func (f *overlapFixture) issue(code, backlog, step string, files ...string) string {
	f.h.t.Helper()
	made := f.h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{
		"title": "issue " + code, "code": code, "backlog": backlog, "step": step, "files": files,
	})
	id := made["id"].(string)
	f.issues[code] = id
	return id
}

func seedOverlaps(t *testing.T) *overlapFixture {
	t.Helper()
	f := newOverlapFixture(t)
	f.issue("ASKED", "book", "in-progress", menuFile, managerDir)
	f.issue("SAMEFILE", "elsewhere", "in-progress", menuFile, "hub-steward:docs/other.md")
	f.issue("INFOLDER", "book", "in-progress", briefFile)
	f.issue("NOTHING", "book", "in-progress", "hub-steward:documentation/readme.md")
	f.issue("HIDDEN", "hidden", "in-progress", menuFile)
	f.issue("NOTYET", "book", "open", menuFile)
	f.issue("OTHERREPO", "book", "in-progress", "other-repo:code/app/src/menu.go")
	f.issue("NOFILES", "book", "in-progress")
	return f
}

func overlapCodes(body map[string]any) string {
	var codes []string
	for _, raw := range body["items"].([]any) {
		codes = append(codes, raw.(map[string]any)["code"].(string))
	}
	sort.Strings(codes)
	return strings.Join(codes, ",")
}

func overlapItem(t *testing.T, body map[string]any, code string) map[string]any {
	t.Helper()
	for _, raw := range body["items"].([]any) {
		if item := raw.(map[string]any); item["code"] == code {
			return item
		}
	}
	t.Fatalf("no item %s in %v", code, body)
	return nil
}

func overlapPairs(item map[string]any) []string {
	var pairs []string
	for _, raw := range item["overlapping"].([]any) {
		pair := raw.(map[string]any)
		pairs = append(pairs, pair["asked"].(string)+" ~ "+pair["theirs"].(string))
	}
	sort.Strings(pairs)
	return pairs
}

func TestFileOverlapsListsTheInProgressIssuesSharingAFileOrAFolderAndNothingElse(t *testing.T) {
	f := seedOverlaps(t)

	body := f.h.expect(http.StatusOK, "dev", "GET", "/file-overlaps?taskID="+f.issues["ASKED"], nil)

	if got := overlapCodes(body); got != "INFOLDER,SAMEFILE" {
		t.Fatalf("got %s, want INFOLDER and SAMEFILE", got)
	}
}

func TestFileOverlapsNamesTheOverlappingPairsAndTheIssueIdentity(t *testing.T) {
	f := seedOverlaps(t)

	body := f.h.expect(http.StatusOK, "dev", "GET", "/file-overlaps?taskID="+f.issues["ASKED"], nil)

	same := overlapItem(t, body, "SAMEFILE")
	if same["id"] != f.issues["SAMEFILE"] || same["title"] != "issue SAMEFILE" || same["step"] != "In progress" {
		t.Errorf("identity %v", same)
	}
	if got := strings.Join(overlapPairs(same), "|"); got != menuFile+" ~ "+menuFile {
		t.Errorf("same file pairs = %s", got)
	}
	inFolder := overlapItem(t, body, "INFOLDER")
	if got := strings.Join(overlapPairs(inFolder), "|"); got != managerDir+" ~ "+briefFile {
		t.Errorf("folder pairs = %s", got)
	}
}

func TestFileOverlapsNeverListsTheIssueAgainstItself(t *testing.T) {
	f := seedOverlaps(t)

	body := f.h.expect(http.StatusOK, "owner", "GET", "/file-overlaps?taskID="+f.issues["INFOLDER"], nil)

	if got := overlapCodes(body); got != "ASKED" {
		t.Fatalf("got %s, want ASKED only", got)
	}
}

func TestFileOverlapsDoesNotCountAnIssueInABacklogTheCallerCannotSee(t *testing.T) {
	f := seedOverlaps(t)

	asOwner := f.h.expect(http.StatusOK, "owner", "GET", "/file-overlaps?taskID="+f.issues["ASKED"], nil)
	asDev := f.h.expect(http.StatusOK, "dev", "GET", "/file-overlaps?taskID="+f.issues["ASKED"], nil)

	if got := overlapCodes(asOwner); got != "HIDDEN,INFOLDER,SAMEFILE" {
		t.Errorf("owner sees %s", got)
	}
	if got := overlapCodes(asDev); got != "INFOLDER,SAMEFILE" {
		t.Errorf("dev sees %s", got)
	}
}

func TestFileOverlapsWorksFromAListOfFilesWithNoIssue(t *testing.T) {
	f := seedOverlaps(t)
	query := url.Values{"files": {menuFile, "hub-steward:code/wakala-api/"}}

	body := f.h.expect(http.StatusOK, "dev", "GET", "/file-overlaps?"+query.Encode(), nil)

	if got := overlapCodes(body); got != "ASKED,INFOLDER,SAMEFILE" {
		t.Fatalf("got %s", got)
	}
	if got := strings.Join(overlapPairs(overlapItem(t, body, "INFOLDER")), "|"); got != "hub-steward:code/wakala-api/ ~ "+briefFile {
		t.Errorf("folder pairs = %s", got)
	}
}

func TestFileOverlapsOfAnIssueWithNoFilesIsEmpty(t *testing.T) {
	f := seedOverlaps(t)

	body := f.h.expect(http.StatusOK, "owner", "GET", "/file-overlaps?taskID="+f.issues["NOFILES"], nil)

	if items := body["items"].([]any); len(items) != 0 {
		t.Errorf("items = %v", items)
	}
}

func TestFileOverlapsNeedsAnIssueOrFilesAndRefusesBadEntries(t *testing.T) {
	f := seedOverlaps(t)

	f.h.expect(http.StatusBadRequest, "owner", "GET", "/file-overlaps", nil)
	f.h.expect(http.StatusBadRequest, "owner", "GET", "/file-overlaps?files=/etc/passwd", nil)
	f.h.expect(http.StatusNotFound, "owner", "GET", "/file-overlaps?taskID=missing", nil)
	f.h.expect(http.StatusNotFound, "nobody", "GET", "/file-overlaps?taskID="+f.issues["ASKED"], nil)
}
