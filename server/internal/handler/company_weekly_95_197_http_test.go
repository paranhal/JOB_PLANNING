package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-197: 업무 활동 분류(경영지원·기술영업·…)를 사업처럼 블록으로 보인다.
func seed197(t *testing.T) env190 {
	t.Helper()
	v := setup190(t, true) // 「시험 사업」 수동 행(sort 1)
	v.e.POST("/stats/company-weekly/tasks/project", v.h.Stats.LinkWeeklyTasksProject, v.h.Auth.AuthMiddleware)
	repo := v.h.Stats.repo
	// 기술영업·자산관리 활동 행을 사업 행보다 앞(sort 0)에 둔다: 사업이 먼저 나오는지 본다
	if _, err := repo.InsertWeeklyReportRow(repository.WeeklyReportRowInput{OrgID: "O03", DisplayName: "기술영업활동", Kind: "activity", ActivityKind: "tech", SortOrder: 0, Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertWeeklyReportRow(repository.WeeklyReportRowInput{OrgID: "O03", DisplayName: "자산관리활동", Kind: "activity", ActivityKind: "asset", SortOrder: 0, Active: true}); err != nil {
		t.Fatal(err)
	}
	db := repo.DB()
	for _, q := range []string{
		`INSERT INTO work_projects(project_id,name,status,org_id) VALUES ('WPT197','연결할 사업','active','O03')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date,source_type,activity_kind) VALUES
			('T197-M','admin','경영 회의 준비','complete',NULL,'O03','2026-10-21','2026-10-21','','mgmt'),
			('T197-T','admin','제안서 검토','complete',NULL,'O03','2026-10-22','2026-10-22','','tech'),
			('T197-N','admin','분류 없는 업무','complete',NULL,'O03','2026-10-22','2026-10-22','',NULL)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return v
}

func cardTitleIdx(body, title string) int {
	return strings.Index(body, `<b class="block text-ink">`+title+`</b>`)
}

func TestHTTP95_197_ActivityBlocks(t *testing.T) {
	v := seed197(t)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	body := v.do(t, "GET", w190+"&edit=0", nil, oa).Body.String()
	// 값 있는 활동 블록만: 경영지원(가상 행)·기술영업(활동 행). 값 없는 자산관리·업무지원 등은 없다
	for _, want := range []string{"경영지원활동", "기술영업활동"} {
		if cardTitleIdx(body, want) < 0 {
			t.Fatalf("%s 블록이 없다", want)
		}
	}
	for _, no := range []string{"자산관리활동", "업무지원활동", "전략및혁신활동", "고객서비스활동", "역량강화활동"} {
		if cardTitleIdx(body, no) >= 0 {
			t.Fatalf("값이 없는 %s 가 보인다", no)
		}
	}
	// 사업 블록이 먼저, 활동 블록은 그 뒤
	p := cardTitleIdx(body, "시험 사업")
	if p < 0 || p > cardTitleIdx(body, "기술영업활동") || p > cardTitleIdx(body, "경영지원활동") {
		t.Fatalf("사업 블록이 활동보다 앞이 아니다")
	}
	// 분류된 업무 줄이 해당 블록에 들어 있다
	if !strings.Contains(body, "경영 회의 준비") || !strings.Contains(body, "제안서 검토") {
		t.Fatal("분류한 업무 줄이 보고서에 없다")
	}
	// 분류된 업무는 「사업 미연결」 구역에 없고, 분류 없는 업무만 있다
	blk := unassignedNoProjectBlock(t, body)
	if strings.Contains(blk, "경영 회의 준비") || strings.Contains(blk, "제안서 검토") {
		t.Fatal("활동에 연결된 업무가 사업 미연결 구역에 남아 있다")
	}
	if !strings.Contains(blk, "분류 없는 업무") {
		t.Fatal("분류 없는 업무가 미연결 구역에 없다")
	}
}

func TestHTTP95_197_OrgIsolationAndEmpty(t *testing.T) {
	v := seed197(t)
	db := v.h.Stats.repo.DB()
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES ('O01','O01','도서관사업부','도서관',1,1)`); err != nil {
		t.Fatal(err)
	}
	oa1 := jwtCookieOrg(t, model.RoleOrgAdmin, "O01")
	body := v.do(t, "GET", "/stats/company-weekly?week=2026-10-19&org=O01&edit=0", nil, oa1).Body.String()
	for _, no := range []string{"경영지원활동", "기술영업활동"} {
		if cardTitleIdx(body, no) >= 0 {
			t.Fatalf("다른 조직 업무가 O01 보고서에 보인다(%s)", no)
		}
	}
}

func TestHTTP95_197_LinkActivityAndRelink(t *testing.T) {
	v := seed197(t)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	next := w190
	form := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "next": {next}, "item": {"work_task:T197-N"}, "activity_kind": {"service"}}
	r := v.do(t, "POST", "/stats/company-weekly/tasks/project", form, oa)
	if r.Code != http.StatusSeeOther {
		t.Fatalf("활동 연결 %d %s", r.Code, r.Body.String())
	}
	db := v.h.Stats.repo.DB()
	var ak, pid string
	_ = db.QueryRow(`SELECT COALESCE(activity_kind,''), COALESCE(project_id,'') FROM work_tasks WHERE task_id='T197-N'`).Scan(&ak, &pid)
	if ak != "service" || pid != "" {
		t.Fatalf("활동 분류 저장 %q/%q", ak, pid)
	}
	body := v.do(t, "GET", w190+"&edit=0", nil, oa).Body.String()
	if cardTitleIdx(body, "고객서비스활동") < 0 || !strings.Contains(body, "분류 없는 업무") {
		t.Fatal("연결 뒤 고객서비스 블록이 없다")
	}
	if i := strings.Index(body, "사업이 연결되지 않은 업무"); i >= 0 && strings.Contains(body[i:], "분류 없는 업무") {
		t.Fatal("연결한 업무가 미연결 구역에 남아 있다")
	}
	// 활동 → 다른 활동으로 변경
	form.Set("activity_kind", "growth")
	if r := v.do(t, "POST", "/stats/company-weekly/tasks/project", form, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("활동 변경 %d", r.Code)
	}
	_ = db.QueryRow(`SELECT COALESCE(activity_kind,'') FROM work_tasks WHERE task_id='T197-N'`).Scan(&ak)
	if ak != "growth" {
		t.Fatalf("활동 변경 저장 %q", ak)
	}
	// 활동 → 사업으로 변경하면 활동 분류가 해제된다
	pf := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "next": {next}, "item": {"work_task:T197-N"}, "project_id": {"WPT197"}}
	if r := v.do(t, "POST", "/stats/company-weekly/tasks/project", pf, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("사업 변경 %d %s", r.Code, r.Body.String())
	}
	_ = db.QueryRow(`SELECT COALESCE(activity_kind,''), COALESCE(project_id,'') FROM work_tasks WHERE task_id='T197-N'`).Scan(&ak, &pid)
	if ak != "" || pid != "WPT197" {
		t.Fatalf("사업 변경 뒤 %q/%q", ak, pid)
	}
	// 사업 → 활동으로 되돌리면 사업 연결이 해제된다
	form.Set("activity_kind", "mgmt")
	if r := v.do(t, "POST", "/stats/company-weekly/tasks/project", form, oa); r.Code != http.StatusSeeOther {
		t.Fatalf("활동으로 복귀 %d", r.Code)
	}
	_ = db.QueryRow(`SELECT COALESCE(activity_kind,''), COALESCE(project_id,'') FROM work_tasks WHERE task_id='T197-N'`).Scan(&ak, &pid)
	if ak != "mgmt" || pid != "" {
		t.Fatalf("복귀 뒤 %q/%q", ak, pid)
	}
	// 잘못된 분류·타 조직·권한 없음
	bad := url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "next": {next}, "item": {"work_task:T197-N"}, "activity_kind": {"nope"}}
	if r := v.do(t, "POST", "/stats/company-weekly/tasks/project", bad, oa); r.Code != http.StatusBadRequest {
		t.Fatalf("잘못된 분류 %d", r.Code)
	}
	other := jwtCookieOrg(t, model.RoleOrgAdmin, "O01")
	form.Set("org", "O03")
	if r := v.do(t, "POST", "/stats/company-weekly/tasks/project", form, other); r.Code == http.StatusSeeOther {
		t.Fatal("타 조직 관리자가 활동 연결을 했다")
	}
	if r := v.do(t, "POST", "/stats/company-weekly/tasks/project", form, jwtCookieRole(t, model.RoleCLevel)); r.Code != http.StatusForbidden {
		t.Fatalf("C-Level %d", r.Code)
	}
}

func TestHTTP95_197_LinkedTasksSectionAndModal(t *testing.T) {
	v := seed197(t)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	body := v.do(t, "GET", w190+"&edit=0", nil, oa).Body.String()
	for _, want := range []string{`data-weekly-linked`, `data-linked-task="T197-M"`, `data-linked-task="T197-T"`, `name="activity_kind"`, `value="biz_support"`, `value="growth"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("%s 없음", want)
		}
	}
	if n := strings.Count(body, `id="weekly-link-modal"`); n != 1 {
		t.Fatalf("연결 창이 %d개", n)
	}
	// 읽기 전용(C-Level)에는 연결 변경 구역이 없다
	if strings.Contains(v.do(t, "GET", w190+"&edit=0", nil, jwtCookieRole(t, model.RoleCLevel)).Body.String(), `data-weekly-linked`) {
		t.Fatal("읽기 전용에 연결 변경 구역")
	}
}

func TestHTTP95_197_SetTaskActivityRepo(t *testing.T) {
	v := seed197(t)
	wb := repository.NewWBRepo(v.h.Stats.repo.DB())
	if _, _, err := wb.SetTaskActivity("O03", "T197-N", "nope"); err == nil {
		t.Fatal("알 수 없는 분류가 통과")
	}
	if _, after, err := wb.SetTaskActivity("O03", "T197-N", "asset"); err != nil || after != "자산관리" {
		t.Fatalf("%q %v", after, err)
	}
	if _, _, err := wb.SetTaskActivity("O03", "T197-N", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := wb.SetTaskActivity("O01", "T197-N", "asset"); err == nil {
		t.Fatal("타 조직 업무가 바뀐다")
	}
}
