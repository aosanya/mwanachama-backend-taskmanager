package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	CaseStatusOpen      = models.CaseStatusOpen
	CaseStatusDone      = models.CaseStatusDone
	CaseStatusCancelled = models.CaseStatusCancelled
)

var (
	ErrCaseNotFound      = errors.New("case not found")
	ErrCaseNotOpen       = errors.New("case is not open")
	ErrTaskAlreadyInCase = errors.New("task already serves another case")
	ErrInvalidCase       = errors.New("invalid case")
)

const (
	TopicCaseOpened = "case.opened"
	TopicCaseClosed = "case.closed"
)

type CaseOpenedPayload struct {
	CaseID      string `json:"case_id"`
	RequesterID string `json:"requester_id"`
	SubjectKind string `json:"subject_kind,omitempty"`
	SubjectID   string `json:"subject_id,omitempty"`
}

type CaseClosedPayload struct {
	CaseID   string     `json:"case_id"`
	Status   CaseStatus `json:"status"`
	ClosedAt string     `json:"closed_at"`
}

func (m *taskManager) OpenCase(ctx context.Context, requesterID string, c Case) (Case, error) {
	if requesterID == "" {
		return Case{}, fmt.Errorf("%w: a case needs a requester", ErrInvalidCase)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	c.ID = newSpecID()
	c.RequesterID = requesterID
	c.Status = CaseStatusOpen
	c.OpenedAt = now
	c.ClosedAt = ""
	c.CreatedAt = now
	c.UpdatedAt = now
	if err := m.store.Insert(ctx, roleCase, c); err != nil {
		return Case{}, fmt.Errorf("OpenCase: %w", err)
	}
	m.publish(ctx, TopicCaseOpened, CaseOpenedPayload{
		CaseID:      c.ID,
		RequesterID: c.RequesterID,
		SubjectKind: c.SubjectKind,
		SubjectID:   c.SubjectID,
	})
	return c, nil
}

func (m *taskManager) GetCase(ctx context.Context, caseID string) (Case, error) {
	var c Case
	q := m.store.Query(ctx, roleCase).Where("id = ?", caseID)
	if err := m.store.Take(q, roleCase, &c, ErrCaseNotFound); err != nil {
		if errors.Is(err, ErrCaseNotFound) {
			return Case{}, err
		}
		return Case{}, fmt.Errorf("GetCase: %w", err)
	}
	return c, nil
}

func (m *taskManager) ListCases(ctx context.Context, filter CaseFilter) ([]Case, error) {
	q := m.store.Query(ctx, roleCase)
	if filter.RequesterID != "" {
		q = q.Where("requester_id = ?", filter.RequesterID)
	}
	if filter.SubjectKind != "" {
		q = q.Where("subject_kind = ?", filter.SubjectKind)
	}
	if filter.SubjectID != "" {
		q = q.Where("subject_id = ?", filter.SubjectID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", string(filter.Status))
	}
	q = q.Order("opened_at DESC").Order("id").Limit(maxListPage)
	out, err := specstore.List[Case](m.store, q, roleCase)
	if err != nil {
		return nil, fmt.Errorf("ListCases: %w", err)
	}
	return out, nil
}

func (m *taskManager) LinkCaseTask(ctx context.Context, caseID, taskID string) error {
	c, err := m.GetCase(ctx, caseID)
	if err != nil {
		return err
	}
	if c.Status != CaseStatusOpen {
		return ErrCaseNotOpen
	}
	if _, err := m.GetTask(ctx, taskID); err != nil {
		return err
	}
	existing, found, err := m.caseOfTask(ctx, taskID)
	if err != nil {
		return err
	}
	if found {
		if existing == caseID {
			return nil
		}
		return ErrTaskAlreadyInCase
	}
	link := CaseTask{TaskID: taskID, CaseID: caseID, LinkedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := m.store.Insert(ctx, roleCaseTask, link); err != nil {
		return fmt.Errorf("LinkCaseTask: %w", err)
	}
	return nil
}

func (m *taskManager) ListCaseTasks(ctx context.Context, caseID string) ([]Task, error) {
	if _, err := m.GetCase(ctx, caseID); err != nil {
		return nil, err
	}
	links, err := m.caseLinks(ctx, caseID)
	if err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(links))
	for _, l := range links {
		t, err := m.GetTask(ctx, l.TaskID)
		if errors.Is(err, ErrTaskNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("ListCaseTasks: %w", err)
		}
		out = append(out, t)
	}
	return out, nil
}

func (m *taskManager) caseLinks(ctx context.Context, caseID string) ([]CaseTask, error) {
	q := m.store.Query(ctx, roleCaseTask).Where("case_id = ?", caseID).Order("linked_at").Order("task_id")
	links, err := specstore.List[CaseTask](m.store, q, roleCaseTask)
	if err != nil {
		return nil, fmt.Errorf("caseLinks: %w", err)
	}
	return links, nil
}

func (m *taskManager) caseOfTask(ctx context.Context, taskID string) (string, bool, error) {
	q := m.store.Query(ctx, roleCaseTask).Where("task_id = ?", taskID).Limit(1)
	links, err := specstore.List[CaseTask](m.store, q, roleCaseTask)
	if err != nil {
		return "", false, fmt.Errorf("caseOfTask: %w", err)
	}
	if len(links) == 0 {
		return "", false, nil
	}
	return links[0].CaseID, true, nil
}

func settlesCase(s TaskStatus) bool {
	return s == TaskStatusCompleted || s == TaskStatusCancelled
}

func (m *taskManager) closeCaseIfSettled(ctx context.Context, taskID string) error {
	caseID, found, err := m.caseOfTask(ctx, taskID)
	if err != nil || !found {
		return err
	}
	links, err := m.caseLinks(ctx, caseID)
	if err != nil {
		return err
	}
	for _, l := range links {
		t, err := m.GetTask(ctx, l.TaskID)
		if errors.Is(err, ErrTaskNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if !settlesCase(t.Status) {
			return nil
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res := m.store.Query(ctx, roleCase).
		Where("id = ? AND status = ?", caseID, string(CaseStatusOpen)).
		Updates(map[string]any{"status": string(CaseStatusDone), "closed_at": now, "updated_at": now})
	if res.Error != nil {
		return fmt.Errorf("closeCaseIfSettled: %w", res.Error)
	}
	if res.RowsAffected == 1 {
		m.publish(ctx, TopicCaseClosed, CaseClosedPayload{CaseID: caseID, Status: CaseStatusDone, ClosedAt: now})
	}
	return nil
}
