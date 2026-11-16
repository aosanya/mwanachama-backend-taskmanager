package routes_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func managerOnMemory(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../spec/examples/agency.taskmanager.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["instance"] = "mount"
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	s, err := mwanachamataskmanager.ParseSpec(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Migrate(db, s); err != nil {
		t.Fatal(err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func patterns(list []routes.Route) []string {
	out := make([]string, len(list))
	for i, r := range list {
		out[i] = r.Pattern("")
	}
	return out
}

func serve(list []routes.Route, method, path string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	for _, r := range list {
		mux.HandleFunc(r.Pattern(""), r.Handler)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader("")))
	return rec
}

func TestMount_ShapeNamesTheSameOperationsAsTheBuiltRoutes(t *testing.T) {
	mgr := managerOnMemory(t)
	built := routes.Routes(mgr)
	shape := routes.Shape()

	if len(shape) == 0 || len(shape) != len(built) {
		t.Fatalf("shape has %d routes, built has %d", len(shape), len(built))
	}
	for _, r := range shape {
		if r.Method == "" || r.Path == "" || r.Action == "" {
			t.Fatalf("a declared route is missing a part: %+v", r)
		}
		if r.Handler != nil {
			t.Fatalf("shape carries a handler for %s %s", r.Method, r.Path)
		}
	}
	got := map[string]bool{}
	for _, p := range patterns(built) {
		got[p] = true
	}
	for _, p := range patterns(shape) {
		if !got[p] {
			t.Fatalf("shape names %q but no built route has it", p)
		}
	}
}

func TestMount_NothingIsReadableWithoutACaller(t *testing.T) {
	mgr := managerOnMemory(t)
	split := routes.Split(mgr)

	if len(split.Anonymous) != 0 {
		t.Fatalf("anonymous routes: %v", patterns(split.Anonymous))
	}
	if len(split.Gated) != len(routes.Routes(mgr)) {
		t.Fatalf("gated has %d routes, want all %d", len(split.Gated), len(routes.Routes(mgr)))
	}
	operator := routes.OperatorRoutes(mgr)
	if len(operator) != len(split.Gated) {
		t.Fatalf("operator routes %d, gated %d", len(operator), len(split.Gated))
	}
}

func TestMount_AnAuthorizerThatRefusesStopsEveryRouteBeforeTheManager(t *testing.T) {
	mgr := managerOnMemory(t)
	var asked []string
	refuse := func(_ context.Context, action string) error {
		asked = append(asked, action)
		return errors.New("no")
	}

	rec := serve(routes.RoutesWith(mgr, refuse), http.MethodGet, "/backlogs")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a refused list answered %d %s, want 403", rec.Code, rec.Body.String())
	}
	if len(asked) != 1 {
		t.Fatalf("the authorizer was asked %v, want once for the one action", asked)
	}

	allow := func(context.Context, string) error { return nil }
	rec = serve(routes.RoutesWith(mgr, allow), http.MethodGet, "/backlogs")
	if rec.Code != http.StatusOK {
		t.Fatalf("an allowed list answered %d %s", rec.Code, rec.Body.String())
	}
}

func TestMount_OperatorRoutesAreTheGatedOnesWithTheMountsAuthorizer(t *testing.T) {
	mgr := managerOnMemory(t)
	refuse := func(context.Context, string) error { return errors.New("no") }

	rec := serve(routes.OperatorRoutesWith(mgr, refuse), http.MethodGet, "/backlogs")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("operator routes with a refusing authorizer answered %d", rec.Code)
	}
	if rec := serve(routes.OperatorRoutes(mgr), http.MethodGet, "/backlogs"); rec.Code != http.StatusOK {
		t.Fatalf("operator routes with no authorizer answered %d %s", rec.Code, rec.Body.String())
	}
}

func TestMount_EveryToolNamesAnActionAMatchingRouteHasAndEveryRoutePathIsUnique(t *testing.T) {
	mgr := managerOnMemory(t)
	seen := map[string]bool{}
	actions := map[string]bool{}
	for _, r := range routes.Routes(mgr) {
		if seen[r.Pattern("")] {
			t.Fatalf("two routes share %q", r.Pattern(""))
		}
		seen[r.Pattern("")] = true
		actions[r.Action] = true
	}
	tools := routes.ToolShape()
	if len(tools) == 0 {
		t.Fatal("the work board declares no tools")
	}
	names := map[string]bool{}
	for _, tool := range tools {
		if tool.Name == "" || tool.Description == "" || len(tool.InputSchema) == 0 {
			t.Fatalf("a tool is missing a part: %+v", tool)
		}
		if names[tool.Name] {
			t.Fatalf("two tools are named %q", tool.Name)
		}
		names[tool.Name] = true
		if !actions[tool.Action] {
			t.Fatalf("tool %q names action %q that no route has", tool.Name, tool.Action)
		}
	}
}

func TestMount_ToolsBuiltWithAnAuthorizerRefuseBeforeTheManager(t *testing.T) {
	mgr := managerOnMemory(t)
	var asked []string
	refuse := func(_ context.Context, action string) error {
		asked = append(asked, action)
		return errors.New("no")
	}
	built, err := routes.BuildToolsFor(mgr, routes.Mount{Authorize: refuse})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range built {
		if tool.Name != "taskmanager_backlog_list" {
			continue
		}
		if _, err := tool.Invoke(context.Background(), json.RawMessage(`{}`)); err == nil {
			t.Fatal("the list tool ran for a refused caller")
		}
		if len(asked) != 1 || asked[0] != "taskmanager.backlog.list" {
			t.Fatalf("the authorizer was asked %v", asked)
		}
		allow := func(context.Context, string) error { return nil }
		open, err := routes.BuildToolsFor(mgr, routes.Mount{Authorize: allow})
		if err != nil {
			t.Fatal(err)
		}
		for _, again := range open {
			if again.Name == "taskmanager_backlog_list" {
				if _, err := again.Invoke(context.Background(), json.RawMessage(`{}`)); err != nil {
					t.Fatalf("the list tool refused an allowed caller: %v", err)
				}
				return
			}
		}
	}
	t.Fatal("the work board has no taskmanager_backlog_list tool")
}
