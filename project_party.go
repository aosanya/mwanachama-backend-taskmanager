package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const (
	maxContractReference = 200
	maxPartyName         = 200
	recordRowIDLength    = 36
)

var partyRolePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

type ProjectFilter struct {
	PartyRowID        string   `query:"party_row_id"`
	PartyRowIDs       []string `query:"-"`
	PartyRole         string   `query:"party_role"`
	ContractReference string   `query:"contract_reference"`
}

func checkContractReference(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if utf8.RuneCountInString(reference) > maxContractReference || !utf8.ValidString(reference) {
		return "", fmt.Errorf("%w: contract_reference is longer than %d characters or not valid text", ErrInvalidTask, maxContractReference)
	}
	if strings.IndexFunc(reference, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%w: contract_reference carries a control character", ErrInvalidTask)
	}
	return reference, nil
}

func checkPartyRole(role string) error {
	if !partyRolePattern.MatchString(role) {
		return fmt.Errorf("%w: role %q is not lower-case letters, digits and underscores starting with a letter, at most 63 characters", ErrInvalidProjectParty, role)
	}
	return nil
}

func checkPartyRecordRowID(recordRowID string) (string, error) {
	canonical, ok := canonicalRecordRowID(recordRowID)
	if !ok {
		return "", fmt.Errorf("%w: record_row_id %q is not a record row id", ErrInvalidProjectParty, recordRowID)
	}
	return canonical, nil
}

func checkPartyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > maxPartyName || !utf8.ValidString(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%w: party_name is longer than %d characters, not valid text or carries a control character", ErrInvalidProjectParty, maxPartyName)
	}
	return name, nil
}

func (m *taskManager) AddProjectParty(ctx context.Context, party ProjectParty) (ProjectParty, error) {
	party.Role = strings.TrimSpace(party.Role)
	if err := checkPartyRole(party.Role); err != nil {
		return ProjectParty{}, err
	}
	recordRowID, err := checkPartyRecordRowID(party.RecordRowID)
	if err != nil {
		return ProjectParty{}, err
	}
	party.RecordRowID = recordRowID
	name, err := checkPartyName(party.PartyName)
	if err != nil {
		return ProjectParty{}, err
	}
	party.PartyName = name
	if _, err := m.GetProject(ctx, party.ProjectID); err != nil {
		return ProjectParty{}, err
	}
	if err := m.requirePartyRow(ctx, party.RecordRowID); err != nil {
		return ProjectParty{}, fmt.Errorf("AddProjectParty: %w", err)
	}
	held, err := m.countProjectParties(ctx, party.ProjectID, party.Role, party.RecordRowID)
	if err != nil {
		return ProjectParty{}, fmt.Errorf("AddProjectParty: %w", err)
	}
	if held > 0 {
		return ProjectParty{}, fmt.Errorf("%w: role %q, record row %s", ErrProjectPartyExists, party.Role, party.RecordRowID)
	}
	party.AddedAt = time.Now().UTC().Format(time.RFC3339)
	if err := m.store.Insert(ctx, roleProjectParty, party); err != nil {
		return ProjectParty{}, fmt.Errorf("AddProjectParty: %w", err)
	}
	return party, nil
}

func (m *taskManager) countProjectParties(ctx context.Context, projectID, role, recordRowID string) (int64, error) {
	var held int64
	err := m.store.Query(ctx, roleProjectParty).
		Where("project_id = ? AND role = ? AND record_row_id = ?", projectID, role, recordRowID).
		Count(&held).Error
	return held, err
}

func (m *taskManager) RemoveProjectParty(ctx context.Context, projectID, role, recordRowID string) error {
	recordRowID, err := checkPartyRecordRowID(recordRowID)
	if err != nil {
		return err
	}
	if _, err := m.GetProject(ctx, projectID); err != nil {
		return err
	}
	res := m.store.Query(ctx, roleProjectParty).
		Where("project_id = ? AND role = ? AND record_row_id = ?", projectID, role, recordRowID).
		Delete(nil)
	if res.Error != nil {
		return fmt.Errorf("RemoveProjectParty: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%w: role %q, record row %s", ErrProjectPartyNotFound, role, recordRowID)
	}
	return nil
}

func (m *taskManager) ListProjectParties(ctx context.Context, projectID string) ([]ProjectParty, error) {
	if _, err := m.GetProject(ctx, projectID); err != nil {
		return nil, err
	}
	q := m.store.Query(ctx, roleProjectParty).Where("project_id = ?", projectID).
		Order("role, added_at, record_row_id").Limit(maxListPage)
	out, err := specstore.List[ProjectParty](m.store, q, roleProjectParty)
	if err != nil {
		return nil, fmt.Errorf("ListProjectParties: %w", err)
	}
	return out, nil
}

func (m *taskManager) FindProjects(ctx context.Context, filter ProjectFilter) ([]Project, error) {
	q := m.store.Query(ctx, roleProject).Where("deleted = ?", false)
	reference, err := checkContractReference(filter.ContractReference)
	if err != nil {
		return nil, fmt.Errorf("FindProjects: %w", errors.Join(ErrInvalidFilter, err))
	}
	if reference != "" {
		q = q.Where("contract_reference = ?", reference)
	}
	role := strings.TrimSpace(filter.PartyRole)
	if role != "" && !partyRolePattern.MatchString(role) {
		return nil, fmt.Errorf("FindProjects: %w: party_role %q is not a role word", ErrInvalidFilter, role)
	}
	ids, err := m.filterRowIDs(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("FindProjects: %w", err)
	}
	if role != "" || len(ids) > 0 {
		parties := m.store.Query(ctx, roleProjectParty).Select("project_id")
		if role != "" {
			parties = parties.Where("role = ?", role)
		}
		if len(ids) > 0 {
			parties = parties.Where("record_row_id IN ?", ids)
		}
		q = q.Where("id IN (?)", parties)
	}
	q = q.Order("project_name").Limit(maxListPage)
	out, err := specstore.List[Project](m.store, q, roleProject)
	if err != nil {
		return nil, fmt.Errorf("FindProjects: %w", err)
	}
	return out, nil
}
