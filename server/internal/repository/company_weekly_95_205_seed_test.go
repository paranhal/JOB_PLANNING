package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

// 95-205: 캡처와 같은 구조의 TEST 데이터. 사업 WP-A(행 R-A) 한 곳에 연속·중복 후보를 모두 심는다.
func seed205(t *testing.T) *StatsRepo {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "r205.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ex := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	ex(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES ('O03','O03','SI','SI',1,3)`)
	ex(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,is_test) VALUES ('WP-A','충남도서관 사업','active','O03','project',0),('WP-B','다른 사업','active','O03','project',0)`)
	ex(`INSERT INTO weekly_report_rows(row_key,sheet_row,display_name,project_id,row_kind,is_active,org_id,sort_order) VALUES ('R-A',20,'충남도서관','WP-A','project',1,'O03',1),('R-B',21,'다른','WP-B','project',1,'O03',2)`)
	// 금주(11/9~11/15) 완료
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,recurrence_role) VALUES ('P1','admin','제안서 작성','open','WP-A','O03','parent')`)
	for i, d := range []string{"2026-11-09", "2026-11-10", "2026-11-11"} {
		ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,parent_task_id,recurrence_role,work_date,complete_date) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("D%d", i), "admin", "제안서 작성", "complete", "WP-A", "O03", "P1", "occurrence", d, d)
	}
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,work_date,complete_date) VALUES
		('S1','admin','사업 제안서 작성','complete','WP-A','O03','2026-11-11','2026-11-11'),
		('S2','admin','사업 제안서 작성','complete','WP-A','O03','2026-11-11','2026-11-11'),
		('S3','admin','  사업  제안서 작성 ','complete','WP-A','O03','2026-11-12','2026-11-12'),
		('Z1','admin','다른 제목 업무','complete','WP-A','O03','2026-11-12','2026-11-12'),
		('B1','admin','사업 제안서 작성','complete','WP-B','O03','2026-11-11','2026-11-11')`)
	// 하위 일정(role 빈 값 + parent_task_id): 상위 업무 자신은 완료 아님
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id) VALUES ('PS','admin','입찰 대응','open','WP-A','O03')`)
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,parent_task_id,work_date,complete_date) VALUES
		('PS-1','admin','입찰서 검토','complete','WP-A','O03','PS','2026-11-12','2026-11-12'),
		('PS-2','admin','입찰서 작성','complete','WP-A','O03','PS','2026-11-13','2026-11-13')`)
	// 차주(11/16~11/22) 이월: 상위 다른 같은 제목 2개, 같은 업무 2건 등록
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,recurrence_role) VALUES
		('P2','admin','신구대조표 작성','open','WP-A','O03','parent'),
		('P3','admin','신구대조표 작성','open','WP-A','O03','parent'),
		('P4','admin','제안서 작성','open','WP-A','O03','parent')`)
	for i, d := range []string{"2026-11-10", "2026-11-11", "2026-11-12"} {
		ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,parent_task_id,recurrence_role,work_date) VALUES (?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("C%d", i), "admin", "신구대조표 작성", "open", "WP-A", "O03", "P2", "occurrence", d)
		ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,parent_task_id,recurrence_role,work_date) VALUES (?,?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("E%d", i), "admin", "신구대조표 작성", "open", "WP-A", "O03", "P3", "occurrence", d)
	}
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id) VALUES ('X1','admin','입찰서류 준비','open','WP-A','O03'),('X2','admin','입찰서류 준비','open','WP-A','O03')`)
	// 같은 상위: 과거 2건(이월) + 차주 1건(계획)
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,parent_task_id,recurrence_role,work_date) VALUES
		('F1','admin','제안서 작성','open','WP-A','O03','P4','occurrence','2026-11-05'),
		('F2','admin','제안서 작성','open','WP-A','O03','P4','occurrence','2026-11-06'),
		('F3','admin','제안서 작성','open','WP-A','O03','P4','occurrence','2026-11-17')`)
	// 차주 예정: 연속 3일 하위 일정 / 비연속 2일
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id) VALUES ('PL','admin','현장 실사','open','WP-A','O03')`)
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,parent_task_id,work_date) VALUES
		('PL-1','admin','실사 1일차','open','WP-A','O03','PL','2026-11-17'),
		('PL-2','admin','실사 2일차','open','WP-A','O03','PL','2026-11-18'),
		('PL-3','admin','실사 3일차','open','WP-A','O03','PL','2026-11-20')`)
	return NewStatsRepo(db)
}

func draft205(t *testing.T, repo *StatsRepo) model.CompanyWeeklyRow {
	t.Helper()
	d, err := repo.BuildCompanyWeeklyDraftOrg(time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range d.Rows {
		if r.RowKey == "R-A" {
			return r
		}
	}
	t.Fatal("R-A 행 없음")
	return model.CompanyWeeklyRow{}
}

func dump205(r model.CompanyWeeklyRow) string {
	var b strings.Builder
	b.WriteString("== 금주 추진내역\n")
	for _, l := range r.AutoDone {
		fmt.Fprintf(&b, "  [%s] %s\n", l.Key, l.Text)
	}
	b.WriteString("== 차주 추진계획\n")
	for _, l := range r.AutoPlan {
		fmt.Fprintf(&b, "  [%s] %s\n", l.Key, l.Text)
	}
	return b.String()
}

// JP_SAMPLE_OUT 이 있으면 현재 줄 생성 결과를 그 파일에 쓴다(수정 전·후 비교용).
func TestCompanyWeekly95_205_Sample(t *testing.T) {
	out := os.Getenv("JP_SAMPLE_OUT")
	if out == "" {
		t.Skip("JP_SAMPLE_OUT 없음")
	}
	if err := os.WriteFile(out, []byte(dump205(draft205(t, seed205(t)))), 0o644); err != nil {
		t.Fatal(err)
	}
}
