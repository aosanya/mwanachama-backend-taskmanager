package routes_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/spec"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type testTables struct {
	Tasks                  string
	TaskTodos              string
	Agents                 string
	Projects               string
	Tags                   string
	Deliverables           string
	AcceptanceCriteria     string
	WorkflowRuns           string
	ImportProjectJobs      string
	TaskBlocks             string
	TaskDependencies       string
	TaskProjectMemberships string
	TaskTags               string
	CodeSequences          string
}

func specForInstance(t *testing.T, path, instance string) (*spec.Spec, testTables) {
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

	table := func(name string) string {
		o, ok := s.Object(name)
		if !ok {
			t.Fatalf("spec declares no object %q", name)
		}
		return s.TableFor(o)
	}
	return s, testTables{
		Tasks:                  table("task"),
		TaskTodos:              table("task_todo"),
		Agents:                 table("agent"),
		Projects:               table("project"),
		Tags:                   table("tag"),
		Deliverables:           table("deliverable"),
		AcceptanceCriteria:     table("acceptance_criteria"),
		WorkflowRuns:           table("workflow_run"),
		ImportProjectJobs:      table("import_project_job"),
		TaskBlocks:             table("task_block"),
		TaskDependencies:       table("task_dependency"),
		TaskProjectMemberships: table("task_project_membership"),
		TaskTags:               table("task_tag"),
		CodeSequences:          table("code_sequence"),
	}
}
