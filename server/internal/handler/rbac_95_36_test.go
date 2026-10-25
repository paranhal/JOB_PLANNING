package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func ctxRole36(role, name, uid, org string, ro bool) echo.Context {
	c := echoCtxRole(role, name)
	c.Set("user_id", uid)
	c.Set("org_id", org)
	if ro {
		c.Set("is_readonly", true)
	}
	return c
}

func Test95_36PermissionMatrix(t *testing.T) {
	asMine := &model.ASReceipt{
		Status: "in_progress", CreatedByUserID: "u-tech", AssignedTo: "기술", AssignedUserID: "u-tech", OrgID: "O01",
	}
	asOther := &model.ASReceipt{
		Status: "in_progress", CreatedByUserID: "u-tech", AssignedTo: "기술", AssignedUserID: "u-tech", OrgID: "O01",
	}
	asClosed := *asOther
	asClosed.Status = "completed"
	task := &model.WorkTask{Assignee: "기술", AssigneeUserID: "u-tech", Status: model.WBTaskWaiting, OrgID: "O01"}
	taskDone := &model.WorkTask{Assignee: "기술", AssigneeUserID: "u-tech", Status: model.WBTaskComplete, OrgID: "O01"}

	type row struct {
		name     string
		c        echo.Context
		asOpen   bool
		asClosed bool
		visit    bool
		task     bool
		taskDone bool
		sales    bool
	}
	h := &ASHandler{}
	rows := []row{
		{"vision", ctxRole36(model.RoleVisionAdmin, "비전", "u-vs", "O01", false), true, false, true, true, true, true},
		{"org_admin", ctxRole36(model.RoleOrgAdmin, "팀장", "u-oa", "O01", false), true, true, true, true, true, true},
		{"support", ctxRole36(model.RoleSupport, "지원", "u-sup", "O01", false), true, true, true, true, true, true},
		{"tech-own", ctxRole36(model.RoleTech, "기술", "u-tech", "O01", false), true, true, true, true, true, true},
		{"tech-other", ctxRole36(model.RoleTech, "다른기술", "u-tech2", "O01", false), false, false, false, false, false, false},
		{"sales-other", ctxRole36(model.RoleSales, "영업", "u-sa", "O01", false), false, false, false, false, false, false},
		{"c_level", ctxRole36(model.RoleCLevel, "임원", "u-cl", "O01", false), false, false, false, false, false, false},
		{"observer", ctxRole36(model.RoleOrgAdmin, "옵저버", "u-ob", "O01", true), false, false, false, false, false, false},
	}
	for _, r := range rows {
		if got := canEditASReceipt(r.c, asOther); got != r.asOpen {
			t.Errorf("%s AS접수수정 got=%v want=%v", r.name, got, r.asOpen)
		}
		if got := h.canModifyAS(r.c, &asClosed); got != r.asClosed {
			t.Errorf("%s 완료AS got=%v want=%v", r.name, got, r.asClosed)
		}
		if got := canEditVisitDate(r.c, asOther); got != r.visit {
			t.Errorf("%s 방문일 got=%v want=%v", r.name, got, r.visit)
		}
		if got := canEditTask(r.c, task.Assignee, task.AssigneeUserID); got != r.task {
			t.Errorf("%s 업무 got=%v want=%v", r.name, got, r.task)
		}
		if got := canEditTask(r.c, taskDone.Assignee, taskDone.AssigneeUserID); got != r.taskDone {
			t.Errorf("%s 완료업무 got=%v want=%v", r.name, got, r.taskDone)
		}
		if got := canEditSalesActivity(r.c, "기술"); got != r.sales {
			t.Errorf("%s 영업활동 got=%v want=%v", r.name, got, r.sales)
		}
	}
	ownTech := ctxRole36(model.RoleTech, "기술", "u-tech", "O01", false)
	if !canEditASReceipt(ownTech, asMine) {
		t.Fatal("기술 본인 건")
	}
	salesOwn := ctxRole36(model.RoleSales, "영업", "u-sa", "O01", false)
	if !canEditSalesActivity(salesOwn, "영업") {
		t.Fatal("영업 본인 활동")
	}
	empty := &model.ASReceipt{Status: "in_progress", OrgID: "O01"}
	oa := ctxRole36(model.RoleOrgAdmin, "팀장", "u-oa", "O01", false)
	sup := ctxRole36(model.RoleSupport, "지원", "u-sup", "O01", false)
	if sessionOwnsAS(oa, empty) {
		t.Fatal("담당 없는 건 org_admin 소유 아님")
	}
	if !sessionOwnsAS(sup, empty) {
		t.Fatal("담당 없는 건 support 는 본인 지정 가능")
	}
}

func Test95_36AssigneeRequiredAndBehalfAndViewAsOff(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "p36.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,sort_order) VALUES('O02','O02','다른팀',2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c1','도서관','도서관',1,'O01'),('c2','다른관','다른관',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	must := func(u *model.User) *model.User {
		t.Helper()
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
		got, err := users.GetByUsername(u.Username)
		if err != nil || got == nil {
			t.Fatal(err)
		}
		return got
	}
	oa := must(&model.User{Username: "oa36", PasswordHash: HashPassword("pw"), FullName: "조직장36", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true})
	sup := must(&model.User{Username: "sup36", PasswordHash: HashPassword("pw"), FullName: "지원36", Role: model.RoleSupport, IsActive: true, OrgID: "O01", ProfileDone: true})
	tech := must(&model.User{Username: "tech36", PasswordHash: HashPassword("pw"), FullName: "기술36", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true})
	oa2 := must(&model.User{Username: "oa2x", PasswordHash: HashPassword("pw"), FullName: "타팀장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true})

	asRepo := repository.NewASRepo(db)
	makeAS := func(assigned string, uid, org, status string) *model.ASReceipt {
		t.Helper()
		rec := &model.ASReceipt{
			CustomerID: "c1", Symptom: "증상36", AssignedTo: assigned, AssignedUserID: uid,
			Status: status, ReceiptDatetime: time.Now(), OrgID: org, CreatedByUserID: tech.UserID,
			ReceivedBy: "기술36",
		}
		if org == "O02" {
			rec.CustomerID = "c2"
		}
		if err := asRepo.Create(rec); err != nil {
			t.Fatal(err)
		}
		_, _ = db.Exec(`UPDATE as_receipts SET status=?, assigned_to=?, assigned_user_id=?, created_by_user_id=? WHERE as_id=?`,
			status, assigned, uid, tech.UserID, rec.ASID)
		return rec
	}
	open := makeAS("기술36", tech.UserID, "O01", "in_progress")
	emptyAS := makeAS("", "", "O01", "in_progress")
	otherOrg := makeAS("기술36", tech.UserID, "O02", "in_progress")

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.InjectViewAs)
	g.Use(h.Auth.GuardViewAsWrite)
	g.GET("/as/:id/action", h.AS.Action)
	g.POST("/as/:id/update", h.AS.Update)
	g.POST("/as", h.AS.Create)
	g.GET("/as/new", h.AS.New)
	g.POST("/view-as", h.Auth.SetViewAs)
	g.GET("/", h.Dashboard)

	oaCK := jwtCookieUser(t, oa)
	supCK := jwtCookieUser(t, sup)
	oa2CK := jwtCookieUser(t, oa2)

	form := url.Values{
		"customer_id": {"c1"}, "symptom": {"접수36"}, "received_by": {"조직장36"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
	}
	miss := postForm(t, e, "/as", form, oaCK)
	if miss.Code != http.StatusSeeOther || !strings.Contains(miss.Header().Get("Location"), "assignee_required") {
		t.Fatalf("담당 없이 저장 loc=%s code=%d", miss.Header().Get("Location"), miss.Code)
	}
	newPage := doGetWith(t, e, "/as/new?err=assignee_required&focus=assignee", oaCK)
	if newPage.Code != http.StatusOK {
		t.Fatalf("접수 폼 %d", newPage.Code)
	}
	body := newPage.Body.String()
	if !strings.Contains(body, "담당자를 선택하세요") || !strings.Contains(body, `data-focus-target="assignee"`) || !strings.Contains(body, `data-require-assignee`) {
		t.Fatal("담당자 필수 마크업 없음")
	}
	form.Set("assigned_to_code", tech.UserID)
	ok := postForm(t, e, "/as", form, oaCK)
	if ok.Code != http.StatusSeeOther || strings.Contains(ok.Header().Get("Location"), "err=") {
		t.Fatalf("담당 지정 저장 loc=%s", ok.Header().Get("Location"))
	}

	act := url.Values{
		"work_place": {"office"}, "process_type": {"remote"}, "cause_type": {"hw"},
		"action_taken": {"원격 안내함"}, "result_code": {model.ResultDone}, "time_spent": {"20"},
		"action_short_ok": {"1"}, "assigned_to_code": {tech.UserID},
	}
	fillCauseCatsFromLegacyType(act)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+open.ASID+"/update", strings.NewReader(act.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(oaCK)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("조치 %d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	procs, err := h.AS.processRepo.ListByAS(open.ASID)
	if err != nil || len(procs) == 0 {
		t.Fatal("조치 이력")
	}
	p := procs[len(procs)-1]
	if p.Worker != "기술36" {
		t.Fatalf("Worker=%s", p.Worker)
	}
	if !p.OnBehalf || p.ActedByName != "조직장36" {
		t.Fatalf("on_behalf=%v acted=%s", p.OnBehalf, p.ActedByName)
	}
	if strings.Contains(p.Worker, "조직장") {
		t.Fatal("관리자가 Worker")
	}

	supNew := doGetWith(t, e, "/as/new", supCK)
	if !strings.Contains(supNew.Body.String(), sup.UserID) {
		t.Fatal("지원 기본 담당자가 본인이 아님")
	}
	sf := url.Values{
		"customer_id": {"c1"}, "symptom": {"지원본인"}, "received_by": {"지원36"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")}, "assigned_to_code": {sup.UserID},
	}
	srec := postForm(t, e, "/as", sf, supCK)
	if srec.Code != http.StatusSeeOther || strings.Contains(srec.Header().Get("Location"), "err=") {
		t.Fatalf("지원 본인 접수 loc=%s", srec.Header().Get("Location"))
	}
	act2 := url.Values{
		"work_place": {"office"}, "process_type": {"remote"}, "cause_type": {"hw"},
		"action_taken": {"대신 조치함"}, "result_code": {model.ResultDone}, "time_spent": {"15"},
		"action_short_ok": {"1"}, "assigned_to_code": {tech.UserID},
	}
	fillCauseCatsFromLegacyType(act2)
	sact := httptest.NewRecorder()
	sreq := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+open.ASID+"/update", strings.NewReader(act2.Encode()))
	sreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	sreq.AddCookie(supCK)
	e.ServeHTTP(sact, sreq)
	if sact.Code != http.StatusSeeOther {
		t.Fatalf("지원 대신 조치 %d", sact.Code)
	}

	emptyPage := doGetWith(t, e, "/as/"+emptyAS.ASID+"/action", oaCK)
	if emptyPage.Code != http.StatusOK {
		t.Fatalf("빈 담당 action %d", emptyPage.Code)
	}

	other := doGetWith(t, e, "/as/"+otherOrg.ASID+"/action", oaCK)
	if other.Code != http.StatusNotFound && other.Code != http.StatusForbidden {
		if other.Code == http.StatusOK {
			t.Fatal("타 조직 건이 열림")
		}
	}
	other2 := doGetWith(t, e, "/as/"+otherOrg.ASID+"/action", oa2CK)
	if other2.Code != http.StatusOK && other2.Code != http.StatusSeeOther {
		// 타팀장은 자기 조직이므로 열려야 함
	}

	off := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "/view-as", strings.NewReader(url.Values{"user_id": {tech.UserID}}.Encode()))
	oreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	oreq.AddCookie(oaCK)
	e.ServeHTTP(off, oreq)
	if off.Code != http.StatusNotFound {
		t.Fatalf("시점 보기 off 인데 %d", off.Code)
	}
	dash := httptest.NewRecorder()
	dreq := httptest.NewRequest(http.MethodGet, "/", nil)
	dreq.AddCookie(oaCK)
	dreq.AddCookie(&http.Cookie{Name: "view_as_user_id", Value: tech.UserID, Path: "/"})
	e.ServeHTTP(dash, dreq)
	if dash.Code != http.StatusOK {
		t.Fatalf("대시보드 %d", dash.Code)
	}
	if strings.Contains(dash.Body.String(), "시점으로 보고 있습니다") || strings.Contains(dash.Body.String(), "누구 시점으로 보기") {
		t.Fatal("시점 보기 UI가 있다")
	}
	del := false
	for _, ck := range dash.Result().Cookies() {
		if ck.Name == "view_as_user_id" && (ck.MaxAge < 0 || ck.Value == "") {
			del = true
		}
	}
	if !del {
		t.Fatal("view_as 쿠키가 삭제되지 않음")
	}
	emptyCtx := echoCtxRole(model.RoleOrgAdmin, "조직장36")
	if viewAsActive(emptyCtx) {
		t.Fatal("viewAsActive")
	}
}

func Test95_36ActedByMigrationIdempotent(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "p36mig.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.ApplyActedByV85ForTest(db)
	repository.ApplyActedByV85ForTest(db)
	for _, table := range []string{"work_tasks", "sales_activities", "maintenance_visits"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('` + table + `') WHERE name='acted_by_name'`).Scan(&n); err != nil || n != 1 {
			t.Fatalf("%s.acted_by_name n=%d err=%v", table, n, err)
		}
	}
}
