package handler

import (
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func extractPickerSiteJSON(body string) string {
	raw := html.UnescapeString(body)
	raw = strings.ReplaceAll(raw, `\u0022`, `"`)
	key := `"Site":`
	i := strings.Index(raw, key)
	if i < 0 {
		return ""
	}
	rest := strings.TrimSpace(raw[i+len(key):])
	if strings.HasPrefix(rest, "null") {
		return "null"
	}
	if !strings.HasPrefix(rest, "[") {
		return ""
	}
	depth := 0
	for j, c := range rest {
		switch c {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return rest[:j+1]
			}
		}
	}
	return ""
}

func Test95_144D_T1_SiteRowsMatchCount(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid, plan_year)
		VALUES ('WP-A','사업A','active',?,0,1,2026),
		       ('WP-B','사업B','active',?,0,1,2025),
		       ('WP-C','사업C','active',?,0,1,2026)`, org, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-A','WP-A','c47','서산고등학교','in_progress',?,1),
		       ('PS-B','WP-B','c47','서산고등학교','in_progress',?,1)`, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c47x','무등록사이트','무등록사이트',1,?)`, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, project_id)
		VALUES ('v144d2','MP47','2026-09-18','c47',0,'normal','최혜영','KLAS',0,''),
		       ('v144d1','MP47','2026-09-19','c47b',0,'normal','최혜영','KLAS',0,''),
		       ('v144d0','MP47','2026-09-20','c47x',0,'normal','최혜영','KLAS',0,'')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-A1','WP-A','c47b','타학교','in_progress',?,1)`, org); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, err := users.GetByUsername("oa47")
	if err != nil || oa == nil {
		t.Fatal(err)
	}
	ck := jwtCookieVerified(t, oa)

	t.Run("2건", func(t *testing.T) {
		body := doMnt47Get(t, e, "/maintenance/visits/v144d2/action", ck).Body.String()
		if !strings.Contains(body, "data-site-project-list") || !strings.Contains(body, "data-site-project-row") {
			t.Fatal("행 목록 칸 없음")
		}
		if !strings.Contains(body, "SiteCount") || !(strings.Contains(body, `"SiteCount":2`) || strings.Contains(body, "SiteCount&#34;:2")) {
			t.Fatal("N=2 아님")
		}
		site := extractPickerSiteJSON(body)
		if !strings.Contains(site, "WP-A") || !strings.Contains(site, "WP-B") {
			t.Fatalf("행 2개 아님 %s", site)
		}
		if strings.Contains(site, "WP-C") {
			t.Fatal("미등록 C가 행에 있음")
		}
		if strings.Count(site, `"project_id"`) != 2 {
			t.Fatalf("행 수 %s", site)
		}
	})
	t.Run("1건", func(t *testing.T) {
		body := doMnt47Get(t, e, "/maintenance/visits/v144d1/action", ck).Body.String()
		site := extractPickerSiteJSON(body)
		if !strings.Contains(site, "WP-A") || strings.Contains(site, "WP-B") {
			t.Fatalf("1건 행 %s", site)
		}
		if strings.Count(site, `"project_id"`) != 1 {
			t.Fatalf("행 수 %s", site)
		}
		if !(strings.Contains(body, `"SiteCount":1`) || strings.Contains(body, "SiteCount&#34;:1")) {
			t.Fatal("N=1 아님")
		}
	})
	t.Run("0건", func(t *testing.T) {
		body := doMnt47Get(t, e, "/maintenance/visits/v144d0/action", ck).Body.String()
		if !strings.Contains(body, "이 사이트가 등록된 사업이 없습니다. 사업 → 사이트 등록에서 이 사이트를 먼저 등록하세요") {
			t.Fatal("0건 안내 없음")
		}
		site := extractPickerSiteJSON(body)
		if site != "[]" && site != "null" {
			t.Fatalf("0건인데 행 있음 %s", site)
		}
		if !(strings.Contains(body, `"SiteCount":0`) || strings.Contains(body, "SiteCount&#34;:0")) {
			t.Fatal("N=0 아님")
		}
	})
}

func Test95_144D_T2_SelectRowSaves(t *testing.T) {
	e, db, _ := newMnt47Server(t)
	org := model.OrgIDLibrary
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, status, org_id, is_test, is_paid, plan_year)
		VALUES ('WP-A','사업A','active',?,0,1,2026),
		       ('WP-B','사업B','active',?,0,1,2025)`, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_sites(id,project_id,customer_id,site_name,status,org_id,is_active)
		VALUES ('PS-A','WP-A','c47','서산고등학교','in_progress',?,1),
		       ('PS-B','WP-B','c47','서산고등학교','in_progress',?,1)`, org, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits
		(visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed, project_id)
		VALUES ('v144ds','MP47','2026-09-18','c47',0,'normal','최혜영','KLAS',0,'WP-A')`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa, err := users.GetByUsername("oa47")
	if err != nil || oa == nil {
		t.Fatal(err)
	}
	ckGet := jwtCookieVerified(t, oa)
	body := doMnt47Get(t, e, "/maintenance/visits/v144ds/action", ckGet).Body.String()
	raw := html.UnescapeString(body)
	if !strings.Contains(raw, `"Selected":"WP-A"`) && !strings.Contains(body, "Selected&#34;:&#34;WP-A") {
		t.Fatal("연결된 사업이 선택되지 않음")
	}
	form := url.Values{
		"assignee":   {"최혜영"},
		"visit_date": {"2026-09-18"},
		"status":     {"open"},
		"notes":      {"행선택"},
		"start_time": {"10:00"},
		"end_time":   {"11:00"},
		"project_id": {"WP-B"},
	}
	ok := doMnt47Form(t, e, "/maintenance/visits/v144ds/action", form, jwtCookie(t))
	if ok.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", ok.Code, ok.Body.String())
	}
	got := doMnt47Get(t, e, "/maintenance/visits/v144ds/action", ckGet).Body.String()
	if !strings.Contains(html.UnescapeString(got), `"Selected":"WP-B"`) && !strings.Contains(got, "Selected&#34;:&#34;WP-B") {
		t.Fatal("새로고침 후 선택이 유지되지 않음")
	}
	var pid string
	if err := db.QueryRow(`SELECT COALESCE(project_id,'') FROM maintenance_visits WHERE visit_id='v144ds'`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if pid != "WP-B" {
		t.Fatalf("db=%s", pid)
	}
}

func Test95_144D_T3_144BCRegression(t *testing.T) {
	Test95_144B_T1_SiteProjectsNotOrg(t)
	Test95_144B_T2_EmptyHintNoOther(t)
	Test95_144B_T4_OtherScreensKeepOtherPicker(t)
}
