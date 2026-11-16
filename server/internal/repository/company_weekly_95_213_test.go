package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
)

// 95-213: 업무 상태별 금주 추진내역 / 차주 추진계획 줄 규칙. 실적 주 11/9~11/15, 차주 11/16~11/22.
func seed213(t *testing.T) *StatsRepo {
	t.Helper()
	db, err := InitDB(filepath.Join(t.TempDir(), "r213.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ex := func(q string, a ...interface{}) {
		if _, err := db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	ex(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES ('O03','O03','SI','SI',1,3),('O04','O04','타팀','타팀',1,4)`)
	ex(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,is_test) VALUES ('WP-A','충남도서관 사업','active','O03','project',0),('WP-T','시험 사업','active','O03','project',1)`)
	ex(`INSERT INTO weekly_report_rows(row_key,sheet_row,display_name,project_id,row_kind,is_active,org_id,sort_order) VALUES ('R-A',20,'충남도서관','WP-A','project',1,'O03',1)`)
	task := func(id, title, status, date string, extra ...string) {
		ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,work_date) VALUES (?,?,?,?,?,?,?)`, id, "admin", title, status, "WP-A", "O03", date)
	}
	act := func(taskID, content, at string) {
		ex(`INSERT INTO work_activities(activity_id,task_id,activity_type,content,created_at) VALUES (?,?,?,?,?)`, "Y-"+taskID+at+content[:2], taskID, "other", content, at)
	}
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,work_date,complete_date) VALUES ('a','admin','(a)완료','complete','WP-A','O03','2026-11-09','2026-11-09')`)
	task("b1", "(b1)진행중+조치", "in_progress", "2026-11-10")
	act("b1", "방문하여  서버 점검\n완료", "2026-11-10 10:00:00")
	task("b2", "(b2)진행중 조치없음", "in_progress", "2026-11-10")
	task("c1", "(c1)미완료+조치", "open", "2026-11-12")
	act("c1", "자료 전달", "2026-11-12 09:00:00")
	task("c2", "(c2)미완료 조치없음", "open", "2026-11-12")
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id) VALUES ('d1','admin','(d1)일정없음+조치','open','WP-A','O03'),('d2','admin','(d2)일정없음 조치없음','open','WP-A','O03')`)
	act("d1", "전화 협의", "2026-11-11 15:00:00")
	task("e1", "(e1)취소+조치", "cancelled", "2026-11-10")
	act("e1", "고객 요청으로 중단", "2026-11-10 17:00:00")
	task("e2", "(e2)취소 조치없음", "cancelled", "2026-11-10")
	// 실적 주 밖 조치 기록은 쓰지 않는다
	task("o1", "(o1)조치가 주 밖", "open", "2026-11-12")
	act("o1", "지난주 기록", "2026-11-04 09:00:00")
	// 긴 조치 기록
	task("l1", "(l1)긴 조치", "open", "2026-11-12")
	act("l1", strings.Repeat("가나다라마바사아자차", 12), "2026-11-12 09:00:00")
	// 차주 계획: 하위 일정(차주) / 세부 조치 항목(차주) / 취소+차주 계획 / 계획 없음
	task("p1", "(p1)상위+차주하위", "open", "2026-11-10")
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,parent_task_id,work_date) VALUES ('p1c','admin','(p1c)재방문','open','WP-A','O03','p1','2026-11-18')`)
	task("p2", "(p2)조치항목계획", "open", "2026-11-10")
	ex(`INSERT INTO work_actions(action_id,task_id,title,status,scheduled_date) VALUES ('wa1','p2','재방문 설치','todo','2026-11-19'),('wa2','p2','구간밖항목','todo','2026-11-30'),('wa3','p2','끝난항목','complete','2026-11-19')`)
	task("p3", "(p3)취소+차주계획", "cancelled", "2026-11-10")
	act("p3", "취소 전 협의", "2026-11-10 11:00:00")
	ex(`INSERT INTO work_actions(action_id,task_id,title,status,scheduled_date) VALUES ('wa4','p3','대체 방문','todo','2026-11-20')`)
	// 다른 조직·테스트 사업
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,work_date) VALUES ('x1','admin','타조직','open','WP-A','O04','2026-11-10'),('x2','admin','테스트사업','open','WP-T','O03','2026-11-10')`)
	return NewStatsRepo(db)
}

func row213(t *testing.T, s *StatsRepo) model.CompanyWeeklyRow {
	t.Helper()
	d, err := s.BuildCompanyWeeklyDraftOrg(time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range d.Rows {
		if r.RowKey == "R-A" {
			return r
		}
	}
	t.Fatal("행 없음")
	return model.CompanyWeeklyRow{}
}

func texts213(a []model.CompanyWeeklyAutoLine) string { return strings.Join(linesOf(a), "\n") }

func linesOf(a []model.CompanyWeeklyAutoLine) []string {
	var o []string
	for _, l := range a {
		o = append(o, l.Text)
	}
	return o
}

func TestCompanyWeekly95_213_Table(t *testing.T) {
	r := row213(t, seed213(t))
	done, plan := texts213(r.AutoDone), texts213(r.AutoPlan)
	wantDone := []string{
		"(a)완료(11/9)",
		"(b1)진행중+조치: 방문하여 서버 점검 완료(진행 중)",
		"(b2)진행중 조치없음(진행 중)",
		"(c1)미완료+조치: 자료 전달(진행 중)",
		"(c2)미완료 조치없음(진행 중)",
		"(d1)일정없음+조치: 전화 협의(진행 중)",
		"(e1)취소+조치: 고객 요청으로 중단(취소)",
		"(p3)취소+차주계획: 취소 전 협의(취소)",
	}
	for _, w := range wantDone {
		if !strings.Contains(done, w) {
			t.Errorf("금주에 %q 없음:\n%s", w, done)
		}
	}
	for _, no := range []string{"(d2)", "(e2)", "타조직", "테스트사업"} {
		if strings.Contains(done, no) {
			t.Errorf("금주에 나오면 안 됨 %q:\n%s", no, done)
		}
	}
	if !strings.Contains(done, "(o1)조치가 주 밖(진행 중)") {
		t.Errorf("주 밖 조치는 쓰지 않고 일정 기준 진행 줄:\n%s", done)
	}
	// 차주: 계획이 없으면 이월 1번, 계획이 있으면 계획 줄만
	for _, w := range []string{"(b1)진행중+조치(이월)", "(b2)진행중 조치없음(이월)", "(c1)미완료+조치(이월)", "(c2)미완료 조치없음(이월)", "(d1)일정없음+조치(이월)", "(d2)일정없음 조치없음(이월)"} {
		if strings.Count(plan, w) != 1 {
			t.Errorf("차주 이월 %q 1회 아님:\n%s", w, plan)
		}
	}
	if strings.Contains(plan, "(e1)") || strings.Contains(plan, "(e2)") {
		t.Errorf("취소 건은 계획이 없으면 차주에 없다:\n%s", plan)
	}
	if strings.Contains(plan, "(p1)상위+차주하위(이월)") || strings.Contains(plan, "(p2)조치항목계획(이월)") || strings.Contains(plan, "(p3)") && strings.Contains(plan, "(이월)") && strings.Contains(plan, "(p3)취소+차주계획(이월)") {
		t.Errorf("계획이 있으면 이월 줄이 없다:\n%s", plan)
	}
	for _, w := range []string{"(p1)상위+차주하위(11/18)", "(p2)조치항목계획: 재방문 설치(11/19)", "(p3)취소+차주계획: 대체 방문(11/20)"} {
		if strings.Count(plan, w) != 1 {
			t.Errorf("차주 계획 줄 %q:\n%s", w, plan)
		}
	}
	if strings.Contains(plan, "구간밖항목") || strings.Contains(plan, "끝난항목") {
		t.Errorf("구간 밖·끝난 조치 항목은 계획 아님:\n%s", plan)
	}
	// 완료 줄과 진행 줄은 별도, Key 접두
	for _, l := range r.AutoDone {
		if strings.Contains(l.Text, "진행 중)") && !strings.Contains(l.Key, "prog:") {
			t.Errorf("진행 줄 Key %+v", l)
		}
	}
	// 화면·엑셀·HWPX 공통 문자열
	if !strings.Contains(r.PrevText, "·(b1)진행중+조치: 방문하여 서버 점검 완료(진행 중)") {
		t.Errorf("PrevText: %s", r.PrevText)
	}
	// 같은 구역 중복 없음
	seen := map[string]bool{}
	for _, l := range append(append([]model.CompanyWeeklyAutoLine{}, r.AutoDone...), r.AutoPlan...) {
		if seen[l.Text] {
			t.Errorf("중복 줄 %q", l.Text)
		}
		seen[l.Text] = true
	}
}

func TestCompanyWeekly95_213_Abbrev(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"  가  나\n다 ", ""}, "가 나 다"},
		{[]string{"A", "B", "A"}, "A·B"},
		{nil, ""},
		{[]string{strings.Repeat("한", 100)}, strings.Repeat("한", 80) + "…"},
		{[]string{strings.Repeat("한", 80)}, strings.Repeat("한", 80)},
		{[]string{"서버 #1 (점검) / ✓"}, "서버 #1 (점검) / ✓"},
	}
	for _, c := range cases {
		if got := abbrevWeeklyAction(c.in); got != c.want {
			t.Errorf("abbrev(%q)=%q want %q", c.in, got, c.want)
		}
	}
	r := row213(t, seed213(t))
	if !strings.Contains(texts213(r.AutoDone), "…(진행 중)") {
		t.Errorf("긴 조치 줄임:\n%s", texts213(r.AutoDone))
	}
}

// 조치·진행 대상이 없는 데이터는 결과가 같다(골든): 완료 줄만 있는 주.
func TestCompanyWeekly95_213_NoStatusNoChange(t *testing.T) {
	s := seed213(t)
	if _, err := s.db.Exec(`UPDATE work_tasks SET status='complete', complete_date=work_date WHERE status!='complete' AND work_date >= '2026-11-09' AND work_date < '2026-11-16'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM work_activities`); err != nil {
		t.Fatal(err)
	}
	r := row213(t, s)
	for _, l := range r.AutoDone {
		if strings.Contains(l.Text, "진행 중") || strings.Contains(l.Text, "(취소)") {
			t.Errorf("상태 줄 대상이 없는데 표시: %s", l.Text)
		}
	}
}

// 활동 행(기술영업): 일정이 실적 주인 미완료 업무가 금주에 「(진행 중)」으로, 구간 밖은 안 나온다.
func TestCompanyWeekly95_213_ActivityRowProgress(t *testing.T) {
	s := seed213(t)
	for _, q := range []string{
		`INSERT INTO weekly_report_rows(row_key,sheet_row,display_name,project_id,row_kind,is_active,org_id,sort_order,activity_kind) VALUES ('R-ACT',21,'기술영업','','activity',1,'O03',2,'tech')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,org_id,work_date,activity_kind) VALUES ('f','admin','활동미완료','open','O03','2026-11-11','tech'),('g','admin','활동구간밖','open','O03','2026-11-20','tech')`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	d, err := s.BuildCompanyWeeklyDraftOrg(time.Date(2026, 11, 16, 0, 0, 0, 0, time.Local), "O03")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range d.Rows {
		if r.RowKey == "R-ACT" {
			if !strings.Contains(texts213(r.AutoDone), "활동미완료(진행 중)") || strings.Contains(texts213(r.AutoDone), "활동구간밖") {
				t.Errorf("활동 행 금주:\n%s", texts213(r.AutoDone))
			}
			return
		}
	}
	t.Fatal("활동 행 없음")
}
