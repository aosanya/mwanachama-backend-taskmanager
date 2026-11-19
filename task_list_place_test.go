package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"net/http"
	"testing"
)

func TestListTasks_AStaleStoredBacklogIdIsInTheFirstBacklog(t *testing.T) {
	ctx := context.Background()
	mgr, db, table := newTestManagerWithDB(t)
	if _, err := mgr.CreateBacklog(ctx, "Release", []string{"Plan", "Ship"}); err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	stale := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "stale id"})
	empty := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "empty id"})
	moved := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "moved"})
	if err := db.Table(table).Where("id = ?", stale.ID).Updates(map[string]any{"backlog": "ghost", "step": "x"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table(table).Where("id = ?", empty.ID).Updates(map[string]any{"backlog": "", "step": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, moved.ID, "release", "ship"); err != nil {
		t.Fatalf("MoveTaskToStep: %v", err)
	}
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog"}), "stale id", "empty id")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "Release"}), "moved")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "Release", Step: "ship"}), "moved")
}
func TestListTasks_AStaleIdTaskMatchesTheStepItsStatusNamesAndNoOtherStep(t *testing.T) {
	ctx := context.Background()
	mgr, db, table := newTestManagerWithDB(t)
	stale := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "stale id"})
	if err := db.Table(table).Where("id = ?", stale.ID).Updates(map[string]any{"backlog": "ghost", "step": "x"}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(ctx, stale.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Backlog != "backlog" || got.Step != "to-do" {
		t.Fatalf("GetTask places it at %q/%q, want backlog/to-do", got.Backlog, got.Step)
	}
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "To do"}), "stale id")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Step: "To do"}), "stale id")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "Doing"}))
}
func TestListTasks_AStaleIdTaskWhoseStatusNoStepNamesIsInTheBacklogAtNoStep(t *testing.T) {
	ctx := context.Background()
	mgr, db, table := newTestManagerWithDB(t)
	if _, err := mgr.ReplaceBacklogs(ctx, []models.Backlog{{ID: "backlog", Name: "Backlog", Steps: []models.BacklogStep{{ID: "open", Name: "Open", Statuses: []models.TaskStatus{models.TaskStatusPending}}}}}); err != nil {
		t.Fatalf("ReplaceBacklogs: %v", err)
	}
	stale := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "stale id"})
	if err := db.Table(table).Where("id = ?", stale.ID).Updates(map[string]any{"backlog": "ghost", "step": "x", "status": "completed"}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(ctx, stale.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Backlog != "backlog" || got.Step != "" {
		t.Fatalf("GetTask places it at %q/%q, want backlog at no step", got.Backlog, got.Step)
	}
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog"}), "stale id")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "Open"}))
}
func TestListTasks_OffsetPagesAThousandTwoHundredIssuesOfOneBucket(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	const total = 1200
	for i := 0; i < total; i++ {
		mustCreate(t, mgr, mwanachamataskmanager.Task{Title: fmt.Sprintf("issue %04d", i), Priority: mwanachamataskmanager.TaskPriorityHigh})
	}
	seen := map[string]bool{}
	for offset := 0; ; offset += 500 {
		page, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "To do", Priority: mwanachamataskmanager.TaskPriorityHigh, Offset: offset})
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
		mustCreate(t, mgr, mwanachamataskmanager.Task{Title: fmt.Sprintf("issue %d", i)})
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
func TestListTasksRoute_OffsetPagesAndANegativeOffsetIs400(t *testing.T) {
	b := newBoard(t)
	for i := 0; i < 5; i++ {
		b.create(map[string]any{"title": fmt.Sprintf("issue %d", i)})
	}
	type tasksReply struct {
		Items []map[string]any `json:"items"`
	}
	var whole, window tasksReply
	code, raw := b.call(http.MethodGet, "/tasks", nil)
	if code != http.StatusOK || json.Unmarshal(raw, &whole) != nil || len(whole.Items) != 5 {
		t.Fatalf("GET /tasks = %d %s", code, raw)
	}
	code, raw = b.call(http.MethodGet, "/tasks?offset=3&limit=10", nil)
	if code != http.StatusOK || json.Unmarshal(raw, &window) != nil || len(window.Items) != 2 {
		t.Fatalf("GET /tasks?offset=3 = %d %s", code, raw)
	}
	all, page := whole.Items, window.Items
	if page[0]["id"] != all[3]["id"] || page[1]["id"] != all[4]["id"] {
		t.Fatalf("offset page is not the tail of the whole list")
	}
	if code, _ = b.call(http.MethodGet, "/tasks?offset=-1", nil); code != http.StatusBadRequest {
		t.Fatalf("GET /tasks?offset=-1 = %d, want 400", code)
	}
}
