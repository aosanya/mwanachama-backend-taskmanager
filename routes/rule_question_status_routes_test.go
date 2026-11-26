package routes_test

import (
	"net/http"
	"strings"
	"testing"
)

func questionCodesOf(t *testing.T, f *ruleFixture, path string) []string {
	t.Helper()
	got := f.h.expect(http.StatusOK, "ann", "GET", path, nil)
	var codes []string
	items, _ := got["questions"].([]any)
	for _, item := range items {
		codes = append(codes, item.(map[string]any)["code"].(string))
	}
	return codes
}

func TestTheQuestionsOfAnIssueTakeAStatusOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	askedOverTheRoutes(f.h)
	f.h.expect(http.StatusNoContent, "bob", "POST", "/rules/Q-1/links", map[string]any{"issue_id": f.issue, "relation": "applies_to"})
	path := "/tasks/" + f.issue + "/rules"
	if codes := questionCodesOf(t, f, path); strings.Join(codes, ",") != "Q-1" {
		t.Fatalf("open by default: %v", codes)
	}
	f.h.expect(http.StatusOK, "bob", "PUT", "/rules/Q-1/answer", map[string]any{"option": 0})
	if codes := questionCodesOf(t, f, path); len(codes) != 0 {
		t.Fatalf("an answered question under the default: %v", codes)
	}
	if codes := questionCodesOf(t, f, path+"?status=answered"); strings.Join(codes, ",") != "Q-1" {
		t.Fatalf("answered: %v", codes)
	}
	if codes := questionCodesOf(t, f, path+"?status=open&status=answered&status=dropped"); strings.Join(codes, ",") != "Q-1" {
		t.Fatalf("all: %v", codes)
	}
	f.h.expect(http.StatusBadRequest, "ann", "GET", path+"?status=bogus", nil)
}

func TestAnAnsweredQuestionKeepsItsEarlierAnswerInTheLogOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	askedOverTheRoutes(f.h)
	f.h.expect(http.StatusOK, "bob", "PUT", "/rules/Q-1/answer", map[string]any{"option": 0})
	again := f.h.expect(http.StatusOK, "bob", "PUT", "/rules/Q-1/answer", map[string]any{"other": "Ask the council."})
	if again["answer"] != "Ask the council." {
		t.Fatalf("got %v", again)
	}
	code, raw := f.h.rawGet("ann", "/rules/notes?rule=Q-1")
	if code != http.StatusOK || !strings.Contains(string(raw), "Earlier answer: Spaces") {
		t.Fatalf("notes %d %s", code, raw)
	}
}

func TestOptionsAndRoleOfAQuestionAreEditedByPatchOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	askedOverTheRoutes(f.h)
	f.h.expect(http.StatusOK, "bob", "PUT", "/rules/Q-1/answer", map[string]any{"option": 0})
	got := f.h.expect(http.StatusOK, "bob", "PATCH", "/rules/Q-1", map[string]any{
		"target_role": "Council",
		"options":     []map[string]any{{"text": "Yes"}, {"text": "No"}},
	})
	options, _ := got["options"].([]any)
	if got["target_role"] != "Council" || len(options) != 2 || got["status"] != "answered" {
		t.Fatalf("got %v", got)
	}
	f.h.expect(http.StatusBadRequest, "bob", "PATCH", "/rules/Q-1", map[string]any{"target_role": "  "})
	seven := []map[string]any{}
	for _, text := range strings.Fields("a b c d e f g") {
		seven = append(seven, map[string]any{"text": text})
	}
	f.h.expect(http.StatusBadRequest, "bob", "PATCH", "/rules/Q-1", map[string]any{"options": seven})
}
