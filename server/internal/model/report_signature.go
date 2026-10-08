package model

import (
	"encoding/json"
	"strings"
)

// 조치완료보고서 사인 전사 설정. 표 구조는 그대로 두고 키만 넣는다. §71.3.3-3 · 62-F-1
const (
	SettingReportSignatureSize     = "report.signature.size_hwpunit"
	SettingReportSignatureRightGap = "report.signature.right_gap_hwpunit"
	SettingReportSignatureNudgeY   = "report.signature.nudge_y_hwpunit"
)

// 2026-10-07 확정값. 시드·빈 칸 채움에만 쓴다. 발급 자리는 설정·사람별 칸에서 읽는다.
const (
	DefaultReportSignatureSize     = 4600
	DefaultReportSignatureRightGap = 900
	DefaultReportSignatureNudgeY   = 0
)

// ReportSignatureBox 크기·보정. users.signature_box JSON 과 같다. {"size":4600,"gap":900,"ny":0}
type ReportSignatureBox struct {
	Size int `json:"size"`
	Gap  int `json:"gap"`
	NY   int `json:"ny"`
}

func DefaultReportSignatureBox() ReportSignatureBox {
	return ReportSignatureBox{
		Size: DefaultReportSignatureSize,
		Gap:  DefaultReportSignatureRightGap,
		NY:   DefaultReportSignatureNudgeY,
	}
}

// ParseReportSignatureBox 사람별 칸. 비면 base(전사 기본값). 음수 보정을 막지 않는다.
func ParseReportSignatureBox(raw string, base ReportSignatureBox) ReportSignatureBox {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return base
	}
	var part struct {
		Size *int `json:"size"`
		Gap  *int `json:"gap"`
		NY   *int `json:"ny"`
	}
	if err := json.Unmarshal([]byte(raw), &part); err != nil {
		return base
	}
	out := base
	if part.Size != nil && *part.Size > 0 {
		out.Size = *part.Size
	}
	if part.Gap != nil {
		out.Gap = *part.Gap
	}
	if part.NY != nil {
		out.NY = *part.NY
	}
	return out
}
