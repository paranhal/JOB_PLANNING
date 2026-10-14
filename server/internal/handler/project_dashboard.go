package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

type dashTodo struct {
	Track   string
	Urgency string
	Title   string
	Person  string
	When    string
	Due     bool
}

type dashUpcoming struct {
	Track string
	Title string
	When  string
}

type dashProgram struct {
	Track    string
	Kind     string
	Name     string
	Progress string
}

type dashDomain struct {
	Track     string
	Title     string
	Count     string
	Programs  []dashProgram
	WorkTitle string
	WorkNote  string
}

type dashSeed struct {
	DateLabel    string
	AsOf         string
	TrackCounts  string
	StageCounts  string
	TodayN       int
	PeopleN      int
	ApproveN     int
	SoonN        int
	TodayBreak   string
	PeopleBreak  string
	ApproveBreak string
	SoonBreak    string
	Todos        []dashTodo
	Approvals    []dashTodo
	Upcoming     []dashUpcoming
	Domains      []dashDomain
	BriefItems   string
}

func dashboardTab(raw string) string {
	switch strings.TrimSpace(raw) {
	case "construction", "network", "dev", "maintenance":
		return strings.TrimSpace(raw)
	default:
		return "ops"
	}
}

func (h *ProjectHandler) Dashboard(c echo.Context) error {
	if !canViewProjects(c) {
		return echo.ErrForbidden
	}
	tab := dashboardTab(c.QueryParam("tab"))
	return c.Render(http.StatusOK, "project/dashboard.html", map[string]interface{}{
		"Title":       "사업 대시보드",
		"Active":      NavProjectsDashboard,
		"Tab":         tab,
		"Seed":        dashSeed{},
		"Example":     false,
		"ReportURL":   "/projects/diary",
		"ProjectsURL": "/projects",
		"DocsURL":     "/projects/docs",
		"FilterURL":   "/projects",
	})
}
