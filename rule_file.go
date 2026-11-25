package mwanachamataskmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

var (
	ErrInvalidRuleFile   = errors.New("a rule file link needs a repo, a path that stays inside the repo, and a line of zero or more")
	ErrDuplicateRuleFile = errors.New("the rule already links to that file")
	ErrRuleFileNotFound  = errors.New("rule file link not found")
)

type RepoPathChecker interface {
	PathExists(ctx context.Context, repo, path string) (bool, error)
}

func WithRepoPathChecker(mgr TaskManager, checker RepoPathChecker) TaskManager {
	if impl, ok := mgr.(*taskManager); ok && impl != nil {
		impl.pathChecker = checker
	}
	return mgr
}

func ruleFileKey(ruleID, repo, path string, line int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s%d:%s%d:%s%d", len(ruleID), ruleID, len(repo), repo, len(path), path, line)))
	return hex.EncodeToString(sum[:])
}

func (m *taskManager) ruleFile(ctx context.Context, ruleID, id string) (RuleFile, error) {
	var file RuleFile
	q := m.store.Query(ctx, roleRuleFile).Where("id = ?", id).Where("rule_id = ?", ruleID)
	if err := m.store.Take(q, roleRuleFile, &file, ErrRuleFileNotFound); err != nil {
		return RuleFile{}, err
	}
	return file, nil
}

func (m *taskManager) missingPathWarning(ctx context.Context, repo, path string) string {
	if m.pathChecker == nil {
		return ""
	}
	exists, err := m.pathChecker.PathExists(ctx, repo, path)
	if err != nil {
		return fmt.Sprintf("%s in %s could not be checked: %v", path, repo, err)
	}
	if !exists {
		return fmt.Sprintf("%s is not on main in %s", path, repo)
	}
	return ""
}

func (m *taskManager) AddRuleFile(ctx context.Context, requesterID, code, repo, path string, line int, note string) (RuleFileResult, error) {
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return RuleFileResult{}, err
	}
	repo = strings.TrimSpace(repo)
	cleanPath, ok := repoRelativePath(path)
	if !ok || repo == "" || line < 0 {
		return RuleFileResult{}, ErrInvalidRuleFile
	}
	rule, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return RuleFileResult{}, err
	}
	file := RuleFile{
		ID: ruleFileKey(rule.ID, repo, cleanPath, line), RuleID: rule.ID, Repo: repo, Path: cleanPath, Line: line,
		Note: strings.TrimSpace(note), AddedBy: requesterID, CreatedAt: nowStamp(),
	}
	if _, err := m.ruleFile(ctx, rule.ID, file.ID); err == nil {
		return RuleFileResult{}, ErrDuplicateRuleFile
	} else if !errors.Is(err, ErrRuleFileNotFound) {
		return RuleFileResult{}, fmt.Errorf("AddRuleFile: %w", err)
	}
	if err := m.store.Insert(ctx, roleRuleFile, file); err != nil {
		if _, lookupErr := m.ruleFile(ctx, rule.ID, file.ID); lookupErr == nil {
			return RuleFileResult{}, ErrDuplicateRuleFile
		}
		return RuleFileResult{}, fmt.Errorf("AddRuleFile: %w", err)
	}
	return RuleFileResult{File: file, Warning: m.missingPathWarning(ctx, repo, cleanPath)}, nil
}

func (m *taskManager) pathIsGone(ctx context.Context, repo, path string) bool {
	if m.pathChecker == nil {
		return false
	}
	exists, err := m.pathChecker.PathExists(ctx, repo, path)
	return err == nil && !exists
}

func (m *taskManager) ListRuleFiles(ctx context.Context, requesterID, code string) ([]RuleFileLink, error) {
	rule, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return nil, err
	}
	q := m.store.Query(ctx, roleRuleFile).Where("rule_id = ?", rule.ID).Order("created_at").Order("id").Limit(maxListPage)
	out, err := specstore.List[RuleFile](m.store, q, roleRuleFile)
	if err != nil {
		return nil, fmt.Errorf("ListRuleFiles: %w", err)
	}
	links := make([]RuleFileLink, 0, len(out))
	for _, file := range out {
		links = append(links, RuleFileLink{RuleFile: file, Missing: m.pathIsGone(ctx, file.Repo, file.Path)})
	}
	return links, nil
}

func (m *taskManager) RemoveRuleFile(ctx context.Context, requesterID, code, fileID string) error {
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return err
	}
	rule, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return err
	}
	res := m.store.Query(ctx, roleRuleFile).Where("id = ?", fileID).Where("rule_id = ?", rule.ID).Delete(nil)
	if res.Error != nil {
		return fmt.Errorf("RemoveRuleFile: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrRuleFileNotFound
	}
	return nil
}
