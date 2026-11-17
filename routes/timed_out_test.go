package routes_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const noteStamp = "2006-01-02T15:04:05.000000000Z07:00"

type timedOutFixture struct {
	h      *accessHub
	db     *gorm.DB
	notes  []string
	issues map[string]string
}

func newTimedOutFixture(t *testing.T) *timedOutFixture {
	t.Helper()
	dsn := "file:timed_out_" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	h := newAccessHubOn(t, dsn, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	const noteTable = `SELECT m.name FROM sqlite_master m WHERE m.type = 'table' AND (SELECT COUNT(*) FROM pragma_table_info(m.name) p WHERE p.name IN ('task_id', 'author_id', 'automatic', 'links')) = 4`
	if err := db.Raw(noteTable).Scan(&tables).Error; err != nil || len(tables) == 0 {
		t.Fatalf("no task notes table: %v", err)
	}
	f := &timedOutFixture{h: h, db: db, notes: tables, issues: map[string]string{}}
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Book", "steps": []string{"Open", "In progress", "Done"}})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Elsewhere", "steps": []string{"Open", "In progress"}})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/book/access/g-dev", map[string]string{"level": "see"})
	return f
}

func (f *timedOutFixture) issue(code, backlog, step string, statusInProgress bool) string {
	f.h.t.Helper()
	made := f.h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "issue " + code, "code": code, "backlog": backlog, "step": step})
	id := made["id"].(string)
	f.issues[code] = id
	if statusInProgress {
		f.h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"status": "in_progress"})
	}
	return id
}

func (f *timedOutFixture) logAt(code, text string, ago time.Duration) {
	f.h.t.Helper()
	added := f.h.expect(http.StatusCreated, "owner", "POST", "/tasks/"+f.issues[code]+"/notes", map[string]any{"kind": "worker", "text": text})
	stamp := time.Now().UTC().Add(-ago).Format(noteStamp)
	var rows int64
	for _, table := range f.notes {
		changed := f.db.Exec("UPDATE "+table+" SET created_at = ? WHERE id = ?", stamp, added["id"])
		if changed.Error != nil {
			f.h.t.Fatal(changed.Error)
		}
		rows += changed.RowsAffected
	}
	if rows != 1 {
		f.h.t.Fatalf("backdating %v changed %d rows", added["id"], rows)
	}
}

func seedTimedOut(t *testing.T) *timedOutFixture {
	t.Helper()
	f := newTimedOutFixture(t)
	f.issue("LATE", "book", "in-progress", false)
	f.logAt("LATE", "Taken.", 20*time.Minute)
	f.logAt("LATE", "Todo 1 of 2 done: the failing test, and a very long line that goes on and on "+strings.Repeat("more words ", 30), 3*time.Minute)
	f.issue("FRESH", "book", "in-progress", true)
	f.logAt("FRESH", "Taken.", 5*time.Minute)
	f.issue("OWNCAP", "book", "in-progress", true)
	f.logAt("OWNCAP", "Taken.", 20*time.Minute)
	f.h.expect(http.StatusOK, "owner", "PUT", "/tasks/"+f.issues["OWNCAP"]+"/caps", map[string]any{"runtime_seconds": 3600})
	f.issue("NOLOG", "book", "in-progress", true)
	f.issue("WAITING", "book", "open", false)
	f.logAt("WAITING", "Taken.", 40*time.Minute)
	f.issue("STATUSONLY", "book", "open", true)
	f.logAt("STATUSONLY", "Taken.", 40*time.Minute)
	f.issue("OTHER", "elsewhere", "in-progress", true)
	f.logAt("OTHER", "Taken.", 40*time.Minute)
	return f
}

func timedOutCodes(body map[string]any) string {
	var codes []string
	for _, raw := range body["items"].([]any) {
		codes = append(codes, raw.(map[string]any)["code"].(string))
	}
	return strings.Join(codes, ",")
}

func timedOutItem(t *testing.T, body map[string]any, code string) map[string]any {
	t.Helper()
	for _, raw := range body["items"].([]any) {
		if item := raw.(map[string]any); item["code"] == code {
			return item
		}
	}
	t.Fatalf("no item %s in %v", code, body)
	return nil
}

func TestTimedOutReturnsTheInProgressIssuesPastTheirCapAndTheOneWithNoLogOnly(t *testing.T) {
	f := seedTimedOut(t)

	body := f.h.expect(http.StatusOK, "owner", "GET", "/timed-out?backlog=book", nil)

	if got := timedOutCodes(body); got != "LATE,NOLOG" && got != "NOLOG,LATE" {
		t.Fatalf("got %s, want LATE and NOLOG", got)
	}
}

func TestTimedOutNamesTheMinutesTheCapAndTheLastEntryOfAnIssue(t *testing.T) {
	f := seedTimedOut(t)

	body := f.h.expect(http.StatusOK, "owner", "GET", "/timed-out?backlog=Book", nil)

	late := timedOutItem(t, body, "LATE")
	if late["id"] != f.issues["LATE"] || late["title"] != "issue LATE" {
		t.Errorf("identity %v", late)
	}
	if late["minutes_since_first_entry"] != float64(20) || late["cap_minutes"] != float64(15) || late["minutes_since_last_entry"] != float64(3) {
		t.Errorf("minutes %v", late)
	}
	if late["clock_stopped"] != false {
		t.Errorf("clock_stopped %v", late["clock_stopped"])
	}
	last := late["last_entry"].(map[string]any)
	text := last["text"].(string)
	if last["kind"] != "worker" || !strings.HasPrefix(text, "Todo 1 of 2 done") || len([]rune(text)) != 200 || last["created_at"] == "" {
		t.Errorf("last entry %v", last)
	}
}

func TestTimedOutReportsAnIssueWithNoWorkerEntryWithNullFirstEntryAndOnlyTheHubsOwnLastEntry(t *testing.T) {
	f := seedTimedOut(t)

	body := f.h.expect(http.StatusOK, "owner", "GET", "/timed-out?backlog=book", nil)

	empty := timedOutItem(t, body, "NOLOG")
	if v, has := empty["minutes_since_first_entry"]; !has || v != nil {
		t.Errorf("minutes_since_first_entry %v present %v, want null", v, has)
	}
	last, _ := empty["last_entry"].(map[string]any)
	if last["kind"] != "automatic" || empty["cap_minutes"] != float64(15) {
		t.Errorf("no-worker-entry item %v", empty)
	}
}

func TestTimedOutUsesTheIssueOwnCapAndLeavesAnIssueUnderItOut(t *testing.T) {
	f := seedTimedOut(t)
	f.h.expect(http.StatusOK, "owner", "PUT", "/tasks/"+f.issues["OWNCAP"]+"/caps", map[string]any{"runtime_seconds": 600})

	body := f.h.expect(http.StatusOK, "owner", "GET", "/timed-out?backlog=book", nil)

	own := timedOutItem(t, body, "OWNCAP")
	if own["cap_minutes"] != float64(10) || own["minutes_since_first_entry"] != float64(20) {
		t.Errorf("own cap item %v", own)
	}
}

func TestTimedOutReportsTheClockAsTheCapsToolDoes(t *testing.T) {
	f := seedTimedOut(t)
	f.h.expect(http.StatusOK, "owner", "PUT", "/tasks/"+f.issues["LATE"]+"/usage", map[string]any{"input_tokens": 10, "output_tokens": 10, "seconds_used": 30, "clock_stopped": true})

	body := f.h.expect(http.StatusOK, "owner", "GET", "/timed-out?backlog=book", nil)

	if timedOutItem(t, body, "LATE")["clock_stopped"] != true {
		t.Errorf("clock_stopped %v", timedOutItem(t, body, "LATE"))
	}
	if timedOutItem(t, body, "NOLOG")["clock_stopped"] != false {
		t.Errorf("an issue no worker has run on reads %v", timedOutItem(t, body, "NOLOG"))
	}
}

func TestTimedOutMovesAndTagsNothing(t *testing.T) {
	f := seedTimedOut(t)

	f.h.expect(http.StatusOK, "owner", "GET", "/timed-out?backlog=book", nil)

	read := f.h.expect(http.StatusOK, "owner", "GET", "/tasks/"+f.issues["LATE"], nil)
	if read["status"] != "pending" || read["step"] != "in-progress" {
		t.Errorf("status %v", read["status"])
	}
	if notes := f.h.notesAs("owner", "/tasks/"+f.issues["LATE"]+"/notes"); len(notes) != 2 {
		t.Errorf("notes %v", notes)
	}
}

func TestTimedOutRefusesAMissingBacklogAsABadRequest(t *testing.T) {
	f := seedTimedOut(t)

	for _, path := range []string{"/timed-out", "/timed-out?backlog=", "/timed-out?backlog=%20"} {
		if status, body := f.h.call("owner", "GET", path, nil); status != http.StatusBadRequest {
			t.Errorf("%s answered %d %v, want 400", path, status, body)
		}
	}
}

func TestTimedOutRefusesABacklogThePersonCannotSeeExactlyAsAnUnknownOne(t *testing.T) {
	f := seedTimedOut(t)

	hiddenStatus, hidden := f.h.call("dev", "GET", "/timed-out?backlog=elsewhere", nil)
	unknownStatus, unknown := f.h.call("dev", "GET", "/timed-out?backlog=nothing-here", nil)

	if hiddenStatus != http.StatusBadRequest || unknownStatus != http.StatusBadRequest {
		t.Fatalf("hidden answered %d %v, unknown answered %d %v", hiddenStatus, hidden, unknownStatus, unknown)
	}
	hiddenMessage, _ := hidden["error"].(string)
	unknownMessage, _ := unknown["error"].(string)
	if !strings.Contains(unknownMessage, "unknown backlog") || strings.ReplaceAll(hiddenMessage, "elsewhere", "nothing-here") != unknownMessage {
		t.Errorf("the hidden backlog is told apart from an unknown one: %q and %q", hiddenMessage, unknownMessage)
	}
	if status, _ := f.h.call("nobody", "GET", "/timed-out?backlog=book", nil); status != http.StatusBadRequest {
		t.Errorf("a person with no access got %d, want 400", status)
	}
	if status, body := f.h.call("dev", "GET", "/timed-out?backlog=book", nil); status != http.StatusOK || len(body["items"].([]any)) != 2 {
		t.Errorf("a person who can see the backlog got %d %v", status, body)
	}
}

func TestTimedOutHasNoItemsWhenNothingIsPastItsCap(t *testing.T) {
	f := newTimedOutFixture(t)
	f.issue("FRESH", "book", "in-progress", true)
	f.logAt("FRESH", "Taken.", time.Minute)

	body := f.h.expect(http.StatusOK, "owner", "GET", "/timed-out?backlog=book", nil)

	if items, ok := body["items"].([]any); !ok || len(items) != 0 {
		t.Errorf("got %v, want an empty items list", body)
	}
}
