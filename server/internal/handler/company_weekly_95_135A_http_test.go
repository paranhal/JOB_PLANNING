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
	if strings.Contains(html, "정기점검에서 사업 지정") {
		t.Fatal("95-140: 135A 차단 안내는 제거한다")
	}
	if strings.Contains(js, "submit.hidden = onlyMnt") {
		t.Fatal("95-140: 연결 버튼을 숨기지 않는다")
	}
}

func Test95_135A_LinkHandlerDoesNotWriteMntProject(t *testing.T) {
	src := string(mustRead(t, filepath.Join(findHandlerRoot(t), "stats_company_weekly_link.go")))
	if !strings.Contains(src, "SetVisitProject") {
		t.Fatal("95-140: 방문 사업 연결이 없다")
	}
}
