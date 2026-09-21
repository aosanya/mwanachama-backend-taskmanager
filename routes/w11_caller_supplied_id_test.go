package routes_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

// wk11Manager and wk11Mux are the same real-mux harness routes_test.go's own
// mux()/doJSON() helpers use elsewhere in this package — duplicated locally
// so this file stands alone as the reproduction for board row W11.
func wk11Manager(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	tables := mwanachamataskmanager.DefaultTableNames("wk11")
	if err := mwanachamataskmanager.Migrate(db, tables); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, tables, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr
}

func wk11Mux(tm mwanachamataskmanager.TaskManager) *http.ServeMux {
	m := http.NewServeMux()
	for _, rt := range routes.Routes(tm) {
		m.HandleFunc(rt.Pattern(""), rt.Handler)
	}
	return m
}

func wk11Post(t *testing.T, m *http.ServeMux, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest("POST", path, bytes.NewReader(b))
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	return rec
}

// Board row W11: CreateTask clears a caller-supplied Task.ID (as the other
// Create<Type> paths do), so a POST /tasks body naming its own "id" gets a
// server-minted one back.
func TestCreateTask_IgnoresCallerSuppliedID(t *testing.T) {
	tm := wk11Manager(t)
	m := wk11Mux(tm)

	const wanted = "attacker-chosen-task-id"
	rec := wk11Post(t, m, "/tasks", map[string]any{"id": wanted, "title": "probe task"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task: got %d, body %s", rec.Code, rec.Body.String())
	}
	var out mwanachamataskmanager.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ID == wanted {
		t.Fatalf("a caller-supplied id was honoured (got %q) — the server must mint its own", out.ID)
	}
}

// Board row W11's second half: with the id server-owned, re-POSTing the same
// body mints a second distinct row instead of colliding on the primary key.
func TestCreateTask_DuplicateCallerSuppliedIDMintsDistinctIDs(t *testing.T) {
	tm := wk11Manager(t)
	m := wk11Mux(tm)

	const id = "attacker-chosen-task-id-2"
	first := wk11Post(t, m, "/tasks", map[string]any{"id": id, "title": "first"})
	if first.Code != http.StatusCreated {
		t.Fatalf("first create: got %d, body %s", first.Code, first.Body.String())
	}
	second := wk11Post(t, m, "/tasks", map[string]any{"id": id, "title": "second"})
	if second.Code != http.StatusCreated {
		t.Fatalf("second create: got %d, body %s", second.Code, second.Body.String())
	}
	var a, b mwanachamataskmanager.Task
	if err := json.Unmarshal(first.Body.Bytes(), &a); err != nil {
		t.Fatalf("decode first: %v", err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &b); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if a.ID == id || b.ID == id || a.ID == b.ID {
		t.Fatalf("expected two distinct server-minted ids, got %q and %q", a.ID, b.ID)
	}
}

// Board row W11 on CreateProject.
func TestCreateProject_IgnoresCallerSuppliedID(t *testing.T) {
	tm := wk11Manager(t)
	m := wk11Mux(tm)

	const wanted = "attacker-chosen-project-id"
	rec := wk11Post(t, m, "/projects", map[string]any{"id": wanted, "name": "Probe Project"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project: got %d, body %s", rec.Code, rec.Body.String())
	}
	var out mwanachamataskmanager.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ID == wanted {
		t.Fatalf("a caller-supplied id was honoured (got %q) — the server must mint its own", out.ID)
	}
}

// Board row W11 on CreateTaskTodo.
func TestCreateTaskTodo_IgnoresCallerSuppliedID(t *testing.T) {
	tm := wk11Manager(t)
	m := wk11Mux(tm)

	taskRec := wk11Post(t, m, "/tasks", map[string]any{"title": "parent"})
	var parent mwanachamataskmanager.Task
	if err := json.Unmarshal(taskRec.Body.Bytes(), &parent); err != nil {
		t.Fatalf("decode parent task: %v", err)
	}

	const wanted = "attacker-chosen-todo-id"
	rec := wk11Post(t, m, "/todos", map[string]any{
		"id": wanted, "title": "probe todo", "instructions": "do it", "parent_task_id": parent.ID,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create todo: got %d, body %s", rec.Code, rec.Body.String())
	}
	var out mwanachamataskmanager.TaskTodo
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ID == wanted {
		t.Fatalf("a caller-supplied id was honoured (got %q) — the server must mint its own", out.ID)
	}
}
