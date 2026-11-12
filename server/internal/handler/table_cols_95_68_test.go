package handler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func tableColsTemplateFiles(t *testing.T) []string {
	t.Helper()
	root := filepath.Join(testWebRoot(t), "templates")
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(b)
		if strings.Contains(s, "<table") && (strings.Contains(s, "<th") || strings.Contains(s, "<thead")) {
			rel, _ := filepath.Rel(root, path)
			out = append(out, filepath.ToSlash(rel))
			return nil
		}
		if strings.Contains(s, "wl-head") {
			rel, _ := filepath.Rel(root, path)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var tableColsSkipFiles = []string{
	"admin/data_reset.html",
	"admin/mail.html",
	"admin/notify.html",
	"admin/system.html",
	"as/_action_moved.html",
	"as/_gap_receipts.html",
	"as/_history_table.html",
	"as/action.html",
	"as/show.html",
	"asset/import.html",
	"asset/show.html",
	"customer/org_import.html",
	"items/form.html",
	"maintenance/plan_show.html",
	"meeting/show.html",
	"meeting/weekly.html",
	"orders/form.html",
	"orders/show.html",
	"product_group/import.html",
	"project/changes.html",
	"project/import.html",
	"project/show.html",
	"quotes/form.html",
	"quotes/labor_upload.html",
	"quotes/show.html",
	"sales/_activity_73i.html",
	"sales/group_show.html",
	"space/edit.html",
	"work_group/_form.html",
	"workboard/project_work_action.html",
	"workboard/task_show.html",
}

func Test95_68_Survey94Templates(t *testing.T) {
	got := tableColsTemplateFiles(t)
	if len(got) != 100 {
		t.Fatalf("표/grid 템플릿 %d개 want 100: %v", len(got), got)
	}
	set := map[string]bool{}
	for _, f := range got {
		set[f] = true
	}
	for _, f := range tableColsSkipFiles {
		if !set[f] {
			t.Errorf("제외 목록에 있는데 조사 대상이 아니다: %s", f)
		}
	}
	if set["workboard/board_list.html"] {
		t.Fatal("제거된 board_list.html 이 조사 대상에 있다")
	}
	if !set["as/_list_table.html"] || !set["product_group/list.html"] || !set["work_item_template/list.html"] {
		t.Fatal("자동 적용 리스트가 빠졌다")
	}
	if len(tableColsSkipFiles) != 31 {
		t.Fatalf("제외 %d want 31", len(tableColsSkipFiles))
	}
	if 100-len(tableColsSkipFiles) != 69 {
		t.Fatalf("자동 적용 %d want 69", 100-len(tableColsSkipFiles))
	}
}

func Test95_68_LayoutScriptAndSkipAttrs(t *testing.T) {
	root := testWebRoot(t)
	base, err := os.ReadFile(filepath.Join(root, "templates", "layout", "base.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(base), "/static/js/table-cols.js") {
		t.Fatal("base.html 에 table-cols.js 없음")
	}
	css, err := os.ReadFile(filepath.Join(root, "css", "input.css"))
	if err != nil {
		t.Fatal(err)
	}
	cs := string(css)
	if !strings.Contains(cs, ".col-resize-handle::after") {
		t.Fatal("핸들 구분선 CSS 없음")
	}
	if strings.Contains(cs, ".table-cols { table-layout: fixed; width: 100%") {
		t.Fatal("table-cols 가 width 100%")
	}
	tpl := filepath.Join(root, "templates")
	for _, rel := range tableColsSkipFiles {
		b, err := os.ReadFile(filepath.Join(tpl, rel))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "data-table-cols-skip") {
			t.Errorf("제외 표시 없음: %s", rel)
		}
	}
	for _, rel := range []string{"as/_list_table.html", "product_group/list.html", "contracts/list.html", "work/list.html"} {
		b, err := os.ReadFile(filepath.Join(tpl, rel))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "data-table-cols-skip") {
			t.Errorf("리스트에 skip 이 붙었다: %s", rel)
		}
	}
}

func Test95_68_BoardListGridAndASListControls(t *testing.T) {
	root := testWebRoot(t)
	as, err := os.ReadFile(filepath.Join(root, "templates", "as", "_list_table.html"))
	if err != nil {
		t.Fatal(err)
	}
	ab := string(as)
	if !strings.Contains(ab, `class="js-wl-all"`) || !strings.Contains(ab, `class="js-wl-check"`) {
		t.Fatal("AS 리스트 체크박스가 빠졌다")
	}
	if !strings.Contains(ab, `{{template "sort_th"`) {
		t.Fatal("AS 정렬 헤더가 빠졌다")
	}
	th, err := os.ReadFile(filepath.Join(root, "templates", "sort", "_th.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(th), "relative") || !strings.Contains(string(th), `href="{{.Href}}"`) {
		t.Fatal("정렬 th 링크/relative 없음")
	}
}

func Test95_68_JSModuleStoreCompatAndNoPercentWidth(t *testing.T) {
	js := filepath.Join(testWebRoot(t), "static", "js", "table-cols.js")
	src, err := os.ReadFile(js)
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, need := range []string{
		`table_cols:`, `product_groups`, `projects`, `contracts`,
		`data-table-cols-skip`, `data-grid-cols`, `--wl-cols`,
		`applyAll`, `mergeWidths`,
	} {
		if !strings.Contains(s, need) {
			t.Fatalf("JS 없음: %s", need)
		}
	}
	if !strings.Contains(s, `style.width = sumPx`) && !strings.Contains(s, `style.width = sumPx(widths`) {
		if !strings.Contains(s, `table.style.width = sumPx`) {
			t.Fatal("표 너비를 열 합 px 로 안 넣는다")
		}
	}
	cmd := exec.Command("node", "table-cols_test.js")
	cmd.Dir = filepath.Join(testWebRoot(t), "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js 테스트 실패: %v\n%s", err, out)
	}
}
