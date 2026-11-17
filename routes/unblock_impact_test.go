package routes_test

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
)

type impactHub struct {
	*accessHub
	ids map[string]string
}

func seedImpactHub(t *testing.T, issues map[string]string) *impactHub {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	for _, name := range []string{"Software", "Hidden"} {
		h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": name, "steps": []string{"To do", "In progress", "Done"}})
	}
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})
	out := &impactHub{accessHub: h, ids: map[string]string{}}
	codes := make([]string, 0, len(issues))
	for code := range issues {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for rank, code := range codes {
		made := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{
			"title": "Title " + code, "code": code, "rank": (rank + 1) * 100, "effort": "low", "description": "a description of " + code,
		})
		id := made["id"].(string)
		out.ids[code] = id
		backlog, step := "software", "to-do"
		if parts := strings.Split(issues[code], "/"); len(parts) == 2 {
			backlog, step = parts[0], parts[1]
		}
		h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": backlog, "step": step})
	}
	return out
}

func (h *impactHub) waits(waiter, blocker string) {
	h.expect(http.StatusCreated, "owner", "POST", "/relationships", map[string]any{"label": "depends_on", "from_id": h.ids[waiter], "to_id": h.ids[blocker]})
}

func impactRows(body map[string]any) string {
	rows := []string{}
	for _, raw := range body["items"].([]any) {
		entry := raw.(map[string]any)
		dependents := []string{}
		for _, code := range entry["dependents"].([]any) {
			dependents = append(dependents, code.(string))
		}
		rows = append(rows, fmt.Sprintf("%s:%v/%v[%s]", entry["code"], entry["direct"], entry["total"], strings.Join(dependents, "+")))
	}
	return strings.Join(rows, " ")
}

func wantImpact(t *testing.T, body map[string]any, want string) {
	t.Helper()
	if got := impactRows(body); got != want {
		t.Errorf("impact %q, want %q (%v)", got, want, body)
	}
}

func TestUnblockImpactCountsAChainDownToItsLastLink(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": "", "C": ""})
	h.waits("B", "A")
	h.waits("C", "B")

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil)

	wantImpact(t, body, "A:1/2[B+C] B:1/1[C]")
}

func TestUnblockImpactItemsAreCompact(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": ""})
	h.waits("B", "A")

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil)

	item := body["items"].([]any)[0].(map[string]any)
	for _, field := range []string{"id", "code", "title", "step", "priority", "direct", "total", "dependents"} {
		if _, has := item[field]; !has {
			t.Errorf("an item lacks %q: %v", field, item)
		}
	}
	if _, has := item["description"]; has {
		t.Errorf("an item carries a description: %v", item)
	}
	if item["step"] != "To do" {
		t.Errorf("step %v, want To do", item["step"])
	}
}

func TestUnblockImpactRanksTheBiggestUnblockerFirst(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": "", "C": "", "D": "", "E": ""})
	h.waits("B", "A")
	h.waits("C", "A")
	h.waits("E", "D")

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil)

	wantImpact(t, body, "A:2/2[B+C] D:1/1[E]")
}

func TestUnblockImpactCountsEachDependentOnceWhenTwoPathsMeet(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": "", "C": "", "D": ""})
	h.waits("B", "A")
	h.waits("C", "A")
	h.waits("D", "B")
	h.waits("D", "C")

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil)

	wantImpact(t, body, "A:2/3[B+C+D] B:1/1[D] C:1/1[D]")
}

func TestUnblockImpactAnswersOnACycleWithoutLooping(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": "", "C": ""})
	h.waits("B", "A")
	h.waits("C", "B")
	h.waits("A", "C")

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil)

	wantImpact(t, body, "A:1/2[B+C] B:1/2[A+C] C:1/2[A+B]")
}

func TestUnblockImpactDoesNotListAnIssueNothingWaitsOn(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": "", "Lonely": ""})
	h.waits("B", "A")

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil)

	wantImpact(t, body, "A:1/1[B]")
}

func TestUnblockImpactDoesNotListADoneBlocker(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "software/done", "B": ""})
	h.waits("B", "A")

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil)

	wantImpact(t, body, "")
}

func TestUnblockImpactDoesNotCountADoneDependent(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": "software/done", "C": ""})
	h.waits("B", "A")
	h.waits("C", "A")

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil)

	wantImpact(t, body, "A:1/1[C]")
}

func TestUnblockImpactDoesNotLetADoneIssueCarryTheChain(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": "software/done", "C": ""})
	h.waits("B", "A")
	h.waits("C", "B")

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil)

	wantImpact(t, body, "")
}

func TestUnblockImpactHonoursTheLimit(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": "", "C": "", "D": "", "E": ""})
	h.waits("B", "A")
	h.waits("C", "A")
	h.waits("D", "B")
	h.waits("E", "C")

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact?limit=2", nil)

	wantImpact(t, body, "A:2/4[B+C+D+E] B:1/1[D]")
}

func TestUnblockImpactLimitDefaultsToTenAndStopsAtFifty(t *testing.T) {
	issues := map[string]string{"Z00": ""}
	for i := 1; i <= 12; i++ {
		issues[fmt.Sprintf("W%02d", i)] = ""
		issues[fmt.Sprintf("V%02d", i)] = ""
	}
	h := seedImpactHub(t, issues)
	for i := 1; i <= 12; i++ {
		h.waits(fmt.Sprintf("W%02d", i), fmt.Sprintf("V%02d", i))
	}

	defaulted := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil)
	if got := len(defaulted["items"].([]any)); got != 10 {
		t.Errorf("default limit gave %d items, want 10", got)
	}
	h.expect(http.StatusOK, "owner", "GET", "/unblock-impact?limit=50", nil)
	h.expect(http.StatusBadRequest, "owner", "GET", "/unblock-impact?limit=51", nil)
}

func TestUnblockImpactNarrowsByBacklog(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": "", "H1": "hidden/to-do", "H2": "hidden/to-do"})
	h.waits("B", "A")
	h.waits("H2", "H1")

	wantImpact(t, h.expect(http.StatusOK, "owner", "GET", "/unblock-impact?backlog=hidden", nil), "H1:1/1[H2]")
	wantImpact(t, h.expect(http.StatusOK, "owner", "GET", "/unblock-impact?backlog=software", nil), "A:1/1[B]")
}

func TestUnblockImpactRefusesABacklogTheCallerCannotSee(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": ""})

	if status, _ := h.call("dev", "GET", "/unblock-impact?backlog=hidden", nil); status != http.StatusNotFound && status != http.StatusBadRequest {
		t.Errorf("an unseen backlog answered %d, want 404 or 400", status)
	}
}

func TestUnblockImpactDoesNotCountAnIssueTheCallerCannotSee(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": "", "H": "hidden/to-do"})
	h.waits("B", "A")
	h.waits("H", "A")

	wantImpact(t, h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil), "A:2/2[B+H]")
	wantImpact(t, h.expect(http.StatusOK, "dev", "GET", "/unblock-impact", nil), "A:1/1[B]")
}

func TestUnblockImpactNarrowsByTag(t *testing.T) {
	h := seedImpactHub(t, map[string]string{"A": "", "B": "", "C": "", "D": ""})
	h.waits("B", "A")
	h.waits("D", "C")
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+h.ids["C"], map[string]any{"tags": []string{"Urgent"}})

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact?tag=Urgent", nil)

	wantImpact(t, body, "C:1/1[D]")
}

func TestUnblockImpactOnAnEmptyBoardIsEmptyNotNull(t *testing.T) {
	h := seedImpactHub(t, map[string]string{})

	body := h.expect(http.StatusOK, "owner", "GET", "/unblock-impact", nil)

	if items, ok := body["items"].([]any); !ok || len(items) != 0 {
		t.Errorf("items %v, want an empty list", body["items"])
	}
}
