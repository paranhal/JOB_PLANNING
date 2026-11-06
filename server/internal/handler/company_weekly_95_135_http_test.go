package handler

import (
	"path/filepath"
	"strings"
	"testing"
)

func Test95_135WeeklyUnlinkedLabels(t *testing.T) {
	root := findTemplateRoot(t)
	rows := string(mustRead(t, filepath.Join(root, "stats", "company_weekly_rows.html")))
	weekly := string(mustRead(t, filepath.Join(root, "stats", "company_weekly.html")))
	un := string(mustRead(t, filepath.Join(root, "stats", "_weekly_unassigned.html")))
	if !strings.Contains(rows, `value="unlinked"`) || !strings.Contains(rows, "사업 미연결 업무(통칭)") {
		t.Fatal("행 관리 통칭 옵션")
	}
	if !strings.Contains(weekly, "실적") || !strings.Contains(weekly, "회의일") {
		t.Fatal("주차 제목 실적·회의일")
	}
	if !strings.Contains(un, "사업 미연결 업무(통칭)") {
		t.Fatal("미포함 안내")
	}
}
