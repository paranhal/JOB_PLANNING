package handler

import (
	"net/url"
	"regexp"
	"strings"
	"testing"

	"customer-support/internal/model"
)

// 95-194: 사업 연결 창의 사업 목록에서 같은 이름(같은 조직) 사업은 대표 1개만 보인다.

func seed194Projects(t *testing.T, v env190) {
	t.Helper()
	db := v.h.Stats.repo.DB()
	for _, q := range []string{
		`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,short_name,is_active,sort_order) VALUES ('O02','O02','미디어사업팀','미디어',1,2)`,
		// 시드 사업(실적 있음, 영업 연결 없음)과 영업 승격 사업(실적 없음, 계약금액 있음): 같은 이름
		`INSERT INTO work_projects(project_id,name,status,org_id,created_at) VALUES ('WPT-SEED','세종시 도서관 ICT 유지관리(2026년)','active','O03','2026-08-07 10:00:00')`,
		`INSERT INTO work_projects(project_id,name,status,org_id,created_at,sales_project_id,contract_amount) VALUES ('WPT-SALES','세종시 도서관 ICT 유지관리 2026년','active','O03','2026-09-30 10:00:00','SP-035',89590000)`,
		// 실적 둘 다 없고 영업 연결로 갈리는 그룹
		`INSERT INTO work_projects(project_id,name,status,org_id,created_at) VALUES ('WPT-A1','누리집 도입','active','O03','2026-09-01 10:00:00')`,
		`INSERT INTO work_projects(project_id,name,status,org_id,created_at,sales_project_id) VALUES ('WPT-A2','누리집  도입','active','O03','2026-10-01 10:00:00','SP-023')`,
		// 같은 이름이지만 다른 조직은 합치지 않는다
		`INSERT INTO work_projects(project_id,name,status,org_id,created_at) VALUES ('WPT-O2','세종시 도서관 ICT 유지관리(2026년)','active','O02','2026-08-07 10:00:00')`,
		`INSERT INTO work_projects(project_id,name,status,org_id,created_at) VALUES ('WPT-UNIQ','이름이 하나뿐인 사업','active','O03','2026-08-07 10:00:00')`,
		`INSERT INTO work_tasks(task_id,work_type,title,status,project_id,org_id,complete_date,work_date) VALUES
			('T194-1','project_work','시드 업무1','complete','WPT-SEED','O03','2026-10-21','2026-10-21'),
			('T194-2','project_work','시드 업무2','complete','WPT-SEED','O03','2026-10-22','2026-10-22')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
}

func modalProjectIDs(t *testing.T, body string) []string {
	t.Helper()
	i := strings.Index(body, "data-project-list")
	if i < 0 {
		t.Fatal("사업 연결 창의 사업 목록이 없다")
	}
	blk := body[i:]
	if j := strings.Index(blk, "</ul>"); j > 0 {
		blk = blk[:j]
	}
	var ids []string
	for _, m := range regexp.MustCompile(`name="project_id" value="([^"]+)"`).FindAllStringSubmatch(blk, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

func TestHTTP95_194_ModalShowsOneProjectPerName(t *testing.T) {
	v := setup190(t, false)
	seed193Weekly(t, v) // 사업 미연결 업무가 있어야 연결 창이 그려진다
	seed194Projects(t, v)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	ids := modalProjectIDs(t, v.do(t, "GET", w190+"&edit=0", nil, oa).Body.String())
	has := map[string]bool{}
	for _, id := range ids {
		has[id] = true
	}
	// 대표: 실적이 있는 시드 사업. 영업 승격 사업은 목록에서 빠진다
	if !has["WPT-SEED"] || has["WPT-SALES"] {
		t.Fatalf("세종시 ICT: 시드만 보여야 한다 %v", ids)
	}
	// 실적이 같으면(둘 다 0) 영업 연결이 있는 쪽이 대표
	if has["WPT-A1"] || !has["WPT-A2"] {
		t.Fatalf("누리집: 영업 연결 사업만 보여야 한다 %v", ids)
	}
	// 다른 조직의 같은 이름은 그대로, 이름이 하나뿐인 사업도 그대로
	if !has["WPT-O2"] || !has["WPT-UNIQ"] {
		t.Fatalf("타 조직·단독 사업이 사라졌다 %v", ids)
	}
	// 같은 id 가 두 번 나오지 않는다
	if len(ids) != len(has) {
		t.Fatalf("중복 id %v", ids)
	}
}

// 연결 저장은 전체 사업 목록을 그대로 쓴다: 목록에서 숨은 사업으로도 직접 연결할 수 있다.
func TestHTTP95_194_SaveStillAcceptsHiddenProject(t *testing.T) {
	v := setup190(t, false)
	seed193Weekly(t, v)
	seed194Projects(t, v)
	v.e.POST("/stats/company-weekly/tasks/project", v.h.Stats.LinkWeeklyTasksProject, v.h.Auth.AuthMiddleware)
	oa := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	r := v.do(t, "POST", "/stats/company-weekly/tasks/project", url.Values{
		"item": {"work_task:T193-N"}, "project_id": {"WPT-SALES"}, "org": {"O03"}, "week": {"2026-10-19"},
	}, oa)
	if r.Code != 303 {
		t.Fatalf("숨은 사업(WP-011)으로 연결 %d %s", r.Code, r.Body.String())
	}
	var pid string
	_ = v.h.Stats.repo.DB().QueryRow(`SELECT COALESCE(project_id,'') FROM work_tasks WHERE task_id='T193-N'`).Scan(&pid)
	if pid != "WPT-SALES" {
		t.Fatalf("연결 결과 project_id=%q", pid)
	}
}
