package handler

import (
	"path/filepath"
	"strings"
	"testing"
)

func Test95_135A_WeeklyLinkMntDeadEndGuidance(t *testing.T) {
	root := findTemplateRoot(t)
	html := string(mustRead(t, filepath.Join(root, "stats", "_weekly_unassigned.html")))
	js := string(mustRead(t, filepath.Join(findStaticRoot(t), "js", "weekly-link.js")))

	if !strings.Contains(html, "정기점검에서 사업 지정") || !strings.Contains(html, `data-mnt-goto`) {
		t.Fatal("T1 정기점검에서 사업 지정 링크 없음")
	}
	if !strings.Contains(html, `data-weekly-link-close`) || !strings.Contains(html, ">닫기<") {
		t.Fatal("T1 닫기 없음")
	}
	if strings.Contains(html, ">취소<") {
		t.Fatal("T1 취소가 남아 있다")
	}
	if !strings.Contains(js, "asgBox.hidden = onlyMnt") {
		t.Fatal("T1 담당자 숨김 없음")
	}
	if !strings.Contains(js, "submit.hidden = onlyMnt") {
		t.Fatal("T1 연결 버튼 숨김 없음")
	}
	if strings.Contains(js, "UPDATE maintenance_visits") {
		t.Fatal("T3 금지: JS에서 점검 사업 변경")
	}

	if !strings.Contains(js, "이번 연결에서 제외됩니다") {
		t.Fatal("T2 혼합 제외 안내 없음")
	}
	if !strings.Contains(js, "if (isMnt(it)) return") {
		t.Fatal("T2 일괄 연결에서 점검을 빼지 않는다")
	}
	if !strings.Contains(html, "data-href") {
		t.Fatal("T1 점검 일정 href 없음")
	}
}

func Test95_135A_LinkHandlerDoesNotWriteMntProject(t *testing.T) {
	src := string(mustRead(t, filepath.Join(findHandlerRoot(t), "stats_company_weekly_link.go")))
	if !strings.Contains(src, "weeklyKindMnt") {
		t.Fatal("T4 점검 분기 없음")
	}
	if strings.Contains(src, "SetVisitProject") || strings.Contains(src, "UPDATE maintenance_visits SET project") {
		t.Fatal("T4 점검 사업 변경을 만들었다")
	}
	if !strings.Contains(src, "점검 일정에서 사업 확인") {
		t.Fatal("T4 서버가 점검을 건너뛰는 안내가 없다")
	}
}
