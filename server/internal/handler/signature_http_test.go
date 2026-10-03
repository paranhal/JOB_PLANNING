package handler

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newSignatureApp(t *testing.T) (*echo.Echo, *repository.UserRepo, *Handler, string) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "sig.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	up := filepath.Join(t.TempDir(), "uploads")
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	h.Auth.uploadDir = up
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/account", h.Auth.AccountPage)
	g.POST("/account/signature", h.Auth.AccountSignature)
	g.GET("/account/signature.png", h.Auth.AccountSignatureImage)
	g.POST("/account/signature/delete", h.Auth.AccountSignatureDelete)
	return e, users, h, up
}

func inkPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 80; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	for x := 10; x < 30; x++ {
		img.Set(x, 20, color.RGBA{R: 10, G: 10, B: 10, A: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAccountSignatureSelfOnlyAndFileStore(t *testing.T) {
	e, users, _, up := newSignatureApp(t)
	u := &model.User{
		Username: "techs", PasswordHash: HashPassword("pw"), FullName: "기술사인",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-1",
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	other := &model.User{
		Username: "other", PasswordHash: HashPassword("pw"), FullName: "다른이",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-2",
	}
	if err := users.Create(other); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("user_id", other.UserID)
	fw, err := w.CreateFormFile("file", "sign.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(inkPNG(t))
	_ = w.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/account/signature", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(jwtCookieUserClaims(t, u))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "본인만") {
		t.Fatalf("남의 user_id 를 받아야 한다 code=%d", rec.Code)
	}

	body.Reset()
	w = multipart.NewWriter(&body)
	fw, _ = w.CreateFormFile("file", "sign.png")
	_, _ = fw.Write(inkPNG(t))
	_ = w.Close()
	ok := httptest.NewRecorder()
	oreq := httptest.NewRequest(http.MethodPost, "/account/signature", &body)
	oreq.Header.Set("Content-Type", w.FormDataContentType())
	oreq.AddCookie(jwtCookieUserClaims(t, u))
	e.ServeHTTP(ok, oreq)
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("본인 업로드 code=%d %s", ok.Code, ok.Body.String())
	}
	got, _ := users.GetByID(u.UserID)
	if got == nil || !strings.Contains(got.SignaturePath, u.UserID+".png") {
		t.Fatalf("경로 %q", got.SignaturePath)
	}
	if _, err := os.Stat(filepath.Join(up, "signatures", u.UserID+".png")); err != nil {
		t.Fatal(err)
	}
	otherGot, _ := users.GetByID(other.UserID)
	if otherGot != nil && strings.TrimSpace(otherGot.SignaturePath) != "" {
		t.Fatal("남의 사인 경로가 생겼다")
	}

	page := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, "/account", nil)
	preq.AddCookie(jwtCookieUserClaims(t, u))
	e.ServeHTTP(page, preq)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "사인") {
		t.Fatalf("계정 화면에 사인이 없다 code=%d", page.Code)
	}
}

func TestAccountSignatureMissingIsNotErrorOnPage(t *testing.T) {
	e, users, _, _ := newSignatureApp(t)
	u := &model.User{
		Username: "nosig", PasswordHash: HashPassword("pw"), FullName: "무사인",
		Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, Mobile: "010-1",
	}
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	req.AddCookie(jwtCookieUserClaims(t, u))
	e.ServeHTTP(page, req)
	if page.Code != http.StatusOK {
		t.Fatalf("code=%d", page.Code)
	}
	if strings.Contains(page.Body.String(), "오류") && strings.Contains(page.Body.String(), "사인") {
		t.Fatal("사인 없음이 오류로 보인다")
	}
}
