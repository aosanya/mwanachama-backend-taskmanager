package mwanachamataskmanager_test

import (
	"context"
	"fmt"
	"testing"

	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type legacyRow struct {
	project, title, linkedRef string
	deleted                   bool
}

var hubBeforeCodes = []legacyRow{
	{"mwanachama-frontend-kazi", "DEV-127 Create the party plane's structure tables", "mwanachama-frontend-kazi/DEV-127", false},
	{"mwanachama-frontend-kazi", "DEV-128 Show the chapter on the card", "mwanachama-frontend-kazi/DEV-128", false},
	{"mwanachama-frontend-kazi", "DEV-130 Wire the sign-in", "mwanachama-frontend-kazi/DEV-130", false},
	{"mwanachama-frontend-kazi", "DEV-127 The same row copied from another board", "mwanachama-frontend-kazi/DEV-127", false},
	{"mwanachama-frontend-kazi", "Fix the walker that hangs", "", false},
	{"hub-steward-swe", "SW185 Group roles", "hub-steward-swe/SW185", false},
	{"hub-steward-swe", "SW192 The Log of an issue", "hub-steward-swe/SW192", false},
	{"hub-steward-swe", "**SW193** Typed by hand over MCP", "", false},
	{"hub-steward-swe", "SW29a Old row with a letter", "hub-steward-swe/SW29a", false},
	{"hub-steward-swe", "SW1 Loaded with the folder in front", "projects/hub-steward-swe/SW1", false},
	{"hub-steward-swe", "Raised in Chat with no number", "", false},
	{"hub-steward-swe", "SW150 Removed long ago", "hub-steward-swe/SW150", true},
	{"mwanachama-backend-taskmanager", "W74 Port the lock", "mwanachama-backend-taskmanager/W74", false},
	{"mwanachama-backend-taskmanager", "Review the group", "c6a1f0f2-4b0e-4d4e-9d7e-1f6f4a0a9d11", false},
	{"fresh-repo", "First issue ever raised here", "", false},
}

func hubWithoutCodes(t *testing.T) (mwanachamataskmanager.TaskManager, *gorm.DB, string, map[string]string) {
	t.Helper()
	mgr, db, table := newTestManagerWithDB(t)
	ids := map[string]string{}
	for at, row := range hubBeforeCodes {
		made := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: row.title, ProjectName: row.project, LinkedRef: row.linkedRef, Code: fmt.Sprintf("TEMP%d", at)})
		ids[row.title] = made.ID
		if row.deleted {
			if err := mgr.DeleteTask(context.Background(), made.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.Exec("update " + table + " set code = '', code_series = '', code_number = 0").Error; err != nil {
		t.Fatal(err)
	}
	return mgr, db, table, ids
}

func codeInDB(t *testing.T, db *gorm.DB, table, id string) string {
	t.Helper()
	var code string
	if err := db.Raw("select code from "+table+" where id = ?", id).Scan(&code).Error; err != nil {
		t.Fatal(err)
	}
	return code
}

func TestTaskCodeBackfill_AReportWritesNothing(t *testing.T) {
	mgr, db, table, ids := hubWithoutCodes(t)

	report, err := mwanachamataskmanager.BackfillTaskCodes(context.Background(), mgr, false)

	if err != nil || report.Applied || report.Read != len(hubBeforeCodes) || len(report.Changes) != len(hubBeforeCodes) {
		t.Fatalf("report %+v, %v", report, err)
	}
	for title, id := range ids {
		if got := codeInDB(t, db, table, id); got != "" {
			t.Fatalf("%q was written %q by a report", title, got)
		}
	}
}

func TestTaskCodeBackfill_ARowKeepsItsOwnCodeAndAnIssueWithNoneIsNumberedInItsRepoSeries(t *testing.T) {
	mgr, db, table, ids := hubWithoutCodes(t)

	report, err := mwanachamataskmanager.BackfillTaskCodes(context.Background(), mgr, true)
	if err != nil || !report.Applied {
		t.Fatalf("report %+v, %v", report, err)
	}

	want := map[string]string{
		"DEV-127 Create the party plane's structure tables": "DEV-127",
		"DEV-128 Show the chapter on the card":              "DEV-128",
		"DEV-130 Wire the sign-in":                          "DEV-130",
		"DEV-127 The same row copied from another board":    "DEV-131",
		"Fix the walker that hangs":                         "DEV-132",
		"SW185 Group roles":                                 "SW185",
		"SW192 The Log of an issue":                         "SW192",
		"**SW193** Typed by hand over MCP":                  "SW193",
		"SW29a Old row with a letter":                       "SW29a",
		"SW1 Loaded with the folder in front":               "SW1",
		"Raised in Chat with no number":                     "SW194",
		"SW150 Removed long ago":                            "SW150",
		"W74 Port the lock":                                 "W74",
		"Review the group":                                  "W75",
		"First issue ever raised here":                      "ISS-1",
	}
	for title, code := range want {
		if got := codeInDB(t, db, table, ids[title]); got != code {
			t.Errorf("%q got %q, want %q", title, got, code)
		}
	}
	if report.FromRow != 9 || report.FromTitle != 1 || report.Numbered != 5 {
		t.Errorf("counts from row %d, from title %d, numbered %d, want 9, 1 and 5", report.FromRow, report.FromTitle, report.Numbered)
	}
	if len(report.Unusual) != 1 || report.Unusual[0].Code != "SW29a" {
		t.Errorf("unusual %+v, want SW29a only", report.Unusual)
	}
	if len(report.Duplicates) != 1 || report.Duplicates[0].Title != "DEV-127 The same row copied from another board" {
		t.Errorf("duplicates %+v, want the copied row", report.Duplicates)
	}
}

func TestTaskCodeBackfill_TheSeriesAndNumberAreStoredForACodeOfTheUsualShape(t *testing.T) {
	mgr, db, table, ids := hubWithoutCodes(t)
	if _, err := mwanachamataskmanager.BackfillTaskCodes(context.Background(), mgr, true); err != nil {
		t.Fatal(err)
	}

	var kept struct {
		Series string `gorm:"column:code_series"`
		Number int    `gorm:"column:code_number"`
	}
	db.Raw("select code_series, code_number from "+table+" where id = ?", ids["DEV-128 Show the chapter on the card"]).Scan(&kept)
	if kept.Series != "DEV-" || kept.Number != 128 {
		t.Fatalf("got %+v, want DEV- and 128", kept)
	}
	db.Raw("select code_series, code_number from "+table+" where id = ?", ids["SW29a Old row with a letter"]).Scan(&kept)
	if kept.Series != "" || kept.Number != 0 {
		t.Fatalf("an unusual code got %+v, want no series and no number", kept)
	}
}

func TestTaskCodeBackfill_RunningItAgainChangesNothing(t *testing.T) {
	mgr, _, _, _ := hubWithoutCodes(t)
	if _, err := mwanachamataskmanager.BackfillTaskCodes(context.Background(), mgr, true); err != nil {
		t.Fatal(err)
	}

	again, err := mwanachamataskmanager.BackfillTaskCodes(context.Background(), mgr, true)

	if err != nil || again.Read != 0 || len(again.Changes) != 0 {
		t.Fatalf("second run %+v, %v", again, err)
	}
}

func TestTaskCodeBackfill_AnIssueRaisedAfterwardsContinuesTheSeries(t *testing.T) {
	mgr, _, _, _ := hubWithoutCodes(t)
	if _, err := mwanachamataskmanager.BackfillTaskCodes(context.Background(), mgr, true); err != nil {
		t.Fatal(err)
	}

	next := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "raised after", ProjectName: "hub-steward-swe"})

	if next.Code != "SW195" {
		t.Fatalf("got %q, want SW195", next.Code)
	}
}

func TestTaskCodeBackfill_AnIssueRaisedBeforeTheBackfillAvoidsTheNumbersTheOldRowsWillTake(t *testing.T) {
	mgr, _, _, _ := hubWithoutCodes(t)

	early := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "raised early", ProjectName: "hub-steward-swe"})
	report, err := mwanachamataskmanager.BackfillTaskCodes(context.Background(), mgr, true)

	if err != nil || early.Code != "SW195" {
		t.Fatalf("early issue got %q, %v, want SW195", early.Code, err)
	}
	for _, change := range report.Changes {
		if change.Code == "SW195" {
			t.Fatalf("the backfill handed out %+v, which the early issue holds", change)
		}
	}
}

func TestTaskCodeBackfill_AnOldIssueUpdatedBeforeTheBackfillTakesItsRowCode(t *testing.T) {
	ctx := context.Background()
	mgr, _, _, ids := hubWithoutCodes(t)
	old, err := mgr.GetTask(ctx, ids["DEV-128 Show the chapter on the card"])
	if err != nil || old.Code != "" {
		t.Fatalf("read %+v, %v", old, err)
	}

	old.Description = "touched"
	updated, err := mgr.UpdateTask(ctx, old)

	if err != nil || updated.Code != "DEV-128" || updated.CodeNumber != 128 {
		t.Fatalf("got %+v, %v, want DEV-128", updated, err)
	}
}
