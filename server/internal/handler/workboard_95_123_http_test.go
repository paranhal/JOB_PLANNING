package handler

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

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

func Test95_123_T3_NewWorkHasNoItemsOrProgress(t *testing.T) {
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d123-t3.db"))
	ck := jwtCookieUser(t, me)
	form := doGetWith(t, e, "/workboard/new-work/form?kind=admin&date=2026-11-04", ck).Body.String()
	if strings.Contains(form, `name="progress"`) {
		t.Fatal("새 업무에 진행률 입력이 남아 있다")
	}
	if strings.Contains(form, "작업 리스트") || strings.Contains(form, `data-work-items="1"`) {
		t.Fatal("새 업무에 작업 리스트가 남아 있다")
	}
	reg := doGetWith(t, e, "/workboard/register?view=day", ck).Body.String()
	if strings.Contains(reg, `name="progress"`) {
		t.Fatal("일정표 등록 모달에 진행률 입력이 있다")
	}
	day := time.Now().AddDate(0, 0, 6).Format(dateLayout)
	rec := doFormWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"진행률없음"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:00"}, "end_time": {"09:30"},
		"assignee_multi": {"1"}, "owner_user_id": {me.UserID}, "assignee_user_id": {me.UserID},
	}, ck)
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("저장 loc=%s", rec.Header().Get("Location"))
	}
	var id string
	if err := db.QueryRow(`SELECT task_id FROM work_tasks WHERE title='진행률없음'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetTask(id)
	if got == nil || got.Progress != 0 {
		t.Fatalf("progress=%v", got)
	}
}

func Test95_123_T4_EditDropsProgressKeepsItems(t *testing.T) {
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d123-t4.db"))
	day := time.Now().AddDate(0, 0, 7).Format(dateLayout)
	rec := doFormWith(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"항목유지"}, "work_date": {day}, "due_date": {day},
		"start_time": {"09:00"}, "end_time": {"09:30"},
		"assignee_multi": {"1"}, "owner_user_id": {me.UserID}, "assignee_user_id": {me.UserID},
	}, jwtCookieUser(t, me))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d", rec.Code)
	}
	var id string
	if err := db.QueryRow(`SELECT task_id FROM work_tasks WHERE title='항목유지'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_task_items(item_id,task_id,seq,title,status,note) VALUES('WI-123',?,1,'기존항목','todo','')`, id); err != nil {
		t.Fatal(err)
	}
	edit := doGetWith(t, e, "/workboard/tasks/"+id+"/edit", jwtCookieUser(t, me)).Body.String()
	if strings.Contains(edit, `name="progress"`) {
		t.Fatal("수정 폼에 진행률 입력이 있다")
	}
	show := doGetWith(t, e, "/workboard/tasks/"+id, jwtCookieUser(t, me)).Body.String()
	if !strings.Contains(show, "기존항목") && !strings.Contains(show, "data-work-items") {
		t.Fatal("상세에 기존 작업 항목이 없다")
	}
	_ = repo
}

func Test95_123_T5_OrderingPartyCustomerCombo(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "d123-t5.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, has_parent, party_kind)
		VALUES ('C-EDU','충남교육청','충청남도교육청',1,'O01',0,'customer')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id, has_parent, party_kind)
		VALUES ('C-SCH','공주생명과학고등학교','공주생명과학고등학교',1,'O01',0,'customer')`); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/lookup/customers", h.Customer.LookupJSON)
	g.GET("/projects/new", h.Project.New)
	g.POST("/projects", h.Project.Create)
	g.GET("/projects/:id", h.Project.Show)
	g.GET("/projects/:id/edit", h.Project.Edit)
	g.POST("/projects/:id", h.Project.Update)
	g.GET("/workboard/register", h.Workboard.Register)
	ck := jwtCookie(t)

	page := doGetWith(t, e, "/projects/new", ck)
	if page.Code != http.StatusOK {
		t.Fatalf("new %d", page.Code)
	}
	body := page.Body.String()
	if strings.Contains(body, `<select name="ordering_party_id"`) || strings.Contains(body, `<select name="customer_id"`) {
		t.Fatal("사업 등록에 select 목록이 남아 있다")
	}
	if !strings.Contains(body, "발주처 검색") || !strings.Contains(body, "고객명으로 찾기") {
		t.Fatal("발주처·고객 검색 콤보가 없다")
	}
	if !strings.Contains(body, "customerCombo") {
		t.Fatal("customerCombo 가 없다")
	}

	hit := doGetWith(t, e, "/lookup/customers?scope=party&q="+url.QueryEscape("충남")+"&limit=20", ck)
	if hit.Code != http.StatusOK || !strings.Contains(hit.Body.String(), "C-EDU") {
		t.Fatalf("검색 %d %s", hit.Code, hit.Body.String())
	}

	rec := doFormWith(t, e, "/projects", url.Values{
		"name": {"검색사업"}, "status": {"active"}, "is_paid": {"1"},
		"project_kind": {"build"}, "org_id": {"O01"},
		"ordering_party_id": {"C-EDU"}, "customer_id": {"C-SCH"},
	}, ck)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("등록 %d %s", rec.Code, rec.Body.String()[:min(400, rec.Body.Len())])
	}
	id := strings.TrimPrefix(strings.Split(rec.Header().Get("Location"), "?")[0], "/projects/")
	show := doGetWith(t, e, "/projects/"+id, ck).Body.String()
	if !strings.Contains(show, "충남교육청") {
		t.Fatal("상세에 발주처 이름이 없다")
	}
	edit := doGetWith(t, e, "/projects/"+id+"/edit", ck).Body.String()
	if !strings.Contains(edit, "C-EDU") || !strings.Contains(edit, "C-SCH") {
		t.Fatal("수정 화면에 선택값이 없다")
	}
	keep := doFormWith(t, e, "/projects/"+id, url.Values{
		"name": {"검색사업"}, "status": {"active"}, "is_paid": {"1"},
		"project_kind": {"build"}, "org_id": {"O01"},
		"ordering_party_id": {"C-EDU"}, "customer_id": {"C-SCH"},
	}, ck)
	if keep.Code != http.StatusSeeOther {
		t.Fatalf("수정 저장 %d", keep.Code)
	}

	custom := doFormWith(t, e, "/projects", url.Values{
		"name": {"직접입력사업"}, "status": {"active"}, "is_paid": {"1"},
		"project_kind": {"build"}, "org_id": {"O01"},
		"ordering_party_id": {"custom"}, "ordering_party_custom": {"비전직접발주"},
	}, ck)
	if custom.Code != http.StatusSeeOther {
		t.Fatalf("직접입력 %d", custom.Code)
	}
	cid := strings.TrimPrefix(strings.Split(custom.Header().Get("Location"), "?")[0], "/projects/")
	cshow := doGetWith(t, e, "/projects/"+cid, ck).Body.String()
	if !strings.Contains(cshow, "비전직접발주") {
		t.Fatal("직접입력 발주처가 상세에 없다")
	}

	reg := doGetWith(t, e, "/workboard/register?view=day", ck).Body.String()
	if strings.Contains(reg, `<select name="ordering_party_id"`) {
		t.Fatal("일정표 사업 모달에 발주처 select가 남아 있다")
	}
}
