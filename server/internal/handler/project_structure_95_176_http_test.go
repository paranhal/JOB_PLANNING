package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_176_StructureHTTP(t *testing.T) {
	e, repo, _, _, db := newProjectServerWithSQL(t)
	if _, err := db.Exec(`INSERT INTO orgs(org_id, org_no, org_name, is_active) VALUES ('O02','2','미디어사업팀',1),('O03','3','SI사업팀',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO work_projects (project_id, name, short_name, status, org_id, project_kind, contract_no) VALUES
		('WP-MEDIA','미디어유지','미디어','active','O02','maintenance','C-M'),
		('WP-SI','에스아이구축','에스아이','active','O03','build','C-SI'),
		('WP-SRC','원본사업','원본','active','O02','build','C-SRC')`); err != nil {
		t.Fatal(err)
	}
	row := &model.ProjectPhase{ProjectID: "WP-SRC", OrgID: "O02", Name: "착수", Status: model.PhasePlanned}
	if err := repo.CreatePhase(row); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreatePackage(row.PhaseID, "현장", model.TrackModeSite, 1); err != nil {
		t.Fatal(err)
	}
	media := jwtCookieOrg(t, model.RoleOrgAdmin, "O02")
	si := jwtCookieOrg(t, model.RoleOrgAdmin, "O03")
	tech := jwtCookieRole(t, model.RoleTech)
	ro := jwtCookieReadOnly(t, model.RoleOrgAdmin)
	clevel := jwtCookieRole(t, model.RoleCLevel)
	obs := jwtCookieRole(t, model.RoleObserver)

	mediaPage := doGetWith(t, e, "/projects/WP-MEDIA", media)
	if mediaPage.Code != http.StatusOK || !strings.Contains(mediaPage.Body.String(), "사용 안 함") || strings.Contains(mediaPage.Body.String(), "data-phase-tab open") || strings.Contains(mediaPage.Body.String(), "기본 단계로 채우기") {
		t.Fatalf("미디어 빈 구조 %d", mediaPage.Code)
	}
	if !strings.Contains(mediaPage.Body.String(), "다른 사업에서 구조 복사") {
		t.Fatal("미디어에 복사 입력이 없다")
	}
	siPage := doGetWith(t, e, "/projects/WP-SI", si)
	if siPage.Code != http.StatusOK || !strings.Contains(siPage.Body.String(), "data-phase-tab open") || !strings.Contains(siPage.Body.String(), "기본 단계로 채우기") {
		t.Fatal("SI 빈 구조")
	}
	techPage := doGetWith(t, e, "/projects/WP-SI", tech)
	if techPage.Code != http.StatusOK || strings.Contains(techPage.Body.String(), "data-structure-copy") {
		t.Fatal("직원에게 복사 버튼")
	}

	for _, ck := range []*http.Cookie{tech, ro, clevel, obs} {
		rec := doFormWith(t, e, "/projects/WP-SI/structure/copy", url.Values{"from_project_id": {"WP-SRC"}, "confirm": {"1"}}, ck)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("권한 %d", rec.Code)
		}
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_phases WHERE project_id='WP-SI'`).Scan(&n)
	if n != 0 {
		t.Fatal("거절 뒤 저장됨")
	}

	self := doFormWith(t, e, "/projects/WP-SI/structure/copy", url.Values{"from_project_id": {"WP-SI"}, "confirm": {"1"}}, si)
	loc, _ := url.QueryUnescape(self.Header().Get("Location"))
	if self.Code != http.StatusSeeOther || !strings.Contains(loc, "복사할") {
		t.Fatalf("자기 자신 %d %s", self.Code, self.Header().Get("Location"))
	}
	preview := doFormWith(t, e, "/projects/WP-SI/structure/copy", url.Values{"from_project_id": {"WP-SRC"}}, si)
	if preview.Code != http.StatusSeeOther || !strings.Contains(preview.Header().Get("Location"), "copy_from=WP-SRC") {
		t.Fatalf("미리보기 이동 %s", preview.Header().Get("Location"))
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM project_phases WHERE project_id='WP-SI'`).Scan(&n)
	if n != 0 {
		t.Fatal("확인 없이 저장")
	}
	show := doGetWith(t, e, "/projects/WP-SI?copy_from=WP-SRC", si)
	if show.Code != http.StatusOK || !strings.Contains(show.Body.String(), "data-copy-preview") || !strings.Contains(show.Body.String(), "착수") {
		t.Fatal("미리보기 화면")
	}
	missing := doGetWith(t, e, "/projects/WP-SI/structure/copy/preview?from=WP-없는", si)
	gone := doGetWith(t, e, "/projects/WP-SI/structure/copy/preview?from=WP-없는2", si)
	if missing.Code != http.StatusNotFound || gone.Code != http.StatusNotFound || missing.Body.String() != gone.Body.String() || !strings.Contains(missing.Body.String(), "사업을 찾을 수 없습니다") {
		t.Fatalf("404 %d %d %s", missing.Code, gone.Code, missing.Body.String())
	}
	cross := doFormWith(t, e, "/projects/WP-SI/structure/copy", url.Values{"from_project_id": {"WP-SRC"}, "confirm": {"1"}}, si)
	if cross.Code != http.StatusSeeOther || !strings.Contains(cross.Header().Get("Location"), "ok=copy") {
		t.Fatalf("타 조직 복사 %d %s", cross.Code, cross.Header().Get("Location"))
	}
	if _, err := db.Exec(`INSERT INTO project_assignments(project_id, user_id, duty) VALUES ('WP-SI','tech-org-id','pm')`); err != nil {
		t.Fatal(err)
	}
	pmCk := jwtCookieOrg(t, model.RoleTech, "O03")
	pm := doFormWith(t, e, "/projects/WP-SI/structure/copy", url.Values{"from_project_id": {"WP-SRC"}, "confirm": {"1"}}, pmCk)
	if pm.Code != http.StatusSeeOther || !strings.Contains(pm.Header().Get("Location"), "ok=copy") {
		t.Fatalf("PM 복사 %d %s", pm.Code, pm.Header().Get("Location"))
	}
	foreign := doFormWith(t, e, "/projects/WP-SI/structure/templates", url.Values{"name": {"미디어가 저장"}}, media)
	if foreign.Code != http.StatusForbidden {
		t.Fatalf("다른 조직 관리자 저장 %d", foreign.Code)
	}
	done := doGetWith(t, e, strings.Split(cross.Header().Get("Location"), "#")[0], si)
	if !strings.Contains(done.Body.String(), "단계 1개, 하부 업무 1개를 복사했습니다") || !strings.Contains(done.Body.String(), "작업 계획과 사이트는 복사되지 않습니다") {
		t.Fatal("복사 안내")
	}

	save := doFormWith(t, e, "/projects/WP-SI/structure/templates", url.Values{"name": {"공통"}, "project_kind": {"build"}}, si)
	if save.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", save.Code, save.Body.String())
	}
	staffSave := doFormWith(t, e, "/projects/WP-SI/structure/templates", url.Values{"name": {"직원"}}, tech)
	if staffSave.Code != http.StatusForbidden {
		t.Fatalf("직원 저장 %d", staffSave.Code)
	}
	var tpl string
	if err := db.QueryRow(`SELECT template_id FROM structure_templates WHERE name='공통'`).Scan(&tpl); err != nil {
		t.Fatal(err)
	}
	other := doGetWith(t, e, "/projects/WP-MEDIA", media)
	if !strings.Contains(other.Body.String(), "공통") {
		t.Fatal("다른 조직 화면에 전사 템플릿이 없다")
	}
	apply := doFormWith(t, e, "/projects/WP-MEDIA/structure/templates/apply", url.Values{"template_id": {tpl}, "confirm": {"1"}}, media)
	if apply.Code != http.StatusSeeOther || !strings.Contains(apply.Header().Get("Location"), "ok=tpl") {
		t.Fatalf("적용 %d %s", apply.Code, apply.Header().Get("Location"))
	}
	noTpl := doFormWith(t, e, "/projects/WP-MEDIA/structure/templates/apply", url.Values{"template_id": {"ST-없는"}, "confirm": {"1"}}, media)
	if noTpl.Code != http.StatusNotFound || !strings.Contains(noTpl.Body.String(), "템플릿을 찾을 수 없습니다") {
		t.Fatalf("없는 템플릿 %d %s", noTpl.Code, noTpl.Body.String())
	}
	off := doFormWith(t, e, "/projects/WP-SI/structure/templates/off", url.Values{"template_id": {tpl}}, si)
	if off.Code != http.StatusSeeOther {
		t.Fatal(off.Code)
	}
	hidden := doGetWith(t, e, "/projects/WP-SI", si)
	if strings.Contains(hidden.Body.String(), `?tpl=`+tpl) {
		t.Fatal("비활성 템플릿이 적용 목록에 있다")
	}
	noSite := doGetWith(t, e, "/projects/WP-SI", si)
	if !strings.Contains(noSite.Body.String(), "data-no-sites") || !strings.Contains(noSite.Body.String(), "이 사업에는 사이트가 없습니다") {
		t.Fatal("사이트 없음 안내")
	}
}
