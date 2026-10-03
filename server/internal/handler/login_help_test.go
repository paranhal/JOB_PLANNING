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

func loginHelpApp(t *testing.T) (*echo.Echo, *sql.DB, *Handler) {
	t.Helper()
	t.Setenv("MAIL_ENABLED", "true")
	t.Setenv("MAIL_ADMIN_TO", "admin@example.com")
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "login-help.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	if err := users.EnsureAdmin(HashPassword("adminpass12")); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	h.Auth.cfg.MailEnabled = true
	h.Auth.cfg.MailAdminTo = "admin@example.com"
	e.GET("/login", h.Auth.LoginPage)
	e.POST("/login/help", h.Auth.LoginHelp)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.InjectAssignNotices)
	g.GET("/", h.Dashboard)
	g.POST("/admin/login-help/:id/handle", h.HandleLoginHelp)
	return e, db, h
}

func TestLoginPageHasHelpForm(t *testing.T) {
	e, _, _ := loginHelpApp(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	e.ServeHTTP(rec, req)
	body := rec.Body.String()
	if strings.Contains(body, "admin / admin") {
		t.Fatal("초기 계정 문구")
	}
	if !strings.Contains(body, "아이디·비밀번호를 잊으셨나요?") {
		t.Fatal("문의 링크 없음")
	}
	if !strings.Contains(body, `name="name"`) || !strings.Contains(body, `name="mobile"`) {
		t.Fatal("이름·핸드폰 칸 없음")
	}
}

func TestLoginHelpSameCopyAndRate(t *testing.T) {
	e, db, _ := loginHelpApp(t)
	post := func() *httptest.ResponseRecorder {
		form := url.Values{"name": {"없는사람"}, "mobile": {"01099998888"}, "kind": {"unknown_id"}}
		req := httptest.NewRequest(http.MethodPost, "/login/help", strings.NewReader(form.Encode()))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	rec := post()
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "접수했습니다. 관리자가 연락드립니다.") {
		t.Fatal("성공 문구 없음")
	}
	if strings.Contains(rec.Body.String(), "없는 사람") {
		t.Fatal("존재 여부 누설")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM login_help_requests`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows=%d err=%v", n, err)
	}
	var title string
	if err := db.QueryRow(`SELECT title FROM work_tasks WHERE source_type='login_help'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "계정 문의: 없는사람" {
		t.Fatalf("title=%s", title)
	}
	var mailN int
	_ = db.QueryRow(`SELECT COUNT(*) FROM mail_outbox`).Scan(&mailN)
	if mailN < 1 {
		t.Fatal("메일 큐가 비었다")
	}
	rec2 := post()
	if !strings.Contains(rec2.Body.String(), "같은 번호로는") {
		t.Fatal("10분 제한이 없다")
	}
}

func TestLoginHelpHoneypotSkipsInsert(t *testing.T) {
	e, db, _ := loginHelpApp(t)
	form := url.Values{"name": {"봇"}, "mobile": {"01011112222"}, "website": {"http://spam"}}
	req := httptest.NewRequest(http.MethodPost, "/login/help", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "접수했습니다. 관리자가 연락드립니다.") {
		t.Fatal("허니팟도 같은 문구")
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM login_help_requests`).Scan(&n)
	if n != 0 {
		t.Fatalf("허니팟이 저장됐다 n=%d", n)
	}
}

func TestLoginHelpHandleCompletesTask(t *testing.T) {
	e, db, h := loginHelpApp(t)
	form := url.Values{"name": {"홍길동"}, "mobile": {"01012345678"}, "kind": {"reset_pw"}}
	req := httptest.NewRequest(http.MethodPost, "/login/help", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	e.ServeHTTP(httptest.NewRecorder(), req)
	var id, taskID string
	if err := db.QueryRow(`SELECT request_id, task_id FROM login_help_requests`).Scan(&id, &taskID); err != nil {
		t.Fatal(err)
	}
	admin, err := h.Auth.userRepo.GetByUsername("admin")
	if err != nil || admin == nil {
		t.Fatal("admin")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": admin.UserID, "username": admin.Username, "role": model.RoleVisionAdmin,
		"name": admin.FullName, "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, _ := token.SignedString([]byte("cs-system-jwt-secret-2026"))
	hf := url.Values{"handle_note": {"초기화함"}, "status": {"done"}, "back": {"/"}}
	hreq := httptest.NewRequest(http.MethodPost, "/admin/login-help/"+id+"/handle", strings.NewReader(hf.Encode()))
	hreq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	hreq.AddCookie(&http.Cookie{Name: "token", Value: s, Path: "/"})
	hrec := httptest.NewRecorder()
	e.ServeHTTP(hrec, hreq)
	var st, note string
	if err := db.QueryRow(`SELECT status, handle_note FROM login_help_requests WHERE request_id=?`, id).Scan(&st, &note); err != nil {
		t.Fatal(err)
	}
	if st != "done" || note != "초기화함" {
		t.Fatalf("st=%s note=%s", st, note)
	}
	var tst string
	if err := db.QueryRow(`SELECT status FROM work_tasks WHERE task_id=?`, taskID).Scan(&tst); err != nil {
		t.Fatal(err)
	}
	if tst != model.WBTaskComplete {
		t.Fatalf("task status=%s", tst)
	}
}

func TestPurgeHandledLoginHelp(t *testing.T) {
	_, db, _ := loginHelpApp(t)
	_, err := db.Exec(`INSERT INTO login_help_requests (request_id,name,mobile,status,handled_at,created_at)
		VALUES ('old','옛','010','done', datetime('now','-91 days'), datetime('now','-91 days'))`)
	if err != nil {
		t.Fatal(err)
	}
	n, err := repository.PurgeHandledLoginHelp(db, 90*24*time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("purged=%d err=%v", n, err)
	}
}
