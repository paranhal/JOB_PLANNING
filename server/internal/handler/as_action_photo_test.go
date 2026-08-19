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
	req.AddCookie(jwtCookie(t))
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
	if !strings.Contains(body, "조치 사진") || !strings.Contains(body, "교체 후") {
		t.Error("조치 화면에 사진·메모가 없다")
	}
	if !strings.Contains(body, "data-photo-gallery=\"action-photo\"") {
		t.Error("조치 사진 갤러리가 없다")
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
