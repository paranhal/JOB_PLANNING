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

func TestAssignNoticeCloseButtonsInHTML(t *testing.T) {
	e, db, choi := newAssignNoticeServer(t)
	ts := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('UA1','R-UA1',?,'C1','미배정','received','','O01',?,?)`, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	page := doGetWith(t, e, "/work", jwtCookieUser(t, choi))
	body := page.Body.String()
	if !strings.Contains(body, `aria-label="닫기"`) || !strings.Contains(body, ">닫기</button>") {
		t.Fatal("닫기 버튼 없음")
	}
	if !strings.Contains(body, `aria-label="이번엔 제외"`) {
		t.Fatal("제외 버튼 없음")
	}
}

func TestAssignOneKeepsPopupUntilLast(t *testing.T) {
	e, db, choi := newAssignNoticeServer(t)
	ts := time.Now().Format("2006-01-02 15:04:05")
	today := time.Now().Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('UA1','R-UA1',?,'C1','하나','received','','O01',?,?),
		       ('UA2','R-UA2',?,'C1','둘','received','','O01',?,?)`, ts, ts, ts, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, choi)
	first := doGetWith(t, e, "/work", ck)
	if !strings.Contains(first.Body.String(), "R-UA1") || !strings.Contains(first.Body.String(), "R-UA2") {
		t.Fatal("두 건이 팝업에 없다")
	}
	if cookieNamed(first, assignNoticeShownCookie) != nil {
		t.Fatal("첫 표시에 쿠키")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/work/assign/one", strings.NewReader(url.Values{
		"key": {"as:UA1"}, "work_date": {today}, "assignee": {choi.FullName}, "back": {"/work"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("assign status=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "assign=1") {
		t.Fatalf("남은 건인데 팝업 유지 안 함 loc=%s", loc)
	}
	if cookieNamed(rec, assignNoticeShownCookie) != nil {
		t.Fatal("한 건 처리에 오늘 봤음 쿠키")
	}
	var assigned string
	if err := db.QueryRow(`SELECT COALESCE(assigned_to,'') FROM as_receipts WHERE as_id='UA1'`).Scan(&assigned); err != nil {
		t.Fatal(err)
	}
	if assigned == "" {
		t.Fatalf("배정이 DB에 안 남음 loc=%s", loc)
	}
	page := doGetWith(t, e, "/work?assign=1", ck)
	body := page.Body.String()
	if strings.Contains(body, "담당자 미배정 2건") {
		t.Fatal("처리한 건이 미배정에 남았다")
	}
	if !strings.Contains(body, "R-UA2") {
		t.Fatal("나머지 건이 사라졌다")
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://localhost/work/assign/one", strings.NewReader(url.Values{
		"key": {"as:UA2"}, "work_date": {today}, "assignee": {choi.FullName}, "back": {"/work"},
	}.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(ck)
	e.ServeHTTP(rec2, req2)
	loc2 := rec2.Header().Get("Location")
	if strings.Contains(loc2, "assign=1") {
		t.Fatalf("모두 처리했는데 팝업 유지 loc=%s", loc2)
	}
	shown := cookieNamed(rec2, assignNoticeShownCookie)
	if shown == nil {
		t.Fatal("모두 처리 후 쿠키 없음")
	}
	closed := doGetWith(t, e, "/work", ck, shown)
	if strings.Contains(closed.Body.String(), "손봐야 할 업무") && strings.Contains(closed.Body.String(), "담당자 미배정") {
		t.Fatal("모두 처리 후에도 미배정 팝업")
	}
}

func TestAssignOneRequiresAssigneeAndDate(t *testing.T) {
	e, db, choi := newAssignNoticeServer(t)
	ts := time.Now().Format("2006-01-02 15:04:05")
	today := time.Now().Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('UA1','R-UA1',?,'C1','미배정','received','','O01',?,?)`, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, choi)
	missing := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/work/assign/one", strings.NewReader(url.Values{
		"key": {"as:UA1"}, "work_date": {today}, "back": {"/work"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(ck)
	e.ServeHTTP(missing, req)
	if !strings.Contains(missing.Header().Get("Location"), "err=assignee") {
		t.Fatalf("담당자 누락 loc=%s", missing.Header().Get("Location"))
	}
	var to string
	_ = db.QueryRow(`SELECT COALESCE(assigned_to,'') FROM as_receipts WHERE as_id='UA1'`).Scan(&to)
	if to != "" {
		t.Fatalf("DB 변경됨 assigned=%s", to)
	}
	hold := httptest.NewRecorder()
	hreq := httptest.NewRequest(http.MethodPost, "http://localhost/work/hold/one", strings.NewReader(url.Values{
		"key": {"as:UA1"}, "back": {"/work"},
	}.Encode()))
	hreq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	hreq.AddCookie(ck)
	e.ServeHTTP(hold, hreq)
	if !strings.Contains(hold.Header().Get("Location"), "err=hold_reason") {
		t.Fatalf("보류 사유 누락 loc=%s", hold.Header().Get("Location"))
	}
}

func TestHoldAndTransferHTTP(t *testing.T) {
	e, db, choi := newAssignNoticeServer(t)
	users := repository.NewUserRepo(db)
	if err := users.Create(&model.User{
		Username: "lee", PasswordHash: HashPassword("pw"), FullName: "이관대상",
		Role: model.RoleTech, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	today := time.Now().Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('H1','R-H1',?,'C1','보류대상','received','','O01',?,?),
		       ('T1','R-T1',?,'C1','이관대상','received','','O01',?,?)`, ts, ts, ts, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_work_items (work_id, work_number, as_id, work_kind, status, assigned_to, created_at, updated_at)
		VALUES ('W1','R-H1-1','H1','confirm','open','',?,?)`, ts, ts); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, choi)
	hrec := httptest.NewRecorder()
	hreq := httptest.NewRequest(http.MethodPost, "http://localhost/work/hold/one", strings.NewReader(url.Values{
		"key": {"as:H1"}, "hold_reason": {"부품대기"}, "back": {"/work"},
	}.Encode()))
	hreq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	hreq.AddCookie(ck)
	e.ServeHTTP(hrec, hreq)
	var st, reason string
	_ = db.QueryRow(`SELECT status, COALESCE(hold_reason,'') FROM as_receipts WHERE as_id='H1'`).Scan(&st, &reason)
	if st != "hold" || reason != "부품대기" {
		t.Fatalf("hold status=%s reason=%s", st, reason)
	}
	wrec := httptest.NewRecorder()
	wreq := httptest.NewRequest(http.MethodPost, "http://localhost/work/hold/one", strings.NewReader(url.Values{
		"key": {"wi:W1"}, "hold_reason": {"일정"}, "back": {"/work"},
	}.Encode()))
	wreq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wreq.AddCookie(ck)
	e.ServeHTTP(wrec, wreq)

	trec := httptest.NewRecorder()
	treq := httptest.NewRequest(http.MethodPost, "http://localhost/work/transfer/one", strings.NewReader(url.Values{
		"key": {"as:T1"}, "assignee": {"이관대상"}, "work_date": {today}, "back": {"/work"},
	}.Encode()))
	treq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	treq.AddCookie(ck)
	e.ServeHTTP(trec, treq)
	var to string
	_ = db.QueryRow(`SELECT COALESCE(assigned_to,'') FROM as_receipts WHERE as_id='T1'`).Scan(&to)
	if to != "이관대상" {
		t.Fatalf("이관 assigned=%s loc=%s", to, trec.Header().Get("Location"))
	}
}

func TestAssignForbiddenObserverAndForeignOrg(t *testing.T) {
	e, db, choi := newAssignNoticeServer(t)
	users := repository.NewUserRepo(db)
	if err := users.EnsureObserver(HashPassword("pw")); err != nil {
		t.Fatal(err)
	}
	obs, err := users.GetByUsername("obs")
	if err != nil || obs == nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "other", PasswordHash: HashPassword("pw"), FullName: "다른조직",
		Role: model.RoleTech, IsActive: true, OrgID: "O99",
	}); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	today := time.Now().Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('UA1','R-UA1',?,'C1','미배정','received','','O01',?,?)`, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	orec := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "http://localhost/work/assign/one", strings.NewReader(url.Values{
		"key": {"as:UA1"}, "work_date": {today}, "assignee": {choi.FullName},
	}.Encode()))
	oreq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	oreq.AddCookie(jwtCookieUser(t, obs))
	e.ServeHTTP(orec, oreq)
	if orec.Code != http.StatusForbidden {
		t.Fatalf("옵저버 status=%d", orec.Code)
	}

	choi.OrgID = model.OrgIDLibrary
	ck := jwtCookieUser(t, choi)
	frec := httptest.NewRecorder()
	freq := httptest.NewRequest(http.MethodPost, "http://localhost/work/assign/one", strings.NewReader(url.Values{
		"key": {"as:UA1"}, "work_date": {today}, "assignee": {"다른조직"}, "back": {"/work"},
	}.Encode()))
	freq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	freq.AddCookie(ck)
	e.ServeHTTP(frec, freq)
	if frec.Code != http.StatusBadRequest && !strings.Contains(frec.Header().Get("Location"), "err=org") {
		t.Fatalf("타 조직 status=%d loc=%s body=%s", frec.Code, frec.Header().Get("Location"), frec.Body.String())
	}
	var to string
	_ = db.QueryRow(`SELECT COALESCE(assigned_to,'') FROM as_receipts WHERE as_id='UA1'`).Scan(&to)
	if to != "" {
		t.Fatalf("타 조직 지정됨 %s", to)
	}
}

func TestAssignNoticeShownNextDay(t *testing.T) {
	e, db, choi := newAssignNoticeServer(t)
	ts := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('UA1','R-UA1',?,'C1','미배정','received','','O01',?,?)`, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, choi)
	yesterday := &http.Cookie{Name: assignNoticeShownCookie, Value: time.Now().AddDate(0, 0, -1).Format("2006-01-02"), Path: "/"}
	page := doGetWith(t, e, "/work", ck, yesterday)
	if !strings.Contains(page.Body.String(), "담당자 미배정") {
		t.Fatal("다음 날 팝업이 안 뜬다")
	}
	todayCk := &http.Cookie{Name: assignNoticeShownCookie, Value: time.Now().Format("2006-01-02"), Path: "/"}
	hidden := doGetWith(t, e, "/work", ck, todayCk)
	if strings.Contains(hidden.Body.String(), "담당자 미배정") {
		t.Fatal("닫은 날 다시 떴다")
	}
}

func TestSalesDailyCardAndCreate(t *testing.T) {
	e, db, choi := newAssignNoticeServer(t)
	page := doGetWith(t, e, "/workboard/register", jwtCookieUser(t, choi))
	if page.Code != http.StatusOK {
		t.Fatalf("register status=%d", page.Code)
	}
	if !strings.Contains(page.Body.String(), "영업 활동") || !strings.Contains(page.Body.String(), "영업 사업·활동 관련 일일 업무") {
		t.Fatal("영업 활동 카드 없음")
	}
	today := time.Now().Format("2006-01-02")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks", strings.NewReader(url.Values{
		"work_type":  {"sales"},
		"title":      {"영업 방문"},
		"work_date":  {today},
		"due_date":   {today},
		"start_time": {"09:00"},
		"end_time":   {"09:30"},
		"assignee":   {choi.FullName},
		"redirect":   {"/workboard/register"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(jwtCookieUser(t, choi))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	listed := doGetWith(t, e, "/workboard/register", jwtCookieUser(t, choi))
	if !strings.Contains(listed.Body.String(), "영업 방문") {
		t.Fatal("일정표에 영업 방문이 없다")
	}
	home := doGetWith(t, e, "/", jwtCookieUser(t, choi))
	if home.Code != http.StatusOK {
		t.Fatalf("대시보드 status=%d", home.Code)
	}
	if !strings.Contains(home.Body.String(), "영업 방문") {
		t.Fatal("대시보드에 영업 방문이 없다")
	}

	if err := repository.NewWBRepo(db).CreateTask(&model.WorkTask{
		WorkType: model.WBWorkSales, Title: "영업 미계획", Status: model.WBTaskWaiting, OrgID: "O01",
	}); err != nil {
		t.Fatal(err)
	}
	unplanned := doGetWith(t, e, "/plan/unplanned?mine=0", jwtCookieUser(t, choi))
	if unplanned.Code != http.StatusOK {
		t.Fatalf("미계획 status=%d", unplanned.Code)
	}
	if !strings.Contains(unplanned.Body.String(), "영업 미계획") {
		t.Fatal("미계획에 영업 업무가 없다")
	}

	obsE, db, _ := newAssignNoticeServer(t)
	if err := repository.NewUserRepo(db).EnsureObserver(HashPassword("pw")); err != nil {
		t.Fatal(err)
	}
	obs, _ := repository.NewUserRepo(db).GetByUsername("obs")
	obsPage := doGetWith(t, obsE, "/workboard/register", jwtCookieUser(t, obs))
	if strings.Contains(obsPage.Body.String(), "영업 사업·활동 관련 일일 업무") {
		t.Fatal("읽기 전용에 영업 카드가 있다")
	}
}
