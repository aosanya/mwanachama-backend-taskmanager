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

// Pins board row W11: CreateTask never clears a caller-supplied Task.ID
// before building the row (task_impl_task.go's CreateTask), and
// TaskRow.BeforeCreate only mints a UUID when the ID is already empty
// (gormstore/task.go), so a POST /tasks body naming its own "id" gets that
// exact id back instead of a server-minted one.
//
// Once W11 is fixed (CreateTask clearing task.ID before use, the same fix
// mwanachama-backend-agency's AG21 applied to its own Create<Type> methods),
// this assertion flips: got.ID must NOT equal the caller-supplied value.
func TestCreateTask_PinsCallerSuppliedIDIsHonored(t *testing.T) {
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
	if out.ID != wanted {
		t.Fatalf("W11 appears fixed: CreateTask no longer honors a caller-supplied id (got %q) — update this pin to assert the new server-minted-id behavior instead", out.ID)
	}
}

// Pins board row W11's second half: re-POSTing the same caller-chosen id
// hits TaskRow's primary-key unique constraint, which CreateTask does not
// translate to the existing ErrTaskAlreadyExists sentinel (writeTaskErr
// already has a 409 arm for it) — so the caller sees an opaque 500 instead
// of a clean 409 Conflict.
//
// Once fixed, this assertion flips: the second POST should return 409, not
// 500 (either because ID spoofing is closed and both calls mint distinct
// ids, or because a genuine collision is mapped to ErrTaskAlreadyExists).
func TestCreateTask_PinsDuplicateCallerSuppliedIDReturns500NotConflict(t *testing.T) {
	tm := wk11Manager(t)
	m := wk11Mux(tm)

	const id = "attacker-chosen-task-id-2"
	first := wk11Post(t, m, "/tasks", map[string]any{"id": id, "title": "first"})
	if first.Code != http.StatusCreated {
		t.Fatalf("first create: got %d, body %s", first.Code, first.Body.String())
	}
	second := wk11Post(t, m, "/tasks", map[string]any{"id": id, "title": "second"})
	if second.Code != http.StatusInternalServerError {
		t.Fatalf("W11 appears fixed: duplicate id now returns %d (body %s), not the unmapped 500 this pin expects — update it to assert 409 instead", second.Code, second.Body.String())
	}
}

// Pins board row W11 on CreateProject — same defect, same file's Create
// path (project.go's CreateProject never clears p.ID either).
func TestCreateProject_PinsCallerSuppliedIDIsHonored(t *testing.T) {
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
	if out.ID != wanted {
		t.Fatalf("W11 appears fixed for CreateProject (got %q) — update this pin", out.ID)
	}
}

// Pins board row W11 on CreateTaskTodo — third and last Create path sharing
// the same "never clears .ID before building the row" gap (todo.go's
// CreateTaskTodo).
func TestCreateTaskTodo_PinsCallerSuppliedIDIsHonored(t *testing.T) {
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
	if out.ID != wanted {
		t.Fatalf("W11 appears fixed for CreateTaskTodo (got %q) — update this pin", out.ID)
	}
}
