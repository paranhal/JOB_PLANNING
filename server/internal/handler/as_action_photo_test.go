package handler

import (
	"bytes"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/imageproc"
	"customer-support/internal/model"
)

func postActionPhotoEcho(t *testing.T, e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, asID, filename, keywords string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	return postActionPhotoEchoCookie(t, e, jwtCookie(t), asID, filename, keywords, body)
}

func postActionPhotoEchoCookie(t *testing.T, e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, cookie *http.Cookie, asID, filename, keywords string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("ref_type", model.RefTypeASActionPhoto)
	_ = w.WriteField("ref_id", asID)
	_ = w.WriteField("keywords", keywords)
	_ = w.WriteField("redirect", "/as/"+asID+"/action")
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(body)
	_ = w.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/attachments", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(cookie)
	e.ServeHTTP(rec, req)
	return rec
}

func TestASActionPhotoStoredSeparatelyAndResized(t *testing.T) {
	e, h, _, attachRepo, asID := newASActionFixture(t)

	orig := makePNG(t, 2000, 1000)
	rec := postActionPhotoEcho(t, e, asID, "조치후.png", "교체 후", orig)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("업로드 실패: status=%d body=%s", rec.Code, rec.Body.String())
	}

	items, err := attachRepo.ListByRef(model.RefTypeASActionPhoto, asID)
	if err != nil || len(items) != 1 {
		t.Fatalf("조치 사진 n=%d err=%v", len(items), err)
	}
	docs, _ := attachRepo.ListByRef(model.RefTypeAS, asID)
	if len(docs) != 0 {
		t.Fatalf("문서 첨부에 섞이면 안 된다: n=%d", len(docs))
	}
	receipts, _ := attachRepo.ListByRef(model.RefTypeASReceipt, asID)
	if len(receipts) != 0 {
		t.Fatalf("접수 사진에 섞이면 안 된다: n=%d", len(receipts))
	}
	att := items[0]
	if att.Keywords != "교체 후" {
		t.Fatalf("메모: %q", att.Keywords)
	}
	slash := filepath.ToSlash(att.FilePath)
	if !strings.Contains(slash, "/as/"+asID+"/action/") {
		t.Fatalf("저장 경로: %q", att.FilePath)
	}
	if _, err := os.Stat(att.FilePath); err != nil {
		t.Fatalf("파일 없음: %v", err)
	}
	if _, err := os.Stat(imageproc.ThumbPath(att.FilePath)); err != nil {
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
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req)
	body := show.Body.String()
	if !strings.Contains(body, "첨부") || !strings.Contains(body, "교체 후") {
		t.Error("조치 화면에 사진·메모가 없다")
	}
	if !strings.Contains(body, `data-action-attach`) {
		t.Error("통합 첨부 영역이 없다")
	}
	if strings.Contains(body, `data-photo-gallery="action-photo"`) {
		t.Error("분리된 조치 사진 갤러리가 남아 있다")
	}
	if strings.Contains(body, "사진 추가") {
		t.Error("접수 사진 추가 버튼이 조치 화면에 있으면 안 된다")
	}
	if !strings.Contains(body, ">올리기<") {
		t.Error("조치 사진 올리기 버튼이 없다")
	}
	if strings.Contains(body, h.Attachment.uploadDir) {
		t.Error("저장 경로 노출")
	}
}

func TestASActionPhotoRejectsPDFAndCapsAtThree(t *testing.T) {
	e, _, _, attachRepo, asID := newASActionFixture(t)

	pdf := postActionPhotoEcho(t, e, asID, "로그.pdf", "", []byte("%PDF-1.4 dummy"))
	if pdf.Code == http.StatusSeeOther {
		loc := pdf.Header().Get("Location")
		if !strings.Contains(loc, "err=") {
			t.Fatalf("PDF가 받아졌다: loc=%s", loc)
		}
	} else if pdf.Code != http.StatusBadRequest && pdf.Code != http.StatusSeeOther {
		t.Fatalf("PDF status=%d", pdf.Code)
	}
	items, _ := attachRepo.ListByRef(model.RefTypeASActionPhoto, asID)
	if len(items) != 0 {
		t.Fatalf("PDF가 저장됐다 n=%d", len(items))
	}

	for i := 0; i < 3; i++ {
		rec := postActionPhotoEcho(t, e, asID, "p.png", "", makePNG(t, 40, 20))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("장 %d 실패 status=%d", i+1, rec.Code)
		}
		if strings.Contains(rec.Header().Get("Location"), "err=") {
			t.Fatalf("장 %d 오류: %s", i+1, rec.Header().Get("Location"))
		}
	}
	over := postActionPhotoEcho(t, e, asID, "extra.png", "", makePNG(t, 40, 20))
	if over.Code == http.StatusSeeOther && !strings.Contains(over.Header().Get("Location"), "err=") {
		t.Fatal("4장째가 받아졌다")
	}
	items, _ = attachRepo.ListByRef(model.RefTypeASActionPhoto, asID)
	if len(items) != 3 {
		t.Fatalf("최대 3장: n=%d", len(items))
	}
}

func TestASActionPhotoClosedLocked(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	got, _ := asRepo.GetByID(asID)
	got.Status = "completed"
	now := time.Now()
	got.CompleteDatetime = &now
	if err := asRepo.Update(got); err != nil {
		t.Fatal(err)
	}
	rec := postActionPhotoEcho(t, e, asID, "x.png", "", makePNG(t, 40, 20))
	if rec.Code == http.StatusSeeOther && !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatal("완료 건 조치 사진 업로드가 성공하면 안 된다")
	}
}

func TestAttachErrMessage(t *testing.T) {
	if got := attachErrMessage("attach_photo_max"); got != "사진은 최대 3장입니다" {
		t.Fatalf("max: %q", got)
	}
	if got := attachErrMessage("attach_forbidden"); got != "권한이 없습니다" {
		t.Fatalf("forbidden: %q", got)
	}
	if got := attachErrMessage("attach_file"); got != "파일이 필요합니다" {
		t.Fatalf("file: %q", got)
	}
	if attachErrMessage("action_required") != "" {
		t.Fatal("조치 저장 오류를 첨부 배너로 쓰면 안 된다")
	}
}

func TestASActionPhotoTechThreeAndMaxBanner(t *testing.T) {
	e, _, _, attachRepo, asID := newASActionFixture(t)
	tech := jwtCookieRole(t, "tech")
	for i := 0; i < 3; i++ {
		rec := postActionPhotoEchoCookie(t, e, tech, asID, "p.png", "", makePNG(t, 40, 20))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("장 %d status=%d body=%s", i+1, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Header().Get("Location"), "err=") {
			t.Fatalf("장 %d 오류: %s", i+1, rec.Header().Get("Location"))
		}
	}
	items, err := attachRepo.ListByRef(model.RefTypeASActionPhoto, asID)
	if err != nil || len(items) != 3 {
		t.Fatalf("n=%d err=%v", len(items), err)
	}

	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(tech)
	e.ServeHTTP(page, req)
	body := page.Body.String()
	if thumb := items[0].ThumbURL(); thumb != "" && !strings.Contains(body, thumb) {
		t.Errorf("썸네일 URL 없음: %s", thumb)
	}
	if !strings.Contains(body, `value="as_action_photo"`) {
		t.Error("종류 값이 as_action_photo 가 아니다")
	}

	over := postActionPhotoEchoCookie(t, e, tech, asID, "extra.png", "", makePNG(t, 40, 20))
	loc := over.Header().Get("Location")
	if over.Code != http.StatusSeeOther || !strings.Contains(loc, "err=") {
		t.Fatalf("4장째 status=%d loc=%s body=%s", over.Code, loc, over.Body.String())
	}
	if !strings.Contains(loc, "attach_photo_max") {
		t.Fatalf("err 코드: %s", loc)
	}
	show := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost"+loc, nil)
	req2.AddCookie(tech)
	e.ServeHTTP(show, req2)
	got := show.Body.String()
	if !strings.Contains(got, `data-attach-err`) || !strings.Contains(got, "사진은 최대 3장입니다") {
		t.Fatal("최대 3장 배너가 없다")
	}
}

func TestASActionPhotoFourAtOnceShowsMaxBanner(t *testing.T) {
	e, _, _, attachRepo, asID := newASActionFixture(t)
	tech := jwtCookieRole(t, "tech")
	png := makePNG(t, 40, 20)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("ref_type", model.RefTypeASActionPhoto)
	_ = w.WriteField("ref_id", asID)
	_ = w.WriteField("redirect", "/as/"+asID+"/action")
	for i := 0; i < 4; i++ {
		fw, err := w.CreateFormFile("file", "p.png")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(png)
	}
	_ = w.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/attachments", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(tech)
	e.ServeHTTP(rec, req)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.Contains(loc, "attach_photo_max") {
		t.Fatalf("4장 동시 status=%d loc=%s body=%s", rec.Code, loc, rec.Body.String())
	}
	items, _ := attachRepo.ListByRef(model.RefTypeASActionPhoto, asID)
	if len(items) != 0 {
		t.Fatalf("한도 초과 요청이 일부라도 저장되면 안 된다 n=%d", len(items))
	}
	show := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost"+loc, nil)
	req2.AddCookie(tech)
	e.ServeHTTP(show, req2)
	got := show.Body.String()
	if !strings.Contains(got, `data-attach-err`) || !strings.Contains(got, "사진은 최대 3장입니다") {
		t.Fatal("4장 한 번에 올리면 최대 3장 배너가 없다")
	}
}

func TestASActionAttachSilentErrorsShowBanner(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("ref_type", model.RefTypeASActionPhoto)
	_ = w.WriteField("ref_id", asID)
	_ = w.WriteField("redirect", "/as/"+asID+"/action")
	_ = w.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/attachments", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "attach_file") {
		t.Fatalf("파일없음: status=%d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	page := httptest.NewRecorder()
	reqG := httptest.NewRequest(http.MethodGet, "http://localhost"+rec.Header().Get("Location"), nil)
	reqG.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, reqG)
	if !strings.Contains(page.Body.String(), "파일이 필요합니다") || !strings.Contains(page.Body.String(), `data-attach-err`) {
		t.Error("파일 필요 배너 없음")
	}

	obs := postActionPhotoEchoCookie(t, e, jwtCookieRole(t, "observer"), asID, "p.png", "", makePNG(t, 40, 20))
	if obs.Code != http.StatusSeeOther || !strings.Contains(obs.Header().Get("Location"), "attach_forbidden") {
		t.Fatalf("옵저버: status=%d loc=%s", obs.Code, obs.Header().Get("Location"))
	}
	page2 := httptest.NewRecorder()
	reqO := httptest.NewRequest(http.MethodGet, "http://localhost"+obs.Header().Get("Location"), nil)
	reqO.AddCookie(jwtCookieRole(t, "observer"))
	e.ServeHTTP(page2, reqO)
	if !strings.Contains(page2.Body.String(), "권한이 없습니다") {
		t.Error("권한 배너 없음")
	}
}
