package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-221: 정기점검 일일업무의 담당이 이동·편집·겹침 처리로 비워진 채 고착되지 않게 한다.

type env221 struct {
	t     *testing.T
	e     *echo.Echo
	wb    *repository.WBRepo
	users *repository.UserRepo
	uid   string
	vid   string
	tid   string
	oa    *http.Cookie
	exec  func(q string, a ...interface{})
	task  func() (assignee, uid, source, workDate string)
}

func setup221(t *testing.T) *env221 {
	e, users, wb := newAssigneeOrgServer(t)
	db := wb.DB()
	h2 := New(db)
	e.GET("/workboard/tasks/:id/edit", h2.Workboard.EditTask, h2.Auth.AuthMiddleware)
	e.POST("/workboard/schedule", h2.Workboard.Schedule, h2.Auth.AuthMiddleware)
	e.GET("/workboard/register/day", h2.Workboard.Register, h2.Auth.AuthMiddleware)
	_, _ = db.Exec(`UPDATE users SET profile_done=1, mobile='010-0000-0000', email='t@example.test'`)
	u := &model.User{Username: "yang", PasswordHash: HashPassword("password10"), FullName: "양기헌", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	ex := func(q string, a ...interface{}) {
		t.Helper()
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	ex(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('c1','한국기술교육대학교다산정보관','한국기술교육대학교다산정보관',1,?)`, model.OrgIDLibrary)
	ex(`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id) VALUES ('MPX',2026,'p','approved',?)`, model.OrgIDLibrary)
	vid, err := repository.NewMaintenanceRepo(db).InsertVisitFullID(model.MaintenanceVisit{PlanID: "MPX", VisitDate: "2026-11-20", CustomerID: "c1", ProductType: "KLAS", Assignee: "양기헌", SortOrder: 1})
	if err != nil {
		t.Fatal(err)
	}
	tid, err := wb.EnsureMaintenanceTaskForVisit(vid)
	if err != nil || tid == "" {
		t.Fatalf("일일업무 %v %q", err, tid)
	}
	ex(`UPDATE work_tasks SET work_date='2026-11-19', due_date='2026-11-20', start_time='09:00', end_time='10:00', duration_min=60, assignee_user_id=?, org_id=? WHERE task_id=?`, u.UserID, model.OrgIDLibrary, tid)
	v := &env221{t: t, e: e, wb: wb, users: users, uid: u.UserID, vid: vid, tid: tid, oa: jwtCookieUser(t, mustUserByName(t, users, "oa01")), exec: ex}
	v.task = func() (string, string, string, string) {
		var a, au, src, wd string
		_ = db.QueryRow(`SELECT COALESCE(assignee,''), COALESCE(assignee_user_id,''), COALESCE(assignee_source,''), COALESCE(work_date,'') FROM work_tasks WHERE task_id=?`, tid).Scan(&a, &au, &src, &wd)
		return a, au, src, wd
	}
	return v
}

func (v *env221) post(path string, f url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(f.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(v.oa)
	rec := httptest.NewRecorder()
	v.e.ServeHTTP(rec, req)
	return rec
}

func (v *env221) updateForm(extra url.Values) url.Values {
	f := url.Values{"scope": {"register"}, "title": {"[점검]한국기술교육대학교다산정보관"}, "work_type": {"maintenance"},
		"work_date": {"2026-11-20"}, "due_date": {"2026-11-20"}, "start_time": {"09:00"}, "end_time": {"10:00"}, "assignee_multi": {"1"}}
	for k, vv := range extra {
		f[k] = vv
	}
	return f
}

// 1) 날짜만 바꿔 저장(선택기 값이 비어 올라와도) → 담당 유지
func TestMntTaskMoveKeepsAssignee95_221(t *testing.T) {
	v := setup221(t)
	rec := v.post("/workboard/tasks/"+v.tid+"/update", v.updateForm(url.Values{"delay_reason": {"일정 이동"}}))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", rec.Code, rec.Body.String())
	}
	a, _, src, wd := v.task()
	if a != "양기헌" || wd != "2026-11-20" {
		t.Errorf("담당=%q 날짜=%s (담당 유지·날짜 이동)", a, wd)
	}
	if src == model.WBAssigneeSourceManual {
		t.Errorf("같은 담당이면 수동으로 바뀌지 않아야 한다 source=%q", src)
	}
}

// 2) 같은 사람의 담당 ID만 채워지는 것은 수동 전환이 아니다 / 다른 사람으로 바뀌면 수동
func TestMntTaskAssigneeIDFillIsNotManual95_221(t *testing.T) {
	v := setup221(t)
	v.exec(`UPDATE work_tasks SET assignee_user_id=NULL WHERE task_id=?`, v.tid)
	if rec := v.post("/workboard/tasks/"+v.tid+"/update", v.updateForm(url.Values{"owner_user_id": {v.uid}, "assignee_user_id": {v.uid}, "delay_reason": {"x"}})); rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d", rec.Code)
	}
	a, uid, src, _ := v.task()
	if a != "양기헌" || uid != v.uid {
		t.Errorf("ID 채움 a=%q uid=%q", a, uid)
	}
	if src == model.WBAssigneeSourceManual {
		t.Errorf("이름·사람이 같으면 수동 전환이 아니다 source=%q", src)
	}
	// 다른 사람으로 바꾸면 수동
	lib := mustUserByName(t, v.users, "libA")
	if rec := v.post("/workboard/tasks/"+v.tid+"/update", v.updateForm(url.Values{"owner_user_id": {lib.UserID}, "assignee_user_id": {lib.UserID}, "delay_reason": {"x"}})); rec.Code != http.StatusSeeOther {
		t.Fatalf("담당 변경 저장 %d %s", rec.Code, rec.Body.String())
	}
	if _, _, src2, _ := v.task(); src2 != model.WBAssigneeSourceManual {
		t.Errorf("다른 사람으로 바꾸면 수동이어야 한다 source=%q", src2)
	}
}

// 3) 명시적 해제는 비워지고 수동
func TestMntTaskExplicitClear95_221(t *testing.T) {
	v := setup221(t)
	admin := jwtCookieUser(t, mustUserByName(t, v.users, "vs1")) // 담당 필수가 아닌 계정(비젼관리자)
	f := v.updateForm(url.Values{"assignee_cleared": {"1"}, "delay_reason": {"x"}})
	req := httptest.NewRequest(http.MethodPost, "/workboard/tasks/"+v.tid+"/update", strings.NewReader(f.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(admin)
	rec := httptest.NewRecorder()
	v.e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("해제 저장 %d %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); strings.Contains(loc, "err=") {
		t.Logf("해제 저장 loc=%s", loc)
	}
	a, _, src, _ := v.task()
	if a != "" || src != model.WBAssigneeSourceManual {
		t.Errorf("명시 해제: 담당=%q source=%q (비움+수동이어야 한다)", a, src)
	}
}

// 4) 담당 빈 값 + manual + 방문 담당 있음 → 동기화 뒤 방문 담당으로 채워지고 방문을 다시 따라간다
func TestMntTaskBlankManualSelfHeals95_221(t *testing.T) {
	v := setup221(t)
	v.exec(`UPDATE work_tasks SET assignee='', assignee_user_id=NULL, assignee_source='manual' WHERE task_id=?`, v.tid)
	if _, err := v.wb.EnsureMaintenanceTaskForVisit(v.vid); err != nil { // 보드를 열 때 도는 동기화와 같다
		t.Fatal(err)
	}
	doGetWith(t, v.e, "/workboard/register?view=day&date=2026-11-19", v.oa)
	a, uid, src, _ := v.task()
	if a != "양기헌" || uid == "" {
		t.Fatalf("자가복구 담당=%q uid=%q", a, uid)
	}
	if src == model.WBAssigneeSourceManual {
		t.Errorf("복구 뒤에는 방문 담당을 다시 따라가야 한다 source=%q", src)
	}
	// 담당이 있는 수동 값은 종전대로 유지(방문 담당이 달라도 덮지 않는다)
	v.exec(`UPDATE work_tasks SET assignee='도서관담당A', assignee_source='manual' WHERE task_id=?`, v.tid)
	doGetWith(t, v.e, "/workboard/register?view=day&date=2026-11-19", v.oa)
	if a2, _, _, _ := v.task(); a2 != "도서관담당A" {
		t.Errorf("담당이 있는 수동 값은 유지 a=%q", a2)
	}
}

// 5) 복구되면 「미배정」 목록에서 빠지고 담당자 업무로 잡힌다
func TestMntTaskHealedLeavesUnassigned95_221(t *testing.T) {
	v := setup221(t)
	wbr := repository.NewWorkBoardRepo(repositoryDB(v.wb))
	inUnassigned := func() bool {
		items, err := wbr.CollectUnassigned(model.OrgIDLibrary)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			if strings.Contains(it.Title, "한국기술교육대학교") {
				return true
			}
		}
		return false
	}
	v.exec(`UPDATE work_tasks SET assignee='', assignee_user_id=NULL, assignee_source='manual' WHERE task_id=?`, v.tid)
	v.exec(`UPDATE maintenance_visits SET assignee='' WHERE visit_id=?`, v.vid) // 방문 담당이 없으면 복구할 값도 없다
	if !inUnassigned() {
		t.Skip("미배정 목록 판정 기준이 이 조건과 다르다") // 시험 전제 확인
	}
	v.exec(`UPDATE maintenance_visits SET assignee='양기헌' WHERE visit_id=?`, v.vid)
	_ = doGetWith(t, v.e, "/workboard/register?view=day&date=2026-11-19", v.oa)
	if a, _, _, _ := v.task(); a != "양기헌" {
		t.Fatalf("복구 안 됨 a=%q", a)
	}
	if inUnassigned() {
		t.Error("복구된 업무가 미배정 목록에 남아 있다")
	}
}

// 6) 겹침 처리 「담당자만 제외」: 정기점검 일일업무는 담당이 비워지지 않는다(보조 담당이 없으면 막고 이유를 알린다)
func TestMntTaskOverlapLeaveKeepsOwner95_221(t *testing.T) {
	v := setup221(t)
	if err := v.wb.CreateTask(&model.WorkTask{WorkType: model.WBWorkAdmin, Title: "다른 업무", Assignee: "양기헌", AssigneeUserID: v.uid, OrgID: model.OrgIDLibrary,
		Status: model.WBTaskWaiting, WorkDate: "2026-11-20", DueDate: "2026-11-20", StartTime: "13:00", EndTime: "14:00", DurationMin: 60}); err != nil {
		t.Fatal(err)
	}
	var otherID string
	_ = repositoryDB(v.wb).QueryRow(`SELECT task_id FROM work_tasks WHERE title='다른 업무'`).Scan(&otherID)
	f := url.Values{"kind": {"task"}, "ref_id": {otherID}, "work_date": {"2026-11-19"}, "start_time": {"09:30"}, "end_time": {"10:30"}, "assignee": {"양기헌"}, "overlap_action": {"leave"}}
	rec := v.post("/workboard/schedule", f)
	if a, _, _, _ := v.task(); a != "양기헌" {
		t.Fatalf("겹침 처리로 정기점검 담당이 비워졌다 a=%q code=%d", a, rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), url.QueryEscape("뺄 수 없습니다")) && !strings.Contains(rec.Body.String(), "뺄 수 없습니다") {
		t.Errorf("막힌 이유를 알려야 한다 loc=%s body=%s", rec.Header().Get("Location"), rec.Body.String())
	}
	// 일반 업무끼리의 「담당자만 제외」는 종전대로 동작한다
	if err := v.wb.CreateTask(&model.WorkTask{WorkType: model.WBWorkAdmin, Title: "일반 겹침 대상", Assignee: "양기헌", AssigneeUserID: v.uid, OrgID: model.OrgIDLibrary,
		Status: model.WBTaskWaiting, WorkDate: "2026-11-21", DueDate: "2026-11-21", StartTime: "09:00", EndTime: "10:00", DurationMin: 60}); err != nil {
		t.Fatal(err)
	}
	var plainID string
	_ = repositoryDB(v.wb).QueryRow(`SELECT task_id FROM work_tasks WHERE title='일반 겹침 대상'`).Scan(&plainID)
	f2 := url.Values{"kind": {"task"}, "ref_id": {otherID}, "work_date": {"2026-11-21"}, "start_time": {"09:30"}, "end_time": {"10:30"}, "assignee": {"양기헌"}, "overlap_action": {"leave"}}
	v.post("/workboard/schedule", f2)
	var pa string
	_ = repositoryDB(v.wb).QueryRow(`SELECT COALESCE(assignee,'') FROM work_tasks WHERE task_id=?`, plainID).Scan(&pa)
	if pa != "" {
		t.Errorf("일반 업무는 종전대로 담당이 빠져야 한다 a=%q", pa)
	}
}

func repositoryDB(wb *repository.WBRepo) *sql.DB { return wb.DB() }

// 7) 95-215 규칙: 조직 밖 담당이 저장된 업무를 열고(선택칸에 현재 담당이 보임) 확인을 눌러도 담당이 유지된다
func TestMntTaskForeignAssigneeKeptOnConfirm95_221(t *testing.T) {
	v := setup221(t)
	si := mustUserByName(t, v.users, "u03") // O03(SI사업팀) 직원
	v.exec(`UPDATE work_tasks SET assignee=?, assignee_user_id=?, assignee_source='manual' WHERE task_id=?`, si.FullName, si.UserID, v.tid)
	body := doGetWith(t, v.e, "/workboard/tasks/"+v.tid+"/edit", v.oa).Body.String()
	if !strings.Contains(body, si.FullName) {
		t.Error("조직 밖 담당이라도 편집 화면 선택칸에 현재 담당 이름이 있어야 한다(빈 칸으로 열리지 않음)")
	}
	rec := v.post("/workboard/tasks/"+v.tid+"/update", v.updateForm(url.Values{"owner_user_id": {si.UserID}, "assignee_user_id": {si.UserID}, "delay_reason": {"x"}}))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("확인(저장) %d %s", rec.Code, rec.Body.String())
	}
	if a, _, _, _ := v.task(); a != si.FullName {
		t.Errorf("조직 밖 담당이 확인 뒤에도 유지돼야 한다 a=%q", a)
	}
}
