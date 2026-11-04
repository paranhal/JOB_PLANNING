package handler

import (
	"database/sql"
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
	if !strings.Contains(body, `data-ui-dd="scope"`) || !strings.Contains(body, `data-ui-dd="view"`) {
		t.Fatal("V1 범위·보기 드롭다운 없음")
	}
	if strings.Contains(body, `class="board-sc"`) {
		t.Fatal("V1 구 범위 버튼 그룹")
	}
	if strings.Contains(body, `aria-label="그룹 포함 보기"`) {
		t.Fatal("V3 그룹 없는 사용자에게 구 토글")
	}
	if strings.Contains(body, `data-ui-dd="group"`) {
		t.Fatal("V3 그룹 없는 사용자에게 그룹 드롭다운")
	}
	if !strings.Contains(body, `id="schedule-show"`) || !strings.Contains(body, `class="sched-show"`) {
		t.Fatal("V4 접수/조치 토글")
	}
	iScope := strings.Index(body, `data-ui-dd="scope"`)
	iShow := strings.Index(body, `id="schedule-show"`)
	iView := strings.Index(body, `data-ui-dd="view"`)
	iNew := strings.Index(body, `data-new-work`)
	if !(iScope >= 0 && iScope < iShow && iShow < iView && iView < iNew) {
		t.Fatalf("V4 순서 scope=%d show=%d view=%d new=%d", iScope, iShow, iView, iNew)
	}
	if !strings.Contains(body, `data-ui-dd-item="mine"`) || !strings.Contains(body, `data-ui-dd-item="all"`) {
		t.Fatal("V2 범위 항목")
	}
	if !strings.Contains(body, `data-ui-dd-item="list"`) || !strings.Contains(body, `data-ui-dd-item="cal"`) || !strings.Contains(body, `data-ui-dd-item="kanban"`) {
		t.Fatal("V2 보기 항목")
	}
	if !strings.Contains(body, `<a class="ui-dd-item`) {
		t.Fatal("V8 a href 항목")
	}

	allPage := doGetWith(t, e, "/workboard/board?mine=0&auto=0", ck).Body.String()
	mineN := regexp.MustCompile(`전체 (\d+)건`).FindStringSubmatch(body)
	allN := regexp.MustCompile(`전체 (\d+)건`).FindStringSubmatch(allPage)
	if mineN == nil || allN == nil {
		t.Fatal("V6 건수")
	}
	if mineN[1] == allN[1] {
		t.Fatalf("V6 내 업무와 전체 건수가 같다 %s", mineN[1])
	}
	if !strings.Contains(body, "내91") || strings.Contains(body, "남91") {
		t.Fatal("V6 내 업무 필터")
	}
	if !strings.Contains(allPage, "남91") {
		t.Fatal("V6 전체 필터")
	}

	cal := doGetWith(t, e, "/workboard/board?view=day&auto=0", ck).Body.String()
	if !strings.Contains(cal, ">일</a>") || !strings.Contains(cal, ">주</a>") || !strings.Contains(cal, ">월</a>") {
		t.Fatal("V5 캘린더 일/주/월")
	}
	kan := doGetWith(t, e, "/workboard/board?view=kanban&auto=0", ck).Body.String()
	if strings.Contains(kan, ">주</a>") {
		t.Fatal("V5 칸반에 주 기간 버튼")
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
	if !strings.Contains(withG, `data-ui-dd="group"`) {
		t.Fatal("V3 그룹 있는 사용자")
	}
	if !strings.Contains(withG, `href="?scope=with"`) || !strings.Contains(withG, `href="?scope=personal"`) {
		t.Fatal("V2 그룹 href")
	}
	if strings.Contains(withG, `aria-label="그룹 포함 보기"`) {
		t.Fatal("V1 구 그룹 버튼")
	}
	iGroup := strings.Index(withG, `data-ui-dd="group"`)
	iShow2 := strings.Index(withG, `id="schedule-show"`)
	if iGroup < 0 || iGroup > iShow2 {
		t.Fatalf("V4 그룹 위치 %d show=%d", iGroup, iShow2)
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
