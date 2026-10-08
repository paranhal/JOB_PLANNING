package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"customer-support/internal/imageproc"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestASReportPreviewAsksToUseSavedSignature(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/as/"+asID+"/report", nil)
	req.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("미리보기 %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="report-sig-ask"`,
		`id="use_saved_sig"`,
		`id="report-sig-load"`,
		`id="btn-as-report-preview"`,
		"저장된 사인이 있습니다.",
		"이 사인으로",
		"새로 그리기",
		"사인 없이",
		"이대로 저장",
		"저장된 사인 불러오기",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("없다: %s", want)
		}
	}
}

func TestASReportUseSavedSigZeroSkipsAccountPNG(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	tech := &model.User{
		Username: "yang-g", PasswordHash: HashPassword("pw"), FullName: "양기헌",
		Role: model.RoleTech, IsActive: true, OrgID: admin.OrgID,
	}
	if err := h.AS.userRepo.Create(tech); err != nil {
		t.Fatal(err)
	}
	writeUserSig(t, h, tech.UserID, inkPNGAt(t, 8, 28))

	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	draft := h.AS.buildASReportDraft(as, time.Now())
	draft.Inspector = "양기헌"
	form := reportForm(draft)
	form.Set("use_saved_sig", "0")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/report", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("발급 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	sec := string(zipFileBytes(t, rec.Body.Bytes(), "Contents/section0.xml"))
	if strings.Contains(sec, "<hp:pic") {
		t.Fatal("넣지 않고 발급인데 저장본이 들어갔다")
	}
	if !strings.Contains(sec, "(사인)") {
		t.Fatal("(사인) 글자가 없다")
	}
}

func TestASReportUseSavedSigAbsentStillEmbeds(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	tech := &model.User{
		Username: "yang-old", PasswordHash: HashPassword("pw"), FullName: "양기헌",
		Role: model.RoleTech, IsActive: true, OrgID: admin.OrgID,
	}
	if err := h.AS.userRepo.Create(tech); err != nil {
		t.Fatal(err)
	}
	writeUserSig(t, h, tech.UserID, inkPNGAt(t, 8, 28))
	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	draft := h.AS.buildASReportDraft(as, time.Now())
	draft.Inspector = "양기헌"
	rec := postASReport(t, e, asID, draft)
	if rec.Code != http.StatusOK {
		t.Fatalf("발급 %d", rec.Code)
	}
	sec := string(zipFileBytes(t, rec.Body.Bytes(), "Contents/section0.xml"))
	if !strings.Contains(sec, "<hp:pic") {
		t.Fatal("옛 요청은 저장본을 넣어야 한다")
	}
}

func TestASReportUseSavedSigZeroKeepsFormPNG(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	draft := h.AS.buildASReportDraft(as, time.Now())
	form := reportForm(draft)
	form.Set("use_saved_sig", "0")
	raw, err := imageproc.ProcessSignature(bytes.NewReader(inkPNGAt(t, 10, 40)))
	if err != nil {
		t.Fatal(err)
	}
	form.Set("confirmer_sig", signatureDataURL(raw))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/report", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("발급 %d", rec.Code)
	}
	sec := string(zipFileBytes(t, rec.Body.Bytes(), "Contents/section0.xml"))
	if !strings.Contains(sec, "<hp:pic") {
		t.Fatal("폼으로 보낸 사인이 빠졌다")
	}
}

func reportForm(draft model.ASReportDraft) url.Values {
	return url.Values{
		"customer_name": {draft.CustomerName}, "department": {draft.Department},
		"manager": {draft.Manager}, "phone": {draft.Phone}, "service": {draft.Service},
		"symptom": {draft.Symptom}, "cause_detail": {draft.CauseDetail},
		"conclusion": {draft.Conclusion}, "report_date": {draft.ReportDate},
		"inspector": {draft.Inspector}, "confirmer": {draft.Confirmer},
		"work_dates": {draft.WorkDates}, "actions": {draft.Actions},
		"format": {"hwpx"}, "return_file": {"1"},
	}
}

