// postgres_integration_test.go exercises TaskManager against a real
// Postgres-backed entitygraph.DataManager (mwanachama-backend-shared's
// postgres.Backend), rather than the in-memory fakeDataManager the rest of
// this package's tests use.
//
// Skipped unless POSTGRES_URL is set — mirrors mwanachama-backend-shared's own
// postgres/backend_test.go split ("go test ./..." needs no database; set
// POSTGRES_URL to also run this file, see the Makefile's test-pg target).
// The unit tests elsewhere in this package already exhaustively cover
// TaskManager's business logic against fakeDataManager; this file's job is
// narrower — prove the real Postgres wiring (schema activation, jsonb
// property round-trips, unique-key upserts, recursive-CTE traversal) works
// end-to-end, not to re-run all ~160 unit tests a second time.
package mwanachamataskmanager_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/postgres"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// applyDDL runs a multi-statement SQL script as one command, the way
// database/sql's ExecContext accepts it via the pgx stdlib driver.
func applyDDL(ctx context.Context, db *sql.DB, script string) error {
	_, err := db.ExecContext(ctx, script)
	return err
}

// newPostgresTaskManager opens POSTGRES_URL, creates a scratch set of
// work_-prefixed tables, seeds+activates DefaultWorkSchema for agencyID, and
// returns a ready-to-use TaskManager plus its recordingPublisher. Skips the
// calling test if POSTGRES_URL is unset. Tables are dropped on cleanup.
func newPostgresTaskManager(t *testing.T, agencyID string) (mwanachamataskmanager.TaskManager, *recordingPublisher) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set; skipping Postgres integration test (see Makefile's test-pg target)")
	}

	ctx := context.Background()
	db, err := postgres.Open(ctx, postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// A unique-enough prefix per test keeps concurrent -run invocations
	// from colliding on the same physical tables.
	tables := postgres.DefaultTableNames("workit_")
	if err := applyDDL(ctx, db, postgres.DDL(tables)); err != nil {
		t.Fatalf("applying DDL: %v", err)
	}
	t.Cleanup(func() {
		_ = applyDDL(context.Background(), db, postgres.DropDDL(tables))
	})

	backend := postgres.NewBackend(db, tables)

	s := mwanachamataskmanager.DefaultWorkSchema()
	s.AgencyID = agencyID
	if err := backend.SetSchema(ctx, s); err != nil {
		t.Fatalf("SetSchema: %v", err)
	}
	if err := backend.Publish(ctx, agencyID); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := backend.Activate(ctx, agencyID, 1); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	pub := &recordingPublisher{}
	mgr, err := mwanachamataskmanager.NewTaskManager(backend, pub)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr, pub
}

func TestPostgres_TaskCRUD_RoundTrip(t *testing.T) {
	const agencyID = "pg-agency-task"
	mgr, pub := newPostgresTaskManager(t, agencyID)
	ctx := context.Background()

	created, err := mgr.CreateTask(ctx, agencyID, mwanachamataskmanager.Task{
		Title:    "Postgres round-trip",
		Priority: mwanachamataskmanager.TaskPriorityHigh,
		Tags:     []string{"pg", "smoke"},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.Status != mwanachamataskmanager.TaskStatusPending {
		t.Errorf("Status = %s, want pending", created.Status)
	}

	got, err := mgr.GetTask(ctx, agencyID, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Title != "Postgres round-trip" || got.Priority != mwanachamataskmanager.TaskPriorityHigh {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags = %v, want 2 entries (jsonb array round-trip)", got.Tags)
	}

	got.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, agencyID, got); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if err := mgr.DeleteTask(ctx, agencyID, created.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if _, err := mgr.GetTask(ctx, agencyID, created.ID); err == nil {
		t.Error("expected error reading soft-deleted task")
	}

	if len(pub.events) == 0 {
		t.Error("expected at least one published event across create/update")
	}
}

func TestPostgres_AgentUpsert_UsesSchemaUniqueKey(t *testing.T) {
	const agencyID = "pg-agency-agent"
	mgr, _ := newPostgresTaskManager(t, agencyID)
	ctx := context.Background()

	first, err := mgr.UpsertAgent(ctx, agencyID, mwanachamataskmanager.Agent{
		AgentID: "dev-01", DisplayName: "First",
	})
	if err != nil {
		t.Fatalf("first UpsertAgent: %v", err)
	}
	second, err := mgr.UpsertAgent(ctx, agencyID, mwanachamataskmanager.Agent{
		AgentID: "dev-01", DisplayName: "Second",
	})
	if err != nil {
		t.Fatalf("second UpsertAgent: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("UpsertEntity's ON CONFLICT unique-key merge did not find the same row: %s vs %s", first.ID, second.ID)
	}
	if second.DisplayName != "Second" {
		t.Errorf("merge did not patch DisplayName: %+v", second)
	}
}

func TestPostgres_AssignTask_And_Relationship_Traversal(t *testing.T) {
	const agencyID = "pg-agency-assign"
	mgr, _ := newPostgresTaskManager(t, agencyID)
	ctx := context.Background()

	task, err := mgr.CreateTask(ctx, agencyID, mwanachamataskmanager.Task{Title: "assign-me"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	agent, err := mgr.UpsertAgent(ctx, agencyID, mwanachamataskmanager.Agent{AgentID: "worker-1"})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	if err := mgr.AssignTask(ctx, agencyID, task.ID, agent.ID, ""); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	edges, err := mgr.TraverseRelationships(ctx, agencyID, task.ID, mwanachamataskmanager.RelLabelAssignedTo, mwanachamataskmanager.DirectionOutbound)
	if err != nil {
		t.Fatalf("TraverseRelationships (recursive CTE): %v", err)
	}
	if len(edges) != 1 || edges[0].ToID != agent.ID {
		t.Errorf("assigned_to edges = %+v, want exactly one pointing at %s", edges, agent.ID)
	}
}

func TestPostgres_WorkflowRun_CreateAndRollback(t *testing.T) {
	const agencyID = "pg-agency-run"
	mgr, pub := newPostgresTaskManager(t, agencyID)
	ctx := context.Background()

	run, err := mgr.CreateWorkflowRun(ctx, agencyID, "pg-run", "next.requested", "tester")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	task, err := mgr.CreateTask(ctx, agencyID, mwanachamataskmanager.Task{
		Title: "run-task", WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	closure, err := mgr.GetWorkflowRunClosure(ctx, agencyID, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRunClosure: %v", err)
	}
	if len(closure.Tasks) != 1 || closure.Tasks[0].ID != task.ID {
		t.Errorf("closure.Tasks = %+v, want [%s]", closure.Tasks, task.ID)
	}

	if _, err := mgr.UpdateWorkflowRunStatus(ctx, agencyID, run.ID, mwanachamataskmanager.WorkflowRunStatusInProgress, ""); err != nil {
		t.Fatalf("→ in_progress: %v", err)
	}
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, agencyID, run.ID, mwanachamataskmanager.WorkflowRunStatusFailed, "pg smoke test"); err != nil {
		t.Fatalf("→ failed: %v", err)
	}

	rolledBack, err := mgr.RollbackWorkflowRun(ctx, agencyID, run.ID, "cleanup")
	if err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}
	if rolledBack.Status != mwanachamataskmanager.WorkflowRunStatusRolledBack {
		t.Errorf("status = %s, want rolled_back", rolledBack.Status)
	}

	after, err := mgr.GetTask(ctx, agencyID, task.ID)
	if err != nil {
		t.Fatalf("GetTask after rollback: %v", err)
	}
	if after.Status != mwanachamataskmanager.TaskStatusPending || after.WorkflowRunID != "" {
		t.Errorf("post-rollback task = %+v, want status=pending workflow_run_id=\"\"", after)
	}
	if !contains(pub.topicList(), mwanachamataskmanager.TopicRunRolledBack) {
		t.Errorf("expected run.rolled_back event; got %v", pub.topicList())
	}
}

func TestPostgres_ImportProject_EndToEnd(t *testing.T) {
	const agencyID = "pg-agency-import"
	mgr, _ := newPostgresTaskManager(t, agencyID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	doc := `{
		"project": "Postgres Import Smoke",
		"task_prefix": "PGI-",
		"tasks": [
			{"name": "PGI-001", "title": "First", "tags": ["a"]},
			{"name": "PGI-002", "title": "Second", "depends_on": ["PGI-001"], "tags": ["a", "b"]}
		]
	}`

	result, err := mgr.ImportProject(ctx, agencyID, doc)
	if err != nil {
		t.Fatalf("ImportProject: %v", err)
	}
	if result.TasksCreated != 2 {
		t.Errorf("TasksCreated = %d, want 2", result.TasksCreated)
	}
	if result.DepsCreated != 1 {
		t.Errorf("DepsCreated = %d, want 1", result.DepsCreated)
	}

	tasks, err := mgr.ListTasksInProject(ctx, agencyID, result.Project.ID)
	if err != nil {
		t.Fatalf("ListTasksInProject: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("ListTasksInProject = %d tasks, want 2", len(tasks))
	}
}
