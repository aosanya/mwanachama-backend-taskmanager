package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type fakeRecordRows struct {
	known  map[string][]string
	failed error
	asked  []string
}

func (f *fakeRecordRows) RowExists(_ context.Context, recordRowID string) (bool, error) {
	f.asked = append(f.asked, recordRowID)
	if f.failed != nil {
		return false, f.failed
	}
	_, ok := f.known[recordRowID]
	return ok, nil
}

func (f *fakeRecordRows) RowChain(_ context.Context, recordRowID string) ([]string, error) {
	if f.failed != nil {
		return nil, f.failed
	}
	return f.known[recordRowID], nil
}

func TestProjectParties_AMountedRowCheckRefusesAnUnknownRow(t *testing.T) {
	mgr, _, _ := newPartyManager(t)
	rows := &fakeRecordRows{known: map[string][]string{lakeshoreBank: {lakeshoreBank}}}
	mgr.UsePartyRows(rows)
	ctx := context.Background()
	project, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Scorecard"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	if _, err := mgr.AddProjectParty(ctx, mwanachamataskmanager.ProjectParty{ProjectID: project.ID, Role: "client", RecordRowID: lakeshoreBank, PartyName: "Lakeshore Central Bank"}); err != nil {
		t.Fatalf("a known row: %v", err)
	}
	_, err = mgr.AddProjectParty(ctx, mwanachamataskmanager.ProjectParty{ProjectID: project.ID, Role: "donor", RecordRowID: strings.ToUpper(northernLakesFund), PartyName: "Northern Lakes Development Fund"})
	if !errors.Is(err, mwanachamataskmanager.ErrProjectPartyRowUnknown) {
		t.Fatalf("an unknown row: got %v, want ErrProjectPartyRowUnknown", err)
	}
	if !strings.Contains(err.Error(), northernLakesFund) {
		t.Errorf("the refusal does not name the row: %v", err)
	}
	if last := rows.asked[len(rows.asked)-1]; last != northernLakesFund {
		t.Errorf("the check was asked about %q, want the canonical %q", last, northernLakesFund)
	}
	if listed, err := mgr.ListProjectParties(ctx, project.ID); err != nil || len(listed) != 1 {
		t.Fatalf("parties after the refusal: %+v, %v", listed, err)
	}

	if _, err := mgr.AddProjectParty(ctx, mwanachamataskmanager.ProjectParty{ProjectID: project.ID, Role: "client", RecordRowID: "00000000-0000-0000-0000-000000000000", PartyName: "x"}); !errors.Is(err, mwanachamataskmanager.ErrInvalidProjectParty) {
		t.Errorf("the nil uuid: got %v, want ErrInvalidProjectParty before the host is asked", err)
	}
}

func TestProjectParties_ARowCheckThatFailsIsNotTakenAsYes(t *testing.T) {
	mgr, _, _ := newPartyManager(t)
	lookupDown := errors.New("record store unreachable")
	mgr.UsePartyRows(&fakeRecordRows{failed: lookupDown})
	ctx := context.Background()
	project, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Scorecard"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := mgr.AddProjectParty(ctx, mwanachamataskmanager.ProjectParty{ProjectID: project.ID, Role: "client", RecordRowID: lakeshoreBank, PartyName: "x"}); !errors.Is(err, lookupDown) {
		t.Fatalf("add while the lookup is down: got %v, want the lookup error", err)
	}
	if _, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{PartyRowID: lakeshoreBank}); !errors.Is(err, lookupDown) {
		t.Fatalf("list while the lookup is down: got %v, want the lookup error", err)
	}
	if listed, err := mgr.ListProjectParties(ctx, project.ID); err != nil || len(listed) != 0 {
		t.Fatalf("parties stored while the lookup was down: %+v, %v", listed, err)
	}
}

func TestFindProjects_AMountedChainFindsAProjectThroughACorrectedRow(t *testing.T) {
	mgr, _, _ := newPartyManager(t)
	seedWorkedExample(t, mgr)
	ctx := context.Background()
	want := []string{"bank_supervision_review", "inclusion_scorecard"}
	mgr.UsePartyRows(&fakeRecordRows{known: map[string][]string{
		lakeshoreBankRenamed: {strings.ToUpper(lakeshoreBank), lakeshoreBankRenamed, lakeshoreBank},
	}})

	byNewID, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{PartyRole: "client", PartyRowID: lakeshoreBankRenamed})
	if err != nil || !reflect.DeepEqual(projectKeys(byNewID), want) {
		t.Fatalf("by the new row id: %v, %v; want %v", projectKeys(byNewID), err, want)
	}
	byOldID, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{PartyRole: "client", PartyRowID: lakeshoreBank})
	if err != nil || !reflect.DeepEqual(projectKeys(byOldID), want) {
		t.Fatalf("by the old row id: %v, %v; want %v", projectKeys(byOldID), err, want)
	}
	other, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{PartyRole: "client", PartyRowID: highlandFederation})
	if err != nil || !reflect.DeepEqual(projectKeys(other), []string{"rural_lending_study", "savings_group_training"}) {
		t.Fatalf("a row with no chain: %v, %v", projectKeys(other), err)
	}
}

func TestFindProjects_AChainWithAMalformedIdIsRefused(t *testing.T) {
	mgr, _, _ := newPartyManager(t)
	seedWorkedExample(t, mgr)
	mgr.UsePartyRows(&fakeRecordRows{known: map[string][]string{
		lakeshoreBankRenamed: {lakeshoreBankRenamed, "not-a-row"},
	}})
	_, err := mgr.FindProjects(context.Background(), mwanachamataskmanager.ProjectFilter{PartyRowID: lakeshoreBankRenamed})
	if err == nil {
		t.Fatal("a chain with a malformed id was accepted")
	}
}

func TestProjectParties_WithNoRowLookupWiredTheShapeIsTheOnlyCheck(t *testing.T) {
	mgr, _, _ := newPartyManager(t)
	ctx := context.Background()
	project, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Scorecard"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := mgr.AddProjectParty(ctx, mwanachamataskmanager.ProjectParty{ProjectID: project.ID, Role: "client", RecordRowID: lakeshoreBank, PartyName: "x"}); err != nil {
		t.Fatalf("add with nothing wired: %v", err)
	}
	mgr.UsePartyRows(nil)
	found, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{PartyRowID: lakeshoreBank})
	if err != nil || len(found) != 1 {
		t.Fatalf("list with the lookup cleared: %v, %v", projectKeys(found), err)
	}
	alone, err := mgr.FindProjects(ctx, mwanachamataskmanager.ProjectFilter{PartyRowID: lakeshoreBankRenamed})
	if err != nil || len(alone) != 0 {
		t.Fatalf("a corrected row with nothing wired: %v, %v", projectKeys(alone), err)
	}
}
