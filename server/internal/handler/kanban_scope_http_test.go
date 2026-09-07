package handler

import (
	"database/sql"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newKanbanScopeServer(t *testing.T) (*echo.Echo, *sql.DB) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "kanban-scope.db"))
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
	g.GET("/plan/unplanned", h.Work.UnplannedList)
	g.GET("/assets", h.Asset.List)
	g.POST("/assets/:id/status", h.Asset.UpdateStatus)
	g.GET("/users", h.Auth.UserList)
	g.GET("/customers", h.Customer.List)
	return e, db
}

func TestCustomersHasNoKanbanButton(t *testing.T) {
	e, db := newKanbanScopeServer(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','분관','분관',1)`); err != nil {
		t.Fatal(err)
	}
	rec := doGet(t, e, "/customers")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, ">칸반<") {
		t.Fatal("/customers 에 칸반 버튼이 있다")
	}
	if !strings.Contains(body, "상위기관 없음") {
		t.Fatal("상위기관별 그룹 보기가 없다")
	}
}

func TestUnplannedKanbanCountsMatchList(t *testing.T) {
	e, db := newKanbanScopeServer(t)
	now := time.Now()
	today := now.Format("2006-01-02")
	past := now.AddDate(0, 0, -5).Format("2006-01-02")
	ts := now.Format("2006-01-02 15:04:05")
	expiredAt := now.AddDate(0, 0, -15).Format("2006-01-02")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','기관A','기관A',1)`); err != nil {
		t.Fatal(err)
	}
	insert := func(id, num, visit, confirmed, assigned, status, reason, reasonAt string) {
		t.Helper()
		_, err := db.Exec(`INSERT INTO as_receipts (
			as_id, as_number, receipt_datetime, customer_id, symptom, urgency, status,
			visit_scheduled_date, schedule_confirmed, assigned_to,
			schedule_no_date_reason, schedule_no_date_at, created_at, updated_at
		) VALUES (?,?,?,'C1','증상','중',?,?,?,?,?,?,?,?)`,
			id, num, ts, status, visit, confirmed, assigned, reason, reasonAt, ts, ts)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("AS-NODATE", "R-ND", "", "0", "테크", "in_progress", "", "")
	insert("AS-DELAY", "R-DL", past, "1", "테크", "in_progress", "", "")
	insert("AS-NEXT", "R-NX", past, "1", "테크", "in_progress", "", "")
	insert("AS-UNAS", "R-UA", today, "1", "", "received", "", "")
	insert("AS-REV", "R-RV", "", "0", "테크", "in_progress", "부품 대기", expiredAt)
	if _, err := db.Exec(`INSERT INTO as_processes (process_id, process_number, as_id, process_datetime, worker, work_content, time_spent)
		VALUES ('P1','P1','AS-NEXT',?,'테크','방문',30)`, past+" 14:00:00"); err != nil {
		t.Fatal(err)
	}

	list := doGet(t, e, "/plan/unplanned")
	kanban := doGet(t, e, "/plan/unplanned?display=kanban")
	if list.Code != http.StatusOK || kanban.Code != http.StatusOK {
		t.Fatalf("list=%d kanban=%d body=%s", list.Code, kanban.Code, kanban.Body.String())
	}
	lb, kb := list.Body.String(), kanban.Body.String()
	if strings.Contains(kb, "draggable=\"true\"") {
		t.Fatal("미계획 칸반은 드래그 금지")
	}
	for _, col := range []string{"예정없음", "예정일경과", "다음일정미정", "담당자미배정", "미정사유만료"} {
		if !strings.Contains(kb, col) {
			t.Errorf("열 %s 없음", col)
		}
	}
	listN := strings.Count(lb, `class="hover:bg-gray-50`)
	kanbanN := strings.Count(kb, `data-item-key="`)
	if listN == 0 || listN != kanbanN {
		t.Fatalf("리스트 %d 칸반 %d", listN, kanbanN)
	}
}

func TestAssetKanbanFiveColumnsAndStatusMove(t *testing.T) {
	e, db := newKanbanScopeServer(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','기관A','기관A',1)`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	sts := []string{model.AssetOpOperating, model.AssetOpMaintenance, model.AssetOpFault, model.AssetOpRetired, model.AssetOpDisposed}
	for i, st := range sts {
		id := "A000" + string(rune('1'+i))
		_, err := db.Exec(`INSERT INTO assets (asset_id, customer_id, product_name, operation_status, is_managed, created_at, updated_at)
			VALUES (?, 'C1', ?, ?, 1, ?, ?)`, id, "제품"+id, st, now, now)
		if err != nil {
			t.Fatal(err)
		}
	}

	list := doGet(t, e, "/assets")
	kanban := doGet(t, e, "/assets?display=kanban")
	if list.Code != http.StatusOK || kanban.Code != http.StatusOK {
		t.Fatalf("list=%d kanban=%d kanbanBody=%s", list.Code, kanban.Code, kanban.Body.String())
	}
	kb := kanban.Body.String()
	if !strings.Contains(list.Body.String(), ">칸반<") {
		t.Fatal("자산 리스트에 칸반 버튼 없음")
	}
	for _, col := range []string{"운영중", "점검중", "장애", "철수", "폐기"} {
		if !strings.Contains(kb, col) {
			t.Errorf("열 %s 없음", col)
		}
	}
	if strings.Count(kb, `data-ref="`) != 5 {
		t.Fatalf("카드 %d want 5", strings.Count(kb, `data-ref="`))
	}

	rec := doForm(t, e, "/assets/A0001/status", url.Values{"status": {model.AssetOpFault}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status move %d %s", rec.Code, rec.Body.String())
	}
}

func TestUsersGroupedByRoleNoKanban(t *testing.T) {
	e, _ := newKanbanScopeServer(t)
	rec := doGet(t, e, "/users")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, ">칸반<") {
		t.Fatal("/users 에 칸반 버튼")
	}
	if !strings.Contains(body, "관리자 소속") {
		t.Fatal("소속별 그룹 없음")
	}
}
