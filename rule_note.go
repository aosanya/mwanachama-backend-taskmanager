package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ruleNoteKindNote          = "note"
	ruleNoteKindContradiction = "contradiction"
	ruleNoteKindCompatible    = "compatible"
	ruleNoteKindQuestion      = "question"

	noteByJudge = "judge"

	ruleNoteOpen     = "open"
	ruleNoteResolved = "resolved"

	maxRuleNoteText    = 4000
	maxRelatedCodes    = 20
	ruleNotesOnARead   = 50
	defaultNoteListCap = maxNoteLimit
)

var (
	ErrInvalidRuleNote       = errors.New("invalid rule note")
	ErrInvalidRuleNoteFilter = errors.New("invalid rule note filter")
	ErrRuleNoteCodeNotFound  = errors.New("the code named on the note is not a rule, decision or question")
	ErrRuleNoteNotFound      = errors.New("rule note not found")
	ErrRuleNoteResolved      = errors.New("the contradiction note is already resolved")
)

var (
	ruleNoteKinds   = []string{ruleNoteKindNote, ruleNoteKindContradiction, ruleNoteKindCompatible, ruleNoteKindQuestion}
	ruleNoteSources = []string{noteByPerson, noteByWorker, noteByJudge}
	ruleNoteStates  = []string{ruleNoteOpen, ruleNoteResolved}
)

func invalidNote(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRuleNote, fmt.Sprintf(format, args...))
}

func settledChoice(value, fallback string, allowed []string, what string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	if !slices.Contains(allowed, value) {
		return "", invalidNote("a note's %s is %s, not %q", what, strings.Join(allowed, ", "), value)
	}
	return value, nil
}

func settledNoteText(text, what string) (string, error) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > maxRuleNoteText {
		return "", invalidNote("%s is at most %d characters", what, maxRuleNoteText)
	}
	return text, nil
}

func cleanCodes(codes []string) []string {
	out := []string{}
	for _, code := range codes {
		if code = strings.TrimSpace(code); code != "" && !slices.Contains(out, code) {
			out = append(out, code)
		}
	}
	return out
}

func (m *taskManager) existingRules(ctx context.Context, codes []string) (map[string]RuleRecord, error) {
	found := map[string]RuleRecord{}
	if len(codes) == 0 {
		return found, nil
	}
	rules, err := m.rulesWithCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	for _, rule := range rules {
		found[rule.Code] = rule
	}
	return found, nil
}

func (m *taskManager) checkedRelatedCodes(ctx context.Context, own string, related []string) ([]string, error) {
	codes := cleanCodes(related)
	if len(codes) > maxRelatedCodes {
		return nil, invalidNote("a note names at most %d related codes", maxRelatedCodes)
	}
	if slices.Contains(codes, own) {
		return nil, invalidNote("a note cannot name its own record %s as related", own)
	}
	found, err := m.existingRules(ctx, codes)
	if err != nil {
		return nil, err
	}
	for _, code := range codes {
		if _, ok := found[code]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrRuleNoteCodeNotFound, code)
		}
	}
	return codes, nil
}

func initialNoteState(kind string) string {
	if kind == ruleNoteKindContradiction {
		return ruleNoteOpen
	}
	return ""
}

func (m *taskManager) AddRuleNote(ctx context.Context, draft RuleNoteDraft) (RuleNote, error) {
	requesterID := draft.RequesterID
	if err := m.requireRuleAccess(ctx, requesterID, false); err != nil {
		return RuleNote{}, err
	}
	rule, err := m.GetRuleByCode(ctx, requesterID, draft.RuleCode)
	if err != nil {
		return RuleNote{}, err
	}
	kind, err := settledChoice(draft.Kind, ruleNoteKindNote, ruleNoteKinds, "kind")
	if err != nil {
		return RuleNote{}, err
	}
	source, err := settledChoice(draft.Source, noteByPerson, ruleNoteSources, "source")
	if err != nil {
		return RuleNote{}, err
	}
	text, err := settledNoteText(draft.Text, "a note")
	if err != nil {
		return RuleNote{}, err
	}
	if text == "" {
		return RuleNote{}, invalidNote("a note needs text")
	}
	related, err := m.checkedRelatedCodes(ctx, rule.Code, draft.RelatedCodes)
	if err != nil {
		return RuleNote{}, err
	}
	note := RuleNote{
		ID: newSpecID(), RuleID: rule.ID, RuleCode: rule.Code, Kind: kind, Source: source, State: initialNoteState(kind),
		Text: text, RelatedCodes: related, Links: append([]string{}, keptLinks(draft.Links)...), AuthorID: requesterID,
		CreatedAt: time.Now().UTC().Format(noteTimeForm),
	}
	if err := m.store.Insert(ctx, roleRuleNote, note); err != nil {
		return RuleNote{}, fmt.Errorf("AddRuleNote: %w", err)
	}
	return note, nil
}
