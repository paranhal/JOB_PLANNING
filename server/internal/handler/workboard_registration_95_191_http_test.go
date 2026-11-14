package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-191: 완료·취소 업무의 등록정보 수정은 관리자급(조직관리자·비전관리자)만. 그 외는 링크를 숨기고 안내한다.

type env191 struct {
	e                         *echo.Echo
	wb                        *repository.WBRepo
	do                        func(method, path string, form url.Values, u *model.User) *httptest.ResponseRecorder
	admin, owner, other       *model.User
	done, cancelled, progress string
}

func setup191(t *testing.T) env191 {
	t.Helper()
	e, db, owner, other, wb := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "r191.db"))
	admin, err := repository.NewUserRepo(db).GetByUsername("oa01")
	if err != nil || admin == nil {
		t.Fatal("oa01")
	}
	slot := 8
	mk := func(title, status string) string {
		slot++
		hh := strings.Repeat("0", 0) + string(rune('0'+slot/10)) + string(rune('0'+slot%10))
		tk := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: title, Assignee: owner.FullName, AssigneeUserID: owner.UserID, OrgID: model.OrgIDLibrary,
			Status: status, WorkDate: "2026-11-10", DueDate: "2026-11-10", StartTime: hh + ":00", EndTime: hh + ":30", DurationMin: 30}
		if err := wb.CreateTask(tk); err != nil {
			t.Fatal(err)
		}
		return tk.TaskID
	}
	v := env191{e: e, wb: wb, admin: admin, owner: owner, other: other,
		done: mk("완료 업무", model.WBTaskComplete), cancelled: mk("취소 업무", model.WBTaskCancelled), progress: mk("진행 업무", model.WBTaskInProgress)}
	v.do = func(method, path string, form url.Values, u *model.User) *httptest.ResponseRecorder {
		var req *http.Request
		if form != nil {
			req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.AddCookie(jwtCookieUser(t, u))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	return v
}

func TestHTTP95_191_AdminCanEditClosedTask(t *testing.T) {
	v := setup191(t)
	for _, id := range []string{v.done, v.cancelled} {
		if r := v.do("GET", "/workboard/tasks/"+id+"/edit", nil, v.admin); r.Code != 200 {
			t.Fatalf("관리자 /edit %s %d %s", id, r.Code, r.Body.String())
		}
		d := v.do("GET", "/workboard/tasks/"+id, nil, v.admin).Body.String()
		if !strings.Contains(d, "data-registration-edit") {
			t.Fatalf("관리자 상세에 「수정」 링크가 없다 %s", id)
		}
		// 이월·계획 수정(canPlanEditTask)은 그대로: 완료·취소 건에는 「계획 수정」이 없다
		if strings.Contains(d, ">계획 수정<") {
			t.Fatalf("계획 수정 링크가 생겼다(canPlanEditTask 변경 금지) %s", id)
		}
	}
	// 저장: 제목이 바뀌고 상태는 그대로
	form := url.Values{"scope": {"register"}, "title": {"완료 업무(수정)"}, "status": {model.WBTaskComplete},
		"assignee": {v.owner.FullName}, "work_date": {"2026-11-10"}, "due_date": {"2026-11-10"}, "start_time": {"09:00"}, "end_time": {"09:30"}, "duration_min": {"30"}, "work_type": {model.WBWorkAdmin}}
	r := v.do("POST", "/workboard/tasks/"+v.done+"/update", form, v.admin)
	if r.Code != http.StatusSeeOther {
		t.Fatalf("관리자 저장 %d %s", r.Code, r.Body.String())
	}
	d := v.do("GET", "/workboard/tasks/"+v.done, nil, v.admin).Body.String()
	if !strings.Contains(d, "완료 업무(수정)") {
		t.Fatal("저장한 제목이 상세에 없다")
	}
	if got, _ := v.wb.GetTask(v.done); got == nil || got.Status != model.WBTaskComplete {
		t.Fatalf("저장 뒤 상태가 바뀌었다 %+v", got)
	}
}

func TestHTTP95_191_NonAdminClosedTaskHiddenWithNotice(t *testing.T) {
	v := setup191(t)
	for _, c := range []struct{ id, msg string }{{v.done, "완료된 업무는 수정할 수 없습니다"}, {v.cancelled, "취소된 업무는 수정할 수 없습니다"}} {
		d := v.do("GET", "/workboard/tasks/"+c.id, nil, v.owner).Body.String()
		if strings.Contains(d, "data-registration-edit") {
			t.Fatalf("수행 담당에게 「수정」 링크 %s", c.id)
		}
		if !strings.Contains(d, c.msg) {
			t.Fatalf("안내 문구 없음: %s", c.msg)
		}
		// 직접 /edit: JSON 403 이 아니라 상세로 돌아가 안내
		r := v.do("GET", "/workboard/tasks/"+c.id+"/edit", nil, v.owner)
		loc, _ := url.QueryUnescape(r.Header().Get("Location"))
		if r.Code != http.StatusSeeOther || !strings.Contains(loc, "/workboard/tasks/"+c.id) || !strings.Contains(loc, c.msg) {
			t.Fatalf("직접 /edit %d %s %s", r.Code, loc, r.Body.String())
		}
		// 저장도 막힌다(제목 불변)
		form := url.Values{"scope": {"register"}, "title": {"바꿈"}, "assignee": {v.owner.FullName}}
		r = v.do("POST", "/workboard/tasks/"+c.id+"/update", form, v.owner)
		if r.Code != http.StatusSeeOther || !strings.Contains(r.Header().Get("Location"), "err=") {
			t.Fatalf("수행 담당 저장 %d %s", r.Code, r.Body.String())
		}
	}
	// 다른 조직 사용자·일반은 기존처럼 거절
	if r := v.do("GET", "/workboard/tasks/"+v.done+"/edit", nil, v.other); r.Code == 200 {
		t.Fatalf("타 조직 사용자가 완료 건 /edit 열람 %d", r.Code)
	}
}

func TestHTTP95_191_OpenTaskUnchanged(t *testing.T) {
	v := setup191(t)
	d := v.do("GET", "/workboard/tasks/"+v.progress, nil, v.owner).Body.String()
	if !strings.Contains(d, "data-registration-edit") || strings.Contains(d, "수정할 수 없습니다") {
		t.Fatal("진행 중 건: 수정 링크가 있고 안내는 없어야 한다")
	}
	if r := v.do("GET", "/workboard/tasks/"+v.progress+"/edit", nil, v.owner); r.Code != 200 {
		t.Fatalf("진행 중 /edit %d", r.Code)
	}
	form := url.Values{"scope": {"register"}, "title": {"진행 업무(수정)"}, "status": {model.WBTaskInProgress},
		"assignee": {v.owner.FullName}, "work_date": {"2026-11-10"}, "due_date": {"2026-11-10"}, "start_time": {"11:00"}, "end_time": {"11:30"}, "duration_min": {"30"}, "work_type": {model.WBWorkAdmin}}
	if r := v.do("POST", "/workboard/tasks/"+v.progress+"/update", form, v.owner); r.Code != http.StatusSeeOther || strings.Contains(r.Header().Get("Location"), "err=") {
		t.Fatalf("진행 중 저장 %d %s", r.Code, r.Header().Get("Location"))
	}
	if !strings.Contains(v.do("GET", "/workboard/tasks/"+v.progress, nil, v.owner).Body.String(), "진행 업무(수정)") {
		t.Fatal("진행 중 건 수정이 반영되지 않았다")
	}
}
