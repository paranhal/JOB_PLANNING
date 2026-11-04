package handler

import (
	"testing"

	"customer-support/internal/model"
)

func Test95_108_SplitProjectsByOrg(t *testing.T) {
	my := model.OrgIDLibrary
	host := model.WorkProject{ProjectID: "A", Name: "주관내", OrgID: my}
	rel := model.WorkProject{ProjectID: "B", Name: "유관", OrgID: "O02", RelatedOrgIDs: []string{my}}
	oth := model.WorkProject{ProjectID: "C", Name: "타부서", OrgID: "O02"}
	empty := model.WorkProject{ProjectID: "D", Name: "미지정", OrgID: ""}
	own, other := splitProjectsByOrg([]model.WorkProject{host, rel, oth, empty}, my)
	if len(own) != 2 || len(other) != 2 {
		t.Fatalf("own=%d other=%d", len(own), len(other))
	}
	var gotOwn, gotRel, gotOth, gotEmpty bool
	for _, p := range own {
		if p.ProjectID == "A" && !p.Related {
			gotOwn = true
		}
		if p.ProjectID == "B" && p.Related {
			gotRel = true
		}
	}
	for _, p := range other {
		if p.ProjectID == "C" {
			gotOth = true
		}
		if p.ProjectID == "D" {
			gotEmpty = true
		}
	}
	if !gotOwn || !gotRel || !gotOth || !gotEmpty {
		t.Fatalf("분류 own=%v rel=%v oth=%v empty=%v", gotOwn, gotRel, gotOth, gotEmpty)
	}
	gs := groupProjectsByOrg(other)
	if len(gs) != 2 {
		t.Fatalf("그룹 %d", len(gs))
	}
	var sawEmpty bool
	for _, g := range gs {
		if g.OrgID == "" && g.OrgName == projectFilterEmptyOrgName {
			sawEmpty = true
		}
	}
	if !sawEmpty {
		t.Fatal("조직 미지정 그룹 없음")
	}
}
