package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func TestFilterMeetingOrgUsesOrgIDNotName(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/meeting", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	c.Set("org_id", "O01")
	c.Set("role", model.RoleOrgAdmin)
	h := &MeetingHandler{}
	items := []model.WorkListItem{
		{RefID: "a", OrgID: "O01", Assignee: "김동일", Title: "도서관"},
		{RefID: "b", OrgID: "O03", Assignee: "김동일", Title: "SI"},
	}
	got := h.filterMeetingOrg(c, items)
	if len(got) != 1 || got[0].RefID != "a" {
		t.Fatalf("got=%+v", got)
	}
}
