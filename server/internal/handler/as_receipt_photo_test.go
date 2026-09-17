package handler

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/imageproc"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newReceiptPhotoFixture(t *testing.T) (*echo.Echo, *Handler, *repository.ASRepo, *repository.AttachmentRepo, string) {
	t.Helper()
	e, h, asRepo, attachRepo, asID := newASActionFixture(t)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as", h.AS.List)
	g.GET("/as/new", h.AS.New)
	g.POST("/as", h.AS.Create)
	g.GET("/as/:id/edit", h.AS.Edit)
	g.GET("/as/:id", h.AS.Show)
	g.POST("/attachments/:id/promote-asset", h.Attachment.PromoteToAsset)
	g.POST("/attachments/:id/delete", h.Attachment.Delete)
	g.POST("/attachments/:id/keywords", h.Attachment.UpdateKeywords)
	return e, h, asRepo, attachRepo, asID
}

func postReceiptPhoto(t *testing.T, e *echo.Echo, asID, filename string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("ref_type", model.RefTypeASReceipt)
	_ = w.WriteField("ref_id", asID)
	_ = w.WriteField("keywords", "오류 화면")
	_ = w.WriteField("redirect", "/as/"+asID)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(body)
	_ = w.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/attachments", &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 80, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestASReceiptPhotoStoredSeparatelyAndResized(t *testing.T) {
	e, h, _, attachRepo, asID := newReceiptPhotoFixture(t)

	orig := makePNG(t, 2000, 1000)
	rec := postReceiptPhoto(t, e, asID, "증상.png", orig)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("업로드 실패: status=%d body=%s", rec.Code, rec.Body.String())
	}

	items, err := attachRepo.ListReceiptPhotos(asID)
	if err != nil || len(items) != 1 {
		t.Fatalf("접수 사진 n=%d err=%v", len(items), err)
	}
	if items[0].RefType != model.AttachRefAS {
		t.Fatalf("DB ref_type=%q want as", items[0].RefType)
	}
	att := items[0]
	if att.Keywords != "오류 화면" {
		t.Fatalf("메모: %q", att.Keywords)
	}
	slash := filepath.ToSlash(att.FilePath)
	if !strings.Contains(slash, "/as/"+asID+"/receipt/") {
		t.Fatalf("저장 경로: %q", att.FilePath)
	}
	if _, err := os.Stat(att.FilePath); err != nil {
		t.Fatalf("파일 없음: %v", err)
	}
	thumb := imageproc.ThumbPath(att.FilePath)
	if _, err := os.Stat(thumb); err != nil {
		t.Fatalf("썸네일 없음: %v", err)
	}
	f, err := os.Open(att.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != imageproc.FullMaxEdge {
		t.Fatalf("긴 변 %d want %d", decoded.Bounds().Dx(), imageproc.FullMaxEdge)
	}

	show := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req)
	if show.Code != http.StatusOK {
		t.Fatalf("상세: status=%d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, "증상 사진") {
		t.Error("접수 상세에 증상 사진 영역이 없다")
	}
	if !strings.Contains(body, "오류 화면") {
		t.Error("사진 메모가 없다")
	}
	if strings.Contains(body, "자산 사진으로") {
		t.Error("자산 미연결인데 승격 버튼이 있다")
	}

	action := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(action, req2)
	abody := action.Body.String()
	if !strings.Contains(abody, "접수 시 증상 사진") {
		t.Error("조치 화면 상단에 접수 사진이 없다")
	}
	if strings.Contains(abody, "사진 추가") || strings.Contains(abody, "자산 사진으로") {
		t.Error("조치 화면은 읽기 전용이어야 한다")
	}
	if strings.Contains(abody, h.Attachment.uploadDir) {
		t.Error("저장 경로 노출")
	}

	list := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "http://localhost/as", nil)
	req3.AddCookie(jwtCookie(t))
	e.ServeHTTP(list, req3)
	if !strings.Contains(list.Body.String(), "📷 1") {
		t.Error("목록에 사진 뱃지가 없다")
	}
}

func TestASReceiptPhotoPDFKeepsName(t *testing.T) {
	e, _, _, attachRepo, asID := newReceiptPhotoFixture(t)
	rec := postReceiptPhoto(t, e, asID, "접수서_스캔.pdf", []byte("%PDF-1.4 dummy"))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	items, _ := attachRepo.ListReceiptPhotos(asID)
	if len(items) != 1 || items[0].FileName != "접수서_스캔.pdf" {
		t.Fatalf("pdf: %+v", items)
	}
	if _, err := os.Stat(imageproc.ThumbPath(items[0].FilePath)); err == nil {
		t.Fatal("pdf에 썸네일이 생기면 안 된다")
	}
	show := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req)
	if !strings.Contains(show.Body.String(), "접수서_스캔.pdf") {
		t.Error("파일명이 없다")
	}
}

func TestASReceiptPhotoClosedLocked(t *testing.T) {
	e, _, asRepo, _, asID := newReceiptPhotoFixture(t)
	got, _ := asRepo.GetByID(asID)
	got.Status = "completed"
	now := time.Now()
	got.CompleteDatetime = &now
	if err := asRepo.Update(got); err != nil {
		t.Fatal(err)
	}
	rec := postReceiptPhoto(t, e, asID, "x.png", makePNG(t, 40, 20))
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusSeeOther {
		t.Fatalf("잠금 status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Code == http.StatusSeeOther {
		loc := rec.Header().Get("Location")
		if !strings.Contains(loc, "err=") {
			t.Fatalf("완료 건 업로드가 성공하면 안 된다: loc=%s", loc)
		}
	}
}

func TestASReceiptPhotoPromoteCopiesToAsset(t *testing.T) {
	e, h, asRepo, attachRepo, asID := newReceiptPhotoFixture(t)

	asset := &model.Asset{CustomerID: "cust_a", ProductName: "게이트", ModelName: "GT-1"}
	if err := h.AS.assetRepo.Create(asset); err != nil {
		t.Fatal(err)
	}
	got, _ := asRepo.GetByID(asID)
	got.AssetID = asset.AssetID
	if err := asRepo.UpdateReceipt(got); err != nil {
		t.Fatal(err)
	}

	rec := postReceiptPhoto(t, e, asID, "현장.png", makePNG(t, 800, 400))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("업로드 status=%d", rec.Code)
	}
	photos, _ := attachRepo.ListReceiptPhotos(asID)
	if len(photos) != 1 {
		t.Fatalf("사진 n=%d", len(photos))
	}

	form := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/attachments/"+photos[0].AttachmentID+"/promote-asset",
		strings.NewReader("redirect=/as/"+asID))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(form, req)
	if form.Code != http.StatusSeeOther {
		t.Fatalf("승격 status=%d body=%s", form.Code, form.Body.String())
	}
	assets, _ := attachRepo.ListByRef(model.RefTypeAsset, asset.AssetID)
	if len(assets) != 1 {
		t.Fatalf("자산 사진 n=%d", len(assets))
	}
	if assets[0].Keywords != "오류 화면" {
		t.Fatalf("키워드 미복사: %q", assets[0].Keywords)
	}
	if assets[0].SlotNo < 1 || assets[0].SlotNo > 3 {
		t.Fatalf("슬롯: %d", assets[0].SlotNo)
	}
	remain, _ := attachRepo.ListReceiptPhotos(asID)
	if len(remain) != 1 {
		t.Fatal("승격은 복사여야 한다")
	}
}

func TestASReceiptPhotoMaxTen(t *testing.T) {
	e, _, _, attachRepo, asID := newReceiptPhotoFixture(t)
	small := makePNG(t, 20, 10)
	for i := 0; i < 10; i++ {
		rec := postReceiptPhoto(t, e, asID, "p.png", small)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%d번째 실패 status=%d", i+1, rec.Code)
		}
	}
	rec := postReceiptPhoto(t, e, asID, "overflow.png", small)
	if rec.Code == http.StatusSeeOther {
		items, _ := attachRepo.ListReceiptPhotos(asID)
		if len(items) > 10 {
			t.Fatalf("11장 저장됨: %d", len(items))
		}
	}
	items, _ := attachRepo.ListReceiptPhotos(asID)
	if len(items) != 10 {
		t.Fatalf("최대 10장: n=%d", len(items))
	}
}

func TestASNewFormHasReceiptPhotoInput(t *testing.T) {
	e, _, _, _, _ := newReceiptPhotoFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/new", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="receipt_photo"`) {
		t.Fatal("신규 접수 화면에 파일 입력이 없다")
	}
	if !strings.Contains(body, `enctype="multipart/form-data"`) {
		t.Fatal("신규 접수 폼이 multipart가 아니다")
	}
}

func TestASEditPhotosNotNestedInReceiptForm(t *testing.T) {
	e, _, _, _, asID := newReceiptPhotoFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/edit", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	start := strings.Index(body, `id="as-form"`)
	if start < 0 {
		t.Fatal("as-form 없음")
	}
	end := strings.Index(body[start:], "</form>")
	if end < 0 {
		t.Fatal("as-form 닫힘 없음")
	}
	inner := body[start : start+end]
	if strings.Contains(inner, `action="/attachments"`) {
		t.Fatal("접수 사진 폼이 접수 저장 폼 안에 중첩되면 브라우저가 파일 입력을 무시한다")
	}
	if !strings.Contains(body, `name="file"`) || !strings.Contains(body, `ref_type" value="as_receipt"`) {
		t.Fatal("수정 화면에 접수 사진 업로드가 없다")
	}
}

func TestASCreateUploadsReceiptPhoto(t *testing.T) {
	e, _, _, attachRepo, _ := newReceiptPhotoFixture(t)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("customer_id", "cust_a")
	_ = w.WriteField("symptom", "게이트 오류")
	_ = w.WriteField("received_by", "테스터")
	_ = w.WriteField("receipt_datetime", time.Now().Format("2006-01-02T15:04"))
	_ = w.WriteField("photo_memo", "오류 화면")
	fw, err := w.CreateFormFile("receipt_photo", "증상.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(makePNG(t, 40, 20))
	_ = w.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as", &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	asID := strings.TrimPrefix(loc, "/as/")
	if i := strings.Index(asID, "?"); i >= 0 {
		asID = asID[:i]
	}
	if asID == "" || strings.Contains(asID, "/") {
		t.Fatalf("redirect: %s", loc)
	}
	items, _ := attachRepo.ListReceiptPhotos(asID)
	if len(items) != 1 {
		t.Fatalf("접수 사진 n=%d loc=%s", len(items), loc)
	}
	if items[0].RefType != model.AttachRefAS {
		t.Fatalf("DB ref_type=%q want as", items[0].RefType)
	}
}
