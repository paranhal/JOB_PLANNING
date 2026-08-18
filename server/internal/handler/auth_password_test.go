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
	"customer-support/internal/passwd"
	"customer-support/internal/repository"
)

func passwordTestApp(t *testing.T) (*echo.Echo, *sql.DB, *Handler, *repository.UserRepo, *repository.SettingsRepo) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "pw.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	userRepo := repository.NewUserRepo(db)
	if err := userRepo.EnsureAdmin(passwd.SHA256Legacy("admin")); err != nil {
		t.Fatal(err)
	}
	settings := repository.NewSettingsRepo(db)
	if err := settings.Set(repository.SettingASCompletedEditPassword, passwd.SHA256Legacy("as-edit")); err != nil {
		t.Fatal(err)
	}
	if err := settings.Set(repository.SettingMaintenanceDeletePassword, passwd.Hash("mnt-del")); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	e.POST("/login", h.Auth.Login)
	g.GET("/users", h.Auth.UserList)
	g.POST("/as/:id/unlock-edit", h.AS.UnlockEdit)
	g.POST("/users/as-edit-password", h.AS.UpdateCompletedEditPassword)
	g.POST("/users/mnt-delete-password", h.Auth.UpdateMaintenanceDeletePassword)
	return e, db, h, userRepo, settings
}

func postForm(t *testing.T, e *echo.Echo, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	e.ServeHTTP(rec, req)
	return rec
}

func getUsers(t *testing.T, e *echo.Echo) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/users", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("사용자 화면: status=%d body=%s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestLoginUpgradesLegacySHA256(t *testing.T) {
	e, _, _, userRepo, _ := passwordTestApp(t)

	before, err := userRepo.GetByUsername("admin")
	if err != nil || before == nil {
		t.Fatal(err)
	}
	if !passwd.NeedsRehash(before.PasswordHash) {
		t.Fatal("시드는 SHA-256이어야 함")
	}

	rec := postForm(t, e, "/login", url.Values{
		"username": {"admin"}, "password": {"admin"},
	}, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("로그인: status=%d body=%s", rec.Code, rec.Body.String())
	}

	after, _ := userRepo.GetByUsername("admin")
	if after == nil || !passwd.IsBcrypt(after.PasswordHash) {
		t.Fatalf("재해시되지 않음: %q", after.PasswordHash)
	}
	if !passwd.Verify(after.PasswordHash, "admin") {
		t.Fatal("재해시 검증 실패")
	}

	again := postForm(t, e, "/login", url.Values{
		"username": {"admin"}, "password": {"admin"},
	}, nil)
	if again.Code != http.StatusSeeOther {
		t.Fatalf("bcrypt 재로그인: status=%d", again.Code)
	}
	stable, _ := userRepo.GetByUsername("admin")
	if stable.PasswordHash != after.PasswordHash {
		t.Fatal("이미 bcrypt인 해시를 다시 씀")
	}
}

func TestLoginWrongPasswordDoesNotRehash(t *testing.T) {
	e, _, _, userRepo, _ := passwordTestApp(t)
	before, _ := userRepo.GetByUsername("admin")

	rec := postForm(t, e, "/login", url.Values{
		"username": {"admin"}, "password": {"wrong"},
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("실패 로그인: status=%d", rec.Code)
	}
	after, _ := userRepo.GetByUsername("admin")
	if after.PasswordHash != before.PasswordHash {
		t.Fatal("틀린 비밀번호인데 해시가 바뀜")
	}
}

func TestUserListShowsHashMigrationStatus(t *testing.T) {
	e, _, _, userRepo, _ := passwordTestApp(t)
	admin, _ := userRepo.GetByUsername("admin")

	body := getUsers(t, e)
	if !strings.Contains(body, "전환 진행 중") {
		t.Fatal("미완료 안내가 없다")
	}
	if !strings.Contains(body, "0/1") {
		t.Fatalf("사용자 건수: %s", body)
	}
	if !strings.Contains(body, "미전환") {
		t.Fatal("AS 수정 비밀번호 미전환 안내가 없다")
	}
	if strings.Contains(body, admin.PasswordHash) {
		t.Fatal("사용자 해시가 화면에 노출됨")
	}

	_ = postForm(t, e, "/login", url.Values{"username": {"admin"}, "password": {"admin"}}, nil)

	body = getUsers(t, e)
	if !strings.Contains(body, "1/1") {
		t.Fatalf("로그인 후 사용자 전환: %s", body)
	}
	if strings.Contains(body, "bcrypt로 전환되었습니다") {
		t.Fatal("AS 해시가 아직인데 완료로 표시됨")
	}
}

func TestUnlockEditUpgradesASPasswordHash(t *testing.T) {
	e, db, h, userRepo, settings := passwordTestApp(t)
	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"cust_pw", "테스트도서관", "테스트도서관"); err != nil {
		t.Fatal(err)
	}
	src := &model.ASReceipt{
		CustomerID: "cust_pw", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "게이트", Urgency: "high", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now().AddDate(0, 0, -2),
	}
	if err := h.AS.repo.Create(src); err != nil {
		t.Fatal(err)
	}
	src.Status = "completed"
	src.ResultCode = "done"
	src.ActionTaken = "조치"
	if err := h.AS.repo.Update(src); err != nil {
		t.Fatal(err)
	}

	before, _ := settings.Get(repository.SettingASCompletedEditPassword)
	if !passwd.NeedsRehash(before) {
		t.Fatal("AS 해시는 SHA-256이어야 함")
	}
	if strings.Contains(getUsers(t, e), before) {
		t.Fatal("AS 해시가 화면에 노출됨")
	}

	rec := postForm(t, e, "/as/"+src.ASID+"/unlock-edit", url.Values{
		"unlock_password": {"as-edit"},
		"redirect":        {"/as/" + src.ASID},
	}, jwtCookie(t))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("잠금 해제: status=%d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if strings.Contains(loc, "올바르지 않습니다") {
		t.Fatalf("비밀번호 거부: %s", loc)
	}

	after, _ := settings.Get(repository.SettingASCompletedEditPassword)
	if !passwd.IsBcrypt(after) {
		t.Fatalf("AS 해시 미전환: %q", after)
	}
	if !passwd.Verify(after, "as-edit") {
		t.Fatal("전환 후 검증 실패")
	}

	_ = postForm(t, e, "/login", url.Values{"username": {"admin"}, "password": {"admin"}}, nil)
	if u, _ := userRepo.GetByUsername("admin"); u == nil || !passwd.IsBcrypt(u.PasswordHash) {
		t.Fatal("로그인 재해시 실패")
	}
	body := getUsers(t, e)
	if !strings.Contains(body, "bcrypt로 전환되었습니다") {
		t.Fatalf("완료 안내: %s", body)
	}
}

func TestUpdateASEditPasswordAcceptsLegacyHash(t *testing.T) {
	e, _, _, _, settings := passwordTestApp(t)
	rec := postForm(t, e, "/users/as-edit-password", url.Values{
		"current_password": {"as-edit"},
		"new_password":     {"new-edit"},
		"confirm_password": {"new-edit"},
	}, jwtCookie(t))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("변경: status=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if strings.Contains(loc, "올바르지 않습니다") {
		t.Fatalf("현재 비밀번호 거부: %s", loc)
	}
	got, _ := settings.Get(repository.SettingASCompletedEditPassword)
	if !passwd.IsBcrypt(got) || !passwd.Verify(got, "new-edit") {
		t.Fatalf("변경 후 해시: %q", got)
	}
}

func TestUpdateMaintenanceDeletePassword(t *testing.T) {
	e, _, _, _, settings := passwordTestApp(t)
	rec := postForm(t, e, "/users/mnt-delete-password", url.Values{
		"current_password": {"mnt-del"},
		"new_password":     {"new-del"},
		"confirm_password": {"new-del"},
	}, jwtCookie(t))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("변경: status=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if strings.Contains(loc, "올바르지 않습니다") {
		t.Fatalf("현재 비밀번호 거부: %s", loc)
	}
	got, _ := settings.Get(repository.SettingMaintenanceDeletePassword)
	if !passwd.IsBcrypt(got) || !passwd.Verify(got, "new-del") {
		t.Fatalf("변경 후 해시: %q", got)
	}
}
