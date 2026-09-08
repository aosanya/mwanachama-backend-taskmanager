package routes

import (
	"errors"
	"net/http"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// writeTaskErr maps mwanachama-backend-taskmanager's sentinel errors onto
// status codes in one place, so a new route cannot invent a different code
// for the same refusal.
func writeTaskErr(w http.ResponseWriter, err error) {
	var blocked *mwanachamataskmanager.BlockedError
	if errors.As(err, &blocked) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":            err.Error(),
			"blocker_task_ids": blocked.BlockerTaskIDs,
		})
		return
	}
	switch {
	case errors.Is(err, mwanachamataskmanager.ErrTaskNotFound),
		errors.Is(err, mwanachamataskmanager.ErrAgentNotFound),
		errors.Is(err, mwanachamataskmanager.ErrProjectNotFound),
		errors.Is(err, mwanachamataskmanager.ErrTagNotFound),
		errors.Is(err, mwanachamataskmanager.ErrTaskTodoNotFound),
		errors.Is(err, mwanachamataskmanager.ErrWorkflowRunNotFound),
		errors.Is(err, mwanachamataskmanager.ErrRelationshipNotFound),
		errors.Is(err, mwanachamataskmanager.ErrImportJobNotFound),
		errors.Is(err, mwanachamataskmanager.ErrDeliverableNotFound),
		errors.Is(err, mwanachamataskmanager.ErrAcceptanceCriteriaNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, mwanachamataskmanager.ErrTaskAlreadyExists),
		errors.Is(err, mwanachamataskmanager.ErrProjectAlreadyExists),
		errors.Is(err, mwanachamataskmanager.ErrWorkflowRunNameExists),
		errors.Is(err, mwanachamataskmanager.ErrRollbackConflict),
		errors.Is(err, mwanachamataskmanager.ErrFailureBudgetAlreadySet),
		errors.Is(err, mwanachamataskmanager.ErrImportJobNotCancellable),
		errors.Is(err, mwanachamataskmanager.ErrCannotCancelTerminalRun),
		errors.Is(err, mwanachamataskmanager.ErrForeignRunDependency),
		errors.Is(err, mwanachamataskmanager.ErrWorkflowRunMismatch):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, mwanachamataskmanager.ErrInvalidStatusTransition),
		errors.Is(err, mwanachamataskmanager.ErrInvalidTask),
		errors.Is(err, mwanachamataskmanager.ErrInvalidRunStatusTransition),
		errors.Is(err, mwanachamataskmanager.ErrInvalidRelationship),
		errors.Is(err, mwanachamataskmanager.ErrInvalidImport),
		errors.Is(err, mwanachamataskmanager.ErrNotRootWorkflowRun):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "task manager store unavailable")
	}
}

// relationshipJSON is [mwanachamataskmanager.Relationship] with JSON tags —
// that struct carries none of its own, so this keeps responses snake_case,
// consistent with every other route in this package.
type relationshipJSON struct {
	ID         string         `json:"id"`
	Label      string         `json:"label"`
	FromID     string         `json:"from_id"`
	ToID       string         `json:"to_id"`
	Properties map[string]any `json:"properties,omitempty"`
	CreatedAt  string         `json:"created_at"`
}

func toRelationshipJSON(r mwanachamataskmanager.Relationship) relationshipJSON {
	return relationshipJSON{
		ID: r.ID, Label: r.Label, FromID: r.FromID,
		ToID: r.ToID, Properties: r.Properties, CreatedAt: r.CreatedAt,
	}
}
