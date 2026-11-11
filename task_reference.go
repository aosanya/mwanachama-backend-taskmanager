package mwanachamataskmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const (
	referenceFile   = "file"
	referenceFolder = "folder"
	referenceIssue  = "issue"
	referenceScreen = "screen"
	referenceLink   = "link"

	referenceByPerson    = "person"
	referenceByAssistant = "assistant"
)

var (
	ErrInvalidTaskReference   = errors.New("a reference needs a kind (file, folder, issue, screen or link), a target, and for a file or folder a repo; a path stays inside the repo and a link is http or https")
	ErrDuplicateTaskReference = errors.New("the issue already refers to that")
	ErrTaskReferenceNotFound  = errors.New("reference not found")
)

func repoKind(kind string) bool {
	return kind == referenceFile || kind == referenceFolder
}

func repoRelativePath(raw string) (string, bool) {
	var kept []string
	for _, part := range strings.Split(strings.TrimSpace(raw), "/") {
		switch part {
		case "", ".":
		case "..":
			return "", false
		default:
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "/"), len(kept) > 0
}

func webAddress(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return raw, true
}

func cleanReference(kind, repo, target string) (string, string, bool) {
	repo = strings.TrimSpace(repo)
	target = strings.TrimSpace(target)
	switch kind {
	case referenceFile, referenceFolder:
		path, ok := repoRelativePath(target)
		return repo, path, ok && repo != ""
	case referenceLink:
		address, ok := webAddress(target)
		return "", address, ok
	case referenceIssue, referenceScreen:
		return "", target, target != ""
	}
	return "", "", false
}

func taskReferenceKey(taskID, kind, repo, target string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s%d:%s%d:%s%s", len(taskID), taskID, len(kind), kind, len(repo), repo, target)))
	return hex.EncodeToString(sum[:])
}

func (m *taskManager) taskReference(ctx context.Context, taskID, id string) (TaskReference, error) {
	var ref TaskReference
	q := m.store.Query(ctx, roleTaskReference).Where("id = ?", id).Where("task_id = ?", taskID)
	if err := m.store.Take(q, roleTaskReference, &ref, ErrTaskReferenceNotFound); err != nil {
		return TaskReference{}, err
	}
	return ref, nil
}

func (m *taskManager) AddTaskReference(ctx context.Context, requesterID, taskID, kind, repo, target string) (TaskReference, error) {
	return m.saveTaskReference(ctx, requesterID, taskID, kind, repo, target, referenceByPerson)
}

func (m *taskManager) ProposeTaskReference(ctx context.Context, requesterID, taskID, kind, repo, target string) (TaskReference, error) {
	return m.saveTaskReference(ctx, requesterID, taskID, kind, repo, target, referenceByAssistant)
}

func (m *taskManager) saveTaskReference(ctx context.Context, requesterID, taskID, kind, repo, target, source string) (TaskReference, error) {
	repo, target, ok := cleanReference(kind, repo, target)
	if !ok {
		return TaskReference{}, ErrInvalidTaskReference
	}
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return TaskReference{}, err
	}
	ref := TaskReference{
		ID: taskReferenceKey(taskID, kind, repo, target), TaskID: taskID, Kind: kind, Repo: repo, Target: target,
		Source: source, Confirmed: source == referenceByPerson, AddedBy: requesterID, CreatedAt: time.Now().UTC().Format(noteTimeForm),
	}
	if _, err := m.taskReference(ctx, taskID, ref.ID); err == nil {
		return TaskReference{}, ErrDuplicateTaskReference
	} else if !errors.Is(err, ErrTaskReferenceNotFound) {
		return TaskReference{}, fmt.Errorf("saveTaskReference: %w", err)
	}
	if err := m.store.Insert(ctx, roleTaskReference, ref); err != nil {
		if _, lookupErr := m.taskReference(ctx, taskID, ref.ID); lookupErr == nil {
			return TaskReference{}, ErrDuplicateTaskReference
		}
		return TaskReference{}, fmt.Errorf("saveTaskReference: %w", err)
	}
	return ref, nil
}

func (m *taskManager) ConfirmTaskReference(ctx context.Context, requesterID, taskID, referenceID string) (TaskReference, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return TaskReference{}, err
	}
	ref, err := m.taskReference(ctx, taskID, referenceID)
	if err != nil {
		return TaskReference{}, err
	}
	if ref.Confirmed {
		return ref, nil
	}
	res := m.store.Query(ctx, roleTaskReference).Where("id = ?", ref.ID).Where("task_id = ?", taskID).Updates(map[string]any{"confirmed": true})
	if res.Error != nil {
		return TaskReference{}, fmt.Errorf("ConfirmTaskReference: %w", res.Error)
	}
	ref.Confirmed = true
	return ref, nil
}

func (m *taskManager) ListTaskReferences(ctx context.Context, requesterID, taskID string) ([]TaskReference, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return nil, err
	}
	q := m.store.Query(ctx, roleTaskReference).Where("task_id = ?", taskID).Order("created_at").Order("id").Limit(maxListPage)
	out, err := specstore.List[TaskReference](m.store, q, roleTaskReference)
	if err != nil {
		return nil, fmt.Errorf("ListTaskReferences: %w", err)
	}
	return out, nil
}

func (m *taskManager) RemoveTaskReference(ctx context.Context, requesterID, taskID, referenceID string) error {
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return err
	}
	res := m.store.Query(ctx, roleTaskReference).Where("id = ?", referenceID).Where("task_id = ?", taskID).Delete(nil)
	if res.Error != nil {
		return fmt.Errorf("RemoveTaskReference: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrTaskReferenceNotFound
	}
	return nil
}
