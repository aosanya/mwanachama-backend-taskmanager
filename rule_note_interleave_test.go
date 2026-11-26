package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func firstResolveLandsBeforeTheSecondWrite(t *testing.T, db *gorm.DB, table string, mgr mwanachamataskmanager.TaskManager, noteID string) *bool {
	t.Helper()
	landed := false
	register := db.Callback().Update().Before("gorm:update").Register("rule_note:first_resolve_between_read_and_write", func(tx *gorm.DB) {
		if landed || tx.Statement.Table != table {
			return
		}
		landed = true
		if _, err := resolveNoteAs(mgr, "first-resolver", noteID, "first", "D-1"); err != nil {
			t.Errorf("the interleaved first resolve: %v", err)
		}
	})
	if register != nil {
		t.Fatalf("register callback: %v", register)
	}
	return &landed
}

func sharedMemoryManager(t *testing.T) (mwanachamataskmanager.TaskManager, *gorm.DB, string) {
	t.Helper()
	dsn := fmt.Sprintf("file:rulenoteinterleave%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = sqlDB.Close() })
	workSpec := specForInstance(t, "spec/examples/agency.taskmanager.json", "test")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	object, ok := workSpec.Object("rule_note")
	if !ok {
		t.Fatal("spec declares no rule_note object")
	}
	return mgr, db, workSpec.TableFor(object)
}

func resolveNoteAs(mgr mwanachamataskmanager.TaskManager, who, id, resolution, byCode string) (mwanachamataskmanager.RuleNote, error) {
	return mgr.ResolveRuleNote(context.Background(), noteResolution(who, id, resolution, byCode))
}

func TestRuleNoteResolveInterleaved_ASecondResolveThatReadTheNoteOpenIsRefusedAndTheFirstStands(t *testing.T) {
	mgr, db, table := sharedMemoryManager(t)
	mustCreateRule(t, mgr, "rule", "A")
	mustCreateRule(t, mgr, "rule", "B")
	mustCreateRule(t, mgr, "decision", "C")
	note := addNote(t, mgr, "R-1", "contradiction", "x", "R-2")
	landed := firstResolveLandsBeforeTheSecondWrite(t, db, table, mgr, note.ID)

	_, err := resolveNoteAs(mgr, "second-resolver", note.ID, "second", "")

	if !*landed {
		t.Fatal("the first resolve never landed between the second's read and write, so nothing was interleaved")
	}
	if !errors.Is(err, mwanachamataskmanager.ErrRuleNoteResolved) || errors.Is(err, mwanachamataskmanager.ErrInvalidRuleNote) {
		t.Fatalf("err %v, want only ErrRuleNoteResolved", err)
	}
	got := noteWithID(t, mgr, note.ID)
	if got.Resolution != "first" || got.ResolvedBy != "first-resolver" || got.ResolvedByCode != "D-1" || got.State != "resolved" {
		t.Fatalf("the second resolve overwrote the first: %+v", got)
	}
}
