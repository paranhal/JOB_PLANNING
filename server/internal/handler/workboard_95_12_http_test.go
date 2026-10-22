package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
)

func TestProjectWorkFetchJSONKeepsModalContract(t *testing.T) {
	e, _, _ := newProjectWorkServer(t)
	form := url.Values{"work_type": {"project_work"}, "title": {""}, "work_subtype": {"build"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set(echo.HeaderAccept, echo.MIMEApplicationJSON)
	req.Header.Set("X-Requested-With", "fetch")
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get(echo.HeaderLocation) != "" {
		t.Fatal("fetch 오류가 리다이렉트됨")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["message"] != "작업명을 입력하세요" {
		t.Fatalf("message=%v", payload["message"])
	}

	root := findTemplateRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "workboard", "_modals.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Count(s, "js-keep-modal") < 4 {
		t.Fatalf("모달 fetch 훅 %d개", strings.Count(s, "js-keep-modal"))
	}
	if !strings.Contains(s, "X-Requested-With") || !strings.Contains(s, "showFlashError") {
		t.Fatal("fetch 제출 스크립트 없음")
	}
}

func TestWorkPickProjects47ContractLikeAndCombo(t *testing.T) {
	e, repo, users, db := newProjectWorkHarness(t)
	kinds := []string{"build", "construction", "maintenance"}
	orgs := []string{"O01", "O02", "O03"}
	statuses := []string{model.WBProjectActive, "hold", "", "in_progress"}
	for i := 1; i <= 47; i++ {
		id := fmt.Sprintf("WP-c%02d", i)
		name := fmt.Sprintf("계약사업%02d 긴이름점검", i)
		st := statuses[(i-1)%len(statuses)]
		kind := kinds[(i-1)%len(kinds)]
		org := orgs[(i-1)%len(orgs)]
		if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,project_kind_set)
			VALUES(?,?,?,?,?,1)`, id, name, st, org, kind); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,project_kind_set)
		VALUES('WP-skip','숨김완료','complete','O01','build',1)`); err != nil {
		t.Fatal(err)
	}

	body := doGet(t, e, "/workboard/register?view=day").Body.String()
	if !strings.Contains(body, "projectCombo") {
		t.Fatal("등록 콤보 없음")
	}
	for i := 1; i <= 47; i++ {
		name := fmt.Sprintf("계약사업%02d 긴이름점검", i)
		if !strings.Contains(body, name) {
			t.Fatalf("등록에 %q 없음", name)
		}
	}
	if strings.Contains(body, "숨김완료") {
		t.Fatal("완료 사업이 목록에 있다")
	}

	oa04, err := users.GetByUsername("oa04")
	if err != nil {
		t.Fatal(err)
	}
	empty := httptest.NewRequest(http.MethodGet, "/workboard/register?view=day", nil)
	empty.AddCookie(jwtCookieUser(t, oa04))
	emptyRec := httptest.NewRecorder()
	e.ServeHTTP(emptyRec, empty)
	if !strings.Contains(emptyRec.Body.String(), "계약사업01 긴이름점검") {
		t.Fatal("O04에서도 47건 목록이 안 보인다")
	}

	ok := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"project_work"}, "title": {"47사업연결"}, "work_subtype": {"build"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "project_id": {"WP-c03"},
		"start_time": {"09:00"}, "end_time": {"09:30"},
	})
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("타 조직 계약사업 저장 %d %s", ok.Code, ok.Body.String())
	}
	id := pwFindTitle(t, repo, "47사업연결")
	got, _ := repo.GetTask(id)
	if got == nil || got.ProjectID != "WP-c03" {
		t.Fatalf("project_id=%v", got)
	}

	edit := doGet(t, e, "/workboard/tasks/"+id+"/edit")
	if !strings.Contains(edit.Body.String(), "계약사업47 긴이름점검") || !strings.Contains(edit.Body.String(), "projectCombo") {
		t.Fatal("수정 화면 콤보/47건 없음")
	}
	act := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if !strings.Contains(act.Body.String(), "계약사업10 긴이름점검") || !strings.Contains(act.Body.String(), "projectCombo") {
		t.Fatal("조치 화면 콤보/47건 없음")
	}

	mf := url.Values{
		"work_type": {"project_work"}, "title": {"없는사업"}, "work_subtype": {"other"},
		"work_date": {"2026-10-21"}, "assignee": {"관리자"}, "project_id": {"WP-NOPE"},
		"start_time": {"10:00"}, "end_time": {"10:30"},
	}
	missJSON := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks", strings.NewReader(mf.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set(echo.HeaderAccept, echo.MIMEApplicationJSON)
	req.Header.Set("X-Requested-With", "fetch")
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(missJSON, req)
	if missJSON.Code != http.StatusBadRequest || !strings.Contains(missJSON.Body.String(), "사업을 찾을 수 없습니다") {
		t.Fatalf("없는 사업 JSON: %d %s", missJSON.Code, missJSON.Body.String())
	}

	rec := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "http://localhost/workboard/tasks", strings.NewReader(mf.Encode()))
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req2.Header.Set(echo.HeaderAccept, "text/html")
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req2)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("없는 사업 HTML %d %s", rec.Code, rec.Body.String())
	}
	if flashCookieFrom(rec) == nil {
		t.Fatal("없는 사업 flash 쿠키 없음")
	}
}

func TestWorkTaskOpenHrefAndAssetQuery(t *testing.T) {
	if got := model.WorkTaskOpenHref("WT1", model.WBWorkProjectWork); got != "/workboard/tasks/WT1/action" {
		t.Fatalf("작업 href=%s", got)
	}
	if got := model.WorkTaskOpenHref("WT2", model.WBWorkAdmin); got != "/workboard/tasks/WT2" {
		t.Fatalf("행정 href=%s", got)
	}
	q := AssetQuery()
	if q == "" {
		t.Fatal("AssetQuery 비어 있음")
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	e.GET("/login", func(c echo.Context) error {
		return c.Render(http.StatusOK, "auth/login.html", map[string]interface{}{})
	})
	login := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	e.ServeHTTP(login, req)
	if !strings.Contains(login.Body.String(), "/static/vendor/tailwind.css?v="+q) {
		t.Fatal("로그인 CSS 캐시 버스터 없음")
	}
	if !strings.Contains(login.Body.String(), "flashDialog") {
		t.Fatal("공통 오류 팝업 스크립트 없음")
	}
}

func TestProjectComboTemplateUILintZero(t *testing.T) {
	root := findTemplateRoot(t)
	for _, rel := range []string{"workboard/project_work_action.html", "workboard/_project_combo.html"} {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		got := countUILint(string(b))
		if !got.zero() {
			t.Fatalf("%s ui_lint %+v", rel, got)
		}
	}
}

