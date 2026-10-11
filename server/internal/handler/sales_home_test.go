package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestSalesHomeRedirects(t *testing.T) {
	e, db, tech := newAssignNoticeServer(t)
	users := repository.NewUserRepo(db)
	if err := users.Create(&model.User{
		Username: "sale1", PasswordHash: HashPassword("pw"), FullName: "영업하나",
		Role: model.RoleSales, IsActive: true, OrgID: model.OrgIDLibrary,
	}); err != nil {
		t.Fatal(err)
	}
	sales, err := users.GetByUsername("sale1")
	if err != nil || sales == nil {
		t.Fatal(err)
	}
	admin, err := users.GetByUsername("admin")
	if err != nil || admin == nil {
		t.Fatal(err)
	}
	insertUnseenNotice(t, db, sales.UserID, "S1", "영업배정알림", time.Now())

	ck := jwtCookieUser(t, sales)
	home := doGetWith(t, e, "/", ck)
	if home.Code != http.StatusFound {
		t.Fatalf("영업 GET / status=%d loc=%s", home.Code, home.Header().Get("Location"))
	}
	if home.Header().Get("Location") != pathSalesDashboard {
		t.Fatalf("Location=%s", home.Header().Get("Location"))
	}
	if cookieNamed(home, assignNoticeShownCookie) != nil {
		t.Fatal("302 에 오늘 봤음 쿠키")
	}

	hx := httptest.NewRecorder()
	hxReq := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	hxReq.Header.Set("HX-Request", "true")
	hxReq.AddCookie(ck)
	e.ServeHTTP(hx, hxReq)
	if hx.Code != http.StatusOK {
		t.Fatalf("HTMX GET / status=%d", hx.Code)
	}
	if hx.Header().Get("HX-Redirect") != pathSalesDashboard {
		t.Fatalf("HX-Redirect=%s", hx.Header().Get("HX-Redirect"))
	}
	if loc := hx.Header().Get("Location"); loc != "" {
		t.Fatalf("HTMX Location=%s", loc)
	}

	techHome := doGetWith(t, e, "/?home=tech", ck)
	if techHome.Code != http.StatusOK {
		t.Fatalf("home=tech status=%d loc=%s", techHome.Code, techHome.Header().Get("Location"))
	}
	if !strings.Contains(techHome.Body.String(), "영업 대시보드로") {
		t.Fatal("기술 대시보드에 전환 링크 없음")
	}

	dash := doGetWith(t, e, pathSalesDashboard, ck)
	if dash.Code != http.StatusOK {
		t.Fatalf("영업 대시보드 status=%d loc=%s", dash.Code, dash.Header().Get("Location"))
	}
	if dash.Header().Get("Location") == "/" {
		t.Fatal("리다이렉트 루프")
	}
	body := dash.Body.String()
	if !strings.Contains(body, "기술 대시보드 보기") {
		t.Fatal("영업 대시보드에 전환 링크 없음")
	}
	if !strings.Contains(body, "손봐야 할 업무") || !strings.Contains(body, "영업배정알림") {
		t.Fatal("영업 홈에서 팝업이 안 뜬다")
	}

	noPerm := jwtCookiePerms(t, model.RoleSales, model.PermStatsView)
	stay := doGetWith(t, e, "/", noPerm)
	if stay.Code != http.StatusOK {
		t.Fatalf("권한 없는 영업 status=%d loc=%s", stay.Code, stay.Header().Get("Location"))
	}

	for _, u := range []*model.User{tech, admin} {
		rec := doGetWith(t, e, "/", jwtCookieUser(t, u))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s GET / status=%d loc=%s", u.Role, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestAssignTransferHoldByKindHTTP(t *testing.T) {
	e, db, choi := newAssignNoticeServer(t)
	users := repository.NewUserRepo(db)
	if err := users.Create(&model.User{
		Username: "lee", PasswordHash: HashPassword("pw"), FullName: "이관대상",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary,
	}); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	today := time.Now().Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('A1','AS-1',?,'C1','증상','received','','O01',?,?)`, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_work_items (work_id, work_number, as_id, work_kind, status, assigned_to, created_at, updated_at)
		VALUES ('W1','AS-1-1','A1','confirm','open','',?,?)`, ts, ts); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('P1',2026,'계획','active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, completed, assignee)
		VALUES ('V1','P1',?,'C1',0,'')`, today); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_other (other_id, work_date, title, phase) VALUES ('G1',?,'기타','receipt')`, today); err != nil {
		t.Fatal(err)
	}
	wb := repository.NewWBRepo(db)
	if err := wb.CreateTask(&model.WorkTask{WorkType: model.WBWorkAdmin, Title: "행정미배정", Status: model.WBTaskWaiting, OrgID: "O01"}); err != nil {
		t.Fatal(err)
	}
	var taskID string
	if err := db.QueryRow(`SELECT task_id FROM work_tasks WHERE title='행정미배정'`).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	sales := repository.NewSalesRepo(db)
	p := &model.SalesProject{Name: "영업건", OrgID: "O01"}
	if err := sales.Create(p); err != nil {
		t.Fatal(err)
	}
	act := &model.SalesActivity{SalesID: p.SalesID, ActivityDate: today, ActivityType: "call", Title: "콜"}
	if err := sales.CreateActivity(act, "테스터", nil); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, choi)
	post := func(path string, form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(ck)
		e.ServeHTTP(rec, req)
		return rec
	}

	assignKeys := []string{"as:A1", "wi:W1", "mnt:V1", "task:" + taskID, "slot:P1|C1|klas", "salesact:" + act.ActivityID, "gen:G1"}
	for _, key := range assignKeys {
		rec := post("/work/assign/one", url.Values{
			"key": {key}, "work_date": {today}, "assignee": {choi.FullName}, "back": {"/work"},
		})
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s assign status=%d loc=%s", key, rec.Code, rec.Header().Get("Location"))
		}
		if strings.Contains(rec.Header().Get("Location"), "err=") {
			t.Fatalf("%s assign err loc=%s", key, rec.Header().Get("Location"))
		}
	}

	tr := post("/work/transfer/one", url.Values{
		"key": {"as:A1"}, "assignee": {"이관대상"}, "work_date": {today}, "back": {"/work"},
	})
	if strings.Contains(tr.Header().Get("Location"), "err=") {
		t.Fatalf("이관 loc=%s", tr.Header().Get("Location"))
	}
	var to string
	_ = db.QueryRow(`SELECT COALESCE(assigned_to,'') FROM as_receipts WHERE as_id='A1'`).Scan(&to)
	if to != "이관대상" {
		t.Fatalf("이관 assigned=%s", to)
	}
	genTr := post("/work/transfer/one", url.Values{
		"key": {"gen:G1"}, "assignee": {"이관대상"}, "work_date": {today}, "back": {"/work"},
	})
	if !strings.Contains(genTr.Header().Get("Location"), "err=") {
		t.Fatalf("gen 이관이 성공 loc=%s", genTr.Header().Get("Location"))
	}

	hold := post("/work/hold/one", url.Values{
		"key": {"wi:W1"}, "hold_reason": {"일정조정"}, "back": {"/work"},
	})
	if strings.Contains(hold.Header().Get("Location"), "err=") {
		t.Fatalf("보류 loc=%s", hold.Header().Get("Location"))
	}
	dismiss := post("/work/notice/dismiss", url.Values{"key": {"mnt:V1"}, "back": {"/work"}})
	if dismiss.Code != http.StatusSeeOther {
		t.Fatalf("제외 status=%d", dismiss.Code)
	}
}
