package handler

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_123_T1_AssigneeSeedSelfAndLoginID(t *testing.T) {
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d123-t1.db"))
	ck := jwtCookieUser(t, me)
	reg := doGetWith(t, e, "/workboard/register?view=day", ck).Body.String()
	if strings.Contains(reg, "이름 미등록") {
		t.Fatal("일정표 새 업무에 이름 미등록")
	}
	if !strings.Contains(reg, `name="owner_user_id" value="`+me.UserID+`"`) {
		t.Fatal("일정표 담당자 초기값이 본인이 아니다")
	}
	if !strings.Contains(reg, me.FullName) {
		t.Fatal("일정표에 본인 이름이 없다")
	}
	form := doGetWith(t, e, "/workboard/new-work/form?kind=admin&date=2026-11-04", ck).Body.String()
	if strings.Contains(form, "이름 미등록") {
		t.Fatal("새 업무 폼에 이름 미등록")
	}
	if !strings.Contains(form, `name="owner_user_id" value="`+me.UserID+`"`) {
		t.Fatal("새 업무 폼 담당자 초기값이 본인이 아니다")
	}

	users := repository.NewUserRepo(db)
	blank := &model.User{Username: "loginid", PasswordHash: HashPassword("password10"), FullName: "", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(blank); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByUsername("loginid")
	body := doGetWith(t, e, "/workboard/register?view=day", jwtCookieUser(t, got)).Body.String()
	if strings.Contains(body, "이름 미등록") {
		t.Fatal("이름 없는 계정에 이름 미등록")
	}
	if !strings.Contains(body, "loginid") {
		t.Fatal("이름 없는 계정에 로그인 ID가 없다")
	}
	if !strings.Contains(body, `name="owner_user_id" value="`+got.UserID+`"`) {
		t.Fatal("이름 없는 계정 초기값이 본인이 아니다")
	}

	oa, _ := users.GetByUsername("oa01")
	oaBody := doGetWith(t, e, "/workboard/register?view=day", jwtCookieUser(t, oa)).Body.String()
	if strings.Contains(oaBody, "이름 미등록") {
		t.Fatal("조직관리자에 이름 미등록")
	}
	if !strings.Contains(oaBody, `name="owner_user_id" value="`+oa.UserID+`"`) {
		t.Fatal("조직관리자 초기값이 본인이 아니다")
	}
	if !strings.Contains(oaBody, oa.FullName) {
		t.Fatal("조직관리자 이름이 없다")
	}
}

func Test95_123_T2_SaveSelfAndOrgAdminChange(t *testing.T) {
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d123-t2.db"))
	day := time.Now().AddDate(0, 0, 4).Format(dateLayout)
	rec := doFormWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"본인담당"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:00"}, "end_time": {"09:30"},
		"assignee_multi": {"1"}, "owner_user_id": {me.UserID}, "assignee_user_id": {me.UserID},
		"assignee": {me.FullName},
	}, jwtCookieUser(t, me))
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("본인 저장 loc=%s code=%d", rec.Header().Get("Location"), rec.Code)
	}
	var id string
	if err := db.QueryRow(`SELECT task_id FROM work_tasks WHERE title='본인담당'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetTask(id)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.AssigneeUserID != me.UserID {
		t.Fatalf("담당자 ID=%s want %s", got.AssigneeUserID, me.UserID)
	}

	users := repository.NewUserRepo(db)
	oa, _ := users.GetByUsername("oa01")
	otherDay := time.Now().AddDate(0, 0, 5).Format(dateLayout)
	chg := doFormWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"관리자지정"}, "work_date": {otherDay}, "due_date": {otherDay},
		"start_time": {"10:00"}, "end_time": {"10:30"},
		"assignee_multi": {"1"}, "owner_user_id": {me.UserID}, "assignee_user_id": {me.UserID},
		"assignee": {me.FullName},
	}, jwtCookieUser(t, oa))
	if chg.Code != http.StatusSeeOther || strings.Contains(chg.Header().Get("Location"), "err=") {
		t.Fatalf("조직관리자 지정 loc=%s code=%d", chg.Header().Get("Location"), chg.Code)
	}
	var id2 string
	if err := db.QueryRow(`SELECT task_id FROM work_tasks WHERE title='관리자지정'`).Scan(&id2); err != nil {
		t.Fatal(err)
	}
	got2, _ := repo.GetTask(id2)
	if got2 == nil || got2.AssigneeUserID != me.UserID {
		t.Fatalf("조직관리자가 다른 사람 지정 실패 %+v", got2)
	}
}
