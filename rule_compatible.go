package mwanachamataskmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

var (
	ErrDuplicateRuleCompatible = errors.New("the two rules are already judged compatible")
	ErrRuleCompatibleNotFound  = errors.New("the two rules are not judged compatible")
)

func orderedRuleIDs(a, b string) (string, string) {
	if b < a {
		return b, a
	}
	return a, b
}

func ruleCompatibleKey(a, b string) string {
	first, second := orderedRuleIDs(a, b)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s%d:%s", len(first), first, len(second), second)))
	return hex.EncodeToString(sum[:])
}

func (m *taskManager) compatiblePair(ctx context.Context, requesterID, code, otherCode string) (RuleRecord, RuleRecord, error) {
	rule, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return RuleRecord{}, RuleRecord{}, err
	}
	other, err := m.GetRuleByCode(ctx, requesterID, otherCode)
	if err != nil {
		return RuleRecord{}, RuleRecord{}, err
	}
	return rule, other, nil
}

func (m *taskManager) AddRuleCompatible(ctx context.Context, requesterID, code, otherCode string) error {
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return err
	}
	rule, other, err := m.compatiblePair(ctx, requesterID, code, otherCode)
	if err != nil {
		return err
	}
	if rule.ID == other.ID {
		return fmt.Errorf("%w: %s cannot be compatible with itself", ErrInvalidRule, rule.Code)
	}
	for _, each := range []RuleRecord{rule, other} {
		if each.Kind != ruleKindRule {
			return fmt.Errorf("%w: only rules are judged compatible, and %s is a %s", ErrInvalidRule, each.Code, each.Kind)
		}
	}
	first, second := orderedRuleIDs(rule.ID, other.ID)
	row := RuleCompatible{ID: ruleCompatibleKey(first, second), FirstRuleID: first, SecondRuleID: second, AddedBy: requesterID, CreatedAt: nowStamp()}
	exists, err := m.compatibleExists(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("AddRuleCompatible: %w", err)
	}
	if exists {
		return ErrDuplicateRuleCompatible
	}
	if err := m.store.Insert(ctx, roleRuleCompatible, row); err != nil {
		if again, lookupErr := m.compatibleExists(ctx, row.ID); lookupErr == nil && again {
			return ErrDuplicateRuleCompatible
		}
		return fmt.Errorf("AddRuleCompatible: %w", err)
	}
	return nil
}

func (m *taskManager) compatibleExists(ctx context.Context, id string) (bool, error) {
	var row RuleCompatible
	q := m.store.Query(ctx, roleRuleCompatible).Where("id = ?", id)
	if err := m.store.Take(q, roleRuleCompatible, &row, ErrRuleCompatibleNotFound); err != nil {
		if errors.Is(err, ErrRuleCompatibleNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (m *taskManager) RemoveRuleCompatible(ctx context.Context, requesterID, code, otherCode string) error {
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return err
	}
	rule, other, err := m.compatiblePair(ctx, requesterID, code, otherCode)
	if err != nil {
		return err
	}
	res := m.store.Query(ctx, roleRuleCompatible).Where("id = ?", ruleCompatibleKey(rule.ID, other.ID)).Delete(nil)
	if res.Error != nil {
		return fmt.Errorf("RemoveRuleCompatible: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrRuleCompatibleNotFound
	}
	return nil
}

func (m *taskManager) compatiblePairs(ctx context.Context) (map[string]bool, error) {
	q := m.store.Query(ctx, roleRuleCompatible).Limit(maxListPage)
	rows, err := specstore.List[RuleCompatible](m.store, q, roleRuleCompatible)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[row.ID] = true
	}
	return out, nil
}

func (m *taskManager) compatibleCodesOf(ctx context.Context, ruleID string) ([]string, error) {
	out := []string{}
	for _, column := range []struct{ own, other string }{{"first_rule_id", "second_rule_id"}, {"second_rule_id", "first_rule_id"}} {
		q := m.store.Query(ctx, roleRuleCompatible).Where(column.own+" = ?", ruleID).Limit(maxListPage)
		rows, err := specstore.List[RuleCompatible](m.store, q, roleRuleCompatible)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			otherID := row.FirstRuleID
			if column.other == "second_rule_id" {
				otherID = row.SecondRuleID
			}
			other, err := m.ruleByID(ctx, otherID)
			if err != nil {
				return nil, err
			}
			if other.Status != ruleSuperseded && other.Status != ruleRetired {
				out = append(out, other.Code)
			}
		}
	}
	sortRuleCodes(out)
	return out, nil
}

func sortRuleCodes(codes []string) {
	sort.Slice(codes, func(i, j int) bool {
		_, a := splitCode(codes[i])
		_, b := splitCode(codes[j])
		return a < b
	})
}

func (m *taskManager) ReadRule(ctx context.Context, requesterID, code string) (RuleView, error) {
	rule, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return RuleView{}, err
	}
	compatible, err := m.compatibleCodesOf(ctx, rule.ID)
	if err != nil {
		return RuleView{}, fmt.Errorf("ReadRule: %w", err)
	}
	return RuleView{RuleRecord: rule, Compatible: compatible}, nil
}
