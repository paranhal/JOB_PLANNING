package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/auditlog"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func seedWeekly32(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,is_test,start_date,end_date) VALUES
		('WP-P1','미행사업','active','O03',0,'2026-08-01','2026-08-31'),
		('WP-P2','행있음사업','active','O03',0,'2026-01-01','2026-12-31'),
		('WP-O01','도서관사업','active',?,0,'2025-01-01','2025-12-31'),
		('WP-TEST','테스트사업','active','O03',1,'2026-01-01','2026-12-31')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('C32','해미초','해미초',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	tech := &model.User{Username: "si-tech", PasswordHash: HashPassword("password10"), FullName: "김기술",
		Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true}
	lib := &model.User{Username: "lib-tech", PasswordHash: HashPassword("password10"), FullName: "박도서",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{tech, lib} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, complete_datetime, status,
		assigned_to, assigned_user_id, org_id, symptom, created_at, updated_at)
		VALUES ('AS-NP','AS-NP','C32','2026-08-12 10:00:00','2026-08-12 16:00:00','completed',
			'김기술', ?, 'O03', '게이트', datetime('now'), datetime('now'))`, tech.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, complete_datetime, status,
		assigned_to, org_id, symptom, created_at, updated_at)
		VALUES ('AS-O01','AS-O01','C32','2026-08-12 10:00:00','2026-08-12 16:00:00','completed',
			'박도서', ?, '게이트', datetime('now'), datetime('now'))`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date,assignee)
		VALUES ('T-NP32','admin','사업없음행정','waiting',NULL,'O03','','2026-08-12','김기술')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO maintenance_plans (plan_id, plan_year, title, status, org_id)
		VALUES ('MP32', 2026, '계획', 'approved', 'O03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES ('V32','MP32','2026-08-12','C32',0,'normal','김기술','KLAS',1)`); err != nil {
		t.Fatal(err)
	}
}

func postWeeklyLink(t *testing.T, e *echo.Echo, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/tasks/project", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func asProjectID(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var pid string
	if err := db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id=?`, id).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return pid
}

func asAssignee(t *testing.T, db *sql.DB, id string) (string, string) {
	t.Helper()
	var name, uid string
	if err := db.QueryRow(`SELECT COALESCE(assigned_to,''), COALESCE(assigned_user_id,'') FROM as_receipts WHERE as_id=?`, id).Scan(&name, &uid); err != nil {
		t.Fatal(err)
	}
	return name, uid
}

// TestWeeklyLinkASReceiptReproMissingTask §4-1: AS ID를 업무 ID로내면 업무가 없습니다.
func TestWeeklyLinkASReceiptReproMissingTask(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw32-repro.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	seedWeekly32(t, db)

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeekly28(e, h)

	rec := postWeeklyLink(t, e, url.Values{
		"task_id": {"AS-NP"}, "project_id": {"WP-P1"}, "org": {"O03"}, "week": {"2026-08-10"},
	}, jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	t.Logf("95-32 재현 Location=%s", loc)
	if !strings.Contains(loc, "업무가 없습니다") {
		t.Fatalf("AS를 task_id로 내면 업무가 없습니다가 나와야 한다: %s", loc)
	}
	if got := asProjectID(t, db, "AS-NP"); got != "" {
		t.Fatalf("연결되면 안 됨 project_id=%s", got)
	}
}

func userIDByName(t *testing.T, db *sql.DB, username string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT user_id FROM users WHERE username=?`, username).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func grantASUnlock(t *testing.T, db *sql.DB, asID, userID string) {
	t.Helper()
	if _, err := repository.NewASUnlockRepo(db).Grant(asID, userID); err != nil {
		t.Fatal(err)
	}
}

func TestWeeklyLinkKindsAssigneeIsolateAndPopup(t *testing.T) {
	accessPath := initTestAccessDB(t)
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw32-kind.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	seedWeekly32(t, db)
	techID := userIDByName(t, db, "si-tech")
	libID := userIDByName(t, db, "lib-tech")

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeekly28(e, h)
	ck := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	adminUID := "org_admin-org-id"

	thisMon := parseWeekMonday("", time.Now())
	getWeek := thisMon.AddDate(0, 0, -7)
	prevDone := getWeek.AddDate(0, 0, 2).Format("2006-01-02") + " 12:00:00"
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, complete_datetime, status,
		assigned_to, assigned_user_id, org_id, symptom, created_at, updated_at)
		VALUES ('AS-NOW','AS-NOW','C32',?,?, 'completed', '김기술', ?, 'O03', '게이트', datetime('now'), datetime('now'))`,
		prevDone, prevDone, techID); err != nil {
		t.Fatal(err)
	}

	get := httptest.NewRecorder()
	greq := httptest.NewRequest(http.MethodGet, "/stats/company-weekly?week="+getWeek.Format("2006-01-02")+"&org=O03", nil)
	greq.AddCookie(ck)
	e.ServeHTTP(get, greq)
	if get.Code != 200 {
		t.Fatalf("화면 %d", get.Code)
	}
	body := get.Body.String()
	if !strings.Contains(body, `id="weekly-link-modal"`) || !strings.Contains(body, `role="dialog"`) {
		t.Fatalf("팝업 마크업 없음: %s", body[0:min(800, len(body))])
	}
	if !strings.Contains(body, "data-project-filter") || !strings.Contains(body, `name="project_id"`) {
		t.Fatal("사업 검색·라디오 없음")
	}
	if !strings.Contains(body, "변경 안 함") || !strings.Contains(body, "data-assignee-radio") {
		t.Fatal("담당자 영역 없음")
	}
	if !strings.Contains(body, "타 조직 사업에 연결합니다") {
		t.Fatal("타 조직 확인 없음")
	}
	if !strings.Contains(body, `data-start="2026-08-01"`) || !strings.Contains(body, `data-end="2026-12-31"`) {
		t.Fatal("사업 기간 속성 없음")
	}
	if !strings.Contains(body, "기간 일치") || !strings.Contains(body, "기간 종료") {
		t.Fatal("기간 배지 없음")
	}
	if strings.Contains(body, "테스트사업") || strings.Contains(body, "WP-TEST") {
		t.Fatal("테스트 사업이 목록에 있다")
	}
	if strings.Contains(body, `<select name="project_id"`) {
		t.Fatal("인라인 사업 select 가 남아 있다")
	}
	if !strings.Contains(body, "/static/js/weekly-link.js") {
		t.Fatal("팝업 스크립트 없음")
	}
	if !strings.Contains(body, "items-end md:items-center") {
		t.Fatal("모바일 시트 마크업 없음")
	}
	js := mustRead(t, filepath.Join(findStaticRoot(t), "js", "weekly-link.js"))
	jsb := string(js)
	if !strings.Contains(jsb, "Escape") || !strings.Contains(jsb, "Tab") {
		t.Fatal("Esc·포커스 트랩 없음")
	}
	if !strings.Contains(jsb, "applyPeriod") || !strings.Contains(jsb, "inPeriod") {
		t.Fatal("기간 일치 정렬 없음")
	}

	locked := postWeeklyLink(t, e, url.Values{
		"kind": {"as_receipt"}, "id": {"AS-NP"}, "project_id": {"WP-P1"},
		"org": {"O03"}, "week": {"2026-08-17"}, "assignee_keep": {"1"},
	}, ck)
	locLocked, _ := url.QueryUnescape(locked.Header().Get("Location"))
	if locked.Code != http.StatusSeeOther || !strings.Contains(locLocked, "잠금 해제") {
		t.Fatalf("완료 AS 잠금 %d %s", locked.Code, locLocked)
	}
	if asProjectID(t, db, "AS-NP") != "" {
		t.Fatal("잠금 중 연결되면 안 됨")
	}

	grantASUnlock(t, db, "AS-NP", adminUID)
	asOK := postWeeklyLink(t, e, url.Values{
		"kind": {"as_receipt"}, "id": {"AS-NP"}, "project_id": {"WP-P1"},
		"org": {"O03"}, "week": {"2026-08-17"}, "assignee_keep": {"1"},
	}, ck)
	locAS, _ := url.QueryUnescape(asOK.Header().Get("Location"))
	if asOK.Code != http.StatusSeeOther || !strings.Contains(locAS, "연결했습니다") {
		t.Fatalf("AS 연결 %d %s", asOK.Code, locAS)
	}
	if asProjectID(t, db, "AS-NP") != "WP-P1" {
		t.Fatalf("AS project %s", asProjectID(t, db, "AS-NP"))
	}
	name, uid := asAssignee(t, db, "AS-NP")
	if name != "김기술" || uid != techID {
		t.Fatalf("변경 안 함 깨짐 %s %s", name, uid)
	}
	var hist int
	if err := db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='as_receipts' AND entity_id='AS-NP'`).Scan(&hist); err != nil || hist < 1 {
		t.Fatalf("AS 이력 %d %v", hist, err)
	}
	var procN int
	if err := db.QueryRow(`SELECT COUNT(*) FROM as_processes WHERE as_id='AS-NP' AND work_content LIKE '사업 연결%'`).Scan(&procN); err != nil || procN < 1 {
		t.Fatalf("AS 조치 기록 %d %v", procN, err)
	}

	taskOK := postWeeklyLink(t, e, url.Values{
		"kind": {"work_task"}, "id": {"T-NP32"}, "project_id": {"WP-P1"},
		"org": {"O03"}, "week": {"2026-08-17"}, "assignee_keep": {"1"},
	}, ck)
	if taskOK.Code != http.StatusSeeOther {
		t.Fatalf("업무 %d", taskOK.Code)
	}
	tk, _ := repository.NewWBRepo(db).GetTask("T-NP32")
	if tk == nil || tk.ProjectID != "WP-P1" {
		t.Fatalf("업무 연결 %+v", tk)
	}

	mnt := postWeeklyLink(t, e, url.Values{
		"item": {"maintenance_visit:V32"}, "project_id": {"WP-P1"},
		"org": {"O03"}, "week": {"2026-08-17"},
	}, ck)
	locM, _ := url.QueryUnescape(mnt.Header().Get("Location"))
	if mnt.Code != http.StatusSeeOther || !strings.Contains(locM, "연결했습니다") {
		t.Fatalf("점검 연결 %d %s", mnt.Code, locM)
	}
	var vpid2, vdate, vcust, vasg string
	var vdone int
	if err := db.QueryRow(`SELECT COALESCE(project_id,''), visit_date, customer_id, COALESCE(assignee,''), COALESCE(completed,0)
		FROM maintenance_visits WHERE visit_id='V32'`).Scan(&vpid2, &vdate, &vcust, &vasg, &vdone); err != nil {
		t.Fatal(err)
	}
	if vpid2 != "WP-P1" {
		t.Fatalf("점검 사업 %s", vpid2)
	}
	if vdate != "2026-08-12" || vcust != "C32" || vasg != "김기술" || vdone != 1 {
		t.Fatalf("점검 다른 칸 변경 date=%s cust=%s asg=%s done=%d", vdate, vcust, vasg, vdone)
	}

	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, status, assigned_to, assigned_user_id, org_id, symptom, created_at, updated_at)
		VALUES ('AS-OPEN','AS-OPEN','C32','2026-08-12 10:00:00','in_progress','김기술',?,'O03','게이트',datetime('now'),datetime('now'))`, techID); err != nil {
		t.Fatal(err)
	}
	asg := postWeeklyLink(t, e, url.Values{
		"kind": {"as_receipt"}, "id": {"AS-OPEN"}, "project_id": {"WP-P2"},
		"org": {"O03"}, "primary_user_id": {techID},
	}, ck)
	if asg.Code != http.StatusSeeOther {
		t.Fatalf("담당자 변경 %d", asg.Code)
	}
	n2, u2 := asAssignee(t, db, "AS-OPEN")
	if n2 != "김기술" || u2 != techID {
		t.Fatalf("주 담당 %s %s", n2, u2)
	}
	if asProjectID(t, db, "AS-OPEN") != "WP-P2" {
		t.Fatal("진행 AS 사업")
	}

	foreignUser := postWeeklyLink(t, e, url.Values{
		"kind": {"work_task"}, "id": {"T-NP32"}, "project_id": {"WP-P1"},
		"org": {"O03"}, "primary_user_id": {libID},
	}, ck)
	if foreignUser.Code != http.StatusBadRequest {
		t.Fatalf("타 조직 담당 want 400 got %d %s", foreignUser.Code, foreignUser.Body.String())
	}

	noConfirm := postWeeklyLink(t, e, url.Values{
		"kind": {"work_task"}, "id": {"T-NP32"}, "project_id": {"WP-O01"}, "org": {"O03"},
	}, ck)
	if noConfirm.Code != http.StatusBadRequest && noConfirm.Code != http.StatusForbidden {
		t.Fatalf("confirm_foreign want 400/403 got %d", noConfirm.Code)
	}

	other := postWeeklyLink(t, e, url.Values{
		"kind": {"as_receipt"}, "id": {"AS-O01"}, "project_id": {"WP-P1"}, "org": {"O03"},
	}, ck)
	if other.Code != http.StatusNotFound {
		t.Fatalf("타 조직 AS want 404 got %d", other.Code)
	}

	cl := postWeeklyLink(t, e, url.Values{
		"kind": {"as_receipt"}, "id": {"AS-OPEN"}, "project_id": {"WP-P1"}, "org": {"O03"},
	}, jwtCookieRole(t, model.RoleCLevel))
	if cl.Code != http.StatusForbidden {
		t.Fatalf("C-Level %d", cl.Code)
	}
	ro := postWeeklyLink(t, e, url.Values{
		"kind": {"as_receipt"}, "id": {"AS-OPEN"}, "project_id": {"WP-P1"}, "org": {"O03"},
	}, jwtCookieReadOnly(t, model.RoleOrgAdmin))
	if ro.Code != http.StatusForbidden {
		t.Fatalf("읽기전용 %d", ro.Code)
	}

	ids := make([]string, 201)
	for i := range ids {
		ids[i] = "as_receipt:X"
	}
	over := httptest.NewRecorder()
	ov := url.Values{"project_id": {"WP-P1"}, "org": {"O03"}}
	ov["item"] = ids
	ovreq := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/tasks/project", strings.NewReader(ov.Encode()))
	ovreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	ovreq.AddCookie(ck)
	e.ServeHTTP(over, ovreq)
	if over.Code != http.StatusBadRequest {
		t.Fatalf("상한 %d", over.Code)
	}

	mix := postWeeklyLink(t, e, url.Values{
		"item": {"work_task:T-NP32", "as_receipt:AS-OPEN", "maintenance_visit:V32"},
		"project_id": {"WP-P2"}, "org": {"O03"}, "week": {"2026-08-17"}, "assignee_keep": {"1"},
	}, ck)
	locMix, _ := url.QueryUnescape(mix.Header().Get("Location"))
	if mix.Code != http.StatusSeeOther {
		t.Fatalf("혼합 %d", mix.Code)
	}
	if !strings.Contains(locMix, "need_row") && !strings.Contains(locMix, "WP-P2") {
		if !strings.Contains(locMix, "연결했습니다") {
			t.Fatalf("혼합 loc %s", locMix)
		}
	}
	var vmix string
	_ = db.QueryRow(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='V32'`).Scan(&vmix)
	if vmix != "WP-P2" {
		t.Fatalf("혼합 점검 사업 %s", vmix)
	}

	logs := readAccessLogs(t, accessPath)
	var saw bool
	for _, l := range logs {
		if l.Action == auditlog.ActionUpdate && (strings.Contains(l.Target, "as_receipts") || strings.Contains(l.Reason, "주간보고 사업 연결")) {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("감사 로그 없음 %+v", logs)
	}
}

func TestWeeklyUnassignedInlineSelectRemoved(t *testing.T) {
	root := findTemplateRoot(t)
	b := string(mustRead(t, filepath.Join(root, "stats/_weekly_unassigned.html")))
	if strings.Contains(b, `<select name="project_id"`) {
		t.Fatal("인라인 select")
	}
	if !strings.Contains(b, `{{template "weekly_assignee_field"`) {
		t.Fatal("담당자 부분 템플릿 없음")
	}
	if !strings.Contains(b, "data-start") || !strings.Contains(b, "data-period-match") {
		t.Fatal("기간 표시 속성 없음")
	}
	got := countUILint(b)
	if !got.zero() {
		t.Fatalf("ui_lint %+v", got)
	}
}
