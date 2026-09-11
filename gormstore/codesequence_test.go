package gormstore_test

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
)

// newCodeTestDB builds an in-memory sqlite database with just the two
// tables NextCode/BackfillCodes need: a sequence table and a
// gormstore.DeliverableRow-shaped table (any Code-bearing row type would
// do — Deliverable is used here since it is one of this package's two
// entity types with no standalone root-level Create function, so its Code
// minting is only exercisable at this package's level).
func newCodeTestDB(t *testing.T) (*gorm.DB, string, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	const seqTable = "test_code_sequences"
	const deliverableTable = "test_deliverables"
	if err := db.Table(seqTable).AutoMigrate(&gormstore.CodeSequenceRow{}); err != nil {
		t.Fatalf("AutoMigrate sequence: %v", err)
	}
	if err := db.Table(deliverableTable).AutoMigrate(&gormstore.DeliverableRow{}); err != nil {
		t.Fatalf("AutoMigrate deliverable: %v", err)
	}
	return db, seqTable, deliverableTable
}

// TestNextCode_SequentialWithinEntityType asserts codes are minted in order
// ("D-1", "D-2", "D-3", ...) for repeated calls against the same entity
// type, matching a real repeated-create sequence.
func TestNextCode_SequentialWithinEntityType(t *testing.T) {
	db, seqTable, _ := newCodeTestDB(t)
	ctx := context.Background()

	var got []string
	for i := 0; i < 3; i++ {
		err := db.Transaction(func(tx *gorm.DB) error {
			code, err := gormstore.NextCode(ctx, tx, seqTable, "deliverable", "D")
			if err != nil {
				return err
			}
			got = append(got, code)
			return nil
		})
		if err != nil {
			t.Fatalf("NextCode: %v", err)
		}
	}
	want := []string{"D-1", "D-2", "D-3"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("code[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// TestNextCode_IndependentPerEntityType asserts two entity types (e.g. Tag
// and Deliverable) maintain independent counters, so both mint "-1" for
// their first row.
func TestNextCode_IndependentPerEntityType(t *testing.T) {
	db, seqTable, _ := newCodeTestDB(t)
	ctx := context.Background()

	var dCode, tgCode string
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		dCode, err = gormstore.NextCode(ctx, tx, seqTable, "deliverable", "D")
		if err != nil {
			return err
		}
		tgCode, err = gormstore.NextCode(ctx, tx, seqTable, "tag", "TG")
		return err
	})
	if err != nil {
		t.Fatalf("NextCode: %v", err)
	}
	if dCode != "D-1" {
		t.Errorf("deliverable code = %q, want D-1", dCode)
	}
	if tgCode != "TG-1" {
		t.Errorf("tag code = %q, want TG-1", tgCode)
	}
}

// TestBackfillCodes_AssignsOnlyToUncodedRows asserts BackfillCodes mints a
// Code for every pre-existing row with an empty code, in created_at order,
// while leaving an already-coded row's Code untouched.
//
// The two uncoded rows are inserted via Omit("code") so the column lands as
// SQL NULL rather than Go's "" zero value — matching what a real ALTER
// TABLE ADD COLUMN (no default) leaves on pre-existing rows, and avoiding a
// spurious collision against the Code column's uniqueIndex, which two
// literal empty strings (as opposed to two NULLs) would violate.
func TestBackfillCodes_AssignsOnlyToUncodedRows(t *testing.T) {
	db, seqTable, deliverableTable := newCodeTestDB(t)

	coded := gormstore.DeliverableRow{ID: "already-coded", Code: "D-99", Title: "pre-coded", CreatedAt: "2026-01-01T00:00:00Z"}
	if err := db.Table(deliverableTable).Create(&coded).Error; err != nil {
		t.Fatalf("seed row %s: %v", coded.ID, err)
	}
	uncoded := []gormstore.DeliverableRow{
		{ID: "uncoded-first", Title: "oldest uncoded", CreatedAt: "2026-01-02T00:00:00Z"},
		{ID: "uncoded-second", Title: "newest uncoded", CreatedAt: "2026-01-03T00:00:00Z"},
	}
	for _, r := range uncoded {
		if err := db.Table(deliverableTable).Omit("code").Create(&r).Error; err != nil {
			t.Fatalf("seed row %s: %v", r.ID, err)
		}
	}

	if err := gormstore.BackfillCodes(db, deliverableTable, seqTable, "deliverable", "D"); err != nil {
		t.Fatalf("BackfillCodes: %v", err)
	}

	var got []gormstore.DeliverableRow
	if err := db.Table(deliverableTable).Order("created_at").Find(&got).Error; err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	if got[0].Code != "D-99" {
		t.Errorf("already-coded row's Code changed: got %q, want D-99", got[0].Code)
	}
	if got[1].Code != "D-1" {
		t.Errorf("oldest uncoded row Code = %q, want D-1", got[1].Code)
	}
	if got[2].Code != "D-2" {
		t.Errorf("newest uncoded row Code = %q, want D-2", got[2].Code)
	}

	// Idempotent: calling again must not re-mint or change anything.
	if err := gormstore.BackfillCodes(db, deliverableTable, seqTable, "deliverable", "D"); err != nil {
		t.Fatalf("second BackfillCodes: %v", err)
	}
	var got2 []gormstore.DeliverableRow
	if err := db.Table(deliverableTable).Order("created_at").Find(&got2).Error; err != nil {
		t.Fatalf("re-read after second backfill: %v", err)
	}
	for i := range got2 {
		if got2[i].Code != got[i].Code {
			t.Errorf("row %d Code changed on second backfill: got %q, want %q", i, got2[i].Code, got[i].Code)
		}
	}
}
