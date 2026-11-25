package routes_test

import (
	"net/http"
	"testing"
)

func TestACanSeeCallerCannotLinkOrUnlinkARuleThroughTheRelationshipRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	rule := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Write plainly"})["id"].(string)

	for _, label := range []string{"applies_to", "decided_in", "broken_by"} {
		h.expect(http.StatusForbidden, "ann", "POST", "/relationships", map[string]string{"label": label, "from_id": rule, "to_id": f.issue})
	}
	if got := f.list("ann", "/rules/R-1/issues"); len(got) != 0 {
		t.Fatalf("a Can see caller linked a rule: %v", got)
	}

	h.expect(http.StatusCreated, "bob", "POST", "/relationships", map[string]string{"label": "applies_to", "from_id": rule, "to_id": f.issue})
	h.expect(http.StatusForbidden, "ann", "DELETE", "/relationships?label=applies_to&from_id="+rule+"&to_id="+f.issue, nil)
	if got := f.list("ann", "/rules/R-1/issues"); len(got) != 1 {
		t.Fatalf("a Can see caller unlinked a rule: %v", got)
	}
	h.expect(http.StatusNoContent, "bob", "DELETE", "/relationships?label=applies_to&from_id="+rule+"&to_id="+f.issue, nil)
}
