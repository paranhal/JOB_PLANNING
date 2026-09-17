package handler

import (
	"database/sql"
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

func TestFilterWorkItemsAssigneeOrAndKind(t *testing.T) {
	items := []model.WorkListItem{
		{Prefix: model.WorkPrefixAS, Assignee: "양기헌", Title: "AS양", Status: "in_progress", ScheduledDate: "2026-09-01"},
		{Prefix: model.WorkPrefixAS, Assignee: "태자운", Title: "AS태", Status: "received", ScheduledDate: "2026-09-01"},
		{Prefix: model.WorkPrefixGeneral, Assignee: "양기헌", Title: "행정양", Status: model.WBTaskWaiting, ScheduledDate: "2026-09-01"},
		{Prefix: model.WorkPrefixAS, Assignee: "최혜영", Title: "AS최", Status: "in_progress", ScheduledDate: "2026-09-01"},
	}
	mapped := mapWorkListItems(items)
	got := filterWorkItemsAssignees(mapped, []string{"양기헌", "태자운"})
	got = filterWorkItemsKinds(got, []string{workKindAS})
	if len(got) != 2 {
		t.Fatalf("n=%d", len(got))
	}
	seen := map[string]bool{}
	for _, it := range got {
		seen[it.Title] = true
		if it.Prefix != model.WorkPrefixAS {
			t.Fatalf("행정가 남았다: %s", it.Title)
		}
	}
	if !seen["AS양"] || !seen["AS태"] || seen["AS최"] || seen["행정양"] {
		t.Fatalf("OR+AND 결과가 틀리다: %+v", seen)
	}
}

func TestParseWorkAllPeriodDefault3m(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/work/all", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.Local)
	from, to, period, rng := parseWorkAllPeriod(c, now)
	if period != "3m" || rng != "3m" {
		t.Fatalf("period=%s range=%s", period, rng)
	}
	if to != "2026-09-07" || from == "" || from >= to {
		t.Fatalf("from=%s to=%s", from, to)
	}
	if from > "2026-06-08" {
		t.Fatalf("3개월 기본이 너무 짧다 from=%s", from)
	}
}

func TestWorkAllFilterChipsResetURLAndDueDate(t *testing.T) {
	e, db := newWorkAllFilterServer(t)
	today := time.Now().Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('c-yang','부여군립도서관','부여군립도서관',1), ('c-tae','공주교육청','공주교육청',1)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	if err := users.Create(&model.User{
		Username: "yang", PasswordHash: HashPassword("pw"), FullName: "양기헌",
		Role: model.RoleTech, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "tae", PasswordHash: HashPassword("pw"), FullName: "태자운",
		Role: model.RoleTech, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	asRepo := repository.NewASRepo(db)
	a1 := &model.ASReceipt{
		CustomerID: "c-yang", Symptom: "게이트양기헌", AssignedTo: "양기헌",
		ReceiptDatetime: time.Now(), Status: "in_progress", Urgency: "normal",
		VisitScheduledDate: today, ScheduleConfirmed: true,
	}
	a2 := &model.ASReceipt{
		CustomerID: "c-tae", Symptom: "서버태자운", AssignedTo: "태자운",
		ReceiptDatetime: time.Now(), Status: "assigned", Urgency: "normal",
		VisitScheduledDate: today, ScheduleConfirmed: true,
	}
	if err := asRepo.Create(a1); err != nil {
		t.Fatal(err)
	}
	if err := asRepo.Create(a2); err != nil {
		t.Fatal(err)
	}
	wb := repository.NewWBRepo(db)
	mustCreateWorkToday(t, wb, &model.WorkTask{
		WorkType: model.WBWorkAdmin, Title: "행정만있는건", Status: model.WBTaskWaiting,
		Assignee: "양기헌", DueDate: today, WorkDate: today, CustomerName: "부여군립도서관",
	})

	q := "/work/all?assignee=" + url.QueryEscape("양기헌,태자운") + "&kind=as"
	rec := doGet(t, e, q)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "게이트양기헌") || !strings.Contains(body, "서버태자운") {
		t.Fatal("담당자 2명 OR + 구분 AS 가 안 걸린다")
	}
	if strings.Contains(body, "행정만있는건") {
		t.Fatal("구분 AS 인데 행정이 남았다")
	}
	if !strings.Contains(body, "✕") {
		t.Fatal("칩 ✕ 가 없다")
	}
	if !strings.Contains(body, "초기화") {
		t.Fatal("초기화 버튼이 없다")
	}
	if !strings.Contains(body, `id="work-cust-q"`) {
		t.Fatal("기관 타이핑 칸이 없다")
	}
	if !strings.Contains(body, "종료일") || !strings.Contains(body, "sort=due_date") {
		t.Fatal("종료일 열이 안 보이거나 정렬이 없다")
	}
	if !strings.Contains(body, "전체 ") || !strings.Contains(body, "건 중") {
		t.Fatal("전체 N건 중 M건 표시가 없다")
	}

	copied := doGet(t, e, q)
	cb := copied.Body.String()
	if !strings.Contains(cb, "게이트양기헌") || !strings.Contains(cb, "서버태자운") || strings.Contains(cb, "행정만있는건") {
		t.Fatal("같은 URL 이 다른 화면이다")
	}

	reset := doGet(t, e, "/work/all")
	rb := reset.Body.String()
	if !strings.Contains(rb, "행정만있는건") {
		t.Fatal("초기화 뒤에 행정이 안 돌아온다")
	}
	if strings.Contains(rb, "검색:") {
		t.Fatal("초기화 뒤에도 검색 칩이 남았다")
	}

	byDue := doGet(t, e, "/work/all?sort=due_date&dir=asc")
	if byDue.Code != http.StatusOK || !strings.Contains(byDue.Body.String(), "dir=desc") {
		t.Fatal("종료일 정렬 토글이 없다")
	}

	cust := doGet(t, e, "/work/all?customer=c-yang")
	ct := cust.Body.String()
	if !strings.Contains(ct, "게이트양기헌") {
		t.Fatal("기관 필터가 안 걸린다")
	}
	if strings.Contains(ct, "서버태자운") {
		t.Fatal("기관 필터가 다른 기관을 남긴다")
	}
}

func newWorkAllFilterServer(t *testing.T) (*echo.Echo, *sql.DB) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "work-filter.db"))
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
	return e, db
}
