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
	g.GET("/as/:id", h.AS.Show)
	g.POST("/as/:id/hold", h.AS.Hold)
	g.GET("/as/work/:work_id/action", h.AS.WorkAction)
	g.POST("/as/work/:work_id/action", h.AS.UpdateWorkAction)
	g.POST("/attachments", h.Attachment.Upload)
	g.GET("/attachments/:id", h.Attachment.Download)
	g.POST("/attachments/:id/delete", h.Attachment.Delete)
	g.POST("/attachments/:id/keywords", h.Attachment.UpdateKeywords)
	return e, h, asRepo, repository.NewAttachmentRepo(db), src.ASID
}

func TestASActionSavesWorkPlace(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)

	form := url.Values{
		"status":       {"in_progress"},
		"work_place":   {"field"},
		"process_type": {"visit"},
		"cause_type":   {"hw"},
		"action_taken": {"현장 점검"},
		"time_spent":   {"30"},
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
	if strings.Contains(body, "조치 첨부(문서)") {
		t.Error("문서 전용 영역이 남아 있다")
	}
	if !strings.Contains(body, `name="ref_type"`) || !strings.Contains(body, `value="as_action_photo"`) ||
		!strings.Contains(body, `value="as"`) {
		t.Error("첨부 종류 선택(사진/문서)이 없다")
	}
	if !strings.Contains(body, `accept="image/*"`) {
		t.Error("기본 종류 사진은 image/* 이어야 한다")
	}
	assertActionAttachFormOutsideSaveForm(t, body)

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

func TestASActionAttachFormOutsideSaveForm(t *testing.T) {
	e, _, _, _, asID := newASActionFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	assertActionAttachFormOutsideSaveForm(t, rec.Body.String())
}

func assertActionAttachFormOutsideSaveForm(t *testing.T, body string) {
	t.Helper()
	start := strings.Index(body, `id="as-action-form"`)
	if start < 0 {
		t.Fatal("조치 저장 폼이 없다")
	}
	endRel := strings.Index(body[start:], "</form>")
	if endRel < 0 {
		t.Fatal("조치 저장 폼이 닫히지 않았다")
	}
	inner := body[start : start+endRel]
	if strings.Contains(inner, `action="/attachments"`) {
		t.Fatal("첨부 업로드가 조치 저장 폼 안에 있다 — 올리기가 조치내용 필수값에 막힌다")
	}
	if strings.Contains(inner, "<form") {
		t.Fatal("조치 저장 폼 안에 다른 form 이 중첩됐다")
	}
	if !strings.Contains(body, `id="as-action-attach-form"`) {
		t.Fatal("첨부 업로드 폼이 없다")
	}
	if !strings.Contains(body, `enctype="multipart/form-data"`) {
		t.Fatal("첨부 폼이 multipart 가 아니다")
	}
	if !strings.Contains(body, `name="file"`) || !strings.Contains(body, `form="as-action-attach-form"`) {
		t.Fatal("파일 입력이 첨부 폼에 연결되지 않았다")
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
	fillCauseCatsFromLegacyType(form)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/update",
		strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func fillCauseCatsFromLegacyType(form url.Values) {
	if form == nil || strings.TrimSpace(form.Get("cause_cat2")) != "" || strings.TrimSpace(form.Get("cause_cat1")) != "" {
		return
	}
	switch strings.TrimSpace(form.Get("cause_type")) {
	case "hw":
		form.Set("cause_cat1", "server")
		form.Set("cause_cat2", "server.hw")
	case "sw":
		form.Set("cause_cat1", "server")
		form.Set("cause_cat2", "server.sw")
	case "network":
		form.Set("cause_cat1", "server")
		form.Set("cause_cat2", "server.network")
	case "env":
		form.Set("cause_cat1", "rfid")
		form.Set("cause_cat2", "rfid.env")
		form.Set("cause_cat3", "rfid.env.booth")
	case "user":
		form.Set("cause_cat1", "rfid")
		form.Set("cause_cat2", "rfid.user")
		form.Set("cause_cat3", "rfid.user.training")
	}
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
	for _, s := range []string{"추가조치 필요", "추가 조치 필요", "대기"} {
		if !strings.Contains(body, s) {
			t.Errorf("조치 화면에 %q 없음", s)
		}
	}
	if strings.Contains(body, "처리 확인 후 완료") || strings.Contains(body, "우리 팀 추가 작업") {
		t.Error("이관 라디오(처리 확인 후 완료 / 우리 팀 추가 작업)가 남아 있다")
	}
	formStart := strings.Index(body, `id="as-action-form"`)
	formEndRel := strings.Index(body[formStart:], "</form>")
	form := body[formStart : formStart+formEndRel]
	if strings.Contains(form, `value="revisit_needed"`) || strings.Contains(form, `value="hold"`) {
		t.Error("조치 결과에 재방문·대기가 남아 있다")
	}
	if !strings.Contains(form, `name="transfer_need_followup"`) {
		t.Error("이관 「추가 조치 필요」 체크가 없다")
	}
}

func TestASActionStoresProcessResultAndWait(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)

	rec := postASAction(t, e, asID, url.Values{
		"work_place":             {"field"},
		"process_type":           {"visit"},
		"cause_type":             {"hw"},
		"action_taken":           {"프로그램 재시작"},
		"time_spent":             {"30"},
		"result_code":            {model.ResultPartial},
		"revisit_scheduled_date": {"2026-08-18"},
		"revisit_reason":         {"부품 부족"},
		"revisit_prep":           {"SSD 준비"},
		"schedule_confirmed":     {"1"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("추가조치 저장 status=%d", rec.Code)
	}
	got, _ := asRepo.GetByID(asID)
	if got.Status != model.StatusPartialComplete {
		t.Fatalf("상태=%s want partial_complete", got.Status)
	}
	procs, err := h.AS.processRepo.ListByAS(asID)
	if err != nil || len(procs) != 1 {
		t.Fatalf("이력=%d err=%v", len(procs), err)
	}
	if procs[0].ResultCode != model.ResultPartial || procs[0].NextActionDate != "2026-08-18" {
		t.Fatalf("이력 결과: %+v", procs[0])
	}
	if procs[0].WaitReason != "부품 부족" || procs[0].PrepNotes != "SSD 준비" {
		t.Fatalf("사유/준비: %+v", procs[0])
	}

	hold := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/hold",
		strings.NewReader(url.Values{
			"hold_next_action": {"action"},
			"hold_reason":      {"SSD 입고 대기"},
		}.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(hold, req)
	if hold.Code != http.StatusSeeOther {
		t.Fatalf("대기 저장 status=%d loc=%s", hold.Code, hold.Header().Get("Location"))
	}
	got, _ = asRepo.GetByID(asID)
	if got.Status != "hold" || got.HoldReason != "SSD 입고 대기" {
		t.Fatalf("대기 상태: %+v", got)
	}
	procs, _ = h.AS.processRepo.ListByAS(asID)
	if len(procs) != 2 {
		t.Fatalf("이력 건수=%d want 2", len(procs))
	}
	if procs[0].ResultCode != model.ResultPartial || procs[1].ResultCode != model.ResultHold {
		t.Fatalf("회차 결과 %s → %s", procs[0].ResultCode, procs[1].ResultCode)
	}
	if procs[1].WaitReason != "SSD 입고 대기" {
		t.Fatalf("대기 이력: %+v", procs[1])
	}

	show := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/action", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(show, req)
	body := show.Body.String()
	if !strings.Contains(body, "추가조치 필요") || !strings.Contains(body, "SSD 입고 대기") {
		t.Error("Timeline에 이전 회차·대기 사유가 없다")
	}
}

func TestASActionTransferWaitingKeepsProcessDetail(t *testing.T) {
	e, h, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"work_place":      {"office"},
		"process_type":    {"remote"},
		"cause_type":      {"hw"},
		"action_taken":    {"개발팀 이관"},
		"time_spent":      {"30"},
		"result_code":     {model.ResultTransfer},
		"transfer_detail": {model.TransferDetailWaiting},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("이관 저장 status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(asID)
	if got.Status != "completed" {
		t.Fatalf("체크 없이 이관하면 원 건이 완료되어야 한다: 상태=%s", got.Status)
	}
	procs, _ := h.AS.processRepo.ListByAS(asID)
	if len(procs) != 1 || procs[0].ResultCode != model.ResultTransfer || procs[0].TransferDetail != model.TransferDetailCompleted {
		t.Fatalf("이관 이력: %+v", procs)
	}
	if kids, _ := asRepo.ListTransferFollowups(asID); len(kids) != 0 {
		t.Fatalf("후속 건이 생기면 안 된다: %+v", kids)
	}
}

func TestASActionTransferFollowupCreatesChild(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"work_place":             {"office"},
		"process_type":           {"remote"},
		"cause_type":             {"hw"},
		"action_taken":           {"개발팀 이관"},
		"time_spent":             {"30"},
		"result_code":            {model.ResultTransfer},
		"transfer_need_followup": {"1"},
		"followup_note":          {"펌웨어 재설정 후 재확인"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("이관후속 저장 status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	src, _ := asRepo.GetByID(asID)
	if src.Status != "completed" {
		t.Fatalf("원 건 상태=%s want completed", src.Status)
	}
	if reopens, _ := asRepo.ListReopens(asID); len(reopens) != 0 {
		t.Fatalf("이관후속이 재접수 목록에 있다: %+v", reopens)
	}
	kids, err := asRepo.ListTransferFollowups(asID)
	if err != nil || len(kids) != 1 {
		t.Fatalf("이관후속 %d건 err=%v", len(kids), err)
	}
	child, _ := asRepo.GetByID(kids[0].ASID)
	if child == nil || child.IsReopen || !child.IsTransferFollowup() {
		t.Fatalf("후속 건: %+v", child)
	}
	if child.ParentASID != asID || child.Symptom != src.Symptom || child.FollowupNote != "펌웨어 재설정 후 재확인" {
		t.Fatalf("복제/추가내용: %+v", child)
	}
	if child.CustomerID != src.CustomerID || child.ReceiptChannel != src.ReceiptChannel || child.Requester != src.Requester {
		t.Fatalf("기관·채널·요청자 복제 실패: %+v", child)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/as/"+child.ASID) {
		t.Fatalf("새 건으로 이동해야 한다: loc=%s child=%s", loc, child.ASID)
	}

	showChild := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+child.ASID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(showChild, req)
	body := showChild.Body.String()
	if !strings.Contains(body, "이관 원 건") || !strings.Contains(body, src.ASNumber) {
		t.Error("새 건 상세에 「이관 원 건」이 없다")
	}
	if strings.Contains(body, "원 완료 건 (1차)") {
		t.Error("이관후속이 재접수 배너로 나왔다")
	}

	showSrc := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(showSrc, req)
	srcBody := showSrc.Body.String()
	if !strings.Contains(srcBody, "이관 후속 건") || !strings.Contains(srcBody, child.ASNumber) {
		t.Error("원 건 상세에 「이관 후속 건」이 없다")
	}
}

func TestASActionTransferFollowupRequiresNote(t *testing.T) {
	e, _, asRepo, _, asID := newASActionFixture(t)
	rec := postASAction(t, e, asID, url.Values{
		"work_place":             {"office"},
		"process_type":           {"remote"},
		"cause_type":             {"hw"},
		"action_taken":           {"개발팀 이관"},
		"result_code":            {model.ResultTransfer},
		"transfer_need_followup": {"1"},
		"followup_note":          {""},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "followup_note") {
		t.Fatalf("추가 접수 내용 없이 저장됨: loc=%s", rec.Header().Get("Location"))
	}
	got, _ := asRepo.GetByID(asID)
	if got.Status == "completed" {
		t.Fatal("내용 없이 원 건이 완료되면 안 된다")
	}
}
