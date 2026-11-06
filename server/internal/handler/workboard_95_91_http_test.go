package handler

import (
	"database/sql"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func board95_91App(t *testing.T) (*echo.Echo, *sql.DB, *model.User, *model.User, *repository.WBRepo) {
	t.Helper()
	e, db, me, other, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d91.db"))
	h := New(db)
	e.GET("/as", h.AS.List)
	e.GET("/", h.Dashboard)
	return e, db, me, other, repo
}

func Test95_91_V1toV8_RenderDropdowns(t *testing.T) {
	e, db, me, other, repo := board95_91App(t)
	ck := jwtCookieUser(t, me)
	today := time.Now().Format(dateLayout)
	mustCreateBoardTask(t, repo, "내91", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "남91", today, "11:00", "12:00", model.OrgIDLibrary, other.FullName, other.UserID, model.WBTaskWaiting, model.WBWorkAdmin)

	list := doGetWith(t, e, "/workboard/board?auto=0", ck)
	if list.Code != 200 {
		t.Fatalf("list %d", list.Code)
	}
	body := list.Body.String()
	if !strings.Contains(body, ">요약</a>") || !strings.Contains(body, ">일정표</a>") || !strings.Contains(body, ">칸반</a>") {
		t.Fatal("보기 세그 없음")
	}
	if strings.Contains(body, `data-ui-dd="view"`) {
		t.Fatal("구 보기 드롭다운이 남아 있다")
	}
	if strings.Contains(body, `aria-label="그룹 포함 보기"`) {
		t.Fatal("V3 그룹 없는 사용자에게 구 토글")
	}
	if strings.Contains(body, `data-ui-dd="group"`) {
		t.Fatal("V3 그룹 없는 사용자에게 그룹 드롭다운")
	}
	dayBody := doGetWith(t, e, "/workboard/board?view=day&auto=0", ck).Body.String()
	if !strings.Contains(dayBody, `id="schedule-show"`) || !strings.Contains(dayBody, `class="sched-show"`) {
		t.Fatal("일정표 일 접수/완료 토글")
	}
	if !strings.Contains(dayBody, "내 업무") || !strings.Contains(dayBody, "전체") {
		t.Fatal("범위 토글")
	}
	if !strings.Contains(dayBody, "내91") || strings.Contains(dayBody, "남91") {
		t.Fatal("내 업무 필터")
	}
	allPage := doGetWith(t, e, "/workboard/board?view=day&mine=0&auto=0", ck).Body.String()
	if !strings.Contains(allPage, "남91") {
		t.Fatal("전체 필터")
	}
	cal := dayBody
	if !strings.Contains(cal, ">일</a>") || !strings.Contains(cal, ">주</a>") || !strings.Contains(cal, ">월</a>") {
		t.Fatal("V5 캘린더 일/주/월")
	}

	work := doGetWith(t, e, "/work", ck).Body.String()
	if !strings.Contains(work, `aria-label="그룹 포함 보기"`) || strings.Contains(work, `data-ui-dd="scope"`) {
		t.Fatal("V7 /work 가 바뀌었다")
	}
	as := doGetWith(t, e, "/as", ck)
	if as.Code == 200 {
		ab := as.Body.String()
		if !strings.Contains(ab, `aria-label="그룹 포함 보기"`) {
			t.Fatal("V7 /as 토글 없음")
		}
		if strings.Contains(ab, `data-ui-dd="view"`) {
			t.Fatal("V7 /as 에 업무관리 드롭다운")
		}
	}
	dash := doGetWith(t, e, "/", ck)
	if dash.Code == 200 && !strings.Contains(dash.Body.String(), `aria-label="그룹 포함 보기"`) {
		t.Fatal("V7 대시보드 토글")
	}

	users := repository.NewUserRepo(db)
	oa, err := users.GetByUsername("oa01")
	if err != nil || oa == nil {
		t.Fatal("oa01")
	}
	wg := repository.NewWorkGroupRepo(db)
	g := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "91조"}
	actor := model.WorkGroupActor{UserID: oa.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleOrgAdmin}
	if err := wg.Create(actor, g); err != nil {
		t.Fatal(err)
	}
	if err := wg.AddMember(actor, g.GroupID, me.UserID, model.WorkGroupMember, ""); err != nil {
		t.Fatal(err)
	}
	withG := doGetWith(t, e, "/workboard/board?auto=0", ck).Body.String()
	if !strings.Contains(withG, `aria-label="그룹 포함 보기"`) {
		t.Fatal("그룹 있는 사용자 토글")
	}
	if !strings.Contains(withG, `href="?scope=with"`) || !strings.Contains(withG, `href="?scope=personal"`) {
		t.Fatal("그룹 href")
	}
}

func Test95_91_V8_JSUnitAndV10_CSS(t *testing.T) {
	cmd := exec.Command("node", "ui_dropdown_test.js")
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
	for _, name := range []string{"ui-dd", "ui-dd-btn", "ui-dd-menu", "ui-dd-item"} {
		if !strings.Contains(s, "."+name) {
			t.Errorf("V10 CSS 에 .%s 없음", name)
		}
	}
	got := countUILint(string(mustRead(t, filepath.Join(findTemplateRoot(t), "_partials", "ui_dropdown.html"))))
	if !got.zero() {
		t.Fatalf("ui_lint %+v", got)
	}
}

func Test95_91_V9_MobileNoHScroll(t *testing.T) {
	t.Skip("보기 드롭다운 제거(95-132A). 세그 툴바")
	e, _, me, _, _ := board95_91App(t)
	ck := jwtCookieUser(t, me)
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	cmd := exec.Command("node", "95_91_dropdown_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+ck.Value,
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
