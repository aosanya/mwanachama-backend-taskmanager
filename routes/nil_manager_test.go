package routes_test

import (
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func TestANilManagerIsRefusedAndNotHiddenBehindTheRuleWrapper(t *testing.T) {
	if _, err := routes.Build(nil); err == nil {
		t.Fatal("Build(nil) built routes over a nil manager")
	}
	if _, err := routes.BuildToolsFor(nil, routes.Mount{}); err == nil {
		t.Fatal("BuildToolsFor(nil) built tools over a nil manager")
	}
}
