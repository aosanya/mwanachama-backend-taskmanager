package routes_test

import (
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func TestEveryDeclaredOperationIsATaskmanagerTool(t *testing.T) {
	tools, err := routes.BuildToolsFor(nil, routes.Mount{})
	if err == nil {
		t.Fatalf("a nil manager must be refused, got %d tools", len(tools))
	}
}

func TestToolsAreNamedForTheirModule(t *testing.T) {
	tm := declaredManager(t)
	tools, err := routes.BuildToolsFor(tm, routes.Mount{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != len(routes.Shape()) {
		t.Fatalf("got %d tools for %d operations", len(tools), len(routes.Shape()))
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		if !strings.HasPrefix(tool.Name, "taskmanager_") || seen[tool.Name] {
			t.Fatalf("tool %q is not a unique taskmanager_ name", tool.Name)
		}
		seen[tool.Name] = true
	}
	if !seen["taskmanager_task_create"] || !seen["taskmanager_task_list"] {
		t.Fatalf("missing task tools: %v", seen)
	}
}

func TestToolShapeNeedsNoBoard(t *testing.T) {
	shape := routes.ToolShape()
	bound, err := routes.BuildToolsFor(declaredManager(t), routes.Mount{})
	if err != nil {
		t.Fatal(err)
	}
	for i := range shape {
		if shape[i].Name != bound[i].Name {
			t.Fatalf("shape[%d] is %q, bound is %q", i, shape[i].Name, bound[i].Name)
		}
	}
}
