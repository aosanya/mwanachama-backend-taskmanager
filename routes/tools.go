package routes

import (
	"fmt"

	"github.com/aosanya/mwanachama-backend-shared/dispatch"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type Tool = dispatch.Tool

func BuildToolsFor(tm mwanachamataskmanager.TaskManager, m Mount) ([]Tool, error) {
	s, err := operations()
	if err != nil {
		return nil, err
	}
	return dispatch.Tools(s, dispatch.Deps{
		Manager: tm, Errors: sentinels, Authorize: m.Authorize, Caller: m.Caller,
	})
}

func ToolsFor(tm mwanachamataskmanager.TaskManager, m Mount) []Tool {
	out, err := BuildToolsFor(tm, m)
	if err != nil {
		panic(fmt.Sprintf("taskmanager tools: %v", err))
	}
	return out
}

func ToolShape() []Tool { return ToolsFor(mwanachamataskmanager.Unbound(), Mount{}) }
