package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

const (
	lakeshoreBank        = "c0100000-0000-4000-8000-000000000001"
	lakeshoreBankRenamed = "c0100000-0000-4000-8000-0000000000f1"
	highlandFederation   = "c0200000-0000-4000-8000-000000000002"
	rivermouthAuthority  = "c0300000-0000-4000-8000-000000000003"
	northernLakesFund    = "d0100000-0000-4000-8000-000000000001"
	meridianFacility     = "d0200000-0000-4000-8000-000000000002"
	harbourCommission    = "d0300000-0000-4000-8000-000000000003"
)

func newPartyManager(t *testing.T) (mwanachamataskmanager.TaskManager, *gorm.DB, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	workSpec := specForInstance(t, "spec/examples/agency.taskmanager.json", "test")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	object, ok := workSpec.Object("objective_party")
	if !ok {
		t.Fatal("spec declares no objective_party object")
	}
	return mgr, db, workSpec.TableFor(object)
}

type partyWiring struct {
	role string
	row  string
	name string
}

type exampleProject struct {
	name      string
	contract  string
	parties   []partyWiring
	projectID string
}

func workedExample() []*exampleProject {
	client := func(row, name string) partyWiring { return partyWiring{"client", row, name} }
	donor := func(row, name string) partyWiring { return partyWiring{"donor", row, name} }
	return []*exampleProject{
		{name: "inclusion_scorecard", contract: "NLDF/TA/2026/014", parties: []partyWiring{client(lakeshoreBank, "Lakeshore Central Bank"), donor(northernLakesFund, "Northern Lakes Development Fund")}},
		{name: "savings_group_training", contract: "NLDF/TA/2026/021", parties: []partyWiring{client(highlandFederation, "Highland Farmers' Savings Federation"), donor(northernLakesFund, "Northern Lakes Development Fund"), donor(harbourCommission, "Harbour Commission for Development")}},
		{name: "crop_insurance_design", contract: "MCF-ADV-0037", parties: []partyWiring{client(rivermouthAuthority, "Rivermouth Insurance Authority"), donor(meridianFacility, "Meridian Climate Facility")}},
		{name: "bank_supervision_review", contract: "HCD/2026/PS-9", parties: []partyWiring{client(lakeshoreBank, "Lakeshore Central Bank"), donor(harbourCommission, "Harbour Commission for Development")}},
		{name: "flood_risk_pool", contract: "MCF-ADV-0041", parties: []partyWiring{client(rivermouthAuthority, "Rivermouth Insurance Authority"), donor(meridianFacility, "Meridian Climate Facility"), donor(northernLakesFund, "Northern Lakes Development Fund")}},
		{name: "rural_lending_study", contract: "NLDF/TA/2026/030", parties: []partyWiring{client(highlandFederation, "Highland Farmers' Savings Federation"), donor(northernLakesFund, "Northern Lakes Development Fund")}},
	}
}

func seedWorkedExample(t *testing.T, mgr mwanachamataskmanager.TaskManager) []*exampleProject {
	t.Helper()
	ctx := context.Background()
	projects := workedExample()
	for _, p := range projects {
		created, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: p.name, ContractReference: p.contract})
		if err != nil {
			t.Fatalf("CreateProject %s: %v", p.name, err)
		}
		p.projectID = created.ID
		for _, party := range p.parties {
			if _, err := mgr.AddProjectParty(ctx, mwanachamataskmanager.ProjectParty{ProjectID: created.ID, Role: party.role, RecordRowID: party.row, PartyName: party.name}); err != nil {
				t.Fatalf("AddProjectParty %s %s: %v", p.name, party.role, err)
			}
		}
	}
	return projects
}

func projectKeys(projects []mwanachamataskmanager.Project) []string {
	keys := make([]string, 0, len(projects))
	for _, p := range projects {
		keys = append(keys, p.ProjectName)
	}
	sort.Strings(keys)
	return keys
}

func TestProjectContractReference_IsKeptTrimmedAndChangedByUpdate(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	created, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Scorecard", ContractReference: "  NLDF/TA/2026/014 "})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if created.ContractReference != "NLDF/TA/2026/014" {
		t.Fatalf("reference after create = %q", created.ContractReference)
	}
	read, err := mgr.GetProject(ctx, created.ID)
	if err != nil || read.ContractReference != "NLDF/TA/2026/014" {
		t.Fatalf("GetProject = %+v, %v", read, err)
	}
	read.ContractReference = "NLDF/TA/2026/099"
	updated, err := mgr.UpdateProject(ctx, read)
	if err != nil || updated.ContractReference != "NLDF/TA/2026/099" {
		t.Fatalf("UpdateProject = %+v, %v", updated, err)
	}
	again, err := mgr.GetProject(ctx, created.ID)
	if err != nil || again.ContractReference != "NLDF/TA/2026/099" {
		t.Fatalf("GetProject after update = %+v, %v", again, err)
	}
}

func TestProjectContractReference_RefusesALongOrControlledText(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	for name, reference := range map[string]string{
		"too long":      strings.Repeat("x", 201),
		"control":       "NLDF\n2026",
		"invalid utf-8": "NLDF\xff",
	} {
		_, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "P " + name, ContractReference: reference})
		if !errors.Is(err, mwanachamataskmanager.ErrInvalidTask) {
			t.Errorf("%s: got %v, want ErrInvalidTask", name, err)
		}
	}
	exact, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Exact", ContractReference: strings.Repeat("é", 200)})
	if err != nil {
		t.Fatalf("a reference of 200 characters: %v", err)
	}
	exact.ContractReference = strings.Repeat("é", 201)
	if _, err := mgr.UpdateProject(context.Background(), exact); !errors.Is(err, mwanachamataskmanager.ErrInvalidTask) {
		t.Errorf("update to 201 characters: got %v, want ErrInvalidTask", err)
	}
}

func TestProjectParties_AddListAndRemoveLeaveTheOthers(t *testing.T) {
	mgr, _, _ := newPartyManager(t)
	ctx := context.Background()
	project, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Savings training"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	for _, w := range []partyWiring{
		{"client", highlandFederation, "Highland Farmers' Savings Federation"},
		{"donor", northernLakesFund, "Northern Lakes Development Fund"},
		{"donor", harbourCommission, "Harbour Commission for Development"},
		{"funder", northernLakesFund, "Northern Lakes Development Fund"},
	} {
		added, err := mgr.AddProjectParty(ctx, mwanachamataskmanager.ProjectParty{ProjectID: project.ID, Role: w.role, RecordRowID: w.row, PartyName: w.name})
		if err != nil || added.AddedAt == "" {
			t.Fatalf("AddProjectParty %v = %+v, %v", w, added, err)
		}
	}
	listed, err := mgr.ListProjectParties(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListProjectParties: %v", err)
	}
	var got []string
	for _, p := range listed {
		got = append(got, p.Role+" "+p.RecordRowID+" "+p.PartyName)
	}
	sort.Strings(got)
	want := []string{
		"client " + highlandFederation + " Highland Farmers' Savings Federation",
		"donor " + northernLakesFund + " Northern Lakes Development Fund",
		"donor " + harbourCommission + " Harbour Commission for Development",
		"funder " + northernLakesFund + " Northern Lakes Development Fund",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parties = %v, want %v", got, want)
	}

	if err := mgr.RemoveProjectParty(ctx, project.ID, "donor", northernLakesFund); err != nil {
		t.Fatalf("RemoveProjectParty: %v", err)
	}
	after, err := mgr.ListProjectParties(ctx, project.ID)
	if err != nil || len(after) != 3 {
		t.Fatalf("after removing one: %d parties, %v", len(after), err)
	}
	for _, p := range after {
		if p.Role == "donor" && p.RecordRowID == northernLakesFund {
			t.Fatalf("the removed party is still listed: %+v", p)
		}
	}
	if err := mgr.RemoveProjectParty(ctx, project.ID, "donor", northernLakesFund); !errors.Is(err, mwanachamataskmanager.ErrProjectPartyNotFound) {
		t.Fatalf("removing twice: got %v, want ErrProjectPartyNotFound", err)
	}
}

func TestProjectParties_AHeldRoleIsRefusedAndABadPartyIsRefused(t *testing.T) {
	mgr, _, _ := newPartyManager(t)
	ctx := context.Background()
	project, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Refusals"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	add := func(projectID, role, row, name string) error {
		_, err := mgr.AddProjectParty(ctx, mwanachamataskmanager.ProjectParty{ProjectID: projectID, Role: role, RecordRowID: row, PartyName: name})
		return err
	}
	if err := add(project.ID, "donor", northernLakesFund, "Northern Lakes"); err != nil {
		t.Fatalf("first add: %v", err)
	}
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"same party, same role", add(project.ID, "donor", northernLakesFund, "Northern Lakes"), mwanachamataskmanager.ErrProjectPartyExists},
		{"upper-case role", add(project.ID, "Donor", meridianFacility, ""), mwanachamataskmanager.ErrInvalidProjectParty},
		{"role starting with a digit", add(project.ID, "1donor", meridianFacility, ""), mwanachamataskmanager.ErrInvalidProjectParty},
		{"empty role", add(project.ID, "", meridianFacility, ""), mwanachamataskmanager.ErrInvalidProjectParty},
		{"row id not a uuid", add(project.ID, "donor", "not-a-row", ""), mwanachamataskmanager.ErrInvalidProjectParty},
		{"empty row id", add(project.ID, "donor", "", ""), mwanachamataskmanager.ErrInvalidProjectParty},
		{"name too long", add(project.ID, "donor", meridianFacility, strings.Repeat("n", 201)), mwanachamataskmanager.ErrInvalidProjectParty},
		{"unknown project", add("no-such-project", "donor", meridianFacility, ""), mwanachamataskmanager.ErrProjectNotFound},
	}
	for _, c := range cases {
		if !errors.Is(c.err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, c.err, c.want)
		}
	}
	if _, err := mgr.ListProjectParties(ctx, "no-such-project"); !errors.Is(err, mwanachamataskmanager.ErrProjectNotFound) {
		t.Errorf("list of an unknown project: got %v", err)
	}
	if err := mgr.RemoveProjectParty(ctx, "no-such-project", "donor", northernLakesFund); !errors.Is(err, mwanachamataskmanager.ErrProjectNotFound) {
		t.Errorf("remove from an unknown project: got %v", err)
	}
}

func TestFindProjects_WorkedExampleListsByEachDonorRowTheClientRowAndTheContractReference(t *testing.T) {
	mgr, _, _ := newPartyManager(t)
	seedWorkedExample(t, mgr)
	ctx := context.Background()

	cases := []struct {
		name   string
		filter mwanachamataskmanager.ProjectFilter
		want   []string
	}{
		{"donor d01", mwanachamataskmanager.ProjectFilter{PartyRole: "donor", PartyRowID: northernLakesFund},
			[]string{"flood_risk_pool", "inclusion_scorecard", "rural_lending_study", "savings_group_training"}},
		{"donor d02", mwanachamataskmanager.ProjectFilter{PartyRole: "donor", PartyRowID: meridianFacility},
			[]string{"crop_insurance_design", "flood_risk_pool"}},
		{"donor d03", mwanachamataskmanager.ProjectFilter{PartyRole: "donor", PartyRowID: harbourCommission},
			[]string{"bank_supervision_review", "savings_group_training"}},
		{"client c01", mwanachamataskmanager.ProjectFilter{PartyRole: "client", PartyRowID: lakeshoreBank},
			[]string{"bank_supervision_review", "inclusion_scorecard"}},
		{"client c03", mwanachamataskmanager.ProjectFilter{PartyRowID: rivermouthAuthority},
			[]string{"crop_insurance_design", "flood_risk_pool"}},
		{"contract reference", mwanachamataskmanager.ProjectFilter{ContractReference: "MCF-ADV-0037"},
			[]string{"crop_insurance_design"}},
		{"contract reference is exact", mwanachamataskmanager.ProjectFilter{ContractReference: "MCF-ADV"}, []string{}},
		{"a client row asked for as a donor", mwanachamataskmanager.ProjectFilter{PartyRole: "donor", PartyRowID: lakeshoreBank}, []string{}},
		{"a donor row asked for as a client", mwanachamataskmanager.ProjectFilter{PartyRole: "client", PartyRowID: northernLakesFund}, []string{}},
		{"role alone", mwanachamataskmanager.ProjectFilter{PartyRole: "client"},
			[]string{"bank_supervision_review", "crop_insurance_design", "flood_risk_pool", "inclusion_scorecard", "rural_lending_study", "savings_group_training"}},
		{"party and reference together", mwanachamataskmanager.ProjectFilter{PartyRole: "donor", PartyRowID: northernLakesFund, ContractReference: "NLDF/TA/2026/021"},
			[]string{"savings_group_training"}},
		{"party and a reference of another project", mwanachamataskmanager.ProjectFilter{PartyRole: "donor", PartyRowID: meridianFacility, ContractReference: "NLDF/TA/2026/021"}, []string{}},
		{"a party nobody holds", mwanachamataskmanager.ProjectFilter{PartyRowID: "e0000000-0000-4000-8000-000000000009"}, []string{}},
		{"no filter", mwanachamataskmanager.ProjectFilter{},
			[]string{"bank_supervision_review", "crop_insurance_design", "flood_risk_pool", "inclusion_scorecard", "rural_lending_study", "savings_group_training"}},
	}
	for _, c := range cases {
		got, err := mgr.FindProjects(ctx, c.filter)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if keys := projectKeys(got); !reflect.DeepEqual(keys, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, keys, c.want)
		}
	}
}

func TestFindProjects_ACorrectedRowIsFoundThroughItsHandedChain(t *testing.T) {
	mgr, _, _ := newPartyManager(t)
	seedWorkedExample(t, mgr)
	ctx := context.Background()
	want := []string{"bank_supervision_review", "inclusion_scorecard"}

	byOldID, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{PartyRole: "client", PartyRowID: lakeshoreBank})
	if err != nil || !reflect.DeepEqual(projectKeys(byOldID), want) {
		t.Fatalf("by the old row id: %v, %v", projectKeys(byOldID), err)
	}
	chain := []string{lakeshoreBankRenamed, lakeshoreBank}
	byNewID, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{PartyRole: "client", PartyRowID: lakeshoreBankRenamed, PartyRowIDs: chain})
	if err != nil || !reflect.DeepEqual(projectKeys(byNewID), want) {
		t.Fatalf("by the new row id with the chain: %v, %v", projectKeys(byNewID), err)
	}
	alone, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{PartyRole: "client", PartyRowID: lakeshoreBankRenamed})
	if err != nil || len(alone) != 0 {
		t.Fatalf("by the new row id without a chain: %v, %v", projectKeys(alone), err)
	}
}

func TestFindProjects_ABadRoleIsRefusedAndADeletedProjectIsGone(t *testing.T) {
	mgr, db, parties := newPartyManager(t)
	projects := seedWorkedExample(t, mgr)
	ctx := context.Background()

	if _, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{PartyRole: "Donor"}); !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
		t.Errorf("a bad role: got %v, want ErrInvalidFilter", err)
	}
	if _, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{ContractReference: strings.Repeat("x", 201)}); !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
		t.Errorf("a long reference: got %v, want ErrInvalidFilter", err)
	}

	flood := projects[4]
	if err := mgr.DeleteProject(ctx, flood.projectID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	got, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{PartyRole: "donor", PartyRowID: northernLakesFund})
	if err != nil || !reflect.DeepEqual(projectKeys(got), []string{"inclusion_scorecard", "rural_lending_study", "savings_group_training"}) {
		t.Errorf("after the delete: %v, %v", projectKeys(got), err)
	}
	var left int64
	if err := db.Table(parties).Where("project_id = ?", flood.projectID).Count(&left).Error; err != nil || left != 0 {
		t.Errorf("party rows left of the deleted project: %d, %v", left, err)
	}
	if err := db.Table(parties).Count(&left).Error; err != nil || left != 11 {
		t.Errorf("party rows of the other projects: %d, %v", left, err)
	}
}
