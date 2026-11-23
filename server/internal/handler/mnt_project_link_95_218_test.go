package handler

import (
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-218 ①②: 정기점검 사업 연결 허용 판정(사업 사이트 또는 범위 규칙)과 조치 완료 시 사업 확정.

type linkCase218 struct {
	name        string
	siteOn      []string // c1·c2 를 활성 사이트로 가진 사업
	siteOnParen []string // 상위 기관 P1 만 사이트로 가진 사업
	inactive    bool
	rule        bool
	two         bool
	linkTo      string
	wantLink    bool
	wantProj    string // 조치 완료 후 V2 의 project_id
	wantDone    bool   // 완료가 되는가
}

func TestVisitProjectLinkAndCompleteTable95_218(t *testing.T) {
	for _, c := range []linkCase218{
		{name: "(a) 사업 사이트만", siteOn: []string{"WP-M"}, linkTo: "WP-M", wantLink: true, wantProj: "WP-M", wantDone: true},
		{name: "(b) 범위 규칙만", rule: true, linkTo: "WP-M", wantLink: true, wantProj: "WP-M", wantDone: true},
		{name: "(c) 둘 다 없음", linkTo: "WP-M", wantLink: false, wantProj: "", wantDone: true},
		{name: "(d) 같은 이름 2개·사이트는 WP-M2만 — M 연결 거절", siteOn: []string{"WP-M2"}, two: true, linkTo: "WP-M", wantLink: false, wantProj: "WP-M2", wantDone: true},
		{name: "(d') 같은 이름 2개·사이트는 WP-M2만 — M2 연결 허용", siteOn: []string{"WP-M2"}, two: true, linkTo: "WP-M2", wantLink: true, wantProj: "WP-M2", wantDone: true},
		{name: "(e) 둘 다 있음", siteOn: []string{"WP-M"}, rule: true, linkTo: "WP-M", wantLink: true, wantProj: "WP-M", wantDone: true},
		{name: "(f) 상위 기관만 사이트", siteOnParen: []string{"WP-M"}, linkTo: "WP-M", wantLink: true, wantProj: "WP-M", wantDone: true},
		{name: "(g) 비활성 사이트", siteOn: []string{"WP-M"}, inactive: true, linkTo: "WP-M", wantLink: false, wantProj: "", wantDone: true},
		{name: "(h) 후보 2개 — 선택 없이 완료 불가", siteOn: []string{"WP-M", "WP-M2"}, two: true, linkTo: "WP-M", wantLink: true, wantProj: "", wantDone: false},
	} {
		v := setup205(t)
		ex := func(q string, a ...interface{}) {
			if _, err := v.db.Exec(q, a...); err != nil {
				t.Fatal(err)
			}
		}
		v.e.POST("/stats/company-weekly/tasks/project", v.h.Stats.LinkWeeklyTasksProject, v.h.Auth.AuthMiddleware)
		v.e.POST("/maintenance/visits/:visit_id/action", v.h.Maintenance.UpdateVisitAction, v.h.Auth.AuthMiddleware)
		ex(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('P1','충남교육청','충남교육청',1,'O03'),('c1','고운남측도서관','고운남측도서관',1,'O03'),('c2','한솔도서관','한솔도서관',1,'O03')`)
		ex(`UPDATE customers SET parent_customer_id='P1' WHERE customer_id IN ('c1','c2')`)
		ex(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,is_test) VALUES ('WP-M','정기점검 사업','active','O03','project',0)`)
		if c.two {
			ex(`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,is_test) VALUES ('WP-M2','정기점검 사업','active','O03','project',0)`)
		}
		act := 1
		if c.inactive {
			act = 0
		}
		for _, p := range c.siteOn {
			ex(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,sort_order,is_active,org_id) VALUES (?,?,'c1','고운남측도서관','planned',1,?,'O03')`, "PS1-"+p, p, act)
			ex(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,sort_order,is_active,org_id) VALUES (?,?,'c2','한솔도서관','planned',2,?,'O03')`, "PS2-"+p, p, act)
		}
		for _, p := range c.siteOnParen {
			ex(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,sort_order,is_active,org_id) VALUES (?,?,'P1','충남교육청','planned',1,1,'O03')`, "PP-"+p, p)
		}
		if c.rule {
			ex(`INSERT INTO project_scope_rules(rule_id,project_id,parent_customer_id,product_keys,work_kinds) VALUES ('R1','WP-M','P1',?, 'maintenance')`, model.ProductKeyKLAS)
		}
		ex(`INSERT INTO maintenance_plans(plan_id,plan_year,title,status,org_id) VALUES ('MPX',2026,'p','approved','O03')`)
		for _, vv := range [][]string{{"V1", "2026-10-20", "c1"}, {"V2", "2026-10-21", "c2"}} {
			ex(`INSERT INTO maintenance_visits(visit_id,plan_id,visit_date,customer_id,sort_order,entry_category,assignee,product_type,completed) VALUES (?,'MPX',?,?,0,'normal','양기헌','KLAS',0)`, vv[0], vv[1], vv[2])
		}
		oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
		pidOf := func(id string) string {
			var p string
			_ = v.db.QueryRow(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id=?`, id).Scan(&p)
			return p
		}
		// ① 보고서에서 사업 연결
		rec := v.do(t, "POST", "/stats/company-weekly/tasks/project", url.Values{"week": {"2026-10-19"}, "org": {"O03"}, "item": {"maintenance_visit:V1"}, "project_id": {c.linkTo}}, oa)
		linked := rec.Code == 303 && pidOf("V1") == c.linkTo
		if linked != c.wantLink {
			t.Errorf("%s: 연결 허용=%v want %v (code=%d %s)", c.name, linked, c.wantLink, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
		if !c.wantLink && !strings.Contains(rec.Body.String(), "등록돼 있지 않습니다") {
			t.Errorf("%s: 거절 문구에 이유가 없다: %s", c.name, rec.Body.String())
		}
		// ② 조치 완료(사업 선택 없이)
		form := url.Values{"assignee": {"양기헌"}, "product_type": {"KLAS"}, "visit_date": {"2026-10-21"}, "status": {"done"}, "mark_done": {"1"}, "start_time": {"09:00"}, "end_time": {"10:00"}, "notes": {"점검 완료"}}
		rec2 := v.do(t, "POST", "/maintenance/visits/V2/action", form, oa)
		var done int
		_ = v.db.QueryRow(`SELECT COALESCE(completed,0) FROM maintenance_visits WHERE visit_id='V2'`).Scan(&done)
		if (done == 1) != c.wantDone {
			t.Errorf("%s: 완료=%d want %v (code=%d)", c.name, done, c.wantDone, rec2.Code)
		}
		if got := pidOf("V2"); got != c.wantProj {
			t.Errorf("%s: 완료 후 project_id=%q want %q", c.name, got, c.wantProj)
		}
		// 후보가 여러 개였다면 사업을 고르면 완료된다
		if !c.wantDone {
			form.Set("project_id", "WP-M")
			rec3 := v.do(t, "POST", "/maintenance/visits/V2/action", form, oa)
			_ = v.db.QueryRow(`SELECT COALESCE(completed,0) FROM maintenance_visits WHERE visit_id='V2'`).Scan(&done)
			if done != 1 || pidOf("V2") != "WP-M" {
				t.Errorf("%s: 사업을 고른 뒤 완료되어야 한다 code=%d done=%d project=%q", c.name, rec3.Code, done, pidOf("V2"))
			}
		}
	}
}

// 방문·AS·같은 판정 함수: 같은 입력이면 같은 허용 결과다.
func TestVisitMayLinkProjectSameForVisitAndAS95_218(t *testing.T) {
	v := setup205(t)
	for _, q := range []string{
		`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id) VALUES ('c1','고운남측도서관','고운남측도서관',1,'O03')`,
		`INSERT INTO work_projects(project_id,name,status,org_id,project_kind,is_test) VALUES ('WP-M','정기점검 사업','active','O03','project',0)`,
		`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,sort_order,is_active,org_id) VALUES ('S','WP-M','c1','고운남측도서관','planned',1,1,'O03')`,
	} {
		if _, err := v.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	pr := repository.NewProjectRepo(v.db)
	for _, kind := range []string{model.ScopeWorkMaintenance, model.ScopeWorkAS} {
		ok, why, err := pr.VisitMayLinkProject("c1", "", kind, "WP-M", "")
		if err != nil || !ok {
			t.Errorf("%s: 사이트가 등록돼 있으면 허용 ok=%v why=%s err=%v", kind, ok, why, err)
		}
		ok, why, _ = pr.VisitMayLinkProject("c1", "", kind, "WP-X", "")
		if ok || !strings.Contains(why, "고운남측도서관") {
			t.Errorf("%s: 등록 안 된 사업은 사이트 이름이 든 이유와 함께 거절 ok=%v why=%q", kind, ok, why)
		}
		if ok, _, _ := pr.VisitMayLinkProject("c1", "", kind, "WP-X", "WP-X"); !ok {
			t.Errorf("%s: 이미 연결된 사업은 그대로 통과", kind)
		}
	}
}
