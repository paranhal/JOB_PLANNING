package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestRegisterOrgIsolationO01O03AndVision(t *testing.T) {
	e, users, wb := newAssigneeOrgServer(t)
	oa01 := mustUserByName(t, users, "oa01")
	oa03 := mustUserByName(t, users, "u03")
	vs := mustUserByName(t, users, "vs1")

	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "도서관만O01업무", Assignee: "도서관담당A",
		OrgID: model.OrgIDLibrary, Status: model.WBTaskWaiting,
		WorkDate: "2026-10-19", DueDate: "2026-10-19", StartTime: "09:00", EndTime: "09:30", DurationMin: 30,
	}); err != nil {
		t.Fatal(err)
	}
	t03 := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "SI만O03업무", Assignee: "SI담당C",
		OrgID: "O03", Status: model.WBTaskWaiting,
		WorkDate: "2026-10-19", DueDate: "2026-10-19", StartTime: "10:00", EndTime: "10:30", DurationMin: 30,
	}
	if err := wb.CreateTask(t03); err != nil {
		t.Fatal(err)
	}
	need := &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "자동배치O01", Assignee: "도서관담당A",
		OrgID: model.OrgIDLibrary, Status: model.WBTaskWaiting,
		WorkDate: "2026-10-19", DueDate: "2026-10-19", DurationMin: 30,
	}
	if err := wb.CreateTask(need); err != nil {
		t.Fatal(err)
	}

	si := doGetWith(t, e, "/workboard/register?view=day&date=2026-10-19", jwtCookieUser(t, oa03))
	if si.Code != http.StatusOK {
		t.Fatalf("si %d", si.Code)
	}
	if strings.Contains(si.Body.String(), "도서관만O01업무") || strings.Contains(si.Body.String(), "도서관담당A") {
		t.Fatal("O03 화면에 O01")
	}
	if !strings.Contains(si.Body.String(), "SI만O03업무") {
		t.Fatal("O03 화면에 자기 업무 없음")
	}
	if got, _ := wb.GetTask(need.TaskID); got == nil || strings.TrimSpace(got.StartTime) != "" {
		t.Fatalf("O03 화면이 O01 자동배치를 건드림 %+v", got)
	}

	lib := doGetWith(t, e, "/workboard/register?view=day&date=2026-10-19&auto=0", jwtCookieUser(t, oa01))
	if lib.Code != http.StatusOK {
		t.Fatalf("lib %d", lib.Code)
	}
	if strings.Contains(lib.Body.String(), "SI만O03업무") || strings.Contains(lib.Body.String(), "SI담당C") {
		t.Fatal("O01 화면에 O03")
	}
	if !strings.Contains(lib.Body.String(), "도서관만O01업무") {
		t.Fatal("O01 자기 업무 없음")
	}

	kanban := doGetWith(t, e, "/workboard/register?view=day&date=2026-10-19&display=kanban&auto=0", jwtCookieUser(t, oa03))
	if kanban.Code != http.StatusOK {
		t.Fatalf("kanban %d", kanban.Code)
	}
	if strings.Contains(kanban.Body.String(), "도서관만O01업무") {
		t.Fatal("칸반에 O01")
	}

	all := doGetWith(t, e, "/workboard/register?view=day&date=2026-10-19&auto=0", jwtCookieViewOrg(t, vs, repository.OrgAll))
	if all.Code != http.StatusOK {
		t.Fatalf("vs all %d", all.Code)
	}
	if !strings.Contains(all.Body.String(), "도서관만O01업무") || !strings.Contains(all.Body.String(), "SI만O03업무") {
		t.Fatal("비전 전 조직에 양쪽 업무가 없다")
	}
	o03only := doGetWith(t, e, "/workboard/register?view=day&date=2026-10-19&auto=0", jwtCookieViewOrg(t, vs, "O03"))
	if o03only.Code != http.StatusOK {
		t.Fatalf("vs o03 %d", o03only.Code)
	}
	if strings.Contains(o03only.Body.String(), "도서관만O01업무") {
		t.Fatal("비전 O03 전환에 O01")
	}

	form := url.Values{"task_id": {t03.TaskID}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/unschedule", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookieUser(t, oa01))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Fatalf("타 조직 내리기 %d", rec.Code)
	}

	km := url.Values{"date": {"2026-10-19"}, "from": {model.WorkBucketToday}, "to": {model.WorkBucketInProgress}, "item_key": {"task:" + t03.TaskID}}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/register/kanban-move", strings.NewReader(km.Encode()))
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req2.AddCookie(jwtCookieUser(t, oa01))
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound && rec2.Code != http.StatusForbidden {
		t.Fatalf("타 조직 칸반 %d", rec2.Code)
	}
}
