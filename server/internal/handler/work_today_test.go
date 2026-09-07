package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newWorkTodayServer(t *testing.T) (*echo.Echo, *repository.WBRepo) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "work-today.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	if err := users.Create(&model.User{
		Username: "other", PasswordHash: HashPassword("pw"), FullName: "다른담당",
		Role: model.RoleTech, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}

	today := time.Now().Format("2006-01-02")
	repo := repository.NewWBRepo(db)
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "관리자할일", Status: model.WBTaskWaiting,
		Assignee: "관리자", DueDate: today, WorkDate: today, CustomerName: "중앙도서관",
	})
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "다른담당진행", Status: model.WBTaskInProgress,
		Assignee: "다른담당", DueDate: today, WorkDate: today, CustomerName: "서부도서관",
	})
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "기술보류건", Status: model.WBTaskHold,
		HoldReason: "일정조정", ReviewDate: today,
		Assignee: "tech", DueDate: today, WorkDate: today, CustomerName: "가나도서관",
	})
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "관리자완료", Status: model.WBTaskComplete,
		Assignee: "관리자", DueDate: today, WorkDate: today, CompleteNote: "끝",
	})
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "취소된통합업무", Status: model.WBTaskCancelled,
		CancelReason: "중복", Assignee: "관리자", DueDate: today, WorkDate: today,
	})

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/work", h.Work.List)
	g.GET("/admin-work", h.AdminWork.List)
	return e, repo
}

func mustCreateWorkToday(t *testing.T, repo *repository.WBRepo, task *model.WorkTask) {
	t.Helper()
	if err := repo.CreateTask(task); err != nil {
		t.Fatal(err)
	}
}

func workTodayGet(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func TestWorkTodayAdminSeesTeamKanban(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	rec := workTodayGet(t, e, "/work?display=kanban", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "오늘 내 업무") || !strings.Contains(body, "진행 예정") {
		t.Fatal("칸반 열이 없다")
	}
	if !strings.Contains(body, "팀 전체") {
		t.Fatal("관리자 기본 범위가 팀 전체가 아니다")
	}
	for _, title := range []string{"관리자할일", "다른담당진행", "기술보류건", "관리자완료"} {
		if !strings.Contains(body, title) {
			t.Fatalf("관리자에게 %s 가 안 보인다", title)
		}
	}
	if strings.Contains(body, "취소된통합업무") {
		t.Fatal("취소 건이 올라갔다")
	}
	if !strings.Contains(body, "가나도서관") || !strings.Contains(body, "[일반업무]") {
		t.Fatal("칸반 카드에 접두어·기관이 없다")
	}
	if !strings.Contains(body, "보류") {
		t.Fatal("보류 뱃지가 없다")
	}
}

func TestWorkTodayTechMineAndTeamToggle(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	tech := jwtCookieRole(t, "tech")
	mine := workTodayGet(t, e, "/work?display=kanban", tech)
	if mine.Code != http.StatusOK {
		t.Fatalf("status=%d", mine.Code)
	}
	body := mine.Body.String()
	if !strings.Contains(body, "내 배정 업무") || !strings.Contains(body, "기술보류건") {
		t.Fatal("기술담당 기본이 본인 건이 아니다")
	}
	if strings.Contains(body, "관리자할일") || strings.Contains(body, "다른담당진행") {
		t.Fatal("기술담당 기본에 다른 사람 건이 보인다")
	}
	if !strings.Contains(body, "팀 전체 보기") {
		t.Fatal("팀 전체 토글이 없다")
	}

	team := workTodayGet(t, e, "/work?mine=0", tech)
	if team.Code != http.StatusOK {
		t.Fatalf("team status=%d", team.Code)
	}
	tbody := team.Body.String()
	if !strings.Contains(tbody, "관리자할일") || !strings.Contains(tbody, "기술보류건") {
		t.Fatal("기술담당이 팀 전체를 못 본다")
	}
	if !strings.Contains(tbody, "내 배정만") {
		t.Fatal("본인 보기로 돌아가는 토글이 없다")
	}
}

func TestWorkTodayViewKeepsFilters(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	rec := workTodayGet(t, e, "/work?view=list&mine=0", jwtCookieRole(t, "tech"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "종료일") || !strings.Contains(body, "관리자할일") {
		t.Fatal("리스트 보기가 아니다")
	}
	if !strings.Contains(body, "mine=0") || !strings.Contains(body, "display=kanban") {
		t.Fatal("칸반 링크에 mine·display 가 유지되지 않는다")
	}
}

func TestWorkTodayAdminAssigneeFilter(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	rec := workTodayGet(t, e, "/work?display=kanban&assignee="+url.QueryEscape("다른담당"), jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "다른담당진행") {
		t.Fatal("담당자 필터 대상이 없다")
	}
	if strings.Contains(body, "관리자할일") || strings.Contains(body, "기술보류건") {
		t.Fatal("담당자 필터가 다른 사람 건을 남긴다")
	}
}

func TestWorkTodayHoldSameColumnAsAdminKanban(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	ck := jwtCookie(t)
	admin := workTodayGet(t, e, "/admin-work?view=kanban", ck)
	today := workTodayGet(t, e, "/work?display=kanban", ck)
	if admin.Code != http.StatusOK || today.Code != http.StatusOK {
		t.Fatalf("admin=%d work=%d", admin.Code, today.Code)
	}
	if !strings.Contains(admin.Body.String(), "기술보류건") || !strings.Contains(admin.Body.String(), "보류") {
		t.Fatal("행정 칸반 진행중에 보류 건이 없다")
	}
	if !strings.Contains(today.Body.String(), "기술보류건") || !strings.Contains(today.Body.String(), "보류") {
		t.Fatal("오늘 내 업무 진행중에 보류 건이 없다")
	}
}

func TestWorkTodayRoleUnresolvedBanner(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "work-role.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	req := httptest.NewRequest(http.MethodGet, "http://localhost/work", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/work")
	if err := h.Work.List(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "권한 정보를 불러오지 못했습니다") {
		t.Fatal("역할 미확정 배너가 없다")
	}
	if !strings.Contains(body, "역할을 확인하지 못해 팀 전체로 표시합니다") {
		t.Fatal("역할 미확정 안내가 없다")
	}
}

func TestWorkTodayDefaultIsListAndKanbanCountsMatch(t *testing.T) {
	e, _ := newWorkTodayServer(t)
	ck := jwtCookie(t)
	list := workTodayGet(t, e, "/work", ck)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d", list.Code)
	}
	lb := list.Body.String()
	if !strings.Contains(lb, "종료일") || strings.Contains(lb, "min-w-[260px]") {
		t.Fatal("기본이 리스트가 아니다")
	}
	if !strings.Contains(lb, "display=kanban") {
		t.Fatal("칸반 전환에 display 가 없다")
	}
	kanban := workTodayGet(t, e, "/work?display=kanban", ck)
	if kanban.Code != http.StatusOK {
		t.Fatalf("kanban %d", kanban.Code)
	}
	kb := kanban.Body.String()
	if !strings.Contains(kb, "min-w-[260px]") || !strings.Contains(kb, "진행 예정") {
		t.Fatal("공통 칸반 열이 없다")
	}
	titles := []string{"관리자할일", "다른담당진행", "기술보류건", "관리자완료"}
	for _, title := range titles {
		if !strings.Contains(lb, title) || !strings.Contains(kb, title) {
			t.Fatalf("리스트·칸반 건수가 어긋남: %s", title)
		}
	}
	if strings.Contains(lb, "취소된통합업무") || strings.Contains(kb, "취소된통합업무") {
		t.Fatal("취소 건이 올라갔다")
	}
}
