package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (m *taskManager) ruleNote(ctx context.Context, id string) (RuleNote, error) {
	var note RuleNote
	q := m.store.Query(ctx, roleRuleNote).Where("id = ?", strings.TrimSpace(id))
	if err := m.store.Take(q, roleRuleNote, &note, ErrRuleNoteNotFound); err != nil {
		return RuleNote{}, err
	}
	return normalisedNote(note), nil
}

func (m *taskManager) checkedSettlingCode(ctx context.Context, code string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", nil
	}
	found, err := m.existingRules(ctx, []string{code})
	if err != nil {
		return "", err
	}
	rule, ok := found[code]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrRuleNoteCodeNotFound, code)
	}
	if rule.Kind == ruleKindQuestion {
		return "", invalidNote("%s is a question, and only a rule or a decision settles a contradiction", code)
	}
	return code, nil
}

func (m *taskManager) ResolveRuleNote(ctx context.Context, request RuleNoteResolution) (RuleNote, error) {
	requesterID := request.RequesterID
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return RuleNote{}, err
	}
	note, err := m.ruleNote(ctx, request.NoteID)
	if err != nil {
		return RuleNote{}, err
	}
	if note.Kind != ruleNoteKindContradiction {
		return RuleNote{}, invalidNote("only a contradiction note is resolved, and this one is a %s", note.Kind)
	}
	if note.State == ruleNoteResolved {
		return RuleNote{}, ErrRuleNoteResolved
	}
	resolution, err := settledNoteText(request.Resolution, "a resolution")
	if err != nil {
		return RuleNote{}, err
	}
	settledBy, err := m.checkedSettlingCode(ctx, request.ResolvedByCode)
	if err != nil {
		return RuleNote{}, err
	}
	note.State, note.ResolvedAt, note.ResolvedBy = ruleNoteResolved, time.Now().UTC().Format(noteTimeForm), requesterID
	note.Resolution, note.ResolvedByCode = resolution, settledBy
	res := m.store.Query(ctx, roleRuleNote).Where("id = ? AND state = ?", note.ID, ruleNoteOpen).Updates(map[string]any{
		columnName("State"): note.State, columnName("ResolvedAt"): note.ResolvedAt, columnName("ResolvedBy"): note.ResolvedBy,
		columnName("Resolution"): note.Resolution, columnName("ResolvedByCode"): note.ResolvedByCode,
	})
	if res.Error != nil {
		return RuleNote{}, fmt.Errorf("ResolveRuleNote: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return RuleNote{}, ErrRuleNoteResolved
	}
	return note, nil
}
