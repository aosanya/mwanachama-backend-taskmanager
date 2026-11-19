package mwanachamataskmanager_test

import (
	"context"
	"reflect"
	"testing"
)

func listedIDs(t *testing.T, ids []string, err error) []string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return ids
}
func filteredIDs(t *testing.T, person, backlog string) []string {
	t.Helper()
	mgr := managerWithTwoBacklogs(t)
	list, err := mgr.ListBacklogsMatching(context.Background(), person, backlog)
	ids := []string{}
	for _, item := range list.Items {
		ids = append(ids, item.ID)
	}
	return listedIDs(t, ids, err)
}
func TestBacklogListFilter_AnIdReturnsOnlyThatBacklogWithItsSteps(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	list, err := mgr.ListBacklogsMatching(context.Background(), "", "software")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != "software" || len(list.Items[0].Steps) != 2 {
		t.Fatalf("filtered by id: %+v", list.Items)
	}
}
func TestBacklogListFilter_ANameMatchesWithoutRegardToCase(t *testing.T) {
	for _, name := range []string{"Software", "SOFTWARE", "software"} {
		if got := filteredIDs(t, "", name); !reflect.DeepEqual(got, []string{"software"}) {
			t.Fatalf("%q listed %v", name, got)
		}
	}
}
func TestBacklogListFilter_AnUnknownBacklogGivesAnEmptyList(t *testing.T) {
	if got := filteredIDs(t, "", "no-such-backlog"); len(got) != 0 {
		t.Fatalf("listed %v", got)
	}
}
func TestBacklogListFilter_NoFilterReturnsTheSameListAsBefore(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	want, err := mgr.ListBacklogsFor(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ListBacklogsMatching(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || len(got.Items) != 2 {
		t.Fatalf("unfiltered %+v, want %+v", got, want)
	}
}
func TestBacklogListFilter_ABacklogThePersonCannotSeeStaysHidden(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	list, err := mgr.ListBacklogsMatching(ctx, "dev", "software")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("dev saw %+v", list.Items)
	}
}
