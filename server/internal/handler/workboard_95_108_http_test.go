package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	nameOwn108   = "내조직사업-108-도서관"
	nameRel108   = "유관사업-108-미디어주관"
	nameOther108 = "타부서사업-108-미디어"
	nameEmpty108 = "미지정사업-108-빈조직"
)

func app95_108(t *testing.T) (*echo.Echo, *sql.DB, *model.User, *repository.WBRepo) {
	t.Helper()
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d108.db"))
	return e, db, me, repo
}

func mustProject108(t *testing.T, db *sql.DB, name, org string) *model.WorkProject {
	t.Helper()
	p := &model.WorkProject{Name: name, OrgID: org, Status: model.WBProjectActive, IsPaid: true}
	if err := repository.NewProjectRepo(db).Create(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func Test95_108_V2V3_BoardRenderOwnOnly(t *testing.T) {
	e, db, me, _ := app95_108(t)
	mustProject108(t, db, nameOwn108, model.OrgIDLibrary)
	rel := mustProject108(t, db, nameRel108, "O02")
	if err := repository.NewProjectRepo(db).ReplaceRelatedOrgs(rel.ProjectID, me.UserID, []string{model.OrgIDLibrary}, "O02"); err != nil {
		t.Fatal(err)
	}
	mustProject108(t, db, nameOther108, "O02")
	empty := mustProject108(t, db, nameEmpty108, model.OrgIDLibrary)
	if _, err := db.Exec(`UPDATE work_projects SET org_id='' WHERE project_id=?`, empty.ProjectID); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, me)
	body := doGetWith(t, e, "/workboard/board?view=day&auto=0", ck).Body.String()
	if !strings.Contains(body, `data-project-filter`) {
		t.Fatal("project_filter 없음")
	}
	if strings.Contains(body, nameOther108) || strings.Contains(body, nameEmpty108) {
		t.Fatal("초기 HTML 에 타 부서/미지정 사업명")
	}
	if !strings.Contains(body, nameOwn108) || !strings.Contains(body, nameRel108) {
		t.Fatal("내 조직·유관 사업명 없음")
	}
	if !strings.Contains(body, "유관") {
		t.Fatal("유관 표시 없음")
	}
	if !strings.Contains(body, "타 부서 사업 보기") {
		t.Fatal("타 부서 버튼 없음")
	}
	if !strings.Contains(body, `data-other-count="2"`) {
		t.Fatal("OtherCount 불일치")
	}
}

func Test95_108_V3_NoOtherHidesButton(t *testing.T) {
	e, db, me, _ := app95_108(t)
	mustProject108(t, db, nameOwn108, model.OrgIDLibrary)
	body := doGetWith(t, e, "/workboard/board?view=day&auto=0", jwtCookieUser(t, me)).Body.String()
	if strings.Contains(body, "타 부서 사업 보기") {
		t.Fatal("타 부서 0건인데 버튼")
	}
	if strings.Contains(body, nameOther108) {
		t.Fatal("타 부서 명")
	}
}

func Test95_108_V4_OtherJSON(t *testing.T) {
	e, db, me, _ := app95_108(t)
	mustProject108(t, db, nameOwn108, model.OrgIDLibrary)
	mustProject108(t, db, nameOther108, "O02")
	empty := mustProject108(t, db, nameEmpty108, model.OrgIDLibrary)
	if _, err := db.Exec(`UPDATE work_projects SET org_id='' WHERE project_id=?`, empty.ProjectID); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, me)
	rec := doGetWith(t, e, "/workboard/projects/other", ck)
	if rec.Code != 200 {
		t.Fatalf("code %d %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Groups []projectOrgGroup `json:"groups"`
		Total  int               `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Total < 2 {
		t.Fatalf("total %d", payload.Total)
	}
	var names []string
	var emptyGrp bool
	for _, g := range payload.Groups {
		if g.OrgName == projectFilterEmptyOrgName && g.OrgID == "" {
			emptyGrp = true
		}
		for _, it := range g.Items {
			names = append(names, it.Name)
			if it.Name == nameOwn108 {
				t.Fatal("내 조직 사업이 other JSON")
			}
		}
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, nameOther108) {
		t.Fatal("타 부서 없음")
	}
	if !emptyGrp {
		t.Fatal("조직 미지정 그룹 없음")
	}
	q := doGetWith(t, e, "/workboard/projects/other?q="+nameOther108, ck)
	var qpay struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(q.Body.Bytes(), &qpay)
	if qpay.Total != 1 {
		t.Fatalf("검색 total %d", qpay.Total)
	}
	spoof := doGetWith(t, e, "/workboard/projects/other?org_id=O02", ck)
	var sp struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(spoof.Body.Bytes(), &sp)
	if sp.Total != payload.Total {
		t.Fatal("org_id 위조가 목록을 바꿈")
	}
	anon := doGetWith(t, e, "/workboard/projects/other")
	if anon.Code != http.StatusFound && anon.Code != http.StatusSeeOther && anon.Code != http.StatusForbidden && anon.Code != http.StatusUnauthorized {
		t.Fatalf("비로그인 %d", anon.Code)
	}
}

func Test95_108_V5V6V7_SelectAndEmptyCount(t *testing.T) {
	e, db, me, repo := app95_108(t)
	own := mustProject108(t, db, nameOwn108, model.OrgIDLibrary)
	oth := mustProject108(t, db, nameOther108, "O02")
	today := time.Now().Format(dateLayout)
	tk := &model.WorkTask{
		Title: "108본사업무", WorkDate: today, DueDate: today, StartTime: "09:00", EndTime: "10:00",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin, Priority: model.WBPriorityNormal,
		ProjectID: own.ProjectID,
	}
	if err := repo.CreateTask(tk); err != nil {
		t.Fatal(err)
	}
	tk2 := &model.WorkTask{
		Title: "108타부서연결", WorkDate: today, DueDate: today, StartTime: "11:00", EndTime: "12:00",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin, Priority: model.WBPriorityNormal,
		ProjectID: oth.ProjectID,
	}
	if err := repo.CreateTask(tk2); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, me)
	empty := doGetWith(t, e, "/workboard/board?view=list&date="+today+"&auto=0&mine=0", ck).Body.String()
	nEmpty := regexp.MustCompile(`전체 (\d+)건`).FindStringSubmatch(empty)
	if nEmpty == nil {
		t.Fatal("빈 필터 건수")
	}
	fake := doGetWith(t, e, "/workboard/board?view=list&date="+today+"&auto=0&mine=0&project=WP-NOPE", ck).Body.String()
	nFake := regexp.MustCompile(`전체 (\d+)건`).FindStringSubmatch(fake)
	if nFake == nil || nFake[1] != nEmpty[1] {
		t.Fatalf("위조 ID 무시 실패 empty=%v fake=%v", nEmpty, nFake)
	}
	if strings.Contains(fake, `value="WP-NOPE"`) {
		t.Fatal("위조 ID 가 hidden 에 남음")
	}
	ownPage := doGetWith(t, e, "/workboard/board?view=list&date="+today+"&auto=0&mine=0&project="+own.ProjectID, ck).Body.String()
	if !strings.Contains(ownPage, "108본사업무") || strings.Contains(ownPage, "108타부서연결") {
		t.Fatal("내 조직 사업 필터")
	}
	othPage := doGetWith(t, e, "/workboard/board?view=day&auto=0&project="+oth.ProjectID, ck).Body.String()
	if !strings.Contains(othPage, `data-pf-org-tag`) || !strings.Contains(othPage, "미디어") {
		t.Fatal("타 부서 선택 이름표")
	}
	if !strings.Contains(othPage, nameOther108) {
		t.Fatal("선택 라벨에 타 부서 사업명")
	}
}

func Test95_108_V8_CLevelNoMineBox(t *testing.T) {
	e, db, _, _ := app95_108(t)
	mustProject108(t, db, nameOwn108, model.OrgIDLibrary)
	mustProject108(t, db, nameOther108, "O02")
	users := repository.NewUserRepo(db)
	ceo := &model.User{
		Username: "ceo108", PasswordHash: HashPassword("password10"), FullName: "대표108",
		Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true,
	}
	if err := users.Create(ceo); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByUsername("ceo108")
	body := doGetWith(t, e, "/workboard/board?view=day&auto=0", jwtCookieUser(t, got)).Body.String()
	if strings.Contains(body, "내 조직") {
		t.Fatal("C-Level 에 내 조직")
	}
	if strings.Contains(body, "타 부서 사업 보기") {
		t.Fatal("C-Level 타 부서 버튼")
	}
	if !strings.Contains(body, `data-all-orgs="1"`) {
		t.Fatal("C-Level all-orgs")
	}
	if strings.Contains(body, nameOther108) || strings.Contains(body, nameOwn108) {
		t.Fatal("C-Level 초기 HTML 에 사업명")
	}
	oa, _ := users.GetByUsername("oa01")
	oaBody := doGetWith(t, e, "/workboard/board?view=day&auto=0", jwtCookieUser(t, oa)).Body.String()
	if !strings.Contains(oaBody, "내 조직 사업") || !strings.Contains(oaBody, "타 부서 사업 보기") {
		t.Fatal("조직관리자 구조")
	}
}

func Test95_108_V11_RegisterSameComponent(t *testing.T) {
	e, db, me, _ := app95_108(t)
	mustProject108(t, db, nameOwn108, model.OrgIDLibrary)
	mustProject108(t, db, nameOther108, "O02")
	body := doGetWith(t, e, "/workboard/register", jwtCookieUser(t, me)).Body.String()
	if !strings.Contains(body, `id="reg-project"`) || !strings.Contains(body, `data-project-filter`) {
		t.Fatal("register project_filter")
	}
	if strings.Contains(body, nameOther108) {
		t.Fatal("register 초기 HTML 타 부서명")
	}
	if !strings.Contains(body, nameOwn108) {
		t.Fatal("register 내 조직명")
	}
	if !strings.Contains(body, `name="project"`) {
		t.Fatal("project 쿼리 이름")
	}
}

func Test95_108_V9_JSUnitAndV12_CSS(t *testing.T) {
	cmd := exec.Command("node", "project_filter_test.js")
	cmd.Dir = filepath.Join(testWebRoot(t), "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js: %v\n%s", err, out)
	}
	css, err := os.ReadFile(filepath.Join(testWebRoot(t), "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(css)
	for _, name := range []string{"pf-btn", "pf-panel", "pf-more", "pf-hint"} {
		if !strings.Contains(s, "."+name) {
			t.Errorf("CSS 에 .%s 없음", name)
		}
	}
	got := countUILint(string(mustRead(t, filepath.Join(findTemplateRoot(t), "_partials", "project_filter.html"))))
	if !got.zero() {
		t.Fatalf("ui_lint %+v", got)
	}
}

func Test95_108_V10_Browser(t *testing.T) {
	e, db, me, _ := app95_108(t)
	mustProject108(t, db, nameOwn108, model.OrgIDLibrary)
	mustProject108(t, db, nameOther108, "O02")
	ck := jwtCookieVerified(t, me)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	cmd := exec.Command("node", "95_108_project_filter_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+ck.Value,
		"OTHER_NAME="+nameOther108,
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
