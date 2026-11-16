package routes_test

import (
	"net/http"
	"testing"
)

func TestTheCapsOfAnIssueAreReadSetClearedAndTheUsageWrittenThroughTheRoutes(t *testing.T) {
	f := newMoreWritesFixture(t)
	caps := "/tasks/" + f.issue + "/caps"
	usage := "/tasks/" + f.issue + "/usage"

	first := f.h.expect(http.StatusOK, "ann", "GET", caps, nil)
	in := first["in_force"].(map[string]any)
	if in["input_tokens"] != float64(500000) || in["output_tokens"] != float64(100000) || in["runtime_seconds"] != float64(900) || first["used"] != nil || first["own"] != nil {
		t.Fatalf("got %v", first)
	}

	set := f.h.expect(http.StatusOK, "boss", "PUT", caps, map[string]any{"input_tokens": 2000, "runtime_seconds": 60})
	in = set["in_force"].(map[string]any)
	if in["input_tokens"] != float64(2000) || in["output_tokens"] != float64(100000) || in["runtime_seconds"] != float64(60) {
		t.Fatalf("got %v", set)
	}

	written := f.h.expect(http.StatusOK, "boss", "PUT", usage, map[string]any{"input_tokens": 1500, "output_tokens": 20, "seconds_used": 30, "clock_stopped": true})
	used := written["used"].(map[string]any)
	if used["input_tokens"] != float64(1500) || used["seconds_used"] != float64(30) || used["clock_stopped"] != true || used["updated_at"] == "" {
		t.Fatalf("got %v", written)
	}
	if read := f.h.expect(http.StatusOK, "ann", "GET", caps, nil); read["used"].(map[string]any)["input_tokens"] != float64(1500) {
		t.Fatalf("got %v", read)
	}

	cleared := f.h.expect(http.StatusOK, "boss", "DELETE", caps, nil)
	if cleared["own"] != nil || cleared["in_force"].(map[string]any)["input_tokens"] != float64(500000) {
		t.Fatalf("got %v", cleared)
	}
}

func TestACanSeeReaderReadsTheCapsButCannotChangeThemOrWriteTheUsage(t *testing.T) {
	f := newMoreWritesFixture(t)
	caps := "/tasks/" + f.issue + "/caps"

	f.h.expect(http.StatusOK, "ann", "GET", caps, nil)
	f.h.expect(http.StatusForbidden, "ann", "PUT", caps, map[string]any{"input_tokens": 5})
	f.h.expect(http.StatusForbidden, "ann", "DELETE", caps, nil)
	f.h.expect(http.StatusForbidden, "ann", "PUT", "/tasks/"+f.issue+"/usage", map[string]any{"input_tokens": 5})
	f.h.expect(http.StatusNotFound, "nog", "GET", caps, nil)
}

func TestTheDefaultCapsAreReadByAnyoneAndSetOnlyByTheProductOwnerGroup(t *testing.T) {
	f := newMoreWritesFixture(t)

	f.h.expect(http.StatusOK, "ann", "GET", "/worker-caps", nil)
	f.h.expect(http.StatusForbidden, "ann", "PUT", "/worker-caps", map[string]any{"runtime_seconds": 1200})
	set := f.h.expect(http.StatusOK, "boss", "PUT", "/worker-caps", map[string]any{"runtime_seconds": 1200})
	if set["runtime_seconds"] != float64(1200) || set["input_tokens"] != float64(500000) {
		t.Fatalf("got %v", set)
	}
	f.h.expect(http.StatusBadRequest, "boss", "PUT", "/worker-caps", map[string]any{"input_tokens": 0})
	f.h.expect(http.StatusBadRequest, "boss", "PUT", "/tasks/"+f.issue+"/caps", map[string]any{})
}
