package repository

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var weeklyFromRe = regexp.MustCompile(`(?i)FROM\s+(maintenance_visits|as_receipts|work_tasks)\b`)

func weeklySQLHasOrgGuard(src string) []string {
	allow := []string{
		"mustOrgSQL", "AppendOrgSQL", "filterMnt", "filterAS", "filterAdmin",
		"mntVisitOrgSQL", "reportOrgID", "workTaskVisibleOrgSQL",
	}
	var missing []string
	for _, block := range splitSQLLikeBlocks(src) {
		if !weeklyFromRe.MatchString(block) {
			continue
		}
		ok := false
		for _, a := range allow {
			if strings.Contains(block, a) {
				ok = true
				break
			}
		}
		if !ok {
			missing = append(missing, strings.TrimSpace(block[:min(80, len(block))]))
		}
	}
	return missing
}

func splitSQLLikeBlocks(src string) []string {
	// 함수 단위로 잘라 같은 함수 안의 필터 호출을 본다.
	parts := strings.Split(src, "\nfunc ")
	out := make([]string, 0, len(parts))
	for i, p := range parts {
		if i == 0 {
			out = append(out, p)
			continue
		}
		out = append(out, "func "+p)
	}
	return out
}

func TestWeeklyReportSQLHasOrgFilter(t *testing.T) {
	root := "."
	if _, err := os.Stat("company_weekly.go"); err != nil {
		root = "internal/repository"
	}
	files := []string{
		"company_weekly.go", "company_weekly_activity.go", "stats_weekly.go",
	}
	var bad []string
	for _, name := range files {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range weeklySQLHasOrgGuard(string(b)) {
			bad = append(bad, name+": "+m)
		}
	}
	handlerRoot := filepath.Join("..", "handler")
	if _, err := os.Stat(filepath.Join(handlerRoot, "stats_weekly_excel.go")); err != nil {
		handlerRoot = "internal/handler"
	}
	for _, name := range []string{"stats_weekly_excel.go", "meeting_weekly.go", "meeting_weekly_xlsx.go"} {
		p := filepath.Join(handlerRoot, name)
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, m := range weeklySQLHasOrgGuard(string(b)) {
			bad = append(bad, name+": "+m)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("조직 조건 없는 주간 보고 쿼리:\n%s", strings.Join(bad, "\n"))
	}
}

func TestWeeklyReportSQLGuardCatchesMissingOrg(t *testing.T) {
	src := "func bad() {\n\tdb.Query(`SELECT * FROM as_receipts ar WHERE 1=1`)\n}\n"
	got := weeklySQLHasOrgGuard(src)
	if len(got) == 0 {
		t.Fatal("조건을 뺀 샘플 SQL을 놓쳤다")
	}
}
