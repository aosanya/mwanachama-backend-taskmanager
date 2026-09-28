package routes_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestZZProbeStatuses(t *testing.T) {
	tm, _, _ := w21RaceManagerAndDB(t, "zzprobe")
	ctx := context.Background()
	srv := httptest.NewServer(w21RaceMux(tm))
	defer srv.Close()

	run, err := tm.CreateWorkflowRun(ctx, "probe-run", "t", "i")
	if err != nil {
		t.Fatal(err)
	}
	task, err := tm.CreateTask(ctx, mwanachamataskmanager.Task{Title: "t", WorkflowRunID: run.ID})
	if err != nil {
		t.Fatal(err)
	}
	todo, err := tm.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
		Title: "x", Instructions: "y", ParentTaskID: task.ID, WorkflowRunID: run.ID})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(map[string]any{"status": "completed"})
	req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/todos/%s/status", srv.URL, todo.ID), &buf)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	t.Logf("PUT status -> %d %s", resp.StatusCode, b)

	req2, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/workflow-runs/%s/artifacts", srv.URL, run.ID), nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	t.Logf("DELETE artifacts -> %d %s", resp2.StatusCode, b2)
}
