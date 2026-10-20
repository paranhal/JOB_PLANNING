package handler

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type meeting943Users struct {
	oa01, oa02, tech, sales, support, o02tech, clevel, vis, obs *model.User
}

func newMeeting943Server(t *testing.T) (*echo.Echo, *sql.DB, meeting943Users) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "meeting94_3.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	visSeed := &model.User{Username: "vs1", PasswordHash: "x", FullName: "비젼관리자", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	if err := users.Create(visSeed); err != nil {
		t.Fatal(err)
	}
	seed := []*model.User{
		{Username: "oa01", PasswordHash: "x", FullName: "도서관관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary},
		{Username: "oa02", PasswordHash: "x", FullName: "미디어관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02"},
		{Username: "t01", PasswordHash: "x", FullName: "도서관기술", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary},
		{Username: "s01", PasswordHash: "x", FullName: "도서관영업", Role: model.RoleSales, IsActive: true, OrgID: model.OrgIDLibrary},
		{Username: "p01", PasswordHash: "x", FullName: "도서관지원", Role: model.RoleSupport, IsActive: true, OrgID: model.OrgIDLibrary},
		{Username: "t02", PasswordHash: "x", FullName: "미디어기술", Role: model.RoleTech, IsActive: true, OrgID: "O02"},
		{Username: "cl1", PasswordHash: "x", FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true},
		{Username: "ob1", PasswordHash: "x", FullName: "옵저버", Role: model.RoleObserver, IsActive: true, IsReadOnly: true, OrgID: model.OrgIDLibrary},
	}
	for _, u := range seed {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c1','회의도서관','회의도서관',1,?),('c2','미디어도서관','미디어도서관',1,'O02')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	day := "2026-10-21"
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, symptom, updated_at, org_id
	) VALUES
		('as-t','R2610-T01','c1','2026-10-05',?,1,'in_progress','도서관기술','기술건',?,?),
		('as-s','R2610-S01','c1','2026-10-05',?,1,'in_progress','도서관영업','영업건',?,?),
		('as-p','R2610-P01','c1','2026-10-05',?,1,'in_progress','도서관지원','지원건',?,?),
		('as-u','R2610-U01','c1','2026-10-05',?,1,'in_progress','','미배정건',?,?),
		('as-x','R2610-X01','c1','2026-10-05',?,1,'in_progress','외부김','미등록건',?,?),
		('as-m','R2610-M01','c2','2026-10-05',?,1,'in_progress','미디어기술','미디어건',?,'O02')`,
		day, day+" 09:00:00", model.OrgIDLibrary,
		day, day+" 09:00:00", model.OrgIDLibrary,
		day, day+" 09:00:00", model.OrgIDLibrary,
		day, day+" 09:00:00", model.OrgIDLibrary,
		day, day+" 09:00:00", model.OrgIDLibrary,
		day, day+" 09:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, work_date, due_date, status, assignee, org_id)
		VALUES ('wt1','admin','행정업무',?,?, 'waiting','도서관기술',?)`, day, day, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.InjectViewAs)
	g.GET("/meeting", h.Meeting.Show)
	g.GET("/meeting/weekly", h.Meeting.Weekly)
	g.GET("/meeting/weekly/me", h.Meeting.WeeklyMe)
	g.GET("/meeting/weekly/me.xlsx", h.Meeting.WeeklyMeXLSX)
	g.GET("/meeting/weekly/item", h.Meeting.WeeklyItemForm)
	g.GET("/stats/weekly-report.xlsx", h.Stats.ExportWeeklyReport)
	out := meeting943Users{
		oa01: seed[0], oa02: seed[1], tech: seed[2], sales: seed[3], support: seed[4],
		o02tech: seed[5], clevel: seed[6], vis: visSeed, obs: seed[7],
	}
	return e, db, out
}

func TestMeetingTeamRole(t *testing.T) {
	if !meetingTeamRole(model.RoleOrgAdmin) || !meetingTeamRole(model.RoleVisionAdmin) || !meetingTeamRole(model.RoleCLevel) {
		t.Fatal("관리 등급")
	}
	if meetingTeamRole(model.RoleTech) || meetingTeamRole(model.RoleSales) || meetingTeamRole(model.RoleObserver) {
		t.Fatal("조직원")
	}
}

func TestMeeting943MemberLockedToSelf(t *testing.T) {
	e, _, u := newMeeting943Server(t)
	ck := jwtCookieUserClaims(t, u.tech)
	wk := weeklyGet(t, e, "/meeting/weekly?date=2026-10-19&assignee=all", ck)
	if wk.Code != http.StatusFound || !strings.HasPrefix(wk.Header().Get("Location"), "/meeting/weekly/me") {
		t.Fatalf("weekly %d loc=%s", wk.Code, wk.Header().Get("Location"))
	}
	me := weeklyGet(t, e, "/meeting/weekly/me?date=2026-10-19&assignee="+url.QueryEscape("도서관영업"), ck)
	if me.Code != http.StatusOK {
		t.Fatalf("me %d", me.Code)
	}
	mb := me.Body.String()
	if strings.Contains(mb, "R2610-S01") || strings.Contains(mb, "R2610-M01") {
		t.Fatal("내 자료에 타인")
	}
	if !strings.Contains(mb, "R2610-T01") {
		t.Fatal("본인 건 없음")
	}
	if strings.Contains(mb, `id="meeting-weekly-assignee"`) || strings.Contains(mb, `href="/meeting/weekly?`) {
		t.Fatal("조직원에게 팀 자료 UI")
	}
	day := weeklyGet(t, e, "/meeting?date=2026-10-21&assignee="+url.QueryEscape("도서관영업")+"&mine=0", ck)
	if day.Code != http.StatusOK {
		t.Fatalf("daily %d", day.Code)
	}
	db := day.Body.String()
	if strings.Contains(db, `id="meeting-assignee"`) {
		t.Fatal("조직원 드롭다운")
	}
	if !strings.Contains(db, `id="meeting-assignee-self"`) || !strings.Contains(db, "도서관기술") {
		t.Fatal("본인 고정 표시")
	}
	if strings.Contains(db, "R2610-S01") || strings.Contains(db, "R2610-M01") {
		t.Fatal("일일에 타인")
	}
	if !strings.Contains(db, "R2610-T01") {
		t.Fatal("일일 본인 없음")
	}
	xl := weeklyGet(t, e, "/stats/weekly-report.xlsx?period=week&date=2026-10-19", ck)
	if xl.Code != http.StatusForbidden {
		t.Fatalf("team xlsx %d", xl.Code)
	}
}

func TestMeeting943OrgAdminTeamPickerAndColumn(t *testing.T) {
	e, _, u := newMeeting943Server(t)
	ck := jwtCookieUserClaims(t, u.oa01)
	rec := weeklyGet(t, e, "/meeting/weekly?date=2026-10-19&assignee=all", ck)
	if rec.Code != http.StatusOK {
		t.Fatalf("weekly %d %s", rec.Code, rec.Body.String()[:minInt(300, rec.Body.Len())])
	}
	body := rec.Body.String()
	for _, name := range []string{"도서관기술", "도서관영업", "도서관지원", "도서관관리자", "외부김"} {
		if !strings.Contains(body, name) {
			t.Fatalf("드롭다운/목록에 %s 없음", name)
		}
	}
	if strings.Contains(body, "R2610-M01") || strings.Contains(body, `value="미디어기술"`) {
		t.Fatal("타 조직 업무/담당자")
	}
	if !strings.Contains(body, "R2610-T01") || !strings.Contains(body, "R2610-S01") {
		t.Fatal("팀 전체 업무")
	}
	if !strings.Contains(body, "(미등록)") {
		t.Fatal("미등록 표시")
	}
	if !strings.Contains(body, `class="meeting-assignee-cell`) {
		t.Fatal("담당자 열 없음")
	}
	if !strings.Contains(body, "미배정") {
		t.Fatal("미배정 표시")
	}
	one := weeklyGet(t, e, "/meeting/weekly?date=2026-10-19&assignee="+url.QueryEscape("도서관기술"), ck)
	ob := one.Body.String()
	if strings.Contains(ob, "R2610-S01") {
		t.Fatal("한 사람 선택에 타인")
	}
	if !strings.Contains(ob, "R2610-T01") {
		t.Fatal("선택 담당 없음")
	}
	if !strings.Contains(ob, "도서관기술 님 자료") {
		t.Fatal("범위 문구")
	}

	day := weeklyGet(t, e, "/meeting?date=2026-10-21", ck)
	if day.Code != http.StatusOK {
		t.Fatalf("daily %d", day.Code)
	}
	dbody := day.Body.String()
	if strings.Count(dbody, "<th") > 0 && !strings.Contains(dbody, ">담당자</th>") {
		t.Fatal("일일 담당자 헤더")
	}
	if strings.Contains(dbody, "R2610-M01") {
		t.Fatal("일일 타 조직")
	}
	if !strings.Contains(dbody, "R2610-U01") || !strings.Contains(dbody, "미배정") {
		t.Fatal("미배정 행")
	}
}

func TestMeeting943VisionOrgPickAndCLevelNoEdit(t *testing.T) {
	e, _, u := newMeeting943Server(t)
	vis := weeklyGet(t, e, "/meeting/weekly?date=2026-10-19&view_org_id=*", jwtCookieUserClaims(t, u.vis))
	if vis.Code != http.StatusOK {
		t.Fatalf("vis %d", vis.Code)
	}
	vb := vis.Body.String()
	if !strings.Contains(vb, `id="meeting-weekly-view-org"`) {
		t.Fatal("조직 선택")
	}
	if !strings.Contains(vb, "도서관기술 (도서관사업팀)") && !strings.Contains(vb, "도서관기술 (") {
		t.Fatal("전체 모드 조직명")
	}
	o02 := weeklyGet(t, e, "/meeting/weekly?date=2026-10-19&view_org_id=O02", jwtCookieUserClaims(t, u.vis))
	ob := o02.Body.String()
	if !strings.Contains(ob, "미디어기술") {
		t.Fatal("O02 후보")
	}
	if strings.Contains(ob, "R2610-T01") {
		t.Fatal("O02에 O01 업무")
	}

	cl := weeklyGet(t, e, "/meeting/weekly?date=2026-10-19", jwtCookieUserClaims(t, u.clevel))
	if cl.Code != http.StatusOK {
		t.Fatalf("clevel %d", cl.Code)
	}
	if strings.Contains(cl.Body.String(), "data-weekly-edit") {
		t.Fatal("C-Level 수정 단추")
	}
	form := weeklyGet(t, e, "/meeting/weekly/item?prefix=as&ref=as-t", jwtCookieUserClaims(t, u.clevel))
	if form.Code != http.StatusForbidden {
		t.Fatalf("clevel edit %d", form.Code)
	}
}

func TestMeeting943ViewAsFollowsSimRole(t *testing.T) {
	e, _, u := newMeeting943Server(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/meeting/weekly?date=2026-10-19", nil)
	req.AddCookie(jwtCookieUserClaims(t, u.oa01))
	req.AddCookie(&http.Cookie{Name: "view_as_user_id", Value: u.tech.UserID, Path: "/"})
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("view-as weekly %d", rec.Code)
	}
}

func TestMeeting943ExcelAssigneeColumn(t *testing.T) {
	e, _, u := newMeeting943Server(t)
	rec := weeklyGet(t, e, "/meeting/weekly/me.xlsx?date=2026-10-19", jwtCookieUserClaims(t, u.tech))
	if rec.Code != http.StatusOK {
		t.Fatalf("me xlsx %d", rec.Code)
	}
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	found := false
	for _, sh := range f.GetSheetList() {
		rows, _ := f.GetRows(sh)
		for _, row := range rows {
			for _, cell := range row {
				if cell == "담당자" {
					found = true
				}
				if strings.Contains(cell, "도서관영업") || strings.Contains(cell, "R2610-S01") {
					t.Fatalf("본인 엑셀에 타인 %s", cell)
				}
			}
		}
	}
	if !found {
		t.Fatal("내 자료 엑셀 담당자 열")
	}
	team := weeklyGet(t, e, "/stats/weekly-report.xlsx?period=week&date=2026-10-19", jwtCookieUserClaims(t, u.oa01))
	if team.Code != http.StatusOK {
		t.Fatalf("team xlsx %d", team.Code)
	}
	tf, err := excelize.OpenReader(bytes.NewReader(team.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer tf.Close()
	h, err := tf.GetCellValue("전체 리스트", "G2")
	if err != nil || h != "담당자" {
		t.Fatalf("팀 엑셀 담당자 헤더 %q %v", h, err)
	}
}

func TestMeeting943ObserverRedirect(t *testing.T) {
	e, _, u := newMeeting943Server(t)
	rec := weeklyGet(t, e, "/meeting/weekly", jwtCookieUserClaims(t, u.obs))
	if rec.Code != http.StatusFound {
		t.Fatalf("obs %d", rec.Code)
	}
}
