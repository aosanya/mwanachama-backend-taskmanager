package mwanachamataskmanager_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func newTestManager(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	mgr, _, _ := newTestManagerWithDB(t)
	return mgr
}

func newTestManagerWithDB(t *testing.T) (mwanachamataskmanager.TaskManager, *gorm.DB, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	workSpec := specForInstance(t, "spec/examples/agency.taskmanager.json", "test")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	object, ok := workSpec.Object("work_item")
	if !ok {
		t.Fatal("spec declares no work_item object")
	}
	return mgr, db, workSpec.TableFor(object)
}

func specForInstance(t *testing.T, path, instance string) *spec.Spec {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	doc["instance"] = instance
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("re-encode %s: %v", path, err)
	}
	s, err := mwanachamataskmanager.ParseSpec(out)
	if err != nil {
		t.Fatalf("spec for instance %q: %v", instance, err)
	}
	return s
}
