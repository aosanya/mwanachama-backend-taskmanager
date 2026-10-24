package mwanachamataskmanager_test

import (
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestTheWalkPrefersTheStatesFirstStatusAtTheShortestDepth(t *testing.T) {
	waiting := models.DefaultTaskStates()[2]
	path, ok := mwanachamataskmanager.StatusPathToState(models.TaskStatusInProgress, waiting)
	if !ok || len(path) != 1 || path[0] != models.TaskStatusAwaitingDirection {
		t.Fatalf("path %v %v", path, ok)
	}
}

func TestEveryLegalStepOfAWalkIsATransition(t *testing.T) {
	for _, from := range []models.TaskStatus{models.TaskStatusPending, models.TaskStatusInProgress, models.TaskStatusBlocked, models.TaskStatusFailed, models.TaskStatusSplit, models.TaskStatusAwaitingDirection} {
		for _, target := range models.DefaultTaskStates() {
			path, ok := mwanachamataskmanager.StatusPathToState(from, target)
			if !ok {
				continue
			}
			cur := from
			for _, next := range path {
				if !cur.CanTransitionTo(next) {
					t.Fatalf("%s -> %s is not legal on the way to %s", cur, next, target.Name)
				}
				cur = next
			}
		}
	}
}

func TestATerminalStatusReachesNoOtherState(t *testing.T) {
	doing := models.DefaultTaskStates()[1]
	if _, ok := mwanachamataskmanager.StatusPathToState(models.TaskStatusCompleted, doing); ok {
		t.Fatal("completed reached Doing")
	}
}
