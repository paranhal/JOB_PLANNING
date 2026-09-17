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

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newAssignNoticeServer(t *testing.T) (*echo.Echo, *sql.DB, *model.User) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "assign-notice.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("admin")); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(&model.User{
		Username: "choi", PasswordHash: HashPassword("pw"), FullName: "최혜영",
		Role: model.RoleTech, IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	choi, err := users.GetByUsername("choi")
	if err != nil || choi == nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active)
		VALUES ('C1','부여군립도서관','부여군립도서관',1)`); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.InjectAssignNotices)
	g.GET("/work", h.Work.List)
	g.POST("/as", h.AS.Create)
	g.POST("/work/assign-notices/later", h.Work.AssignNoticeLater)
	g.POST("/work/assign-notices/add", h.Work.AssignNoticeAdd)
	g.POST("/work/assign-notices/transfer", h.Work.AssignNoticeTransfer)
	return e, db, choi
}

func jwtCookieUser(t *testing.T, u *model.User) *http.Cookie {
	t.Helper()
	secret := []byte("cs-system-jwt-secret-2026")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": u.UserID, "username": u.Username, "role": u.Role,
		"name": u.FullName, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "token", Value: s, Path: "/"}
}

func doGetWith(t *testing.T, e *echo.Echo, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	for _, ck := range cookies {
		if ck != nil {
			req.AddCookie(ck)
		}
	}
	e.ServeHTTP(rec, req)
	return rec
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == name {
			return ck
		}
	}
	return nil
}

func insertUnseenNotice(t *testing.T, db *sql.DB, userID, sourceID, title string, at time.Time) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO work_assign_notices (notice_id, user_id, source_type, source_id, assigned_by, assigned_at)
		VALUES (?,?,?,?,?,?)`,
		"WAN-"+sourceID, userID, "as", sourceID, "U-admin", at.Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, symptom, status, assigned_to)
		VALUES (?,?,?,?,?,?,?)`,
		sourceID, sourceID, "C1", at.Format("2006-01-02 15:04:05"), title, "open", "최혜영")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAssignNoticeModalAndBadge(t *testing.T) {
	e, db, choi := newAssignNoticeServer(t)
	old := time.Now().AddDate(0, 0, -3)
	insertUnseenNotice(t, db, choi.UserID, "R2609-012", "예약대출기 오류", old)
	insertUnseenNotice(t, db, choi.UserID, "R2609-015", "로그인 오류", old)

	ck := jwtCookieUser(t, choi)
	first := doGetWith(t, e, "/work", ck)
	if first.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", first.Code, first.Body.String())
	}
	body := first.Body.String()
	if !strings.Contains(body, "새로 배정된 업무 2건") {
		t.Fatal("로그인 직후 모달이 없다")
	}
	if !strings.Contains(body, "R2609-012") || !strings.Contains(body, "부여군립도서관") {
		t.Fatal("3일 전 배정이 모달에 없다")
	}
	if !strings.Contains(body, "bg-red-500") || !strings.Contains(body, ">2</span>") {
		t.Fatal("사이드바 안 본 건수 뱃지가 없다")
	}
	shown := cookieNamed(first, assignNoticeShownCookie)
	if shown == nil || shown.Value != time.Now().Format("2006-01-02") {
		t.Fatal("모달 표시 쿠키가 없다")
	}
	var seen int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_assign_notices WHERE user_id=? AND seen_at IS NOT NULL`, choi.UserID).Scan(&seen); err != nil {
		t.Fatal(err)
	}
	if seen != 0 {
		t.Fatal("조회가 seen_at 을 채웠다")
	}

	second := doGetWith(t, e, "/work", ck, shown)
	if second.Code != http.StatusOK {
		t.Fatalf("second status=%d", second.Code)
	}
	if strings.Contains(second.Body.String(), "새로 배정된 업무") {
		t.Fatal("같은 날 모달이 다시 떴다")
	}
	if !strings.Contains(second.Body.String(), "bg-red-500") {
		t.Fatal("모달을 닫아도 뱃지가 사라졌다")
	}

	later := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/work/assign-notices/later", nil)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	req.AddCookie(shown)
	e.ServeHTTP(later, req)
	if later.Code != http.StatusSeeOther {
		t.Fatalf("later status=%d", later.Code)
	}
	laterCk := cookieNamed(later, assignNoticeShownCookie)
	if laterCk == nil {
		laterCk = shown
	}
	third := doGetWith(t, e, "/work", ck, laterCk)
	if strings.Contains(third.Body.String(), "새로 배정된 업무") {
		t.Fatal("나중에 이후에도 모달이 떴다")
	}
	if !strings.Contains(third.Body.String(), "bg-red-500") {
		t.Fatal("나중에 눌러도 뱃지가 사라졌다")
	}
}

func TestAssignNoticeSelfSkippedAndOtherCreated(t *testing.T) {
	e, db, choi := newAssignNoticeServer(t)
	rec := doForm(t, e, "/as", url.Values{
		"customer_id":      {"C1"},
		"symptom":          {"자기배정"},
		"received_by":      {"관리자"},
		"assigned_to":      {"관리자"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("self create status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_assign_notices`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("자기 배정 알림 n=%d", n)
	}

	rec = doForm(t, e, "/as", url.Values{
		"customer_id":      {"C1"},
		"symptom":          {"예약대출기 오류"},
		"received_by":      {"관리자"},
		"assigned_to":      {"최혜영"},
		"receipt_datetime": {time.Now().Format("2006-01-02T15:04")},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("assign create status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_assign_notices WHERE user_id=? AND seen_at IS NULL`, choi.UserID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("다른 사람 배정 알림 n=%d", n)
	}

	page := doGetWith(t, e, "/work", jwtCookieUser(t, choi))
	if !strings.Contains(page.Body.String(), "새로 배정된 업무 1건") {
		t.Fatal("배정받은 로그인에 모달이 없다")
	}
}
