package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func ruleNoteTableName(t *testing.T) string {
	t.Helper()
	workSpec := specForInstance(t, "spec/examples/agency.taskmanager.json", "test")
	object, ok := workSpec.Object("rule_note")
	if !ok {
		t.Fatal("spec declares no rule_note object")
	}
	return workSpec.TableFor(object)
}

func listsOnTheWire(t *testing.T, note mwanachamataskmanager.RuleNote) (related, links string) {
	t.Helper()
	raw, err := json.Marshal(note)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	return string(fields["related_codes"]), string(fields["links"])
}

func TestRuleNoteNormalised_AStoredNoteWithNoListsIsReadWithEmptyListsOnEveryReadPath(t *testing.T) {
	mgr, db, _ := newTestManagerWithDB(t)
	mustCreateRule(t, mgr, "rule", "A")
	note := addNote(t, mgr, "R-1", "contradiction", "bare")
	if err := db.Exec("UPDATE "+ruleNoteTableName(t)+" SET related_codes = NULL, links = NULL WHERE id = ?", note.ID).Error; err != nil {
		t.Fatal(err)
	}

	read, err := mgr.ReadRule(context.Background(), "ann", "R-1")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveNote(mgr, note.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]mwanachamataskmanager.RuleNote{
		"list":    notesOf(t, mgr, mwanachamataskmanager.RuleNoteFilter{})[0],
		"read":    read.Notes[0],
		"resolve": resolved,
	}

	for name, got := range paths {
		if related, links := listsOnTheWire(t, got); related != "[]" || links != "[]" {
			t.Fatalf("%s: related_codes %s and links %s, want empty arrays", name, related, links)
		}
	}
}
