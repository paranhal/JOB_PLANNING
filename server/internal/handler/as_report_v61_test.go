package handler

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/repository"
)

// 61단계: 고치기 전에 한글 양식으로 한 건을 발급한다. §70.6
func TestASReportIssueUsesOfficialHangulTemplate(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	h.AS.reportTemplateBytes = officialASReportTemplate(t)
	completeASForReport(t, asRepo, asID, "게이트 오른쪽 문이 안 닫힘", "리밋 스위치 불량", "교체 후 정상")
	addReportProcesses(t, h, asID, []string{"1차 점검", "부품 교체"})

	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	draft := h.AS.buildASReportDraft(as, time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local))
	rec := postASReport(t, e, asID, draft)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d loc=%s", rec.Code, rec.Header().Get("Location"))
	}
	data := rec.Body.Bytes()
	if bytes.Contains(data, []byte("{{")) {
		t.Fatal("발급본에 {{ 가 남았다")
	}
	assertHangulMimetypeFirst(t, data)

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var hasPrv bool
	var sec string
	for _, f := range zr.File {
		if strings.Contains(strings.ToLower(f.Name), "prvimage") {
			hasPrv = true
		}
		if f.Name == "Contents/section0.xml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			sec = string(b)
		}
	}
	if !hasPrv {
		t.Fatal("Preview/PrvImage.png 가 없다")
	}
	if !strings.Contains(sec, "부품 교체") || !strings.Contains(sec, "교체 후 정상") {
		t.Fatalf("조치내용·결론이 칸에 없다")
	}
	idxAct := strings.Index(sec, "부품 교체")
	idxCon := strings.Index(sec, "교체 후 정상")
	if idxAct < 0 || idxCon < idxAct {
		t.Fatal("결론이 조치내용 다음 줄이 아니다")
	}
	if !strings.Contains(sec[idxAct:idxCon], "</hp:p>") {
		t.Fatal("조치내용과 결론이 한 문단이다")
	}
	if strings.Contains(sec, "{{지원유형}}") {
		t.Fatal("지원유형 자리표시자가 들어갔다")
	}
}

func TestASReportIssueStopsWithoutHangulTemplate(t *testing.T) {
	e, h, asRepo, _, asID := newASReportFixture(t)
	h.AS.reportTemplateBytes = nil
	h.AS.reportTemplatePath = filepath.Join(t.TempDir(), "없음.hwpx")
	completeASForReport(t, asRepo, asID, "문 고장", "센서", "정상")
	as, _ := asRepo.GetByID(repository.OrgAll, asID)
	d := h.AS.buildASReportDraft(as, time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local))
	rec := postASReport(t, e, asID, d)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("자리표시자로 내보내면 안 된다 status=%d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "한글") && !strings.Contains(loc, "%ED%95%9C%EA%B8%80") {
		t.Fatalf("안내가 없다 loc=%s", loc)
	}
}

func officialASReportTemplate(t *testing.T) []byte {
	t.Helper()
	paths := []string{
		filepath.Join("..", "..", "templates", "report", "장애처리보고서.hwpx"),
		filepath.Join("templates", "report", "장애처리보고서.hwpx"),
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if !hwpxTemplateHasSignatureKeys(b) {
			t.Fatalf("사인 자리표시자가 없는 양식이다 (%s)", p)
		}
		return b
	}
	t.Fatal("운영 양식 장애처리보고서.hwpx 를 읽지 못했다")
	return nil
}

func hwpxTemplateHasSignatureKeys(data []byte) bool {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	var insp, conf bool
	for _, f := range zr.File {
		if f.Name != "Contents/section0.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return false
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return false
		}
		s := string(b)
		insp = strings.Contains(s, "{{점검자사인}}")
		conf = strings.Contains(s, "{{확인자사인}}")
	}
	return insp && conf
}

func assertHangulMimetypeFirst(t *testing.T, data []byte) {
	t.Helper()
	if len(data) < 38 || string(data[0:2]) != "PK" {
		t.Fatal("ZIP 아님")
	}
	if data[8] != 0 || data[9] != 0 {
		t.Fatal("mimetype 이 Store 가 아니다")
	}
	nl := int(data[26]) | int(data[27])<<8
	if string(data[30:30+nl]) != "mimetype" {
		t.Fatalf("첫 항목이 mimetype 아님: %q", data[30:30+nl])
	}
}
