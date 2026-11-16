package mwanachamataskmanager_test

import (
	"context"
	"fmt"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestListTasks_OffsetPagesTwelveHundredIssuesOfOneBucket(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	const total = 1200
	for i := 0; i < total; i++ {
		mustCreate(t, mgr, mwanachamataskmanager.Task{Title: fmt.Sprintf("issue %04d", i), Priority: mwanachamataskmanager.TaskPriorityHigh})
	}
	seen := map[string]bool{}
	for offset := 0; ; offset += 500 {
		page, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Priority: mwanachamataskmanager.TaskPriorityHigh, Offset: offset})
		if err != nil {
			t.Fatalf("ListTasks offset %d: %v", offset, err)
		}
		if len(page) == 0 {
			break
		}
		if len(page) > 500 {
			t.Fatalf("page of %d is over the 500 cap", len(page))
		}
		for _, task := range page {
			if seen[task.ID] {
				t.Fatalf("%s appeared on two pages", task.Title)
			}
			seen[task.ID] = true
		}
	}
	if len(seen) != total {
		t.Fatalf("paging reached %d of %d issues", len(seen), total)
	}
}
