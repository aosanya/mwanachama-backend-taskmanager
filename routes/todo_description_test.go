package routes_test

import (
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func TestTheTodoCreateToolsSayADescriptionIsExpectedAndThatLintFlagsABlankOne(t *testing.T) {
	actions := map[string]bool{
		"taskmanager.task_todo.create":       false,
		"taskmanager.task.create_with_todos": false,
	}
	for _, tool := range routes.ToolShape() {
		if _, ok := actions[tool.Action]; !ok {
			continue
		}
		actions[tool.Action] = true
		for _, phrase := range []string{"description", "never the title", "task_lint", "todo_without_description"} {
			if !strings.Contains(tool.Description, phrase) {
				t.Errorf("%s: the description does not mention %q: %s", tool.Action, phrase, tool.Description)
			}
		}
	}
	for action, found := range actions {
		if !found {
			t.Errorf("no tool for %s", action)
		}
	}
}
