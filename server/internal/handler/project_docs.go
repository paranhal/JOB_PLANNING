package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type docsPick struct {
	ID   string
	Name string
	Kind string
}

type docsItem struct {
	ID         string
	Kind       string
	KindLabel  string
	Stage      string
	Title      string
	Sub        string
	Owner      string
	Due        string
	Status     string
	DocNo      string
	Project    string
	BodyLead   string
	BodyNote   string
	Milestones []docsMilestone
	Writer     string
	Reviewer   string
	Approver   string
}

type docsMilestone struct {
	Label string
	When  string
}

type docsSeed struct {
	DateLabel string
	Weekday   string
	Weather   string
	Picks     []docsPick
	Items     []docsItem
}

func (h *ProjectHandler) Docs(c echo.Context) error {
	if !canViewProjects(c) {
		return echo.ErrForbidden
	}
	return c.Render(http.StatusOK, "project/docs.html", map[string]interface{}{
		"Title": "서류 작성·관리", "Active": NavProjectsDocs,
		"Seed": docsSeed{}, "Example": false,
	})
}
