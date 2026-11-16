package handler

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

// 95-213: 조치 요지가 있는 「진행 중」·「취소」 줄과 차주 계획 줄이 화면·엑셀·HWPX 에서 같다.
func TestHTTP95_213_ScreenExcelHWPXSameLines(t *testing.T) {
	v := setup205(t)
	ex := func(q string, a ...interface{}) {
		if _, err := v.db.Exec(q, a...); err != nil {
			t.Fatal(err)
		}
	}
	ex(`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,work_date) VALUES ('Q1','admin','서버 점검','in_progress','WP-A','O03','2026-10-21'),('Q2','admin','납품 협의','cancelled','WP-A','O03','2026-10-21')`)
	ex(`INSERT INTO work_activities(activity_id,task_id,activity_type,content,created_at) VALUES ('YA','Q1','other','현장 방문하여  로그 확인','2026-10-21 10:00:00'),('YB','Q2','other','고객 요청으로 중단','2026-10-22 10:00:00')`)
	ex(`INSERT INTO work_actions(action_id,task_id,title,status,scheduled_date) VALUES ('WA','Q1','재방문','todo','2026-10-28')`)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	proj := card205(t, v.view205(t), "충남도서관")
	done, plan := strings.Join(weeklyLineTexts(proj.Done), "\n"), strings.Join(weeklyLineTexts(proj.Plan), "\n")
	wantDone := []string{"서버 점검: 현장 방문하여 로그 확인(진행 중)", "납품 협의: 고객 요청으로 중단(취소)"}
	wantPlan := "서버 점검: 재방문(10/28)"
	for _, w := range wantDone {
		if !strings.Contains(done, w) {
			t.Fatalf("금주에 %q 없음:\n%s", w, done)
		}
	}
	if !strings.Contains(plan, wantPlan) || strings.Contains(plan, "서버 점검(이월)") || strings.Contains(plan, "납품 협의") {
		t.Fatalf("차주:\n%s", plan)
	}
	body := v.do(t, "GET", w205+"&edit=0", nil, oa).Body.String()
	x := v.do(t, "GET", "/stats/company-weekly/export?week=2026-10-19&org=O03&scope=team", nil, oa)
	f, err := excelize.OpenReader(bytes.NewReader(x.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, _ := f.GetRows(f.GetSheetName(0))
	xl := ""
	for _, r := range rows {
		xl += strings.Join(r, "|") + "\n"
	}
	sec := zipSection205(t, v.do(t, "GET", "/stats/company-weekly/export-hwpx?week=2026-10-19&org=O03&scope=team", nil, oa).Body.Bytes())
	for _, w := range append(wantDone, wantPlan) {
		if !strings.Contains(body, w) || !strings.Contains(xl, w) || !strings.Contains(sec, w) {
			t.Fatalf("화면·엑셀·HWPX 불일치 %q 화면=%v 엑셀=%v hwpx=%v", w, strings.Contains(body, w), strings.Contains(xl, w), strings.Contains(sec, w))
		}
	}
}
