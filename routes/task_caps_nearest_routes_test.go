package routes_test

import (
	"net/http"
	"testing"
)

func TestTheNearestCapOfManyIssuesIsReadInOneCallByIdsOrByBacklog(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.h.expect(http.StatusOK, "boss", "PUT", "/tasks/"+f.issue+"/usage", map[string]any{"input_tokens": 400000, "output_tokens": 10, "seconds_used": 5})

	byIDs := f.h.expect(http.StatusOK, "ann", "GET", "/caps-nearest?task_id="+f.issue+"&task_id=no-such-issue", nil)
	items := byIDs["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("got %v", byIDs)
	}
	item := items[0].(map[string]any)
	nearest := item["nearest"].(map[string]any)
	if item["task_id"] != f.issue || nearest["key"] != "input_tokens" || nearest["used"] != float64(400000) || nearest["cap"] != float64(500000) || nearest["ratio"] != 0.8 {
		t.Fatalf("got %v", item)
	}

	byBacklog := f.h.expect(http.StatusOK, "ann", "GET", "/caps-nearest?backlog="+f.docs, nil)
	if len(byBacklog["items"].([]any)) != 1 {
		t.Fatalf("got %v", byBacklog)
	}
}

func TestTheNearestCapLeavesOutIssuesTheCallerCannotSeeAndRefusesAnEmptyAsk(t *testing.T) {
	f := newMoreWritesFixture(t)

	hidden := f.h.expect(http.StatusOK, "nog", "GET", "/caps-nearest?task_id="+f.issue, nil)
	if len(hidden["items"].([]any)) != 0 {
		t.Fatalf("got %v", hidden)
	}
	f.h.expect(http.StatusBadRequest, "ann", "GET", "/caps-nearest", nil)
	f.h.expect(http.StatusBadRequest, "ann", "GET", "/caps-nearest?backlog=Imaginary", nil)
}
