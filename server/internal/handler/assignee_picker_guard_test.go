package handler

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 지정 입력이 아닌 조회 필터·숨김 필드·허용된 구 화면.
var assigneeSelectAllow = map[string]string{
	"workboard/register.html":          "목록 필터",
	"workboard/board_list.html":        "목록 필터",
	"workboard/_board_unplaced.html":   "드래그 배치 hidden",
	"work/list.html":                   "목록 필터·재배정(허용 목록, 후속 치환)",
	"meeting/show.html":                "목록 필터",
	"meeting/weekly.html":              "목록 필터",
	"work_status/timeline.html":        "목록 필터",
	"as/form.html":                     "접수 장비별 담당(후속 치환)",
	"as/show.html":                     "배정(후속 치환)",
	"as/work_action.html":              "하부업무(후속 치환)",
	"as/search.html":                   "목록 필터",
	"workboard/task_show.html":         "상세 수정(후속 치환)",
	"workboard/_subtask_modal.html":    "하부 업무(후속 치환)",
	"admin_work/form.html":             "예전 화면",
	"admin_work/_subtask_modal.html":   "예전 화면",
	"plan/unplanned.html":              "미계획 지정 Mode=single 후속",
	"meeting/_item_edit.html":          "회의 항목(후속)",
	"layout/base.html":                 "헤더 빠른등록(후속)",
	"maintenance/plan_manage.html":     "계획 관리 필터/지정(후속)",
	"workboard/project_work_action.html": "작업 조치(후속)",
	"workboard/_modals.html":           "mnt-quick-assignee 간이등록 JS",
}

var assignInputSelectRe = regexp.MustCompile(`(?s)<select[^>]*name="(assignee|assigned_to_code)"`)

func TestAssigneePickerGuardNoNewSelects(t *testing.T) {
	root := filepath.Join("web", "templates")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !assignInputSelectRe.Match(b) {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(os.PathSeparator)))
		if _, ok := assigneeSelectAllow[rel]; ok {
			return nil
		}
		t.Errorf("담당자 지정 select 금지: %s (assignee_picker 를 쓰거나 허용 목록에 사유를 적으세요)", rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAssigneePickerGuardCatchesSample(t *testing.T) {
	sample := []byte(`<select name="assignee" class="x">`)
	if !assignInputSelectRe.Match(sample) {
		t.Fatal("가드 정규식이 위반을 못 잡음")
	}
}
