package routes_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

const (
	routeBank       = "c0100000-0000-4000-8000-000000000001"
	routeFederation = "c0200000-0000-4000-8000-000000000002"
	routeFundOne    = "d0100000-0000-4000-8000-000000000001"
	routeFundTwo    = "d0300000-0000-4000-8000-000000000003"
)

func (h *hub) projectsAt(path string) []map[string]any {
	h.t.Helper()
	req := httptest.NewRequest("GET", path, strings.NewReader(""))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body.String())
	}
	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		h.t.Fatalf("GET %s: %v: %s", path, err, rec.Body.String())
	}
	return items
}

func projectKeysOf(items []map[string]any) []string {
	var keys []string
	for _, item := range items {
		keys = append(keys, item["project_name"].(string))
	}
	return keys
}

func TestProjectPartyRoutes_ListByDonorClientAndContractReference(t *testing.T) {
	h := newHub(t)
	make := func(name, reference string) string {
		created := h.expect(http.StatusCreated, "POST", "/projects", map[string]any{"name": name, "contract_reference": reference})
		return created["id"].(string)
	}
	scorecard := make("inclusion scorecard", "NLDF/TA/2026/014")
	training := make("savings group training", "NLDF/TA/2026/021")
	party := func(projectID, role, row, name string) {
		h.expect(http.StatusCreated, "POST", "/projects/"+projectID+"/parties", map[string]any{"role": role, "record_row_id": row, "party_name": name})
	}
	party(scorecard, "client", routeBank, "Lakeshore Central Bank")
	party(scorecard, "donor", routeFundOne, "Northern Lakes Development Fund")
	party(training, "client", routeFederation, "Highland Farmers' Savings Federation")
	party(training, "donor", routeFundOne, "Northern Lakes Development Fund")
	party(training, "donor", routeFundTwo, "Harbour Commission for Development")

	byDonor := h.projectsAt("/projects?party_role=donor&party_row_id=" + routeFundOne)
	if got := strings.Join(projectKeysOf(byDonor), ","); got != "inclusion_scorecard,savings_group_training" {
		t.Errorf("by the first donor: %s", got)
	}
	if got := strings.Join(projectKeysOf(h.projectsAt("/projects?party_role=donor&party_row_id="+routeFundTwo)), ","); got != "savings_group_training" {
		t.Errorf("by the second donor: %s", got)
	}
	if got := strings.Join(projectKeysOf(h.projectsAt("/projects?party_role=client&party_row_id="+routeBank)), ","); got != "inclusion_scorecard" {
		t.Errorf("by the client: %s", got)
	}
	query := url.Values{"contract_reference": {"NLDF/TA/2026/021"}}.Encode()
	byReference := h.projectsAt("/projects?" + query)
	if len(byReference) != 1 || byReference[0]["project_name"] != "savings_group_training" || byReference[0]["contract_reference"] != "NLDF/TA/2026/021" {
		t.Errorf("by the contract reference: %v", byReference)
	}
	if len(h.projectsAt("/projects?party_role=donor&party_row_id="+routeBank)) != 0 {
		t.Errorf("a client row listed as a donor")
	}
	h.expect(http.StatusBadRequest, "GET", "/projects?party_role=Donor", nil)

	parties := h.projectsAt("/projects/" + training + "/parties")
	if len(parties) != 3 || parties[0]["role"] != "client" {
		t.Fatalf("parties of the training project: %v", parties)
	}
}

func TestProjectPartyRoutes_StatusesOfAddRemoveAndRefusals(t *testing.T) {
	h := newHub(t)
	id := h.expect(http.StatusCreated, "POST", "/projects", map[string]any{"name": "pool"})["id"].(string)
	body := map[string]any{"role": "donor", "record_row_id": routeFundOne, "party_name": "Northern Lakes Development Fund"}
	h.expect(http.StatusCreated, "POST", "/projects/"+id+"/parties", body)
	h.expect(http.StatusConflict, "POST", "/projects/"+id+"/parties", body)
	h.expect(http.StatusBadRequest, "POST", "/projects/"+id+"/parties", map[string]any{"role": "Donor", "record_row_id": routeFundOne})
	h.expect(http.StatusBadRequest, "POST", "/projects/"+id+"/parties", map[string]any{"role": "donor", "record_row_id": "x"})
	h.expect(http.StatusNotFound, "POST", "/projects/no-such/parties", body)
	h.expect(http.StatusBadRequest, "POST", "/projects", map[string]any{"name": "long", "contract_reference": strings.Repeat("x", 201)})
	h.expect(http.StatusNoContent, "DELETE", "/projects/"+id+"/parties/donor/"+routeFundOne, nil)
	h.expect(http.StatusNotFound, "DELETE", "/projects/"+id+"/parties/donor/"+routeFundOne, nil)
	if len(h.projectsAt("/projects/"+id+"/parties")) != 0 {
		t.Errorf("a removed party is still listed")
	}
}

func TestProjectPartyToolsAreNamedByTheirActions(t *testing.T) {
	have := map[string]bool{}
	for _, tool := range routes.ToolShape() {
		have[tool.Name] = true
	}
	for _, name := range []string{"taskmanager_project_party_add", "taskmanager_project_party_remove", "taskmanager_project_party_list", "taskmanager_project_list"} {
		if !have[name] {
			t.Errorf("no tool named %s", name)
		}
	}
}

func TestProjectPartyRoutes_AnUpperCaseRowIsTheSamePartyAndANilOrJunkRowIsRefused(t *testing.T) {
	h := newHub(t)
	id := h.expect(http.StatusCreated, "POST", "/projects", map[string]any{"name": "pool"})["id"].(string)
	h.expect(http.StatusCreated, "POST", "/projects/"+id+"/parties", map[string]any{"role": "donor", "record_row_id": routeFundOne, "party_name": "Northern Lakes Development Fund"})
	h.expect(http.StatusConflict, "POST", "/projects/"+id+"/parties", map[string]any{"role": "donor", "record_row_id": strings.ToUpper(routeFundOne), "party_name": "Northern Lakes Development Fund"})
	h.expect(http.StatusBadRequest, "POST", "/projects/"+id+"/parties", map[string]any{"role": "donor", "record_row_id": "00000000-0000-0000-0000-000000000000"})
	h.expect(http.StatusBadRequest, "GET", "/projects?party_row_id=00000000-0000-0000-0000-000000000000", nil)
	h.expect(http.StatusBadRequest, "GET", "/projects?party_row_id=junk", nil)
	if got := strings.Join(projectKeysOf(h.projectsAt("/projects?party_row_id="+strings.ToUpper(routeFundOne))), ","); got != "pool" {
		t.Errorf("a list by the upper-case row id: %s", got)
	}
	h.expect(http.StatusBadRequest, "DELETE", "/projects/"+id+"/parties/donor/junk", nil)
	h.expect(http.StatusNoContent, "DELETE", "/projects/"+id+"/parties/donor/"+strings.ToUpper(routeFundOne), nil)
}
