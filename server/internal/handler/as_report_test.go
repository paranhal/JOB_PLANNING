package handler

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/hwpx"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newASReportFixture(t *testing.T) (*echo.Echo, *Handler, *repository.ASRepo, *repository.AttachmentRepo, string) {
	t.Helper()
	e, h, asRepo, attachRepo, asID := newASActionFixture(t)
	h.AS.reportTemplateBytes = hwpx.BuildPlaceholderTemplate()
	e.GET("/as/:id", h.AS.Show, h.Auth.AuthMiddleware)
	e.GET("/as/:id/report", h.AS.ReportPreview, h.Auth.AuthMiddleware)
	e.POST("/as/:id/report", h.AS.ReportIssue, h.Auth.AuthMiddleware)
	return e, h, asRepo, attachRepo, asID
}

func completeASForReport(t *testing.T, asRepo *repository.ASRepo, asID, symptom, cause, conclusion string) {
	t.Helper()
	as, err := asRepo.GetByID(asID)
	if err != nil || as == nil {
		t.Fatal(err)
	}
	as.Symptom = symptom
	if err := asRepo.UpdateReceipt(as); err != nil {
		t.Fatal(err)
	}
	as, _ = asRepo.GetByID(asID)
	as.Status = "completed"
	as.CauseDetail = cause
	as.Conclusion = conclusion
	as.CustomerConfirmer = "박확인"
	if err := asRepo.Update(as); err != nil {
		t.Fatal(err)
	}
}

func addReportProcesses(t *testing.T, h *Handler, asID string, contents []string) {
	t.Helper()
	day := time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)
	for i, content := range contents {
		p := &model.ASProcess{
			ASID: asID, WorkContent: content,
			ProcessDatetime: day.AddDate(0, 0, i*4),
			Worker:          "양기헌",
			TimeSpent:       30,
		}
		if err := h.AS.processRepo.Create(p); err != nil {
			t.Fatal(err)
		}
	}
}

func postASReport(t *testing.T, e *echo.Echo, asID string, d model.ASReportDraft) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{
		"customer_name": {d.CustomerName},
		"department":    {d.Department},
		"manager":       {d.Manager},
		"phone":         {d.Phone},
		"service":       {d.Service},
		"symptom":       {d.Symptom},
		"cause_detail":  {d.CauseDetail},
		"conclusion":    {d.Conclusion},
		"report_date":   {d.ReportDate},
		"inspector":     {d.Inspector},
		"confirmer":     {d.Confirmer},
		"work_dates":    {d.WorkDates},
		"actions":       {d.Actions},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/report",
		strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	return rec
}

func TestASShowReportButtonDisabledWhenNotComplete(t *testing.T) {
	e, _, _, _, asID := newASReportFixture(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="btn-as-report-disabled"`) {
		t.Fatal("미완료 건에 비활성 버튼이 없다")
	}
	if !strings.Contains(body, "완료된 건만 발급할 수 있습니다") {
		t.Fatal("비활성 사유가 없다")
	}
	if strings.Contains(body, `id="btn-as-report"`) && !strings.Contains(body, `id="btn-as-report-disabled"`) {
		t.Fatal("미완료인데 발급 버튼이 활성이다")
	}
}

func TestASReportPreviewGuidesMissingFields(t *testing.T) {
	e, _, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "게이트 오작동", "", "")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/report", nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "장애원인") || !strings.Contains(body, "결론") {
		t.Fatal("빠진 칸을 명시하지 않는다")
	}
	if !strings.Contains(body, "/as/"+asID+"/action") {
		t.Fatal("조치 화면으로 유도하지 않는다")
	}
}

func TestASReportIssueSpecialCharsThreeProcessesAndNoPlaceholder(t *testing.T) {
	e, h, asRepo, attachRepo, asID := newASReportFixture(t)
	symptom := `게이트 <고장> & "소음"`
	completeASForReport(t, asRepo, asID, symptom, "전원부 불량", "교체 후 정상")
	addReportProcesses(t, h, asID, []string{"1차 점검", "부품 교체", "정상 확인"})

	as, _ := asRepo.GetByID(asID)
	draft := h.AS.buildASReportDraft(as, time.Date(2026, 8, 18, 12, 0, 0, 0, time.Local))
	if !strings.Contains(draft.Actions, "1차 점검") || !strings.Contains(draft.Actions, "부품 교체") || !strings.Contains(draft.Actions, "정상 확인") {
		t.Fatalf("초안에 조치 3건이 없다: %q", draft.Actions)
	}

	rec := postASReport(t, e, asID, draft)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	data := rec.Body.Bytes()
	assertHWPXXML(t, data)
	sec := zipFileBytes(t, data, "Contents/section0.xml")
	text := string(sec)
	if strings.Contains(text, "{{") {
		t.Fatal("결과 XML에 {{ 가 남았다")
	}
	if !strings.Contains(text, "&lt;") || !strings.Contains(text, "&amp;") {
		t.Fatalf("특수문자 이스케이프 없음: %s", text)
	}
	for _, want := range []string{"1차 점검", "부품 교체", "정상 확인"} {
		if !strings.Contains(text, want) {
			t.Fatalf("조치 %q 가 파일에 없다", want)
		}
	}

	items, err := attachRepo.ListByRef(model.RefTypeASReport, asID)
	if err != nil || len(items) != 1 {
		t.Fatalf("1회 발급 이력=%d err=%v", len(items), err)
	}

	rec2 := postASReport(t, e, asID, draft)
	if rec2.Code != http.StatusOK {
		t.Fatalf("재발급 status=%d", rec2.Code)
	}
	items, err = attachRepo.ListByRef(model.RefTypeASReport, asID)
	if err != nil || len(items) != 2 {
		t.Fatalf("2회 발급 후 이력=%d (덮어쓰면 안 됨) err=%v", len(items), err)
	}
}

func TestASReportIssueIncludesSelectedActionPhoto(t *testing.T) {
	e, h, asRepo, attachRepo, asID := newASReportFixture(t)
	up := postActionPhotoEcho(t, e, asID, "결과.png", "교체 후", makePNG(t, 80, 40))
	if up.Code != http.StatusSeeOther {
		t.Fatalf("사진 업로드 status=%d", up.Code)
	}
	photos, _ := attachRepo.ListByRef(model.RefTypeASActionPhoto, asID)
	if len(photos) != 1 {
		t.Fatalf("조치 사진 n=%d", len(photos))
	}

	completeASForReport(t, asRepo, asID, "게이트 오작동", "전원부 불량", "교체 후 정상")
	preview := httptest.NewRecorder()
	preq := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/report", nil)
	preq.AddCookie(jwtCookie(t))
	e.ServeHTTP(preview, preq)
	if preview.Code != http.StatusOK {
		t.Fatalf("미리보기 status=%d", preview.Code)
	}
	pbody := preview.Body.String()
	if !strings.Contains(pbody, `name="include_photo"`) || !strings.Contains(pbody, photos[0].AttachmentID) {
		t.Fatal("미리보기에 사진 선택이 없다")
	}

	as, _ := asRepo.GetByID(asID)
	draft := h.AS.buildASReportDraft(as, time.Now())
	form := url.Values{
		"customer_name": {draft.CustomerName},
		"department":    {draft.Department},
		"manager":       {draft.Manager},
		"phone":         {draft.Phone},
		"service":       {draft.Service},
		"symptom":       {draft.Symptom},
		"cause_detail":  {draft.CauseDetail},
		"conclusion":    {draft.Conclusion},
		"report_date":   {draft.ReportDate},
		"inspector":     {draft.Inspector},
		"confirmer":     {draft.Confirmer},
		"work_dates":    {draft.WorkDates},
		"actions":       {draft.Actions},
		"include_photo": {photos[0].AttachmentID},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/report",
		strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("발급 status=%d loc=%s body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	assertHWPXXML(t, rec.Body.Bytes())
	sec := string(zipFileBytes(t, rec.Body.Bytes(), "Contents/section0.xml"))
	if !strings.Contains(sec, `binaryItemIDRef="actionphoto1"`) {
		t.Fatal("보고서에 사진이 없다")
	}
	if !strings.Contains(sec, "교체 후") {
		t.Fatal("사진 메모가 보고서에 없다")
	}
	found := false
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(len(rec.Body.Bytes())))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == "Contents/BinData/actionphoto1.jpg" {
			found = true
		}
	}
	if !found {
		t.Fatal("BinData JPEG가 없다")
	}
}

func TestASReportPreviewEditDoesNotWriteBack(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "게이트 오작동", "원본원인", "원본결론")
	as, _ := asRepo.GetByID(asID)
	draft := h.AS.buildASReportDraft(as, time.Now())
	draft.CauseDetail = "다듬은 원인"
	draft.Conclusion = "다듬은 결론"
	rec := postASReport(t, e, asID, draft)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	got, err := asRepo.GetByID(asID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if got.CauseDetail != "원본원인" || got.Conclusion != "원본결론" {
		t.Fatalf("미리보기 수정이 접수에 되쓰였다: detail=%q conclusion=%q", got.CauseDetail, got.Conclusion)
	}
}

func TestASReportIssueRejectedWhenNotComplete(t *testing.T) {
	e, _, _, _, asID := newASReportFixture(t)
	rec := postASReport(t, e, asID, model.ASReportDraft{
		CustomerName: "가나도서관", Symptom: "고장", CauseDetail: "원인", Conclusion: "결론",
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatal("미완료 발급을 막지 않았다")
	}
}

func TestASShowListsReportHistory(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "게이트 오작동", "원인", "결론")
	as, _ := asRepo.GetByID(asID)
	draft := h.AS.buildASReportDraft(as, time.Now())
	if rec := postASReport(t, e, asID, draft); rec.Code != http.StatusOK {
		t.Fatalf("발급 실패 %d", rec.Code)
	}
	page := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID, nil)
	req.AddCookie(jwtCookie(t))
	e.ServeHTTP(page, req)
	body := page.Body.String()
	if !strings.Contains(body, "발급 이력") || !strings.Contains(body, "내려받기") {
		t.Fatal("상세에 발급 이력이 없다")
	}
	if !strings.Contains(body, "관리자") {
		t.Fatal("발급자가 없다")
	}
}

func assertHWPXXML(t *testing.T, data []byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("unzip 실패: %v", err)
	}
	n := 0
	for _, f := range zr.File {
		name := strings.ToLower(f.Name)
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".hpf") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(bytes.NewReader(raw))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s XML 무효: %v", f.Name, err)
			}
		}
		n++
	}
	if n < 3 {
		t.Fatalf("XML 파일이 적다: %d", n)
	}
}

func zipFileBytes(t *testing.T, zipped []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatalf("%s 없음", name)
	return nil
}
