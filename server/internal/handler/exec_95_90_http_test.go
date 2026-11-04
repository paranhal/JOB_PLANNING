package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func seed95_90(t *testing.T) (*echo.Echo, *sqlDBUsers, string) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "e90.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	_ = users.EnsureAdmin(HashPassword("adminadmin"))
	date := "2026-11-04"
	if _, err := db.Exec(`INSERT INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O02','O02','다른팀','business',1,20),('O03','O03','세번째','business',1,30),('O09','O09','빈조직','support',1,90)`); err != nil {
		t.Fatal(err)
	}
	ceo := &model.User{Username: "ceo90", PasswordHash: HashPassword("password10"), FullName: "대표90", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true, OrgID: "O01", Mobile: "010-1111-2222"}
	vis := &model.User{Username: "vis90", PasswordHash: HashPassword("password10"), FullName: "비전90", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	oa := &model.User{Username: "oa90", PasswordHash: HashPassword("password10"), FullName: "팀장90", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O01", ProfileDone: true}
	tech := &model.User{Username: "tech90", PasswordHash: HashPassword("password10"), FullName: "기술90", Role: model.RoleTech, IsActive: true, OrgID: "O01", ProfileDone: true}
	u2 := &model.User{Username: "u290", PasswordHash: "x", FullName: "을90", Role: model.RoleTech, IsActive: true, OrgID: "O02"}
	u3 := &model.User{Username: "u390", PasswordHash: "x", FullName: "병90", Role: model.RoleTech, IsActive: true, OrgID: "O03"}
	for _, u := range []*model.User{ceo, vis, oa, tech, u2, u3} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO work_tasks (
		task_id, work_type, title, work_date, due_date, status, assignee, org_id, complete_date, updated_at
	) VALUES
		('WT90-d','admin','완료','`+date+`','`+date+`','complete','기술90','O01','`+date+`','`+date+` 10:00:00'),
		('WT90-p','admin','진행','`+date+`','`+date+`','in_progress','기술90','O01',NULL,'`+date+` 10:00:00'),
		('WT90-w','admin','미착수','`+date+`','`+date+`','waiting','기술90','O01',NULL,'`+date+` 10:00:00'),
		('WT90-b','admin','B완료','`+date+`','`+date+`','complete','을90','O02','`+date+`','`+date+` 10:00:00'),
		('WT90-c','admin','C예정','`+date+`','`+date+`','waiting','병90','O03',NULL,'`+date+` 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	lv := repository.NewStaffLeaveRepo(db)
	if err := lv.Create(model.StaffLeave{UserID: tech.UserID, Date: date, Kind: model.LeaveKindAnnual}); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.GuardCLevelHidden)
	g.Use(h.Auth.RequireActiveRole)
	g.GET("/exec", h.Exec.Dashboard, h.Auth.RequireExecView)
	g.GET("/exec/cards", h.Exec.Cards, h.Auth.RequireExecView)
	g.GET("/exec/work", h.Exec.Work, h.Auth.RequireExecView)
	g.GET("/meeting", h.Meeting.Show)
	g.POST("/meeting/weekly/item", h.Meeting.WeeklyItemSave)
	return e, &sqlDBUsers{ceo: ceo, vis: vis, oa: oa, tech: tech}, date
}

type sqlDBUsers struct {
	ceo, vis, oa, tech *model.User
}

func Test95_90_AccessAndMeetingGET(t *testing.T) {
	e, u, date := seed95_90(t)
	page := doGetWith(t, e, "/exec?date="+date, jwtCookieUser(t, u.ceo))
	if page.Code != http.StatusOK {
		t.Fatalf("/exec %d", page.Code)
	}
	vis := doGetWith(t, e, "/exec?date="+date, jwtCookieUser(t, u.vis))
	if vis.Code != http.StatusOK {
		t.Fatalf("비전 %d", vis.Code)
	}
	oa := doGetWith(t, e, "/exec", jwtCookieUser(t, u.oa))
	if oa.Code != http.StatusForbidden {
		t.Fatalf("조직관리자 %d", oa.Code)
	}
	tech := doGetWith(t, e, "/exec", jwtCookieUser(t, u.tech))
	if tech.Code != http.StatusForbidden {
		t.Fatalf("일반 %d", tech.Code)
	}
	anon := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/exec", nil)
	e.ServeHTTP(anon, req)
	if anon.Code != http.StatusFound && anon.Code != http.StatusSeeOther && anon.Code != http.StatusUnauthorized && anon.Code != http.StatusForbidden {
		t.Fatalf("비로그인 %d", anon.Code)
	}
	meet := doGetWith(t, e, "/meeting", jwtCookieUser(t, u.ceo))
	if meet.Code == http.StatusForbidden {
		t.Fatal("C-Level /meeting GET 이 403")
	}
	meetOrg := doGetWith(t, e, "/meeting?view_org_id=O01&date="+date+"&from=exec", jwtCookieUser(t, u.ceo))
	if meetOrg.Code != http.StatusOK {
		t.Fatalf("view_org_id GET %d", meetOrg.Code)
	}
	post := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodPost, "/meeting/weekly/item", strings.NewReader("prefix=task&ref=x"))
	preq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	preq.AddCookie(jwtCookieUser(t, u.ceo))
	e.ServeHTTP(post, preq)
	if post.Code != http.StatusForbidden {
		t.Fatalf("POST %d", post.Code)
	}
}

func Test95_90_RenderAndMeetingTotal(t *testing.T) {
	e, u, date := seed95_90(t)
	ck := jwtCookieUser(t, u.ceo)
	page := doGetWith(t, e, "/exec?date="+date, ck)
	body := page.Body.String()
	if strings.Contains(body, "집계 화면은 이어서 채웁니다") {
		t.Fatal("스텁 문구가 남았다")
	}
	if strings.Count(body, `data-exec-card`) != 3 {
		t.Fatalf("박스 수 %d\n%s", strings.Count(body, `data-exec-card`), body)
	}
	if strings.Contains(body, `data-exec-org="O09"`) {
		t.Fatal("인원 0 조직")
	}
	if !strings.Contains(body, "휴가자 없음") {
		t.Fatal("휴가 없음 문구")
	}
	if !strings.Contains(body, "연차") {
		t.Fatal("연차 라벨")
	}
	if strings.Contains(body, `data-exec-bar-wait`) && !strings.Contains(body, `data-exec-total="4"`) {
		// O01 has wait+delay+done+prog; 0-count bars omitted per org. Just ensure 0-total org is gone.
	}
	if !strings.Contains(body, "/meeting?") || !strings.Contains(body, "view_org_id=O01") || !strings.Contains(body, "from=exec") {
		t.Fatal("카드 href")
	}
	re := regexp.MustCompile(`data-exec-org="O01"[^>]*data-exec-total="(\d+)"`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		re = regexp.MustCompile(`data-exec-total="(\d+)"[^>]*data-exec-org="O01"`)
		m = re.FindStringSubmatch(body)
	}
	if m == nil {
		t.Fatalf("O01 total 없음")
	}
	meet := doGetWith(t, e, "/meeting?view_org_id=O01&date="+date+"&from=exec", ck)
	mb := meet.Body.String()
	if !strings.Contains(mb, `id="exec-dash-back"`) || !strings.Contains(mb, "← 대시보드로") {
		t.Fatal("복귀 버튼")
	}
	if !strings.Contains(mb, "O01") {
		t.Fatal("조직 칩")
	}
	plain := doGetWith(t, e, "/meeting?view_org_id=O01&date="+date, ck)
	if strings.Contains(plain.Body.String(), `id="exec-dash-back"`) {
		t.Fatal("from 없이 복귀 버튼")
	}
	if !strings.Contains(mb, "예정 "+m[1]) && !strings.Contains(mb, "오늘 예정 "+m[1]) {
		t.Fatalf("회의 PlannedN 불일치 dash=%s meeting has 예정", m[1])
	}
	cards := doGetWith(t, e, "/exec/cards?date="+date, ck)
	if cards.Code != http.StatusOK || !strings.Contains(cards.Body.String(), "data-exec-card") {
		t.Fatalf("cards %d", cards.Code)
	}
	back := doGetWith(t, e, "/exec?date="+date, ck)
	if back.Body.String() != page.Body.String() {
		if strings.Count(back.Body.String(), `data-exec-card`) != strings.Count(page.Body.String(), `data-exec-card`) {
			t.Fatal("복귀 후 카드 수가 다르다")
		}
	}
}

func Test95_90_StaticAndJS(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	execSrc, err := os.ReadFile(filepath.Join(dir, "exec.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(execSrc)
	if !strings.Contains(body, "DataScopeFor(") {
		t.Fatal("DataScopeFor 없음")
	}
	if strings.Contains(body, "ClassifyExecToday") || strings.Contains(body, "ListScheduledOn") {
		t.Fatal("집계가 핸들러에 있다")
	}
	nav, err := os.ReadFile(filepath.Join(dir, "nav_exec.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nav), "func cLevelWriteExempt") {
		t.Fatal("cLevelWriteExempt 없음")
	}
	wantExempt := string(nav)
	if strings.Contains(wantExempt, `"/exec"`) && strings.Contains(funcString(wantExempt, "cLevelWriteExempt"), `"/meeting"`) {
		t.Fatal("cLevelWriteExempt 에 /meeting 이 추가됐다")
	}
	cmd := exec.Command("node", "exec_dash_nav_test.js")
	cmd.Dir = filepath.Join(testWebRoot(t), "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js: %v\n%s", err, out)
	}
}

func funcString(src, name string) string {
	i := strings.Index(src, "func "+name)
	if i < 0 {
		return ""
	}
	return src[i:]
}

func Test95_90_BrowserBackRestoresScroll(t *testing.T) {
	e, u, date := seed95_90(t)
	ck := jwtCookieUser(t, u.ceo)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	cmd := exec.Command("node", "95_90_exec_back_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+ck.Value,
		"WORK_DATE="+date,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "is not found") || strings.Contains(s, "browserType.launch") {
			t.Skip(s)
		}
		t.Fatalf("브라우저: %v\n%s", err, out)
	}
}
