package handler

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newASActionFixture(t *testing.T) (*echo.Echo, *Handler, *repository.ASRepo, *repository.AttachmentRepo, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "action.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(
		`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"cust_a", "가나도서관", "가나도서관"); err != nil {
		t.Fatal(err)
	}
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	asRepo := repository.NewASRepo(db)
	src := &model.ASReceipt{
		CustomerID: "cust_a", ReceiptChannel: "phone", Requester: "홍길동",
		Symptom: "게이트 오작동", Urgency: "high", Priority: "normal",
		AssignedTo: "양기헌", ReceiptDatetime: time.Now(),
		Status: "in_progress",
	}
	if err := asRepo.Create(src); err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	h.Attachment.uploadDir = filepath.Join(dir, "data", "uploads")

	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/as/:id/action", h.AS.Action)
	g.POST("/as/:id/update", h.AS.Update)
	g.POST("/attachments", h.Attachment.Upload)
	g.GET("/attachments/:id", h.Attachment.Download)
	g.POST("/attachments/:id/delete", h.Attachment.Delete)
	g.POST("/attachments/:id/keywords", h.Attachment.UpdateKeywords)
	return e, h, asRepo, repository.NewAttachmentRepo(db), src.ASID
}

func TestASActionSavesWorkPlace(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)

	form := url.Values{
		"status":        {"in_progress"},
		"work_place":    {"field"},
		"process_type":  {"visit"},
		"cause_type":    {"hw"},
		"action_taken":  {"현장 점검"},
		"time_spent":    {"30"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/update",
		strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 실패: status=%d body=%s", rec.Code, rec.Body.String())
	}

	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil {
		t.Fatalf("조회 실패: %v", err)
	}
	if got.WorkPlace != model.WorkPlaceField {
		t.Fatalf("근무구분: got %q want field", got.WorkPlace)
	}

	show := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req2)
	if show.Code != http.StatusOK {
		t.Fatalf("조치 화면: status=%d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, `name="work_place" value="office"`) ||
		!strings.Contains(body, `name="work_place" value="field"`) {
		t.Error("내근/외근 선택 항목이 없다")
	}
	if !strings.Contains(body, `value="field" checked`) &&
		!strings.Contains(body, `value="field"checked`) {
		t.Error("외근이 선택되어 있어야 한다")
	}
}

func TestASActionUploadStoresNameAndPath(t *testing.T) {
	e, h, _, attachRepo, asID := newASActionFixture(t)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("ref_type", "as")
	_ = w.WriteField("ref_id", asID)
	_ = w.WriteField("redirect", "/as/"+asID+"/action")
	fw, err := w.CreateFormFile("file", "조치완료보고서.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(fw, "%PDF-1.4 dummy")
	fw2, err := w.CreateFormFile("file", `..\..\secret.txt`)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(fw2, "secret")
	_ = w.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/attachments", &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("업로드 실패: status=%d body=%s", rec.Code, rec.Body.String())
	}

	items, err := attachRepo.ListByRef("as", asID)
	if err != nil || len(items) != 2 {
		t.Fatalf("첨부 목록: n=%d err=%v", len(items), err)
	}

	var report *model.Attachment
	for i := range items {
		if items[i].FileName == "조치완료보고서.pdf" {
			report = &items[i]
		}
		if strings.Contains(items[i].FileName, "..") || strings.Contains(items[i].FilePath, "..") {
			t.Fatalf("경로 탈출이 저장됐다: %+v", items[i])
		}
		if items[i].FileName != filepath.Base(items[i].FileName) {
			t.Fatalf("DB 파일명에 경로가 있다: %q", items[i].FileName)
		}
		slash := filepath.ToSlash(items[i].FilePath)
		if !strings.Contains(slash, "/as/"+asID+"/") {
			t.Fatalf("저장 경로에 as/{접수ID}가 없다: %q", items[i].FilePath)
		}
		if _, err := os.Stat(items[i].FilePath); err != nil {
			t.Fatalf("디스크에 파일이 없다: %s (%v)", items[i].FilePath, err)
		}
		base := filepath.Base(items[i].FilePath)
		if base == items[i].FileName {
			t.Fatalf("디스크 파일명이 원본과 같으면 충돌 위험이 있다: %q", base)
		}
	}
	if report == nil {
		t.Fatalf("원본 파일명이 DB에 없다: %+v", items)
	}

	show := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req2.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req2)
	if show.Code != http.StatusOK {
		t.Fatalf("조치 화면: status=%d body=%s", show.Code, show.Body.String())
	}
	body := show.Body.String()
	if !strings.Contains(body, "조치완료보고서.pdf") {
		t.Error("화면에 파일명이 없다")
	}
	if strings.Contains(body, report.FilePath) || strings.Contains(body, h.Attachment.uploadDir) {
		t.Error("화면에 저장 경로가 노출됐다")
	}
	if !strings.Contains(body, "조치 사진") {
		t.Error("조치 사진 영역이 없다")
	}
	if !strings.Contains(body, "조치 첨부(문서)") {
		t.Error("문서 첨부 제목이 없다")
	}
	if strings.Contains(body, "조치완료보고서·관련") || strings.Contains(body, "조치완료보고서 또는 관련") {
		t.Error("문서 안내에 조치완료보고서가 남아 있다")
	}
	if !strings.Contains(body, `ref_type" value="as_action_photo"`) {
		t.Error("조치 사진 업로드가 없다")
	}
	if !strings.Contains(body, `accept="image/*"`) {
		t.Error("조치 사진은 image/* 이어야 한다")
	}
	docStart := strings.Index(body, "조치 첨부(문서)")
	if docStart < 0 {
		t.Fatal("문서 영역 없음")
	}
	docChunk := body[docStart:]
	if strings.Contains(docChunk, `accept="image/*"`) {
		t.Error("문서 첨부는 형식 제한이 없어야 한다")
	}

	dl := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "http://localhost/attachments/"+report.AttachmentID, nil)
	req3.AddCookie(jwtCookie(t))
	e.ServeHTTP(dl, req3)
	if dl.Code != http.StatusOK {
		t.Fatalf("다운로드: status=%d", dl.Code)
	}
	cd := dl.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "조치완료보고서.pdf") {
		t.Fatalf("다운로드 파일명: %q", cd)
	}
}

func TestSafeUploadBaseName(t *testing.T) {
	if got := safeUploadBaseName(`C:\temp\조치완료보고서.pdf`); got != "조치완료보고서.pdf" {
		t.Fatalf("windows path: %q", got)
	}
	if got := safeUploadBaseName("../../etc/passwd"); got != "passwd" {
		t.Fatalf("traversal: %q", got)
	}
}

func postASAction(t *testing.T, e *echo.Echo, asID string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/update",
		strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func TestASActionScreenShowsResultLabels(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, s := range []string{"추가조치 필요", "재방문 필요", "처리 확인 후 완료", "우리 팀 추가 작업", "대기"} {
		if !strings.Contains(body, s) {
			t.Errorf("조치 화면에 %q 없음", s)
		}
	}
}

func TestASActionStoresProcessResultAndWait(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)

	rec := postASAction(t, e, asID, url.Values{
		"work_place":       {"field"},
		"process_type":     {"visit"},
		"cause_type":       {"hw"},
		"action_taken":     {"프로그램 재시작"},
		"time_spent":       {"30"},
		"result_code":      {model.ResultRevisit},
		"revisit_scheduled_date": {"2026-08-18"},
		"revisit_reason":   {"부품 부족"},
		"revisit_prep":     {"SSD 준비"},
		"schedule_confirmed": {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("재방문 저장 status=%d", rec.Code)
	}
	got, _ := asRepo.GetByID(asID)
	if got.Status != model.StatusPartialComplete {
		t.Fatalf("상태=%s want partial_complete", got.Status)
	}
	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil || len(procs) != 1 {
		t.Fatalf("이력=%d err=%v", len(procs), err)
	}
	if procs[0].ResultCode != model.ResultRevisit || procs[0].NextActionDate != "2026-08-18" {
		t.Fatalf("이력 결과: %+v", procs[0])
	}
	if procs[0].WaitReason != "부품 부족" || procs[0].PrepNotes != "SSD 준비" {
		t.Fatalf("사유/준비: %+v", procs[0])
	}

	rec = postASAction(t, e, asID, url.Values{
		"work_place":       {"office"},
		"process_type":     {"visit"},
		"cause_type":       {"hw"},
		"action_taken":     {"부품 입고 대기"},
		"time_spent":       {"30"},
		"result_code":      {model.ResultHold},
		"hold_next_action": {"action"},
		"hold_reason":      {"SSD 입고 대기"},
		"hold_resume_date": {"2026-08-20"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("대기 저장 status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	got, _ = asRepo.GetByID(asID)
	if got.Status != "hold" || got.HoldReason != "SSD 입고 대기" {
		t.Fatalf("대기 상태: %+v", got)
	}
	procs, _ = h.AS.processRepo.ListByAS(asID)
	if len(procs) != 2 {
		t.Fatalf("이력 건수=%d want 2", len(procs))
	}
	if procs[0].ResultCode != model.ResultRevisit || procs[1].ResultCode != model.ResultHold {
		t.Fatalf("회차 결과 %s → %s", procs[0].ResultCode, procs[1].ResultCode)
	}
	if procs[1].WaitReason != "SSD 입고 대기" || procs[1].NextActionDate != "2026-08-20" {
		t.Fatalf("대기 이력: %+v", procs[1])
	}

	show := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req)
	body := show.Body.String()
	if !strings.Contains(body, "재방문 필요") || !strings.Contains(body, "SSD 입고 대기") {
		t.Error("Timeline에 이전 회차·대기 사유가 없다")
	}
}

func TestASActionTransferWaitingKeepsProcessDetail(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"work_place":             {"office"},
		"process_type":           {"visit"},
		"cause_type":             {"hw"},
		"action_taken":           {"개발팀 이관"},
		"time_spent":             {"30"},
		"result_code":            {model.ResultTransfer},
		"transfer_detail":        {model.TransferDetailWaiting},
		"confirm_scheduled_date": {"2026-08-21"},
		"confirm_target":         {"개발팀"},
		"confirm_contact":        {"010-1111-2222"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("이관 저장 status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(asID)
	if got.Status != model.StatusPartialComplete {
		t.Fatalf("상태=%s", got.Status)
	}
	procs, _ := h.AS.processRepo.ListByAS(asID)
	if len(procs) != 1 || procs[0].ResultCode != model.ResultTransfer || procs[0].TransferDetail != model.TransferDetailWaiting {
		t.Fatalf("이관 이력: %+v", procs)
	}
}
