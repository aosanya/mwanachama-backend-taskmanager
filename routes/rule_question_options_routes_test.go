package routes_test

import (
	"net/http"
	"testing"
)

func askedOverTheRoutes(h *accessHub) map[string]any {
	return h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{
		"kind": "question", "title": "Tabs or spaces?", "tags": []string{"docs"}, "target_role": "Product owner",
		"options": []map[string]any{
			{"text": "Tabs"},
			{"text": "Spaces", "recommended": true, "reason": "The formatter already writes them."},
		},
	})
}

func TestAQuestionCarriesItsRoleAndOptionsRecommendedFirstOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	made := askedOverTheRoutes(f.h)
	options, _ := made["options"].([]any)
	if made["target_role"] != "Product owner" || len(options) != 2 || options[0].(map[string]any)["text"] != "Spaces" {
		t.Fatalf("got %v", made)
	}
	if read := f.h.expect(http.StatusOK, "ann", "GET", "/rules/Q-1", nil); read["target_role"] != "Product owner" {
		t.Fatalf("read %v", read)
	}
	if got := f.list("ann", "/rules?kind=question"); len(got) != 1 || got[0]["options"] == nil {
		t.Fatalf("list %v", got)
	}
}

func TestAQuestionWithoutARoleOrWithBadOptionsIs400OverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	f.h.expect(http.StatusBadRequest, "bob", "POST", "/rules", map[string]any{"kind": "question", "title": "No role"})
	f.h.expect(http.StatusBadRequest, "bob", "POST", "/rules", map[string]any{"kind": "question", "title": "Two", "target_role": "Product owner",
		"options": []map[string]any{{"text": "a", "recommended": true, "reason": "r"}, {"text": "b", "recommended": true, "reason": "r"}}})
	f.h.expect(http.StatusBadRequest, "bob", "POST", "/rules", map[string]any{"kind": "decision", "title": "A decision", "target_role": "Product owner"})
}

func TestAnOptionAnswersAQuestionForACanSeeReaderAndRecordsNoDecision(t *testing.T) {
	f := newRuleFixture(t)
	askedOverTheRoutes(f.h)
	f.h.expect(http.StatusBadRequest, "ann", "PUT", "/rules/Q-1/answer", map[string]any{})
	f.h.expect(http.StatusBadRequest, "ann", "PUT", "/rules/Q-1/answer", map[string]any{"option": 0, "other": "x"})
	f.h.expect(http.StatusBadRequest, "ann", "PUT", "/rules/Q-1/answer", map[string]any{"option": 5})
	f.h.expect(http.StatusForbidden, "nog", "PUT", "/rules/Q-1/answer", map[string]any{"option": 0})
	got := f.h.expect(http.StatusOK, "ann", "PUT", "/rules/Q-1/answer", map[string]any{"option": 0})
	if got["status"] != "answered" || got["answer"] != "Spaces" || got["answered_by"] != nil {
		t.Fatalf("got %v", got)
	}
	if decisions := f.list("ann", "/rules?kind=decision&status=proposed&status=decided"); len(decisions) != 0 {
		t.Fatalf("decisions %v", decisions)
	}
}

func TestTypedWordsAnswerAQuestionOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	askedOverTheRoutes(f.h)
	got := f.h.expect(http.StatusOK, "bob", "PUT", "/rules/Q-1/answer", map[string]any{"other": "Ask the council."})
	if got["status"] != "answered" || got["answer"] != "Ask the council." {
		t.Fatalf("got %v", got)
	}
}
