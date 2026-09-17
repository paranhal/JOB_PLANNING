package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func echoCtxRole(role, name string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("role", role)
	c.Set("user_name", name)
	c.Set("username", role)
	c.Set("permissions", model.DefaultPermissions(role))
	return c
}

func TestCanEditTaskRoles(t *testing.T) {
	admin := echoCtxRole(model.RoleAdmin, "관리자")
	if !canEditTask(admin, "다른담당", "") {
		t.Fatal("관리자는 남의 업무도 편집한다")
	}
	office := echoCtxRole(model.RoleOffice, "행정")
	if !canEditTask(office, "다른담당", "") {
		t.Fatal("접수담당은 남의 업무도 편집한다")
	}
	tech := echoCtxRole(model.RoleTech, "양기헌")
	if !canEditTask(tech, "양기헌", "") {
		t.Fatal("기술담당은 내 배정을 편집한다")
	}
	if canEditTask(tech, "태자운", "") {
		t.Fatal("기술담당은 남의 업무를 편집하면 안 된다")
	}
	sales := echoCtxRole(model.RoleSales, "영업")
	if !canEditTask(sales, "영업", "") {
		t.Fatal("영업담당은 내 배정을 편집한다")
	}
	if canEditTask(sales, "태자운", "") {
		t.Fatal("영업담당은 남의 업무를 편집하면 안 된다")
	}
	obs := echoCtxRole(model.RoleObserver, "관찰")
	if canEditTask(obs, "관찰", "") || canEditTask(obs, "양기헌", "") {
		t.Fatal("관찰자는 편집할 수 없다")
	}
}

func TestCanEditSalesActivityRoles(t *testing.T) {
	admin := echoCtxRole(model.RoleAdmin, "관리자")
	if !canEditSalesActivity(admin, "다른사람") {
		t.Fatal("관리자는 남의 활동도 편집한다")
	}
	office := echoCtxRole(model.RoleOffice, "행정")
	if !canEditSalesActivity(office, "다른사람") {
		t.Fatal("행정은 남의 활동도 편집한다")
	}
	sales := echoCtxRole(model.RoleSales, "영업")
	if !canEditSalesActivity(sales, "영업") {
		t.Fatal("영업담당은 본인 활동을 편집한다")
	}
	if canEditSalesActivity(sales, "태자운") {
		t.Fatal("영업담당은 남의 활동을 편집하면 안 된다")
	}
	obs := echoCtxRole(model.RoleObserver, "관찰")
	if canEditSalesActivity(obs, "관찰") || canEditSalesActivity(obs, "양기헌") {
		t.Fatal("관찰자는 활동을 편집할 수 없다")
	}
}

func findWorkTaskIDByTitle(t *testing.T, repo *repository.WBRepo, title string) string {
	t.Helper()
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Title == title {
			return tk.TaskID
		}
	}
	t.Fatalf("업무 %q 를 못 찾았다", title)
	return ""
}

func workFormCookie(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func TestWorkAllTechSeesOthersLocked(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	rec := workTodayGet(t, e, "/work/all?display=list", jwtCookieRole(t, "tech"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "다른담당진행") {
		t.Fatal("기술담당 목록에 남의 업무가 없다")
	}
	if !strings.Contains(body, `title="읽기 전용"`) || !strings.Contains(body, "🔒") {
		t.Fatal("남의 업무에 자물쇠가 없다")
	}
	if !strings.Contains(body, "font-bold") {
		t.Fatal("남의 담당자 이름이 굵게 안 된다")
	}
}

func TestWorkOtherTaskReadOnlyAndForbidden(t *testing.T) {
	e, repo := newWorkTodayServer(t)
	id := findWorkTaskIDByTitle(t, repo, "관리자할일")
	tech := jwtCookieRole(t, "tech")

	show := workTodayGet(t, e, "/workboard/tasks/"+id, tech)
	if show.Code != http.StatusOK {
		t.Fatalf("상세 status=%d body=%s", show.Code, show.Body.String())
	}
	body := show.Body.String()
	if !strings.Contains(body, "읽기 전용") {
		t.Fatal("남의 상세에 읽기 전용 안내가 없다")
	}
	if strings.Contains(body, ">저장</button>") {
		t.Fatal("남의 상세에 저장 버튼이 있다")
	}
	if strings.Contains(body, ">삭제</button>") {
		t.Fatal("남의 상세에 삭제 버튼이 있다")
	}

	post := workFormCookie(t, e, "/workboard/tasks/"+id+"/update", url.Values{
		"status": {"in_progress"},
	}, tech)
	if post.Code != http.StatusForbidden {
		t.Fatalf("남의 업무 POST 가 403 이 아니다 status=%d loc=%s", post.Code, post.Header().Get("Location"))
	}

	adminShow := workTodayGet(t, e, "/workboard/tasks/"+id, jwtCookie(t))
	if adminShow.Code != http.StatusOK {
		t.Fatalf("관리자 상세 status=%d", adminShow.Code)
	}
	ab := adminShow.Body.String()
	if !strings.Contains(ab, ">저장</button>") {
		t.Fatal("관리자 상세에 저장 버튼이 없다")
	}

	adminPost := workFormCookie(t, e, "/workboard/tasks/"+id+"/update", url.Values{
		"status": {"in_progress"},
	}, jwtCookie(t))
	if adminPost.Code == http.StatusForbidden {
		t.Fatal("관리자 POST 가 403 이다")
	}
	if adminPost.Code != http.StatusSeeOther {
		t.Fatalf("관리자 POST status=%d loc=%s", adminPost.Code, adminPost.Header().Get("Location"))
	}
}
