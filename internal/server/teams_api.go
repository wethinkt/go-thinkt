package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/wethinkt/go-thinkt/internal/thinkt"
)

// TeamsResponse lists teams.
type TeamsResponse struct {
	Teams []thinkt.Team `json:"teams"`
}

// TeamTasksResponse lists team tasks.
type TeamTasksResponse struct {
	Tasks []thinkt.TeamTask `json:"tasks"`
}

// TeamMessagesResponse lists team messages.
type TeamMessagesResponse struct {
	Messages []thinkt.TeamMessage `json:"messages"`
}

// handleGetTeams returns all discovered teams.
// @Summary List all teams
// @Description Returns all Claude Code teams discovered on this machine
// @Tags teams
// @Produce json
// @Success 200 {object} TeamsResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 500 {object} ErrorResponse
// @Router /teams [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetTeams(w http.ResponseWriter, r *http.Request) {
	if s.teamStore == nil {
		writeJSON(w, http.StatusOK, TeamsResponse{Teams: []thinkt.Team{}})
		return
	}

	teams, err := s.teamStore.ListTeams(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_teams_failed", err.Error())
		return
	}
	if teams == nil {
		teams = []thinkt.Team{}
	}

	writeJSON(w, http.StatusOK, TeamsResponse{Teams: teams})
}

// handleGetTeam returns a specific team with resolved member mappings.
// @Summary Get team details
// @Description Returns team details including resolved member-to-session mappings
// @Tags teams
// @Produce json
// @Param teamName path string true "Team name"
// @Success 200 {object} thinkt.Team
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /teams/{teamName} [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetTeam(w http.ResponseWriter, r *http.Request) {
	if s.teamStore == nil {
		writeError(w, http.StatusNotFound, "not_found", "Teams not available")
		return
	}

	teamName := chi.URLParam(r, "teamName")
	if teamName == "" {
		writeError(w, http.StatusBadRequest, "missing_team_name", "Team name is required")
		return
	}

	team, err := s.teamStore.GetTeam(r.Context(), teamName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get_team_failed", err.Error())
		return
	}
	if team == nil {
		writeError(w, http.StatusNotFound, "not_found", "Team not found")
		return
	}

	writeJSON(w, http.StatusOK, team)
}

// handleGetTeamTasks returns the shared task board for a team.
// @Summary List team tasks
// @Description Returns the shared task board for a team
// @Tags teams
// @Produce json
// @Param teamName path string true "Team name"
// @Success 200 {object} TeamTasksResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 500 {object} ErrorResponse
// @Router /teams/{teamName}/tasks [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetTeamTasks(w http.ResponseWriter, r *http.Request) {
	if s.teamStore == nil {
		writeJSON(w, http.StatusOK, TeamTasksResponse{Tasks: []thinkt.TeamTask{}})
		return
	}

	teamName := chi.URLParam(r, "teamName")
	if teamName == "" {
		writeError(w, http.StatusBadRequest, "missing_team_name", "Team name is required")
		return
	}

	tasks, err := s.teamStore.GetTeamTasks(r.Context(), teamName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get_team_tasks_failed", err.Error())
		return
	}
	if tasks == nil {
		tasks = []thinkt.TeamTask{}
	}

	writeJSON(w, http.StatusOK, TeamTasksResponse{Tasks: tasks})
}

// handleGetTeamMemberMessages returns inbox messages for a team member.
// @Summary List team member messages
// @Description Returns inbox messages for a specific team member
// @Tags teams
// @Produce json
// @Param teamName path string true "Team name"
// @Param memberName path string true "Member name"
// @Success 200 {object} TeamMessagesResponse
// @Failure 401 {object} ErrorResponse "Unauthorized - invalid or missing token"
// @Failure 500 {object} ErrorResponse
// @Router /teams/{teamName}/members/{memberName}/messages [get]
// @Security BearerAuth
func (s *HTTPServer) handleGetTeamMemberMessages(w http.ResponseWriter, r *http.Request) {
	if s.teamStore == nil {
		writeJSON(w, http.StatusOK, TeamMessagesResponse{Messages: []thinkt.TeamMessage{}})
		return
	}

	teamName := chi.URLParam(r, "teamName")
	memberName := chi.URLParam(r, "memberName")
	if teamName == "" || memberName == "" {
		writeError(w, http.StatusBadRequest, "missing_params", "Team name and member name are required")
		return
	}

	msgs, err := s.teamStore.GetTeamMessages(r.Context(), teamName, memberName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get_messages_failed", err.Error())
		return
	}
	if msgs == nil {
		msgs = []thinkt.TeamMessage{}
	}

	writeJSON(w, http.StatusOK, TeamMessagesResponse{Messages: msgs})
}
