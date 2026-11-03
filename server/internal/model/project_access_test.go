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
