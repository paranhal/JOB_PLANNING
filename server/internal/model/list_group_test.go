package model

import "testing"

func TestGroupCustomersByParent(t *testing.T) {
	items := []CustomerListItem{
		{CustomerID: "c1", OrgName: "분관", ParentCustomerID: "p1", ParentOrgName: "본관"},
		{CustomerID: "c2", OrgName: "단독"},
		{CustomerID: "c3", OrgName: "분관2", ParentCustomerID: "p1", ParentOrgName: "본관"},
	}
	g := GroupCustomersByParent(items)
	if len(g) != 2 {
		t.Fatalf("그룹 %d", len(g))
	}
	if g[0].Label != "본관" || len(g[0].Items) != 2 {
		t.Fatalf("%+v", g[0])
	}
	if g[1].Label != "상위기관 없음" || len(g[1].Items) != 1 {
		t.Fatalf("%+v", g[1])
	}
}

func TestGroupUsersByRole(t *testing.T) {
	g := GroupUsersByRole([]User{
		{UserID: "1", FullName: "가", Role: RoleTech},
		{UserID: "2", FullName: "나", Role: RoleAdmin},
		{UserID: "3", FullName: "다", Role: RoleTech},
	})
	if len(g) != 2 {
		t.Fatalf("빈 소속은 숨긴다 %d", len(g))
	}
	if g[0].Key != RoleTech || len(g[0].Items) != 2 || g[0].Label != "기술 소속" {
		t.Fatalf("%+v", g[0])
	}
	if g[1].Key != RoleAdmin {
		t.Fatalf("%+v", g[1])
	}
}
