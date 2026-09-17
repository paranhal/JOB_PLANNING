package handler

import (
	"bytes"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/imageproc"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func postKBFile(t *testing.T, e *echo.Echo, kbID, filename string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("ref_type", model.RefTypeASKB)
	_ = w.WriteField("ref_id", kbID)
	_ = w.WriteField("redirect", "/as/knowledge?q="+urlQuery("예약"))
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

func urlQuery(s string) string {
	return strings.ReplaceAll(s, " ", "+")
}

func TestKBPhotoResizedVideoKeptRaw(t *testing.T) {
	e, h, asRepo, attachRepo, _ := newASActionFixture(t)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/knowledge", h.AS.Knowledge)

	kb, err := asRepo.PublishKB(repository.KBWrite{
		ActionText: "방화벽 예외를 등록해야 근본 해결", AuthorName: "양기헌",
		Symptom: "예약대출기 투입 목록",
	})
	if err != nil {
		t.Fatal(err)
	}

	png := makePNG(t, 2000, 1000)
	rec := postKBFile(t, e, kb.KBID, "화면.png", png)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("photo status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	mp4 := []byte("ftypisomfake-mp4-body")
	rec = postKBFile(t, e, kb.KBID, "조치.mp4", mp4)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("video status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}

	items, err := attachRepo.ListByRef(model.RefTypeASKB, kb.KBID)
	if err != nil || len(items) != 2 {
		t.Fatalf("n=%d err=%v", len(items), err)
	}
	var photo, video *model.Attachment
	for i := range items {
		it := items[i]
		if it.IsVideo() {
			video = &it
		} else if it.IsImage() {
			photo = &it
		}
	}
	if photo == nil || video == nil {
		t.Fatalf("photo=%v video=%v", photo, video)
	}
	if _, err := os.Stat(imageproc.ThumbPath(photo.FilePath)); err != nil {
		t.Fatal("사진 썸네일이 없다")
	}
	f, err := os.Open(photo.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := jpeg.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != imageproc.FullMaxEdge && decoded.Bounds().Dy() != imageproc.FullMaxEdge {
		t.Fatalf("축소 안 됨 %v", decoded.Bounds())
	}
	if _, err := os.Stat(imageproc.ThumbPath(video.FilePath)); err == nil {
		t.Fatal("동영상에 썸네일을 만들면 안 된다")
	}
	raw, err := os.ReadFile(video.FilePath)
	if err != nil || !bytes.Equal(raw, mp4) {
		t.Fatalf("동영상 원본이 바뀌었다 n=%d", len(raw))
	}
	slash := filepath.ToSlash(video.FilePath)
	if !strings.Contains(slash, "/as_kb/"+kb.KBID+"/") {
		t.Fatalf("경로 %s", video.FilePath)
	}

	dl := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/attachments/"+video.AttachmentID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(dl, req)
	if dl.Code != http.StatusOK {
		t.Fatalf("video get %d", dl.Code)
	}
	cd := dl.Header().Get("Content-Disposition")
	if !strings.Contains(strings.ToLower(cd), "inline") {
		t.Fatalf("재생용 inline 이 아니다: %s", cd)
	}

	page := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, "http://localhost/as/knowledge?q="+urlQuery("예약대출기"), nil)
	preq.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, preq)
	body := page.Body.String()
	if !strings.Contains(body, "<video") || !strings.Contains(body, "controls") {
		t.Fatal("브라우저 재생 video 태그가 없다")
	}
	if !strings.Contains(body, "동영상에는 화면 사진도 한 장 같이 올려 주세요") {
		t.Fatal("사진 권고 문구가 없다")
	}
	if !strings.Contains(body, "/attachments/"+video.AttachmentID) {
		t.Fatal("동영상 src 가 첨부 URL 이 아니다")
	}
}

func TestKBVideoRejectsOversizeAndThirdAndAVI(t *testing.T) {
	e, h, asRepo, attachRepo, _ := newASActionFixture(t)
	kb, err := asRepo.PublishKB(repository.KBWrite{ActionText: "지식 본문 열 글자 이상", AuthorName: "관리자"})
	if err != nil {
		t.Fatal(err)
	}
	fh := &multipart.FileHeader{Filename: "big.mp4", Size: 82 << 20, Header: textproto.MIMEHeader{}}
	err = h.Attachment.saveKBFiles(kb.KBID, "", []*multipart.FileHeader{fh})
	if err == nil || !strings.Contains(err.Error(), "50MB 까지 올릴 수 있습니다 (지금 82MB)") {
		t.Fatalf("상한 문구: %v", err)
	}

	tiny := []byte("mp4")
	if rec := postKBFile(t, e, kb.KBID, "a.mp4", tiny); rec.Code != http.StatusSeeOther {
		t.Fatalf("v1 %d", rec.Code)
	}
	if rec := postKBFile(t, e, kb.KBID, "b.mp4", tiny); rec.Code != http.StatusSeeOther {
		t.Fatalf("v2 %d", rec.Code)
	}
	third := postKBFile(t, e, kb.KBID, "c.mp4", tiny)
	loc := third.Header().Get("Location")
	unesc, _ := url.QueryUnescape(loc)
	if third.Code != http.StatusSeeOther || !strings.Contains(unesc, "2개") {
		t.Fatalf("3번째 동영상: status=%d loc=%s", third.Code, loc)
	}

	avi := postKBFile(t, e, kb.KBID, "clip.avi", []byte("RIFF"))
	if avi.Code != http.StatusSeeOther {
		t.Fatalf("avi status=%d", avi.Code)
	}
	items, _ := attachRepo.ListByRef(model.RefTypeASKB, kb.KBID)
	var sawAVI, videos int
	for _, it := range items {
		if it.IsVideo() {
			videos++
		}
		if strings.HasSuffix(strings.ToLower(it.FileName), ".avi") {
			sawAVI++
			if it.IsVideo() {
				t.Fatal("avi 를 동영상으로 받으면 안 된다")
			}
		}
	}
	if videos != 2 || sawAVI != 1 {
		t.Fatalf("videos=%d avi=%d items=%d", videos, sawAVI, len(items))
	}
}

func TestKBAttachSourceHasNoFFmpeg(t *testing.T) {
	_, this, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(this), "attachment.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(b)), "ffmpeg") {
		t.Fatal("ffmpeg 를 쓰면 안 된다")
	}
}

func TestSystemPageShowsUploadSize(t *testing.T) {
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "sys.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))
	up := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(up, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(up, "a.bin"), bytes.Repeat([]byte("x"), 2048), 0644); err != nil {
		t.Fatal(err)
	}
	h := New(db)
	h.System.uploadDir = up
	e := echo.New()
	e.Renderer = NewRenderer()
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/admin/system", h.System.Page, h.Auth.RequireAdminOnly)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/system", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "첨부 총 용량") {
		t.Fatal("관리자 화면에 첨부 용량이 없다")
	}
	if !strings.Contains(body, "2KB") && !strings.Contains(body, "2048") {
		t.Fatalf("용량 숫자 없음: %s", body)
	}
}

func TestKBAttachRejectsPhotoOverHTTP(t *testing.T) {
	_, h, asRepo, _, _ := newASActionFixture(t)
	kb, err := asRepo.PublishKB(repository.KBWrite{ActionText: "지식 본문 열 글자 이상", AuthorName: "관리자"})
	if err != nil {
		t.Fatal(err)
	}
	fh := &multipart.FileHeader{Filename: "big.jpg", Size: 12 << 20, Header: textproto.MIMEHeader{}}
	err = h.Attachment.saveKBFiles(kb.KBID, "", []*multipart.FileHeader{fh})
	if err == nil || !strings.Contains(err.Error(), "10MB 까지 올릴 수 있습니다 (지금 12MB)") {
		t.Fatalf("사진 상한: %v", err)
	}
}
