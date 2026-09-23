package handler

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

func TestDetectLaborColumns_AandB(t *testing.T) {
	a := [][]string{
		{"구분", "월평균임금", "일평균임금", "시간평균임금"},
		{"① IT기획자", "7,000,000", "318,182", "39,773"},
		{"IT컨설턴트", "6,500,000", "295,455", "36,932"},
	}
	ca := DetectLaborColumns(a)
	if ca.NeedPick || ca.Job != 0 || ca.Monthly != 1 {
		t.Fatalf("A열 머리글: %+v", ca)
	}
	pa := parseLaborSheet(a, nil, nil)
	if pa.NeedPick || len(pa.Rows) != 2 || pa.Rows[0].Name != "IT기획자" {
		t.Fatalf("A열 파싱: need=%v rows=%+v", pa.NeedPick, pa.Rows)
	}

	b := [][]string{
		{"", "구 분", "월평균임금\n(M/M)", "일평균임금\n(M/D)", "시간평균임금"},
		{"", "① IT기획자", "7000000", "318182", "39773"},
		{"", "업무분석가", "6500000", "295455", "36932"},
	}
	cb := DetectLaborColumns(b)
	if cb.NeedPick || cb.Job != 1 || cb.Monthly != 2 {
		t.Fatalf("B열 머리글: %+v", cb)
	}
	pb := parseLaborSheet(b, nil, nil)
	if pb.NeedPick || len(pb.Rows) != 2 || pb.Rows[0].Name != "IT기획자" {
		t.Fatalf("B열 파싱: %+v", pb)
	}
}

func TestDetectLaborColumns_ValuesThenPick(t *testing.T) {
	vals := [][]string{
		{"안내", "이 표는 참고입니다"},
		{"IT기획자", "7000000", "318182", "39773"},
		{"IT컨설턴트", "6500000", "295455", "36932"},
		{"업무분석가", "6000000", "272727", "34091"},
	}
	cv := DetectLaborColumns(vals)
	if cv.NeedPick || cv.Job != 0 {
		t.Fatalf("값으로 찾기: %+v", cv)
	}
	pv := parseLaborSheet(vals, nil, nil)
	if pv.NeedPick || len(pv.Rows) < 3 {
		t.Fatalf("값 파싱: need=%v n=%d reason=%s", pv.NeedPick, len(pv.Rows), pv.Reason)
	}

	noise := [][]string{
		{"제목", "내용"},
		{"안내", "없음"},
		{"비고", "공란"},
	}
	cn := DetectLaborColumns(noise)
	if !cn.NeedPick || strings.TrimSpace(cn.Reason) == "" {
		t.Fatalf("수동 지정이 안 뜬다: %+v", cn)
	}
	pn := parseLaborSheet(noise, nil, nil)
	if !pn.NeedPick || pn.Reason == "" {
		t.Fatalf("미리보기 사유 없음: %+v", pn)
	}
	picked := parseLaborSheet(vals, nil, &laborCols{Head: -1, Job: 0, Monthly: 1, Daily: 2, Hourly: 3})
	if picked.NeedPick || len(picked.Rows) < 3 {
		t.Fatalf("수동 지정 파싱: %+v", picked)
	}
}

func TestCleanJobNameInParse(t *testing.T) {
	if model.CleanJobName("① IT기획자") != "IT기획자" {
		t.Fatal(model.CleanJobName("① IT기획자"))
	}
}
