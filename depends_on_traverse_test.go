package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func linkDependsOn(t *testing.T, mgr mwanachamataskmanager.TaskManager, waiting, waitedOn mwanachamataskmanager.Task) {
	t.Helper()
	_, err := mgr.CreateRelationship(context.Background(), mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelDependsOn, FromID: waiting.ID, ToID: waitedOn.ID,
	})
	if err != nil {
		t.Fatalf("CreateRelationship depends_on: %v", err)
	}
}

func traverseOverHTTP(t *testing.T, mgr mwanachamataskmanager.TaskManager, vertexID, direction string) (int, []byte) {
	t.Helper()
	for _, route := range routes.Routes(mgr) {
		if route.Method != http.MethodGet || route.Path != "/relationships/traverse" {
			continue
		}
		target := "/relationships/traverse?label=depends_on&vertex_id=" + vertexID + "&direction=" + direction
		rec := httptest.NewRecorder()
		route.Handler(rec, httptest.NewRequest(http.MethodGet, target, nil))
		return rec.Code, rec.Body.Bytes()
	}
	t.Fatal("no traverse route")
	return 0, nil
}

func TestTraverse_DependsOnListsWhatATaskWaitsOn(t *testing.T) {
	mgr := newTestManager(t)
	waiting := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "waiting"})
	first := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "first"})
	second := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "second"})
	linkDependsOn(t, mgr, waiting, first)
	linkDependsOn(t, mgr, waiting, second)

	code, body := traverseOverHTTP(t, mgr, waiting.ID, "outbound")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %s", code, body)
	}
	var edges []mwanachamataskmanager.Relationship
	if err := json.Unmarshal(body, &edges); err != nil {
		t.Fatalf("body is not a list of edges: %v: %s", err, body)
	}
	waitedOn := map[string]bool{}
	for _, edge := range edges {
		if edge.Label != mwanachamataskmanager.RelLabelDependsOn || edge.FromID != waiting.ID {
			t.Fatalf("edge = %+v, want a depends_on edge from the waiting task", edge)
		}
		waitedOn[edge.ToID] = true
	}
	if len(edges) != 2 || !waitedOn[first.ID] || !waitedOn[second.ID] {
		t.Fatalf("edges = %+v, want one to each of the two tasks it waits on", edges)
	}
}

func TestTraverse_DependsOnInboundListsWhatWaitsOnATask(t *testing.T) {
	mgr := newTestManager(t)
	waiting := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "waiting"})
	first := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "first"})
	linkDependsOn(t, mgr, waiting, first)

	edges, err := mgr.TraverseRelationships(context.Background(), first.ID, mwanachamataskmanager.RelLabelDependsOn, mwanachamataskmanager.DirectionInbound)
	if err != nil {
		t.Fatalf("TraverseRelationships: %v", err)
	}
	if len(edges) != 1 || edges[0].FromID != waiting.ID || edges[0].ToID != first.ID {
		t.Fatalf("edges = %+v, want the one edge from the waiting task", edges)
	}
}

func TestTraverse_DependsOnOfATaskWithNoneIsAnEmptyList(t *testing.T) {
	mgr := newTestManager(t)
	alone := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "alone"})

	code, body := traverseOverHTTP(t, mgr, alone.ID, "outbound")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %s", code, body)
	}
	if got := string(body); got != "[]" && got != "[]\n" {
		t.Fatalf("body = %q, want an empty list", got)
	}
}
