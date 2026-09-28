package routes

import (
	"fmt"
	"sort"
	"sync"

	"github.com/aosanya/mwanachama-backend-shared/dispatch"
	"github.com/aosanya/mwanachama-backend-shared/httpwire"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type Route = httpwire.Route

var operations = sync.OnceValues(func() (*dispatch.Spec, error) {
	return dispatch.Parse(mwanachamataskmanager.Operations())
})

var sentinels = map[string]error{
	"ErrTaskNotFound":               mwanachamataskmanager.ErrTaskNotFound,
	"ErrAgentNotFound":              mwanachamataskmanager.ErrAgentNotFound,
	"ErrProjectNotFound":            mwanachamataskmanager.ErrProjectNotFound,
	"ErrTagNotFound":                mwanachamataskmanager.ErrTagNotFound,
	"ErrTaskTodoNotFound":           mwanachamataskmanager.ErrTaskTodoNotFound,
	"ErrWorkflowRunNotFound":        mwanachamataskmanager.ErrWorkflowRunNotFound,
	"ErrRelationshipNotFound":       mwanachamataskmanager.ErrRelationshipNotFound,
	"ErrImportJobNotFound":          mwanachamataskmanager.ErrImportJobNotFound,
	"ErrDeliverableNotFound":        mwanachamataskmanager.ErrDeliverableNotFound,
	"ErrAcceptanceCriteriaNotFound": mwanachamataskmanager.ErrAcceptanceCriteriaNotFound,
	"ErrTaskAlreadyExists":          mwanachamataskmanager.ErrTaskAlreadyExists,
	"ErrProjectAlreadyExists":       mwanachamataskmanager.ErrProjectAlreadyExists,
	"ErrWorkflowRunNameExists":      mwanachamataskmanager.ErrWorkflowRunNameExists,
	"ErrRollbackConflict":           mwanachamataskmanager.ErrRollbackConflict,
	"ErrRollbackNotInProgress":      mwanachamataskmanager.ErrRollbackNotInProgress,
	"ErrFailureBudgetAlreadySet":    mwanachamataskmanager.ErrFailureBudgetAlreadySet,
	"ErrImportJobNotCancellable":    mwanachamataskmanager.ErrImportJobNotCancellable,
	"ErrCannotCancelTerminalRun":    mwanachamataskmanager.ErrCannotCancelTerminalRun,
	"ErrForeignRunDependency":       mwanachamataskmanager.ErrForeignRunDependency,
	"ErrWorkflowRunMismatch":        mwanachamataskmanager.ErrWorkflowRunMismatch,
	"ErrBlocked":                    mwanachamataskmanager.ErrBlocked,
	"ErrInvalidStatusTransition":    mwanachamataskmanager.ErrInvalidStatusTransition,
	"ErrInvalidTask":                mwanachamataskmanager.ErrInvalidTask,
	"ErrInvalidRunStatusTransition": mwanachamataskmanager.ErrInvalidRunStatusTransition,
	"ErrInvalidRelationship":        mwanachamataskmanager.ErrInvalidRelationship,
	"ErrInvalidImport":              mwanachamataskmanager.ErrInvalidImport,
	"ErrNotRootWorkflowRun":         mwanachamataskmanager.ErrNotRootWorkflowRun,
}

// AnonymousActions is empty on purpose: nothing on a work board is readable
// without a caller. It stays as the allowlist Split is built on, so an
// operation added to the spec and not named here arrives gated.
var AnonymousActions = []string{}

type Mount struct {
	Authorize dispatch.Authorizer
	Caller    dispatch.Caller
}

func Build(tm mwanachamataskmanager.TaskManager) ([]Route, error) { return BuildWith(tm, nil) }

func BuildWith(tm mwanachamataskmanager.TaskManager, authorize dispatch.Authorizer) ([]Route, error) {
	return BuildFor(tm, Mount{Authorize: authorize})
}

func BuildFor(tm mwanachamataskmanager.TaskManager, m Mount) ([]Route, error) {
	s, err := operations()
	if err != nil {
		return nil, err
	}
	return dispatch.Dispatch(s, dispatch.Deps{
		Manager: tm, Errors: sentinels, Authorize: m.Authorize, Caller: m.Caller,
	})
}

func Routes(tm mwanachamataskmanager.TaskManager) []Route { return RoutesWith(tm, nil) }

func RoutesWith(tm mwanachamataskmanager.TaskManager, authorize dispatch.Authorizer) []Route {
	return RoutesFor(tm, Mount{Authorize: authorize})
}

func RoutesFor(tm mwanachamataskmanager.TaskManager, m Mount) []Route {
	out, err := BuildFor(tm, m)
	if err != nil {
		panic(fmt.Sprintf("taskmanager routes: %v", err))
	}
	return out
}

func Split(tm mwanachamataskmanager.TaskManager) dispatch.Split { return SplitWith(tm, nil) }

func SplitWith(tm mwanachamataskmanager.TaskManager, authorize dispatch.Authorizer) dispatch.Split {
	return SplitFor(tm, Mount{Authorize: authorize})
}

func SplitFor(tm mwanachamataskmanager.TaskManager, m Mount) dispatch.Split {
	public := dispatch.Anonymous(Routes(tm), AnonymousActions...)
	gated := dispatch.Anonymous(RoutesFor(tm, m), AnonymousActions...)
	return dispatch.Split{Anonymous: public.Anonymous, Gated: gated.Gated}
}

func OperatorRoutes(tm mwanachamataskmanager.TaskManager) []Route {
	return OperatorRoutesWith(tm, nil)
}

func OperatorRoutesWith(tm mwanachamataskmanager.TaskManager, authorize dispatch.Authorizer) []Route {
	return OperatorRoutesFor(tm, Mount{Authorize: authorize})
}

func OperatorRoutesFor(tm mwanachamataskmanager.TaskManager, m Mount) []Route {
	return SplitFor(tm, m).Gated
}

// Shape is the declared route table without handlers, for a mount that
// resolves its manager per request rather than holding one. Its order is
// Dispatch's own, so Shape()[i] and Routes(tm)[i] are the same operation.
func Shape() []Route {
	s, err := operations()
	if err != nil {
		panic(fmt.Sprintf("taskmanager routes: %v", err))
	}
	names := make([]string, 0, len(s.Operations))
	for name := range s.Operations {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Route, 0, len(names))
	for _, name := range names {
		op := s.Operations[name]
		out = append(out, Route{Method: op.Method, Path: s.Base + op.Path, Action: op.Action})
	}
	return out
}
