package mwanachamataskmanager

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

func (m *taskManager) rulesWithCodes(ctx context.Context, codes []string) ([]RuleRecord, error) {
	q := m.store.Query(ctx, roleRule).Where("code IN ?", codes).Limit(len(codes))
	rules, err := specstore.List[RuleRecord](m.store, q, roleRule)
	if err != nil {
		return nil, fmt.Errorf("rules by code: %w", err)
	}
	return rules, nil
}

func invalidNoteFilter(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRuleNoteFilter, fmt.Sprintf(format, args...))
}

type ruleNoteQuery struct {
	ruleID  string
	kinds   []string
	state   string
	related string
	since   time.Time
	hasTime bool
	limit   int
}

func checkedKinds(kinds []string) ([]string, error) {
	out := []string{}
	for _, kind := range kinds {
		kind = strings.TrimSpace(kind)
		if !slices.Contains(ruleNoteKinds, kind) {
			return nil, invalidNoteFilter("kind is %s, not %q", strings.Join(ruleNoteKinds, ", "), kind)
		}
		out = append(out, kind)
	}
	return out, nil
}

func (m *taskManager) ruleNoteQueryOf(ctx context.Context, requesterID string, filter RuleNoteFilter) (ruleNoteQuery, error) {
	query := ruleNoteQuery{state: strings.TrimSpace(filter.State), related: strings.TrimSpace(filter.Related), limit: filter.Limit}
	if query.limit < 0 || query.limit > maxNoteLimit {
		return query, invalidNoteFilter("limit %d is not between 1 and %d", query.limit, maxNoteLimit)
	}
	if query.limit == 0 {
		query.limit = defaultNoteListCap
	}
	if query.state != "" && !slices.Contains(ruleNoteStates, query.state) {
		return query, invalidNoteFilter("state is %s, not %q", strings.Join(ruleNoteStates, " or "), query.state)
	}
	kinds, err := checkedKinds(filter.Kind)
	if err != nil {
		return query, err
	}
	query.kinds = kinds
	if since := strings.TrimSpace(filter.Since); since != "" {
		moment, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return query, invalidNoteFilter("since %q is not an RFC 3339 time", since)
		}
		query.since, query.hasTime = moment, true
	}
	if code := strings.TrimSpace(filter.Rule); code != "" {
		rule, err := m.GetRuleByCode(ctx, requesterID, code)
		if err != nil {
			return query, err
		}
		query.ruleID = rule.ID
	}
	return query, nil
}

func (q ruleNoteQuery) keeps(note RuleNote) bool {
	if q.related != "" && note.RuleCode != q.related && !slices.Contains(note.RelatedCodes, q.related) {
		return false
	}
	if !q.hasTime {
		return true
	}
	written, err := time.Parse(time.RFC3339Nano, note.CreatedAt)
	return err != nil || written.After(q.since)
}

func normalisedNote(note RuleNote) RuleNote {
	if note.RelatedCodes == nil {
		note.RelatedCodes = []string{}
	}
	if note.Links == nil {
		note.Links = []string{}
	}
	return note
}

func (m *taskManager) ruleNoteRows(ctx context.Context, query ruleNoteQuery) ([]RuleNote, error) {
	var out []RuleNote
	for offset := 0; ; offset += maxListPage {
		q := m.store.Query(ctx, roleRuleNote)
		if query.ruleID != "" {
			q = q.Where("rule_id = ?", query.ruleID)
		}
		if len(query.kinds) > 0 {
			q = q.Where("kind IN ?", query.kinds)
		}
		if query.state != "" {
			q = q.Where("state = ?", query.state)
		}
		page, err := specstore.List[RuleNote](m.store, q.Order("created_at").Order("id").Limit(maxListPage).Offset(offset), roleRuleNote)
		if err != nil {
			return nil, err
		}
		for _, note := range page {
			if note = normalisedNote(note); query.keeps(note) {
				out = append(out, note)
			}
		}
		if len(page) < maxListPage {
			return out, nil
		}
	}
}

func (m *taskManager) ListRuleNotes(ctx context.Context, filter RuleNoteFilter) ([]RuleNote, error) {
	if err := m.requireRuleAccess(ctx, filter.RequesterID, false); err != nil {
		return nil, err
	}
	query, err := m.ruleNoteQueryOf(ctx, filter.RequesterID, filter)
	if err != nil {
		return nil, err
	}
	notes, err := m.ruleNoteRows(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ListRuleNotes: %w", err)
	}
	if len(notes) > query.limit {
		notes = notes[len(notes)-query.limit:]
	}
	return notes, nil
}

func (m *taskManager) latestRuleNotes(ctx context.Context, ruleID string) ([]RuleNote, int, error) {
	var total int64
	if err := m.store.Query(ctx, roleRuleNote).Where("rule_id = ?", ruleID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	q := m.store.Query(ctx, roleRuleNote).Where("rule_id = ?", ruleID).Order("created_at desc").Order("id desc").Limit(ruleNotesOnARead)
	newest, err := specstore.List[RuleNote](m.store, q, roleRuleNote)
	if err != nil {
		return nil, 0, err
	}
	notes := make([]RuleNote, 0, len(newest))
	for i := len(newest) - 1; i >= 0; i-- {
		notes = append(notes, normalisedNote(newest[i]))
	}
	return notes, int(total), nil
}

func (m *taskManager) ReadRule(ctx context.Context, requesterID, code string) (RuleWithNotes, error) {
	rule, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return RuleWithNotes{}, err
	}
	compatible, err := m.compatibleCodesOf(ctx, rule.ID)
	if err != nil {
		return RuleWithNotes{}, fmt.Errorf("ReadRule: %w", err)
	}
	notes, total, err := m.latestRuleNotes(ctx, rule.ID)
	if err != nil {
		return RuleWithNotes{}, fmt.Errorf("ReadRule: %w", err)
	}
	return RuleWithNotes{RuleRecord: rule, Compatible: compatible, Notes: notes, NoteCount: total}, nil
}
