package routes_test

import (
	"net/http"
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
	question := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "title": "Which tabs?"})["code"].(string)
	decision := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "decision", "title": "Use spaces"})["code"].(string)
	changeRuleStatusBeforeNextUpdate(h, ruleTableOf(h), "dropped")

	h.expect(http.StatusConflict, "bob", "PUT", "/rules/"+question+"/answer", map[string]any{"decision_code": decision})
}
