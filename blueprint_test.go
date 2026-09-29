package mwanachamataskmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	workExamplePath   = "spec/examples/work.taskmanager.json"
	agencyExamplePath = "spec/examples/agency.taskmanager.json"
)

func openSpecTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return db
}

func specExamplePaths(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("spec", "examples", "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("glob examples: %v (%d found)", err, len(paths))
	}
	sort.Strings(paths)
	return paths
}

func tableNames(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var names []string
	if err := db.Raw(`select name from sqlite_master where type='table' and name not like 'sqlite_%'`).
		Scan(&names).Error; err != nil {
		t.Fatalf("list tables: %v", err)
	}
	sort.Strings(names)
	return names
}

func TestBlueprintDeclaresEveryRoleTheModuleReachesFor(t *testing.T) {
	b, err := Blueprint()
	if err != nil {
		t.Fatalf("blueprint: %v", err)
	}
	declared := map[string]bool{}
	for _, role := range b.Roles() {
		declared[role] = true
	}
	for _, role := range roles() {
		if !declared[role] {
			t.Errorf("the module reaches for the role %q and the blueprint declares no object for it", role)
		}
		delete(declared, role)
	}
	for role := range declared {
		t.Errorf("the blueprint declares the role %q and no rule in the module reaches for it", role)
	}
}

// A stored enum value outlives a rename, so the constants Go compares
// against and the values the blueprint declares have to agree in both
// directions.
func TestVocabularyMatchesTheBlueprint(t *testing.T) {
	inGo := map[string][]string{
		roleTask + ".status": {
			string(models.TaskStatusPending),
			string(models.TaskStatusInProgress),
			string(models.TaskStatusCompleted),
			string(models.TaskStatusFailed),
			string(models.TaskStatusCancelled),
			string(models.TaskStatusBlocked),
			string(models.TaskStatusAwaitingDirection),
			string(models.TaskStatusSplit),
		},
		roleTask + ".priority": {
			string(models.TaskPriorityLow),
			string(models.TaskPriorityMedium),
			string(models.TaskPriorityHigh),
			string(models.TaskPriorityCritical),
		},
		roleTaskTodo + ".status": {
			string(models.TodoStatusPending),
			string(models.TodoStatusBlocked),
			string(models.TodoStatusDispatched),
			string(models.TodoStatusCompleted),
			string(models.TodoStatusFailed),
			string(models.TodoStatusSkipped),
		},
		roleWorkflowRun + ".status": {
			string(models.WorkflowRunStatusPending),
			string(models.WorkflowRunStatusInProgress),
			string(models.WorkflowRunStatusCompleted),
			string(models.WorkflowRunStatusFailed),
			string(models.WorkflowRunStatusRolledBack),
			string(models.WorkflowRunStatusRollingBack),
			string(models.WorkflowRunStatusRollbackFailed),
			string(models.WorkflowRunStatusCancelling),
			string(models.WorkflowRunStatusCancelled),
			string(models.WorkflowRunStatusPaused),
		},
		roleImportJob + ".status": {
			models.ImportJobStatusPending,
			models.ImportJobStatusRunning,
			models.ImportJobStatusCompleted,
			models.ImportJobStatusFailed,
			models.ImportJobStatusCancelled,
		},
		roleAcceptanceCriteria + ".result": {
			models.AcceptanceResultPassed,
			models.AcceptanceResultFailed,
			models.AcceptanceResultSkipped,
			models.AcceptanceResultBlocked,
		},
	}

	b, err := Blueprint()
	if err != nil {
		t.Fatalf("blueprint: %v", err)
	}

	for _, o := range b.Objects {
		for _, f := range o.Fields {
			if f.Type != spec.TypeEnum {
				continue
			}
			key := o.Role + "." + f.Name
			want, named := inGo[key]
			if !named {
				t.Errorf("the blueprint declares the enum %s with the values %s, and Go names no constants for them",
					key, strings.Join(f.Values, ", "))
				continue
			}
			delete(inGo, key)
			if !sameValues(want, f.Values) {
				t.Errorf("%s: Go names %s, the blueprint declares %s — a stored value outlives a rename, so these have to agree",
					key, strings.Join(sorted(want), ", "), strings.Join(sorted(f.Values), ", "))
			}
		}
	}

	for key := range inGo {
		t.Errorf("Go names constants for %s, which the blueprint declares no enum for", key)
	}
}

// Every shipped example fills every role, migrates, and lands columns the Go
// types can actually hold. Run over every spec under spec/examples rather
// than the one a test happened to load — the drift it closes is invisible
// under the domain nobody loads.
func TestEveryExampleFitsTheTypes(t *testing.T) {
	for _, path := range specExamplePaths(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			s, err := LoadSpec(path)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if err := s.RequireRoles(roles()...); err != nil {
				t.Fatalf("roles: %v", err)
			}
			db := openSpecTestDB(t)
			if err := spec.Migrate(db, s); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			if _, err := newStore(db, s, carriers()); err != nil {
				t.Fatalf("store: %v", err)
			}
		})
	}
}

// A domain names objects; it does not re-declare them. Setting a type, a
// description or a value set on a field the module declares is the module's
// declaration restated, and it drifts.
func TestExamplesDeclareNoFieldsOfTheirOwn(t *testing.T) {
	for _, path := range specExamplePaths(t) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var doc struct {
			Objects []struct {
				Name   string `json:"name"`
				Role   string `json:"role"`
				Fields []struct {
					Name        string   `json:"name"`
					Type        string   `json:"type"`
					Description string   `json:"description"`
					Values      []string `json:"values"`
				} `json:"fields"`
			} `json:"objects"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, o := range doc.Objects {
			if o.Role == "" {
				continue
			}
			for _, f := range o.Fields {
				if f.Type != "" || f.Description != "" || len(f.Values) > 0 {
					t.Errorf("%s: %s restates the declaration of %q — a domain names the object and sets defaults, the module declares the field",
						filepath.Base(path), o.Name, f.Name)
				}
			}
		}
	}
}

// The same module under two domains, in one database. A work board and an
// agency's own board share a database and neither learns the other's word.
func TestShippedExamplesCoexist(t *testing.T) {
	db := openSpecTestDB(t)

	work, err := LoadSpec(workExamplePath)
	if err != nil {
		t.Fatalf("load work: %v", err)
	}
	agency, err := LoadSpec(agencyExamplePath)
	if err != nil {
		t.Fatalf("load agency: %v", err)
	}

	for _, s := range []*spec.Spec{work, agency} {
		if err := spec.Migrate(db, s); err != nil {
			t.Fatalf("migrate %s: %v", s.Domain, err)
		}
	}

	want := []string{
		"kazi_taskmanager_acceptance_criteria", "kazi_taskmanager_agents",
		"kazi_taskmanager_code_sequences", "kazi_taskmanager_deliverables",
		"kazi_taskmanager_import_project_jobs", "kazi_taskmanager_projects",
		"kazi_taskmanager_tags", "kazi_taskmanager_task_blocks",
		"kazi_taskmanager_task_dependencies", "kazi_taskmanager_task_project_memberships",
		"kazi_taskmanager_task_tags", "kazi_taskmanager_task_todos",
		"kazi_taskmanager_tasks", "kazi_taskmanager_workflow_runs",
		"wakala_taskmanager_checks", "wakala_taskmanager_code_sequences",
		"wakala_taskmanager_gates", "wakala_taskmanager_import_jobs",
		"wakala_taskmanager_labellings", "wakala_taskmanager_labels",
		"wakala_taskmanager_objective_memberships", "wakala_taskmanager_objectives",
		"wakala_taskmanager_operators", "wakala_taskmanager_outputs",
		"wakala_taskmanager_precedences", "wakala_taskmanager_steps",
		"wakala_taskmanager_work_items", "wakala_taskmanager_workflow_runs",
	}
	if got := tableNames(t, db); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tables =\n  %v\nwant\n  %v", got, want)
	}

	for _, tc := range []struct {
		s         *spec.Spec
		wantName  string
		wantTable string
	}{
		{work, "task", "kazi_taskmanager_tasks"},
		{agency, "work_item", "wakala_taskmanager_work_items"},
	} {
		o, ok := tc.s.ByRole(roleTask)
		if !ok {
			t.Fatalf("%s: no object fills the task role", tc.s.Domain)
		}
		if o.Name != tc.wantName || tc.s.TableFor(o) != tc.wantTable {
			t.Errorf("%s: task is %q at %q, want %q at %q",
				tc.s.Domain, o.Name, tc.s.TableFor(o), tc.wantName, tc.wantTable)
		}
	}
}

// The module segment is what stops an agency's own work_items and a
// taskmanager instance of the same name becoming one silently adopted table.
func TestTheModuleSegmentKeepsTwoModulesApart(t *testing.T) {
	agency, err := LoadSpec(agencyExamplePath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	o, ok := agency.ByRole(roleTask)
	if !ok {
		t.Fatal("no object fills the task role")
	}
	if got := agency.TableFor(o); got != "wakala_taskmanager_work_items" {
		t.Errorf("table = %q, want wakala_taskmanager_work_items — without the module segment this collides with the agency module's own work_items", got)
	}
}

// A process runs the migration at every start, so running it twice has to be
// a no-op.
func TestShippedExampleMigratesIdempotently(t *testing.T) {
	db := openSpecTestDB(t)
	s, err := LoadSpec(workExamplePath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := spec.Migrate(db, s); err != nil {
			t.Fatalf("migrate %d: %v", i, err)
		}
	}
	if n := len(tableNames(t, db)); n != 14 {
		t.Errorf("tables = %d, want 14", n)
	}
}

// A required field must not carry a default: the default is exactly what
// would stop an omitted value being noticed.
func TestShippedExamplesHaveNoDefaultedRequiredColumn(t *testing.T) {
	for _, path := range specExamplePaths(t) {
		s, err := LoadSpec(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		for _, stmt := range s.DDL("postgres") {
			for _, line := range strings.Split(stmt, "\n") {
				line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
				if strings.Contains(line, "not null") && strings.Contains(line, "default") {
					t.Errorf("%s: column is both required and defaulted: %q", path, line)
				}
			}
		}
	}
}

// Every object and field says something, rather than restating its own name.
func TestShippedExamplesAreDescribed(t *testing.T) {
	sentence := func(s string) bool { return len(strings.Fields(s)) >= 3 }
	restates := func(desc, name string) bool {
		d := strings.ToLower(strings.Trim(desc, " ."))
		return d == strings.ReplaceAll(name, "_", " ") || d == name
	}

	for _, path := range specExamplePaths(t) {
		s, err := LoadSpec(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		for _, o := range s.Objects {
			if !sentence(o.Description) || restates(o.Description, o.Name) {
				t.Errorf("%s: object %q is not described: %q", path, o.Name, o.Description)
			}
			for _, f := range o.Fields {
				if !sentence(f.Description) || restates(f.Description, f.Name) {
					t.Errorf("%s: %s.%s is not described: %q", path, o.Name, f.Name, f.Description)
				}
			}
		}
	}
}

func sameValues(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := sorted(a), sorted(b)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func sorted(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

func TestTwoMountsOfTheSameModuleCoexist(t *testing.T) {
	db := openSpecTestDB(t)

	second, err := SpecForMount("wakala", "second")
	if err != nil {
		t.Fatalf("SpecForMount second: %v", err)
	}
	third, err := SpecForMount("wakala", "third")
	if err != nil {
		t.Fatalf("SpecForMount third: %v", err)
	}

	for _, s := range []*spec.Spec{second, third} {
		if err := Provision(db, s); err != nil {
			t.Fatalf("provision mount %q: %v", s.Mount, err)
		}
	}

	o, ok := second.ByRole("task")
	if !ok {
		t.Fatal("the shipped spec fills no task")
	}
	po, _ := third.ByRole("task")
	if got := second.TableFor(o); got != "wakala_taskmanager_second_work_items" {
		t.Errorf("second-mount task table = %q", got)
	}
	if got := third.TableFor(po); got != "wakala_taskmanager_third_work_items" {
		t.Errorf("third-mount task table = %q", got)
	}
	if second.TableFor(o) == third.TableFor(po) {
		t.Fatal("two mounts landed in one table")
	}
}

func TestSpecForDefaultsToTheMainMount(t *testing.T) {
	s, err := SpecFor("wakala")
	if err != nil {
		t.Fatalf("SpecFor: %v", err)
	}
	if s.MountName() != spec.DefaultMount {
		t.Errorf("MountName = %q, want %q", s.MountName(), spec.DefaultMount)
	}
}
