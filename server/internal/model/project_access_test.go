package model

import "testing"

func TestProjectAccessRoles(t *testing.T) {
	p := WorkProject{OrgID: "O03", RelatedOrgIDs: []string{"O02"}}
	if ProjectAccess(p, "O03") != ProjectAccessOwner {
		t.Fatal("주관")
	}
	if ProjectAccess(p, "O02") != ProjectAccessRelated {
		t.Fatal("유관")
	}
	if ProjectAccess(p, "O01") != ProjectAccessViewer {
		t.Fatal("전 직원 조회")
	}
	if ProjectAccess(p, "*") != ProjectAccessOwner {
		t.Fatal("비젼")
	}
	if CanRegisterWorkOnProject(p, "O01") {
		t.Fatal("viewer 업무 등록")
	}
	if !CanRegisterWorkOnProject(p, "O02") || !CanRegisterWorkOnProject(p, "O03") {
		t.Fatal("주관·유관 업무 등록")
	}
}

func TestCanPickOtherAndLink(t *testing.T) {
	p := WorkProject{OrgID: "O03", Status: WBProjectActive}
	if CanPickOtherProjects(RoleTech) || CanPickOtherProjects(RoleCLevel) {
		t.Fatal("기술·C-Level 타 부서")
	}
	if !CanPickOtherProjects(RoleOrgAdmin) || !CanPickOtherProjects(RoleSupport) || !CanPickOtherProjects(RoleVisionAdmin) {
		t.Fatal("허용 역할")
	}
	if !ProjectInOwnPickScope(p, "O03", nil) {
		t.Fatal("주관 내 부서")
	}
	if !ProjectInOwnPickScope(p, "O01", []string{"O03"}) {
		t.Fatal("하위 조직은 내 부서")
	}
	if CanLinkWorkProject(p, "O01", RoleTech, nil, false) {
		t.Fatal("기술 타 부서 저장")
	}
	if !CanLinkWorkProject(p, "O01", RoleOrgAdmin, nil, false) {
		t.Fatal("조직관리자 타 부서 저장")
	}
	closed := WorkProject{OrgID: "O03", Status: WBProjectArchived}
	if CanLinkWorkProject(closed, "O03", RoleTech, nil, false) {
		t.Fatal("보관 거절")
	}
	if !CanLinkWorkProject(closed, "O03", RoleTech, nil, true) {
		t.Fatal("IncludeClosed")
	}
}
