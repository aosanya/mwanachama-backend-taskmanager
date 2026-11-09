package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestListTasks_OffsetPagesTwelveHundredIssuesOfOneBucket(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	const total = 1200
	for i := 0; i < total; i++ {
		createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: fmt.Sprintf("issue %04d", i), Priority: mwanachamataskmanager.TaskPriorityHigh})
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

func TestListTasks_OffsetKeepsRankOrderAcrossPages(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	for i := 0; i < 7; i++ {
		createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: fmt.Sprintf("issue %d", i)})
	}
	first, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	second, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatal(err)
	}
	all, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	joined := append(append([]mwanachamataskmanager.Task{}, first...), second...)
	for i := range joined {
		if joined[i].ID != all[i].ID {
			t.Fatalf("page position %d is %q, the whole list has %q", i, joined[i].Title, all[i].Title)
		}
	}
}

func TestListTasks_NegativeOffsetIsRefused(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{Offset: -1})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
		t.Fatalf("negative offset error = %v, want ErrInvalidFilter", err)
	}
}
