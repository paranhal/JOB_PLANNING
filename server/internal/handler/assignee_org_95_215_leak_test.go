package handler

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-215 후속 — 누출 전수 시험(게이트). 다른 조직 직원 이름이 도서관 화면 어디에도 나오면 안 된다.
// 이름이 나올 수 있는 화면은 모두 assigneeLeakPages 에 넣는다. 새 화면을 만들면 여기에 한 줄을 더해야 하고,
// 한 곳이라도 이름이 나오면 실패한다. 완료 건은 실적 이력이라 이름이 남는 것이 맞다(별도 확인).
var assigneeLeakPages = []string{
	"/workboard/register?view=day&date=2026-10-19",
	"/workboard/register?view=week&date=2026-10-19",
	"/workboard/register?view=month&date=2026-10-19",
	"/workboard/register?view=day&date=2026-10-19&display=kanban",
	"/workboard/board",
	"/work",
	"/work/all",
	"/plan/unplanned",
	"/meeting",
	"/meeting/weekly",
	"/meeting/board",
	"/work-status",
	"/maintenance",
	"/sales",
	"/",
}

func TestAssigneeOrgNoLeakAllPages(t *testing.T) {
	e, users, wb := newAssigneeOrgServer(t)
	for _, d := range []string{"2026-10-19", "2026-10-20"} {
		for _, st := range []string{model.WBTaskWaiting, model.WBTaskInProgress} {
			if err := wb.CreateTask(&model.WorkTask{
				WorkType: model.WBWorkAdmin, Title: "타팀담당업무" + d + st, Assignee: "SI담당C",
				OrgID: model.OrgIDLibrary, Status: st,
				WorkDate: d, DueDate: d, StartTime: "09:00", EndTime: "09:30", DurationMin: 30,
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, role := range []string{"oa01"} {
		u := mustUserByName(t, users, role)
		for _, p := range assigneeLeakPages {
			rec := doGetWith(t, e, p, jwtCookieUser(t, u))
			if rec.Code != 200 && rec.Code != 302 && rec.Code != 303 {
				t.Errorf("%s %s: status %d (화면이 열려야 누출 여부를 볼 수 있다)", role, p, rec.Code)
				continue
			}
			body := rec.Body.String()
			// 카드는 숨기지 않는다: 업무가 나오는 화면에서는 제목이 그대로 있어야 한다.
			if (p == "/work/all" || strings.HasPrefix(p, "/workboard/register?view=day&date=2026-10-19")) && !strings.Contains(body, "타팀담당업무") {
				t.Errorf("%s %s: 업무 카드가 숨겨졌다", role, p)
			}
			if i := strings.Index(body, "SI담당C"); i >= 0 {
				lo, hi := i-120, i+60
				if lo < 0 {
					lo = 0
				}
				if hi > len(body) {
					hi = len(body)
				}
				t.Errorf("%s %s: 다른 조직 직원 이름이 나옴: ...%s...", role, p, strings.Join(strings.Fields(body[lo:hi]), " "))
			}
		}
	}
}
