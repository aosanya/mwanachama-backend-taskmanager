package routes_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func changeRuleStatusBeforeNextUpdate(h *accessHub, table, status string) {
	var once sync.Once
	err := h.db.Callback().Update().Before("gorm:update").Register("test:stale_rule_write", func(tx *gorm.DB) {
		if tx.Statement.Table != table {
			return
		}
		once.Do(func() {
			_, execErr := tx.Statement.ConnPool.ExecContext(tx.Statement.Context, "UPDATE "+table+" SET status = ?", status)
			if execErr != nil {
				h.t.Error(execErr)
			}
		})
	})
	if err != nil {
		h.t.Fatal(err)
	}
}

func TestAWriteFromAStaleReadAnswers409AndNotTheBadInput400(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	made := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "No comments in code"})
	code := made["code"].(string)
	var table string
	if err := h.db.Raw("SELECT m.name FROM sqlite_master m, pragma_table_info(m.name) p WHERE m.type = 'table' AND p.name = 'superseded_by' LIMIT 1").Scan(&table).Error; err != nil || table == "" {
		t.Fatalf("rule table %q: %v", table, err)
	}
	changeRuleStatusBeforeNextUpdate(h, table, "in_force")

	h.expect(http.StatusConflict, "bob", "PATCH", "/rules/"+code, map[string]any{"title": "No comments, ever"})
	h.expect(http.StatusBadRequest, "bob", "POST", "/rules", map[string]any{"kind": "law", "title": "x"})
}

func ruleTableOf(h *accessHub) string {
	h.t.Helper()
	var table string
	if err := h.db.Raw("SELECT m.name FROM sqlite_master m, pragma_table_info(m.name) p WHERE m.type = 'table' AND p.name = 'superseded_by' LIMIT 1").Scan(&table).Error; err != nil || table == "" {
		h.t.Fatalf("rule table %q: %v", table, err)
	}
	return table
}

func TestAStatusChangeFromAStaleReadAnswers409(t *testing.T) {
	h := newRuleFixture(t).h
	code := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "No comments in code"})["code"].(string)
	changeRuleStatusBeforeNextUpdate(h, ruleTableOf(h), "retired")

	h.expect(http.StatusConflict, "bob", "PUT", "/rules/"+code+"/status", map[string]any{"status": "in_force"})
}

func TestASupersedeFromAStaleReadAnswers409(t *testing.T) {
	h := newRuleFixture(t).h
	code := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "No comments in code"})["code"].(string)
	h.expect(http.StatusOK, "bob", "PUT", "/rules/"+code+"/status", map[string]any{"status": "in_force"})
	changeRuleStatusBeforeNextUpdate(h, ruleTableOf(h), "retired")

	h.expect(http.StatusConflict, "bob", "POST", "/rules/"+code+"/supersede", map[string]any{"kind": "rule", "title": "No comments, ever"})
}

func TestAnAnswerFromAStaleReadAnswers409(t *testing.T) {
	h := newRuleFixture(t).h
	question := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "target_role": "Product owner", "title": "Which tabs?"})["code"].(string)
	decision := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "decision", "title": "Use spaces"})["code"].(string)
	changeRuleStatusBeforeNextUpdate(h, ruleTableOf(h), "dropped")

	h.expect(http.StatusConflict, "bob", "PUT", "/rules/"+question+"/answer", map[string]any{"decision_code": decision})
}

func TestAnUpdateCarryingTheStampJustReadAnswers200AndTheSameStampAgainAnswers409(t *testing.T) {
	h := newRuleFixture(t).h
	made := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "No comments in code"})
	read := h.expect(http.StatusOK, "bob", "GET", "/rules/"+made["code"].(string), nil)
	stamp := read["updated_at"].(string)

	h.expect(http.StatusOK, "bob", "PATCH", "/rules/R-1", map[string]any{"title": "No comments, ever", "expected_updated_at": stamp})
	h.expect(http.StatusConflict, "bob", "PATCH", "/rules/R-1", map[string]any{"title": "No comments at all", "expected_updated_at": stamp})
	h.expect(http.StatusOK, "bob", "PATCH", "/rules/R-1", map[string]any{"title": "Plainly: none"})
	if got := h.expect(http.StatusOK, "bob", "GET", "/rules/R-1", nil); got["title"] != "Plainly: none" {
		t.Fatalf("got %v", got)
	}
}

func TestTheRuleUpdateToolTakesExpectedUpdatedAtAndRefusesAStaleOne(t *testing.T) {
	h := newRuleFixture(t).h
	made, err := h.invoke("bob", "taskmanager_rule_create", map[string]any{"kind": "rule", "title": "Tests first"})
	if err != nil {
		t.Fatal(err)
	}
	read, err := h.invoke("bob", "taskmanager_rule_read", map[string]any{"ruleCode": made["code"]})
	if err != nil {
		t.Fatal(err)
	}
	args := func(title string) map[string]any {
		return map[string]any{"ruleCode": made["code"], "title": title, "expected_updated_at": read["updated_at"]}
	}
	if _, err := h.invoke("bob", "taskmanager_rule_update", args("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.invoke("bob", "taskmanager_rule_update", args("second")); err == nil {
		t.Fatal("a stale tool write succeeded")
	}
	got, err := h.invoke("bob", "taskmanager_rule_read", map[string]any{"ruleCode": made["code"]})
	if err != nil || got["title"] != "first" {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestAnUpdateWhoseStampMovesBetweenTheCheckAndTheWriteAnswers409(t *testing.T) {
	h := newRuleFixture(t).h
	made := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "No comments in code"})
	stamp := made["updated_at"].(string)
	table := ruleTableOf(h)
	var once sync.Once
	err := h.db.Callback().Update().Before("gorm:update").Register("test:stamp_moves_mid_write", func(tx *gorm.DB) {
		if tx.Statement.Table != table {
			return
		}
		once.Do(func() {
			if _, execErr := tx.Statement.ConnPool.ExecContext(tx.Statement.Context, "UPDATE "+table+" SET updated_at = ?", "2999-01-01T00:00:00Z"); execErr != nil {
				h.t.Error(execErr)
			}
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	h.expect(http.StatusConflict, "bob", "PATCH", "/rules/"+made["code"].(string), map[string]any{"title": "No comments, ever", "expected_updated_at": stamp})
}

func TestTheRuleUpdateToolDeclaresExpectedUpdatedAt(t *testing.T) {
	h := newRuleFixture(t).h
	schema, err := json.Marshal(h.tool("taskmanager_rule_update").InputSchema)
	if err != nil || !strings.Contains(string(schema), `"expected_updated_at"`) {
		t.Fatalf("schema %s, %v", schema, err)
	}
}

func TestABlankExpectedUpdatedAtAnswers400(t *testing.T) {
	h := newRuleFixture(t).h
	made := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "No comments in code"})
	h.expect(http.StatusBadRequest, "bob", "PATCH", "/rules/"+made["code"].(string), map[string]any{"title": "x", "expected_updated_at": " "})
}
