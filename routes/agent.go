package routes

import (
	"net/http"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// AgentRoutes returns the plain Agent routes.
func AgentRoutes(tm mwanachamataskmanager.TaskManager) []Route {
	return []Route{
		{Method: "POST", Path: "/agents", Handler: UpsertAgent(tm)},
		{Method: "GET", Path: "/agents", Handler: ListAgents(tm)},
		{Method: "GET", Path: "/agents/by-agent-id/{agentID}", Handler: GetAgentByAgentID(tm)},
		{Method: "GET", Path: "/agents/{idOrSlug}", Handler: GetAgent(tm)},
	}
}

func UpsertAgent(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var agent mwanachamataskmanager.Agent
		if err := readJSON(r, &agent); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		out, err := tm.UpsertAgent(r.Context(), agent)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func GetAgent(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agent, err := tm.GetAgent(r.Context(), r.PathValue("idOrSlug"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, agent)
	}
}

func GetAgentByAgentID(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agent, err := tm.GetAgentByAgentID(r.Context(), r.PathValue("agentID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, agent)
	}
}

func ListAgents(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agents, err := tm.ListAgents(r.Context())
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, agents)
	}
}
