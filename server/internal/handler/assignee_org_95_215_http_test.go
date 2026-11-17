package handler

import (
	"net/http"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-215 규칙 O: 도서관 화면에 타 팀 담당자 이름이 나오지 않고, 카드는 「담당 재배정 필요」로 남는다.
func TestAssigneeOrgDisplayReassignBadge(t *testing.T) {
	e, users, wb := newAssigneeOrgServer(t)
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "타팀담당업무", Assignee: "SI담당C",
		OrgID: model.OrgIDLibrary, Status: model.WBTaskWaiting,
		WorkDate: "2026-10-19", DueDate: "2026-10-19", StartTime: "09:00", EndTime: "09:30", DurationMin: 30,
	}); err != nil {
		t.Fatal(err)
	}
	oa := mustUserByName(t, users, "oa01")
	for _, path := range []string{
		"/workboard/register?view=day&date=2026-10-19",
		"/workboard/register?view=day&date=2026-10-19&display=kanban",
	} {
		rec := doGetWith(t, e, path, jwtCookieUser(t, oa))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "타팀담당업무") {
			t.Errorf("%s: 카드는 숨기지 않는다", path)
		}
		if strings.Contains(body, "SI담당C") {
			i := strings.Index(body, "SI담당C")
			t.Errorf("%s: 조직 밖 담당자 이름이 노출됨: ...%s...", path, body[max(0, i-250):min(len(body), i+100)])
		}
	}
}
