package routes_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/dispatch"
	"github.com/aosanya/mwanachama-backend-shared/spec"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func declaredManager(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	workSpec, _ := specForInstance(t, "../spec/examples/work.taskmanager.json", "declared")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tm, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatalf("manager: %v", err)
	}
	return tm
}

// Every declared operation has to name a real method and agree with its
// signature — the argument count, their sources and the values rendered.
// Dispatch refuses the whole spec otherwise, which is the point: a drifted
// declaration stops the process starting rather than failing one request.
func TestEveryDeclaredOperationMatchesTheManager(t *testing.T) {
	built, err := routes.Build(declaredManager(t))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(built) == 0 {
		t.Fatal("the spec built no routes")
	}

	s, err := dispatch.Parse(mwanachamataskmanager.Operations())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(built) != len(s.Operations) {
		t.Errorf("built %d routes from %d declared operations", len(built), len(s.Operations))
	}
}

// Every sentinel the spec gives a status to has to be one this package can
// hand the dispatcher, or the refusal it names quietly falls through to 500.
func TestEveryDeclaredErrorIsNamed(t *testing.T) {
	s, err := dispatch.Parse(mwanachamataskmanager.Operations())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := routes.Build(declaredManager(t)); err != nil {
		t.Fatalf("build: %v", err)
	}
	for name := range s.Errors {
		if !strings.HasPrefix(name, "Err") {
			t.Errorf("error %q is not a sentinel name", name)
		}
	}
}

// Nothing on a work board is readable without a caller, so the split leaves
// no route ungated.
func TestNoRouteIsAnonymous(t *testing.T) {
	split := routes.Split(declaredManager(t))
	if len(split.Anonymous) != 0 {
		var paths []string
		for _, r := range split.Anonymous {
			paths = append(paths, r.Method+" "+r.Path)
		}
		sort.Strings(paths)
		t.Errorf("these routes are reachable with no caller: %s", strings.Join(paths, ", "))
	}
	if len(split.Gated) == 0 {
		t.Error("the split gated nothing")
	}
}
