package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

type FileOverlapsFilter struct {
	TaskID string   `query:"taskID"`
	Files  []string `query:"files"`

	RequesterID string `query:"-" json:"-"`
}

type FileOverlapPair struct {
	Asked  string `json:"asked"`
	Theirs string `json:"theirs"`
}

type FileOverlap struct {
	ID          string            `json:"id"`
	Code        string            `json:"code"`
	Title       string            `json:"title"`
	Step        string            `json:"step"`
	Overlapping []FileOverlapPair `json:"overlapping"`
}

type FileOverlaps struct {
	Items []FileOverlap `json:"items"`
}

func (m *taskManager) FileOverlaps(ctx context.Context, filter FileOverlapsFilter) (FileOverlaps, error) {
	out := FileOverlaps{Items: []FileOverlap{}}
	asked, err := m.askedFiles(ctx, filter)
	if err != nil {
		return out, fmt.Errorf("FileOverlaps: %w", err)
	}
	if len(asked) == 0 {
		return out, nil
	}
	candidates, err := m.inProgressWithFiles(ctx, filter)
	if err != nil {
		return out, fmt.Errorf("FileOverlaps: %w", err)
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return out, fmt.Errorf("FileOverlaps: %w", err)
	}
	for _, task := range candidates {
		pairs := overlappingPairs(asked, task.Files)
		if task.ID == filter.TaskID || len(pairs) == 0 {
			continue
		}
		_, step := placeNames(backlogs, pendingRow{Status: string(task.Status), Backlog: task.Backlog, Step: task.Step})
		out.Items = append(out.Items, FileOverlap{ID: task.ID, Code: task.Code, Title: task.Title, Step: step, Overlapping: pairs})
	}
	return out, nil
}

func (m *taskManager) askedFiles(ctx context.Context, filter FileOverlapsFilter) (FileList, error) {
	taskID := strings.TrimSpace(filter.TaskID)
	if taskID == "" && len(filter.Files) == 0 {
		return nil, fmt.Errorf("%w: send taskID or files", ErrInvalidFilter)
	}
	listed, err := checkFiles(filter.Files)
	if err != nil {
		return nil, err
	}
	if taskID == "" {
		return listed, nil
	}
	issue, err := m.GetTaskFor(ctx, filter.RequesterID, taskID)
	if err != nil {
		return nil, err
	}
	return append(issue.Files, listed...), nil
}

func (m *taskManager) inProgressWithFiles(ctx context.Context, filter FileOverlapsFilter) ([]Task, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return nil, err
	}
	q, err := m.scopedTasks(ctx, "", "", nil, filter.RequesterID)
	if err != nil {
		return nil, err
	}
	where, args := atInProgressSteps(backlogs)
	return specstore.List[Task](m.store, q.Where(where, args...).Order(taskOrder).Limit(maxListPage), roleTask)
}

func overlappingPairs(asked, theirs FileList) []FileOverlapPair {
	pairs := []FileOverlapPair{}
	for _, mine := range asked {
		for _, other := range theirs {
			if filesOverlap(mine, other) {
				pairs = append(pairs, FileOverlapPair{Asked: mine, Theirs: other})
			}
		}
	}
	return pairs
}

func filesOverlap(a, b string) bool {
	pathA, pathB := pathOfFileEntry(a), pathOfFileEntry(b)
	if a[:len(a)-len(pathA)] != b[:len(b)-len(pathB)] {
		return false
	}
	return pathA == pathB || insideFolder(pathA, pathB) || insideFolder(pathB, pathA)
}

func insideFolder(folder, path string) bool {
	return strings.HasSuffix(folder, "/") && strings.HasPrefix(path, folder)
}
