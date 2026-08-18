package model

import (
	"strings"
	"testing"
	"time"
)

func TestBuildASReportDraftMappingAndProcessLines(t *testing.T) {
	as := &ASReceipt{
		OrgName: "가나도서관", Requester: "홍길동", ConfirmContact: "010-1111-2222",
		ProductName: "게이트", Symptom: "오작동", CauseDetail: "센서 불량",
		Conclusion: "교체 후 정상", AssignedTo: "양기헌", CustomerConfirmer: "박확인",
	}
	day1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.Local)
	day2 := time.Date(2026, 8, 5, 11, 0, 0, 0, time.Local)
	day3 := time.Date(2026, 8, 10, 9, 0, 0, 0, time.Local)
	procs := []ASProcess{
		{ProcessDatetime: day1, WorkContent: "1차 점검"},
		{ProcessDatetime: day2, WorkContent: "부품 교체"},
		{ProcessDatetime: day3, WorkContent: "정상 확인"},
	}
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.Local)
	d := BuildASReportDraft(as, procs, nil, nil, nil, now)
	if d.CustomerName != "가나도서관" || d.Manager != "홍길동" || d.Phone != "010-1111-2222" {
		t.Fatalf("고객/담당/연락처: %+v", d)
	}
	if d.Service != "게이트" || d.Symptom != "오작동" || d.CauseDetail != "센서 불량" || d.Conclusion != "교체 후 정상" {
		t.Fatalf("본문 매핑: %+v", d)
	}
	if d.Inspector != "양기헌" || d.Confirmer != "박확인" || d.ReportDate != "2026-08-18" {
		t.Fatalf("보고/점검/확인: %+v", d)
	}
	for _, want := range []string{"2026-08-01", "2026-08-05", "2026-08-10"} {
		if !strings.Contains(d.WorkDates, want) {
			t.Fatalf("작업일자에 %s 없음: %q", want, d.WorkDates)
		}
	}
	for _, want := range []string{"1차 점검", "부품 교체", "정상 확인"} {
		if !strings.Contains(d.Actions, want) {
			t.Fatalf("조치내용에 %s 없음: %q", want, d.Actions)
		}
	}
	if strings.Count(d.Actions, "\n") != 2 {
		t.Fatalf("조치 3건이 여러 줄이 아님: %q", d.Actions)
	}
}

func TestASReportFilenameSanitizesAndClips(t *testing.T) {
	d := ASReportDraft{CustomerName: `가/나:도서관`, Symptom: `게이트 <고장> & "소음" 아주긴증상요약입니다여분`}
	name := d.Filename(time.Date(2026, 8, 18, 0, 0, 0, 0, time.Local))
	if strings.ContainsAny(name, `\/:*?"<>|`) {
		t.Fatalf("금지문자 남음: %q", name)
	}
	if !strings.HasSuffix(name, "_조치완료보고서_20260818.hwpx") {
		t.Fatalf("접미사: %q", name)
	}
	if strings.Contains(name, "여분") {
		t.Fatalf("증상 20자를 넘김: %q", name)
	}
}

func TestASReportDraftValuesCoverAllKeys(t *testing.T) {
	d := ASReportDraft{}
	vals := d.Values()
	for _, k := range []string{
		"고객명", "부서", "담당자", "연락처", "서비스",
		"장애사항", "장애원인", "결론", "보고일자", "점검자", "확인자", "작업일자", "조치내용",
	} {
		if _, ok := vals[k]; !ok {
			t.Fatalf("키 없음: %s", k)
		}
	}
}

func TestMissingReportFields(t *testing.T) {
	d := ASReportDraft{}
	got := d.MissingReportFields()
	if len(got) != 2 || got[0] != "장애원인" || got[1] != "결론" {
		t.Fatalf("%v", got)
	}
	d.CauseDetail = "원인"
	d.Conclusion = "결론문"
	if miss := d.MissingReportFields(); len(miss) != 0 {
		t.Fatalf("%v", miss)
	}
}

func TestAttachmentReportIssuer(t *testing.T) {
	a := Attachment{Keywords: "발급자:관리자"}
	if a.ReportIssuer() != "관리자" {
		t.Fatalf("%q", a.ReportIssuer())
	}
}
