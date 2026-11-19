package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type PartyRows interface {
	RowExists(ctx context.Context, recordRowID string) (bool, error)
	RowChain(ctx context.Context, recordRowID string) ([]string, error)
}

func (m *taskManager) UsePartyRows(rows PartyRows) { m.partyRows = rows }

func canonicalRecordRowID(recordRowID string) (string, bool) {
	recordRowID = strings.TrimSpace(recordRowID)
	if len(recordRowID) != recordRowIDLength {
		return "", false
	}
	parsed, err := uuid.Parse(recordRowID)
	if err != nil || parsed == uuid.Nil {
		return "", false
	}
	return parsed.String(), true
}

func (m *taskManager) requirePartyRow(ctx context.Context, recordRowID string) error {
	if m.partyRows == nil {
		return nil
	}
	exists, err := m.partyRows.RowExists(ctx, recordRowID)
	if err != nil {
		return fmt.Errorf("check record row %s: %w", recordRowID, err)
	}
	if !exists {
		return fmt.Errorf("%w: record row %s", ErrProjectPartyRowUnknown, recordRowID)
	}
	return nil
}

func (m *taskManager) filterRowIDs(ctx context.Context, filter ProjectFilter) ([]string, error) {
	seen := map[string]bool{}
	var ids []string
	add := func(recordRowID string) error {
		canonical, ok := canonicalRecordRowID(recordRowID)
		if !ok {
			return fmt.Errorf("%w: record row id %q is not a record row id", ErrInvalidFilter, recordRowID)
		}
		if !seen[canonical] {
			seen[canonical] = true
			ids = append(ids, canonical)
		}
		return nil
	}
	if strings.TrimSpace(filter.PartyRowID) != "" {
		if err := add(filter.PartyRowID); err != nil {
			return nil, err
		}
		chain, err := m.partyRowChain(ctx, filter.PartyRowID)
		if err != nil {
			return nil, err
		}
		for _, id := range chain {
			if err := add(id); err != nil {
				return nil, err
			}
		}
	}
	for _, id := range filter.PartyRowIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		if err := add(id); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (m *taskManager) partyRowChain(ctx context.Context, recordRowID string) ([]string, error) {
	if m.partyRows == nil {
		return nil, nil
	}
	canonical, ok := canonicalRecordRowID(recordRowID)
	if !ok {
		return nil, nil
	}
	chain, err := m.partyRows.RowChain(ctx, canonical)
	if err != nil {
		return nil, fmt.Errorf("follow record row %s: %w", canonical, err)
	}
	return chain, nil
}
