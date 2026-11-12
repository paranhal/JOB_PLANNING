package handler

import (
	"bytes"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_70_MeetingMenuMoveAndLegacyRoutes(t *testing.T) {
	vis := FilterNav(visionMenuCtx(), NavMeetingBoard, NavBadges{})
	hrefs := collectNavHrefs(vis.Groups)
	if !containsHref(hrefs, "/meeting/board") {
		t.Fatal("업무 회의 메뉴가 /meeting/board 가 아니다")
	}
	if !containsHref(hrefs, "/meeting") || !containsHref(hrefs, "/meeting/weekly") || !containsHref(hrefs, "/meeting/weekly/me") {
		t.Fatal("예전 회의 주소가 사이드바에 없다")
	}
	var workMeeting navDefItem
	for _, g := range navDefinition() {
		if g.ID != "work" {
			continue
		}
		for _, it := range g.Items {
			if it.ID == "work_meeting" {
				workMeeting = it
			}
		}
	}
	if len(workMeeting.Children) != 0 || workMeeting.Href != "/meeting/board" {
		t.Fatalf("업무 회의가 단일 메뉴가 아니다 children=%d href=%s", len(workMeeting.Children), workMeeting.Href)
	}

	sales := FilterNav(MenuCtx{Role: model.RoleSales, LoginRole: model.RoleSales, ShowProjects: true,
		Perms: model.DefaultPermissions(model.RoleSales)}, NavDashboard, NavBadges{})
	sh := collectNavHrefs(sales.Groups)
	if containsHref(sh, "/meeting/weekly") {
		t.Fatal("영업에 주간 팀 (예전) 이 보인다")
	}
	if !containsHref(sh, "/meeting/board") || !containsHref(sh, "/meeting/weekly/me") {
		t.Fatal("영업에 업무 회의·내 자료 (예전) 이 없다")
	}

	legacy := FilterNav(visionMenuCtx(), NavMeeting, NavBadges{})
	if !containsHref(legacy.CurParents, "legacy_views") {
		t.Fatal("일일 회의 (예전) 이 예전 화면 보기 아래가 아니다")
	}

	db, err := repository.InitDB(filepath.Join(t.TempDir(), "m70.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/meeting/board", h.Meeting.Board)
	g.GET("/meeting/board.xlsx", h.Meeting.BoardXLSX)
	g.POST("/meeting/board", h.Meeting.BoardWrite)
	g.GET("/meeting", h.Meeting.Show)
	g.GET("/meeting/weekly", h.Meeting.Weekly)
	g.GET("/meeting/weekly/me", h.Meeting.WeeklyMe)

	ck := jwtCookie(t)
	board := httptest.NewRecorder()
	breq := httptest.NewRequest(http.MethodGet, "/meeting/board", nil)
	breq.AddCookie(ck)
	e.ServeHTTP(board, breq)
	if board.Code != http.StatusOK {
		t.Fatalf("새 화면 %d", board.Code)
	}
	bb := board.Body.String()
	if !strings.Contains(bb, "업무 회의") {
		t.Fatal("새 화면 제목 없음")
	}
	if !strings.Contains(bb, `href="/meeting"`) || !strings.Contains(bb, "예전") {
		t.Fatal("예전 화면으로 링크 없음")
	}
	if !strings.Contains(bb, "담당자별 보기") || !strings.Contains(bb, `data-view="people"`) {
		t.Fatal("담당자별 보기가 기본이 아니다")
	}
	if strings.Contains(bb, "다음 단계에서 열립니다") {
		t.Fatal("빈 안내 화면이 남았다")
	}

	old := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodGet, "/meeting?date=2026-10-19", nil)
	oreq.AddCookie(ck)
	e.ServeHTTP(old, oreq)
	if old.Code != http.StatusOK {
		t.Fatalf("/meeting?date= 가 바뀌면 안 됨 %d loc=%s", old.Code, old.Header().Get("Location"))
	}
	if !strings.Contains(old.Body.String(), "일일 업무 회의") {
		t.Fatal("예전 일일 화면이 아니다")
	}

	if ParentPage("/meeting/board") != "/" {
		t.Fatal("새 화면 이전이 대시보드가 아니다")
	}
	if ParentPage("/meeting") != "/" || ParentPage("/meeting/weekly") != "/" {
		t.Fatal("예전 회의 이전이 바뀌었다")
	}
}

func Test95_70_MeetingBoardTemplateUsesKnownClasses(t *testing.T) {
	root := findTemplateRoot(t)
	b := string(mustRead(t, filepath.Join(root, "meeting/board.html")))
	got := countUILint(b)
	if !got.zero() {
		t.Fatalf("ui_lint %+v", got)
	}
	if !strings.Contains(b, "grid-cols-1") || !strings.Contains(b, "overflow-x-auto") || !strings.Contains(b, "min-w-[320px]") {
		t.Fatal("95-69 모바일 클래스 없음")
	}
	if !strings.Contains(b, `data-table-cols="meeting_board"`) {
		t.Fatal("95-68 열 너비 없음")
	}
}

func htmlDataAttr(body, key string) string {
	needle := key + `="`
	i := strings.Index(body, needle)
	if i < 0 {
		return ""
	}
	rest := body[i+len(needle):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func htmlDataInt(t *testing.T, body, key string) int {
	t.Helper()
	s := htmlDataAttr(body, key)
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("%s=%q", key, s)
	}
	return n
}

func dump970Shot(t *testing.T, name, html string) {
	t.Helper()
	root := findTemplateRoot(t)
	dir := filepath.Join(root, "..", "..", "..", "docs", "리뷰", "screenshots", "95-70")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(html), 0644); err != nil {
		t.Fatal(err)
	}
}

type meeting970Users struct {
	oa01, oa02, oa03, tech, vis, clevel *model.User
}

// meeting970DB 마지막으로 만든 회의 시험 DB(다른 시험이 자료를 더 넣을 때 쓴다).
var meeting970DB *sql.DB

func newMeeting970Server(t *testing.T) (*echo.Echo, *Handler, meeting970Users) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "m70b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	meeting970DB = db
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O02','O02','미디어사업팀','business',1,20),
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	visSeed := &model.User{Username: "vs70", PasswordHash: "x", FullName: "비젼관리자", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	if err := users.Create(visSeed); err != nil {
		t.Fatal(err)
	}
	seed := []*model.User{
		{Username: "oa01", PasswordHash: "x", FullName: "도서관관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true},
		{Username: "oa02", PasswordHash: "x", FullName: "미디어관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O02", ProfileDone: true},
		{Username: "oa03", PasswordHash: "x", FullName: "SI관리자", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true},
		{Username: "t01", PasswordHash: "x", FullName: "도서관기술", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true},
		{Username: "cl1", PasswordHash: "x", FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true},
	}
	for _, u := range seed {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c1','회의도서관','회의도서관',1,?),('c2','미디어도서관','미디어도서관',1,'O02'),('c3','SI도서관','SI도서관',1,'O03'),
		       ('c-mnt2','점검2','점검2',1,?)`, model.OrgIDLibrary, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	day := "2026-10-21"
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, visit_scheduled_date,
		schedule_confirmed, status, assigned_to, symptom, updated_at, org_id
	) VALUES
		('as-t','R2610-T01','c1','2026-10-05',?,1,'in_progress','도서관기술','기술건',?,?),
		('as-s','R2610-S01','c1','2026-10-05',?,1,'in_progress','도서관영업','영업건',?,?),
		('as-m','R2610-M01','c2','2026-10-05',?,1,'in_progress','미디어기술','미디어건',?,'O02'),
		('as-3','R2610-Z01','c3','2026-10-05',?,1,'in_progress','SI기술','SI건',?,'O03')`,
		day, day+" 09:00:00", model.OrgIDLibrary,
		day, day+" 09:00:00", model.OrgIDLibrary,
		day, day+" 09:00:00",
		day, day+" 09:00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,project_kind_set)
		VALUES('WP-A','도서관공사','active',?,'construction',1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (task_id, work_type, title, work_date, due_date, status, assignee, org_id, project_id)
		VALUES ('wt-p','project_work','공사현장',?,?, 'waiting','도서관기술',?,'WP-A')`, day, day, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('p70',2026,'t','draft')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_site_config (customer_id, short_name, has_klas, has_rfid, inspection_cycle)
		VALUES ('c1','회의도서관',1,0,'monthly'),('c-mnt2','점검2',1,0,'monthly')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, completed, completed_date, product_type, assignee)
		VALUES ('v1','p70',?,'c1',1,?,'KLAS','도서관기술'),
		       ('v2','p70',?,'c-mnt2',0,NULL,'KLAS','')`, day, day, day); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewStaffLeaveRepo(db).Create(model.StaffLeave{
		UserID: seed[3].UserID, Date: day, Kind: model.LeaveKindAnnual,
	}); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/meeting/board", h.Meeting.Board)
	g.GET("/meeting/board.xlsx", h.Meeting.BoardXLSX)
	g.POST("/meeting/board", h.Meeting.BoardWrite)
	out := meeting970Users{oa01: seed[0], oa02: seed[1], oa03: seed[2], tech: seed[3], vis: visSeed, clevel: seed[4]}
	return e, h, out
}

func Test95_70_BoardPeopleProjectExcelOrgMntLeave(t *testing.T) {
	e, h, u := newMeeting970Server(t)
	fromto := "from=2026-10-21&to=2026-10-21"
	oa := weeklyGet(t, e, "/meeting/board?"+fromto, jwtCookieUserClaims(t, u.oa01))
	if oa.Code != http.StatusOK {
		t.Fatalf("oa %d %s", oa.Code, oa.Body.String()[:minInt(400, oa.Body.Len())])
	}
	body := oa.Body.String()
	if strings.Contains(body, `data-org-tab=`) {
		t.Fatal("조직관리자에 조직 탭")
	}
	if !strings.Contains(body, `id="meeting-board-org-locked"`) || !strings.Contains(body, "도서관사업팀") {
		t.Fatal("조직관리자 소속 고정 없음")
	}
	if strings.Contains(body, "R2610-M01") || strings.Contains(body, "R2610-Z01") {
		t.Fatal("O01에 타 조직 업무")
	}
	if !strings.Contains(body, "R2610-T01") {
		t.Fatal("O01 AS 없음")
	}
	if !strings.Contains(body, "공사현장") {
		t.Fatal("O01 작업 없음")
	}
	if !strings.Contains(body, `data-view="people"`) || !strings.Contains(body, `aria-current="page"`) {
		t.Fatal("담당자별 기본 선택")
	}
	if !strings.Contains(body, "주간 자료 함께 보기") || !strings.Contains(body, "모두 펼치기") {
		t.Fatal("주간 함께 보기·접기 없음")
	}
	if !strings.Contains(body, `id="page-back"`) {
		t.Fatal("95-67 이전 버튼 없음")
	}
	if !strings.Contains(body, `data-leave-name="도서관기술"`) || !strings.Contains(body, `data-leave-badge="도서관기술"`) {
		t.Fatal("휴가자·배지 없음")
	}
	peopleN := htmlDataInt(t, body, "data-people-total")
	projectN := htmlDataInt(t, body, "data-project-total")
	itemN := htmlDataInt(t, body, "data-item-total")
	if peopleN != projectN || peopleN != itemN || peopleN == 0 {
		t.Fatalf("합계 불일치 people=%d project=%d item=%d", peopleN, projectN, itemN)
	}
	schedN := htmlDataInt(t, body, "data-sched-total")
	sched, err := h.Meeting.work.ListScheduledOn("2026-10-21", "", nil, 300)
	if err != nil {
		t.Fatal(err)
	}
	oaSched := uniqueWorkItems(h.Meeting.filterMeetingOrg(echoCtxOrg(t, u.oa01), sched))
	if schedN != len(oaSched) {
		t.Fatalf("95-60 예정 건수 HTML=%d repo=%d", schedN, len(oaSched))
	}

	anchor := time.Date(2026, 10, 21, 0, 0, 0, 0, time.Local)
	quota, cum, _, err := h.Meeting.stats.CompanyMntMonthProgress(model.OrgIDLibrary, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if htmlDataInt(t, body, "data-mnt-target") != quota || htmlDataInt(t, body, "data-mnt-done") != cum {
		t.Fatalf("집행 대상/완료 HTML=%s/%s repo=%d/%d", htmlDataAttr(body, "data-mnt-target"), htmlDataAttr(body, "data-mnt-done"), quota, cum)
	}
	wantRate := "—"
	if quota > 0 {
		wantRate = strconv.FormatFloat(repository.ProgressPct(cum, quota), 'f', 1, 64) + "%"
	}
	if htmlDataAttr(body, "data-mnt-rate") != wantRate {
		t.Fatalf("집행률 HTML=%s want %s", htmlDataAttr(body, "data-mnt-rate"), wantRate)
	}
	if strings.Contains(body, "CountMonthMntQuota") {
		t.Fatal("내부 함수명이 화면에 노출")
	}

	proj := weeklyGet(t, e, "/meeting/board?"+fromto+"&view=project", jwtCookieUserClaims(t, u.oa01))
	if proj.Code != http.StatusOK {
		t.Fatalf("project %d", proj.Code)
	}
	pb := proj.Body.String()
	if !strings.Contains(pb, "사업별 보기") || !strings.Contains(pb, "도서관공사") {
		t.Fatal("사업별 보기·사업명 없음")
	}
	if htmlDataInt(t, pb, "data-project-total") != peopleN {
		t.Fatalf("사업별 합계 %s", htmlDataAttr(pb, "data-project-total"))
	}
	if strings.Count(pb, `data-item-key="project_work:wt-p"`) != 1 {
		t.Fatal("사업 간 중복 집계")
	}

	xl := weeklyGet(t, e, "/meeting/board.xlsx?"+fromto, jwtCookieUserClaims(t, u.oa01))
	if xl.Code != http.StatusOK {
		t.Fatalf("xlsx %d", xl.Code)
	}
	xf, err := excelize.OpenReader(bytes.NewReader(xl.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer xf.Close()
	if !containsAll(xf.GetSheetList(), "요약", "상세") {
		t.Fatalf("시트 %v", xf.GetSheetList())
	}
	detail, _ := xf.GetRows("상세")
	if len(detail)-1 != itemN {
		t.Fatalf("엑셀 상세 %d HTML %d", len(detail)-1, itemN)
	}
	sum, _ := xf.GetRows("요약")
	foundPeople := false
	for _, row := range sum {
		if len(row) >= 3 && row[0] == "담당자별" && row[1] == "합계" {
			foundPeople = true
			if row[2] != strconv.Itoa(peopleN) {
				t.Fatalf("엑셀 담당자 합계 %v", row)
			}
		}
		if len(row) >= 3 && row[0] == "사업별" && row[1] == "합계" && row[2] != strconv.Itoa(peopleN) {
			t.Fatalf("엑셀 사업 합계 %v", row)
		}
	}
	if !foundPeople {
		t.Fatal("엑셀 담당자별 합계 행 없음")
	}

	o02 := weeklyGet(t, e, "/meeting/board?"+fromto, jwtCookieUserClaims(t, u.oa02))
	if strings.Contains(o02.Body.String(), "R2610-T01") || strings.Contains(o02.Body.String(), "R2610-Z01") {
		t.Fatal("O02에 타 조직")
	}
	if !strings.Contains(o02.Body.String(), "R2610-M01") {
		t.Fatal("O02 본 건 없음")
	}
	o03 := weeklyGet(t, e, "/meeting/board?"+fromto, jwtCookieUserClaims(t, u.oa03))
	if strings.Contains(o03.Body.String(), "R2610-T01") || strings.Contains(o03.Body.String(), "R2610-M01") {
		t.Fatal("O03에 타 조직")
	}
	if !strings.Contains(o03.Body.String(), "R2610-Z01") {
		t.Fatal("O03 본 건 없음")
	}

	bypass := weeklyGet(t, e, "/meeting/board?"+fromto+"&view_org_id=O02", jwtCookieUserClaims(t, u.oa01))
	if strings.Contains(bypass.Body.String(), "R2610-M01") {
		t.Fatal("조직관리자가 view_org_id 로 우회")
	}

	cl := weeklyGet(t, e, "/meeting/board?"+fromto+"&view_org_id=*", jwtCookieUserClaims(t, u.clevel))
	if cl.Code != http.StatusOK {
		t.Fatalf("clevel %d", cl.Code)
	}
	cb := cl.Body.String()
	if !strings.Contains(cb, `data-org-tab="*"`) || !strings.Contains(cb, "전체 조직") {
		t.Fatal("C-Level 조직 탭 없음")
	}
	if !strings.Contains(cb, "도서관사업팀") || !strings.Contains(cb, "미디어사업팀") || !strings.Contains(cb, "SI사업팀") {
		t.Fatal("C-Level 조직 이름")
	}
	if !strings.Contains(cb, "R2610-T01") || !strings.Contains(cb, "R2610-M01") || !strings.Contains(cb, "R2610-Z01") {
		t.Fatal("C-Level 전체 조직 누락")
	}
	if !strings.Contains(cb, "조회 전용") {
		t.Fatal("C-Level 조회 전용 문구")
	}
	clO02 := weeklyGet(t, e, "/meeting/board?"+fromto+"&view_org_id=O02", jwtCookieUserClaims(t, u.clevel))
	if strings.Contains(clO02.Body.String(), "R2610-T01") {
		t.Fatal("C-Level O02에 O01")
	}

	post := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodPost, "/meeting/board", nil)
	preq.AddCookie(jwtCookieUserClaims(t, u.clevel))
	e.ServeHTTP(post, preq)
	if post.Code != http.StatusForbidden {
		t.Fatalf("C-Level POST %d", post.Code)
	}

	vis := weeklyGet(t, e, "/meeting/board?"+fromto+"&view_org_id=O02", jwtCookieUserClaims(t, u.vis))
	vb := vis.Body.String()
	if !strings.Contains(vb, `id="meeting-board-view-org"`) {
		t.Fatal("비전 조직 선택 없음")
	}
	if !strings.Contains(vb, `value="O02"`) || !strings.Contains(vb, "selected") {
		t.Fatal("비전 O02 선택 상태")
	}
	if strings.Contains(vb, "R2610-T01") || !strings.Contains(vb, "R2610-M01") {
		t.Fatal("비전 O02 범위")
	}

	tech := weeklyGet(t, e, "/meeting/board?"+fromto+"&assignee=all", jwtCookieUserClaims(t, u.tech))
	tb := tech.Body.String()
	if strings.Contains(tb, "R2610-S01") || strings.Contains(tb, "R2610-M01") {
		t.Fatal("일반 팀원 타인 업무")
	}
	if !strings.Contains(tb, "R2610-T01") {
		t.Fatal("일반 팀원 본인 없음")
	}

	dump970Shot(t, "clevel-all.html", cb)
	dump970Shot(t, "org-admin.html", body)
	dump970Shot(t, "project-view.html", pb)
}

func echoCtxOrg(t *testing.T, u *model.User) echo.Context {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("role", u.Role)
	c.Set("org_id", u.OrgID)
	c.Set("user_id", u.UserID)
	c.Set("user_name", u.FullName)
	return c
}

func containsAll(list []string, want ...string) bool {
	set := map[string]bool{}
	for _, s := range list {
		set[s] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}
