package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func seedWeekly32(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','SI',1,3)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects(project_id,name,status,org_id,is_test) VALUES
		('WP-P1','미행사업','active','O03',0),
		('WP-P2','행있음사업','active','O03',0),
		('WP-O01','도서관사업','active',?,0),
		('WP-TEST','테스트사업','active','O03',1)`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('C32','해미초','해미초',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	tech := &model.User{Username: "si-tech", PasswordHash: HashPassword("password10"), FullName: "김기술",
		Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true}
	lib := &model.User{Username: "lib-tech", PasswordHash: HashPassword("password10"), FullName: "박도서",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{tech, lib} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, complete_datetime, status,
		assigned_to, assigned_user_id, org_id, symptom, created_at, updated_at)
		VALUES ('AS-NP','AS-NP','C32','2026-08-12 10:00:00','2026-08-12 16:00:00','completed',
			'김기술', ?, 'O03', '게이트', datetime('now'), datetime('now'))`, tech.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (
		as_id, as_number, customer_id, receipt_datetime, complete_datetime, status,
		assigned_to, org_id, symptom, created_at, updated_at)
		VALUES ('AS-O01','AS-O01','C32','2026-08-12 10:00:00','2026-08-12 16:00:00','completed',
			'박도서', ?, '게이트', datetime('now'), datetime('now'))`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date,assignee)
		VALUES ('T-NP32','admin','사업없음행정','waiting',NULL,'O03','','2026-08-12','김기술')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO maintenance_plans (plan_id, plan_year, title, status)
		VALUES ('MP32', 2026, '계획', 'approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES ('V32','MP32','2026-08-12','C32',0,'normal','김기술','KLAS',1)`); err != nil {
		t.Fatal(err)
	}
}

func postWeeklyLink(t *testing.T, e *echo.Echo, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/stats/company-weekly/tasks/project", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func asProjectID(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var pid string
	if err := db.QueryRow(`SELECT COALESCE(project_id,'') FROM as_receipts WHERE as_id=?`, id).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return pid
}

func asAssignee(t *testing.T, db *sql.DB, id string) (string, string) {
	t.Helper()
	var name, uid string
	if err := db.QueryRow(`SELECT COALESCE(assigned_to,''), COALESCE(assigned_user_id,'') FROM as_receipts WHERE as_id=?`, id).Scan(&name, &uid); err != nil {
		t.Fatal(err)
	}
	return name, uid
}

// TestWeeklyLinkASReceiptReproMissingTask §4-1: AS ID를 업무 ID로내면 업무가 없습니다.
func TestWeeklyLinkASReceiptReproMissingTask(t *testing.T) {
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "cw32-repro.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	seedWeekly32(t, db)

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	mountWeekly28(e, h)

	rec := postWeeklyLink(t, e, url.Values{
		"task_id": {"AS-NP"}, "project_id": {"WP-P1"}, "org": {"O03"}, "week": {"2026-08-10"},
	}, jwtCookieOrg(t, model.RoleOrgAdmin, "O03"))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
	t.Logf("95-32 재현 Location=%s", loc)
	if !strings.Contains(loc, "업무가 없습니다") {
		t.Fatalf("AS를 task_id로 내면 업무가 없습니다가 나와야 한다: %s", loc)
	}
	if got := asProjectID(t, db, "AS-NP"); got != "" {
		t.Fatalf("연결되면 안 됨 project_id=%s", got)
	}
}
