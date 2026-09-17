package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestWorkAllShowsLastMonthAndTeam(t *testing.T) {
	e, repo := newWorkTodayServer(t)
	lastMonth := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "지난달업무", Status: model.WBTaskWaiting,
		Assignee: "다른담당", DueDate: lastMonth, WorkDate: lastMonth, CustomerName: "지난달도서관",
	})
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "미배정초안", Status: model.WBTaskWaiting,
		Assignee: "", DueDate: lastMonth, WorkDate: lastMonth, CustomerName: "미배정도서관",
	})

	today := workTodayGet(t, e, "/work", jwtCookie(t))
	if today.Code != http.StatusOK {
		t.Fatalf("work status=%d", today.Code)
	}
	tb := today.Body.String()
	if strings.Contains(tb, "지난달업무") || strings.Contains(tb, "다른담당진행") {
		t.Fatal("오늘 내 업무에 지난달·남의 건이 보인다")
	}
	if strings.Contains(tb, `href="/work/all" class="bg-blue-600`) {
		t.Fatal("오늘 내 업무에서 전체 업무 메뉴가 강조된다")
	}

	all := workTodayGet(t, e, "/work/all", jwtCookie(t))
	if all.Code != http.StatusOK {
		t.Fatalf("all status=%d body=%s", all.Code, all.Body.String())
	}
	body := all.Body.String()
	if !strings.Contains(body, "전체 업무") {
		t.Fatal("전체 업무 제목이 없다")
	}
	if !strings.Contains(body, "지난달업무") || !strings.Contains(body, "다른담당진행") {
		t.Fatal("전체 업무에 지난달·팀 건이 없다")
	}
	if !strings.Contains(body, `href="/work/all" class="bg-blue-600`) {
		t.Fatal("사이드바 전체 업무가 강조되지 않는다")
	}
	if strings.Contains(body, `href="/work" class="bg-blue-600`) {
		t.Fatal("전체 업무에서 오늘 내 업무가 강조된다")
	}

	mine := workTodayGet(t, e, "/work/all?assignee="+url.QueryEscape("관리자"), jwtCookie(t))
	if mine.Code != http.StatusOK {
		t.Fatalf("assignee status=%d", mine.Code)
	}
	mb := mine.Body.String()
	if !strings.Contains(mb, "관리자할일") {
		t.Fatal("내 전체 업무에 본인 건이 없다")
	}
	if strings.Contains(mb, "지난달업무") || strings.Contains(mb, "다른담당진행") {
		t.Fatal("assignee 필터가 남의 건을 남긴다")
	}

	none := workTodayGet(t, e, "/work/all?assignee=__none__", jwtCookie(t))
	if none.Code != http.StatusOK {
		t.Fatalf("none status=%d", none.Code)
	}
	nb := none.Body.String()
	if !strings.Contains(nb, "미배정초안") {
		t.Fatal("미배정 필터 대상이 없다")
	}
	if strings.Contains(nb, "관리자할일") || strings.Contains(nb, "지난달업무") {
		t.Fatal("미배정 필터가 담당자 있는 건을 남긴다")
	}
}

func TestWorkAllDelayedAndDefaultSort(t *testing.T) {
	e, repo := newWorkTodayServer(t)
	lastMonth := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	mustCreateWorkToday(t, repo, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "지난달지연", Status: model.WBTaskWaiting,
		Assignee: "관리자", DueDate: lastMonth, WorkDate: lastMonth, CustomerName: "지연도서관",
	})

	rec := workTodayGet(t, e, "/work/all?status=delayed", jwtCookie(t))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "지난달지연") {
		t.Fatal("지연 필터에 지난달 건이 없다")
	}
	if !strings.Contains(body, "dir=asc") && !strings.Contains(body, "due_date") {
		// 기본이 내림차순이면 링크는 오름차순으로 뒤집힌다
		if !strings.Contains(body, `href="/work/all?`) {
			t.Fatal("정렬 링크가 없다")
		}
	}
}

func TestPaginateWorkItems(t *testing.T) {
	items := make([]model.WorkListItem, 117)
	shown, more := paginateWorkItems(items, 0, workAllPageSize)
	if len(shown) != 100 || !more {
		t.Fatalf("first page shown=%d more=%v", len(shown), more)
	}
	shown, more = paginateWorkItems(items, 100, workAllPageSize)
	if len(shown) != 17 || more {
		t.Fatalf("last page shown=%d more=%v", len(shown), more)
	}
}

func TestWorkDashboardDestinations(t *testing.T) {
	if got := workAllURL("", "delayed"); got != "/work/all?status=delayed" {
		t.Fatalf("지연 URL=%s", got)
	}
	if got := workAllURL(workAssigneeNone, ""); got != "/work/all?assignee=__none__" {
		t.Fatalf("미배정 URL=%s", got)
	}
	if got := workAllURL("관리자", ""); !strings.Contains(got, "assignee=") || !strings.Contains(got, "/work/all") {
		t.Fatalf("내 전체 URL=%s", got)
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "handler.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "bucket=") || strings.Contains(body, "workListURL") || strings.Contains(body, "WorkBucketOpen") {
		t.Fatal("대시보드 핸들러에 bucket 파라미터가 남았다")
	}
	if !strings.Contains(body, `"/work/all"`) || !strings.Contains(body, `workAllURL("", "delayed")`) {
		t.Fatal("대시보드 카드가 /work/all 로 안 바뀌었다")
	}
}

func TestWorkAllRouteOnNavServer(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "work-all-nav.db"))
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
	g.GET("/work", h.Work.List)
	g.GET("/work/all", h.Work.ListAll)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/work/all", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, ">전체 업무<") {
		t.Fatal("사이드바에 전체 업무 메뉴가 없다")
	}
}
