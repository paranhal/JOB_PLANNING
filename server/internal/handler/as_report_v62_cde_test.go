package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/imageproc"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func writeUserSig(t *testing.T, h *Handler, userID string, raw []byte) {
	t.Helper()
	pngBytes, err := imageproc.ProcessSignature(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(h.AS.reportUploadDir(), "signatures")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, userID+".png")
	if err := os.WriteFile(path, pngBytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := h.AS.userRepo.UpdateSignaturePath(userID, path); err != nil {
		t.Fatal(err)
	}
}

func inkPNGAt(t *testing.T, x0, x1 int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 80, 40))
	for x := x0; x < x1; x++ {
		img.Set(x, 18, color.RGBA{R: 10, G: 10, B: 10, A: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func strokePNGAt(t *testing.T, slope int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 400, 400))
	ink := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	for x := 40; x < 360; x++ {
		y := 200 + (x-200)*slope/2
		for t := -4; t <= 4; t++ {
			yy := y + t
			if yy < 0 || yy >= 400 {
				continue
			}
			img.Set(x, yy, ink)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestASReportPreviewHasTwoSignatureButtons(t *testing.T) {
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
	if strings.Count(body, `id="btn-report-sig-inspector"`) != 1 {
		t.Fatal("점검자 사인 단추가 하나가 아니다")
	}
	if strings.Count(body, `id="btn-report-sig-confirmer"`) != 1 {
		t.Fatal("확인자 사인 단추가 하나가 아니다")
	}
	if strings.Count(body, `id="btn-report-sig-`) != 2 {
		t.Fatal("사인 단추는 점검자·확인자 각 1개여야 한다")
	}
	if !strings.Contains(body, `id="btn-report-sig-confirmer" data-slot="confirmer" data-has-account="0"`) {
		t.Fatal("확인자(고객) 저장 불가가 표시되지 않는다")
	}
}

func TestASReportInspectorUsesNamedAccountNotLogin(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	tech := &model.User{
		Username: "yang", PasswordHash: HashPassword("pw"), FullName: "양기헌",
		Role: model.RoleTech, IsActive: true, OrgID: admin.OrgID,
	}
	if err := h.AS.userRepo.Create(tech); err != nil {
		t.Fatal(err)
	}
	writeUserSig(t, h, tech.UserID, inkPNGAt(t, 8, 28))
	writeUserSig(t, h, admin.UserID, inkPNGAt(t, 50, 70))

	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	draft := h.AS.buildASReportDraft(as, time.Now())
	draft.Inspector = "양기헌"
	rec := postASReport(t, e, asID, draft)
	if rec.Code != http.StatusOK {
		t.Fatalf("발급 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	sec := string(zipFileBytes(t, rec.Body.Bytes(), "Contents/section0.xml"))
	if !strings.Contains(sec, "양기헌") {
		t.Fatal("점검자 이름이 없다")
	}
	if !strings.Contains(sec, "<hp:pic") {
		t.Fatal("점검자 계정 사인 그림이 없다")
	}
}

func TestASReportApplySignatureStaysOnFormNotDisk(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	sigDir := filepath.Join(h.AS.reportUploadDir(), "signatures")
	if err := os.MkdirAll(sigDir, 0755); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(sigDir)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := imageproc.ProcessSignature(bytes.NewReader(inkPNGAt(t, 10, 40)))
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"slot": {"confirmer"}, "intent": {"apply"},
		"name": {"박확인"}, "drawn": {signatureDataURL(raw)},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/report/signature", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("적용 %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["source"] != "이 보고서만" || out["data_url"] == "" {
		t.Fatalf("적용 응답 %+v", out)
	}
	after, err := os.ReadDir(sigDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatal("적용이 서버에 사인 파일을 남겼다")
	}

	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	draft := h.AS.buildASReportDraft(as, time.Now())
	issue := url.Values{
		"customer_name": {draft.CustomerName}, "department": {draft.Department},
		"manager": {draft.Manager}, "phone": {draft.Phone}, "service": {draft.Service},
		"symptom": {draft.Symptom}, "cause_detail": {draft.CauseDetail},
		"conclusion": {draft.Conclusion}, "report_date": {draft.ReportDate},
		"inspector": {draft.Inspector}, "confirmer": {draft.Confirmer},
		"work_dates": {draft.WorkDates}, "actions": {draft.Actions},
		"format": {"hwpx"}, "return_file": {"1"}, "confirmer_sig": {out["data_url"]},
	}
	irec := httptest.NewRecorder()
	ireq := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/report", strings.NewReader(issue.Encode()))
	ireq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ireq.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(irec, ireq)
	if irec.Code != http.StatusOK {
		t.Fatalf("발급 %d", irec.Code)
	}
	sec := string(zipFileBytes(t, irec.Body.Bytes(), "Contents/section0.xml"))
	if !strings.Contains(sec, "<hp:pic") {
		t.Fatal("확인자 적용 사인 그림이 없다")
	}
	if !strings.Contains(sec, `textWrap="IN_FRONT_OF_TEXT"`) {
		t.Fatal("글 앞으로가 아니다")
	}
}

func TestASReportSaveConfirmerWithoutAccountRejected(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "증상", "원인", "결론")
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	raw, err := imageproc.ProcessSignature(bytes.NewReader(inkPNGAt(t, 10, 30)))
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"slot": {"confirmer"}, "intent": {"save"},
		"name": {"박확인"}, "drawn": {signatureDataURL(raw)},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/report/signature", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(jwtCookieUserClaims(t, admin))
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("계정 없는 저장은 막혀야 한다 %d %s", rec.Code, rec.Body.String())
	}
}

func TestHangulCheckExport(t *testing.T) {
	dir := strings.TrimSpace(os.Getenv("HANGUL_CHECK_DIR"))
	if dir == "" {
		t.Skip("HANGUL_CHECK_DIR 없음")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	e, h, asRepo, _, asID := newASReportFixture(t)
	completeASForReport(t, asRepo, asID, "게이트 오른쪽 문이 안 닫힘", "리밋 스위치 불량", "교체 후 정상")
	addReportProcesses(t, h, asID, []string{"1차 점검", "부품 교체"})
	admin, _ := h.AS.userRepo.GetByUsername("admin")
	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	draft := h.AS.buildASReportDraft(as, time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local))
	inspRaw, err := imageproc.ProcessSignature(bytes.NewReader(strokePNGAt(t, 1)))
	if err != nil {
		t.Fatal(err)
	}
	confRaw, err := imageproc.ProcessSignature(bytes.NewReader(strokePNGAt(t, -1)))
	if err != nil {
		t.Fatal(err)
	}
	inspSig := signatureDataURL(inspRaw)
	confSig := signatureDataURL(confRaw)
	writeCase := func(name, insp, conf, inspPNG, confPNG string) {
		form := url.Values{
			"customer_name": {draft.CustomerName}, "department": {draft.Department},
			"manager": {draft.Manager}, "phone": {draft.Phone}, "service": {draft.Service},
			"symptom": {draft.Symptom}, "cause_detail": {draft.CauseDetail},
			"conclusion": {draft.Conclusion}, "report_date": {draft.ReportDate},
			"inspector": {insp}, "confirmer": {conf},
			"work_dates": {draft.WorkDates}, "actions": {draft.Actions},
			"format": {"hwpx"}, "return_file": {"1"},
		}
		if inspPNG != "" {
			form.Set("inspector_sig", inspPNG)
		}
		if confPNG != "" {
			form.Set("confirmer_sig", confPNG)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost/as/"+asID+"/report", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(jwtCookieUserClaims(t, admin))
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s 발급 %d %s", name, rec.Code, rec.Header().Get("Location"))
		}
		if err := os.WriteFile(filepath.Join(dir, name+".hwpx"), rec.Body.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeCase("with_sig", draft.Inspector, draft.Confirmer, inspSig, confSig)
	writeCase("long_name", "남궁민수", "황보미르", inspSig, confSig)
	writeCase("no_sig", draft.Inspector, draft.Confirmer, "", "")
}

func zipBinPNGBlobs(t *testing.T, data []byte) [][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out [][]byte
	for _, f := range zr.File {
		name := strings.ToLower(f.Name)
		if !strings.Contains(name, "bindata") || !(strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg")) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}
