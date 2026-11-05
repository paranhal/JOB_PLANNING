package repository

import (
	"testing"

	"customer-support/internal/model"
)

func TestListSchoolSiteCandidatesOrgIndustryAndRegion(t *testing.T) {
	r, pid := testEquipRepo(t)
	cr := NewCustomerRepo(r.db)
	parent := &model.Customer{OrgName: "교육청", OfficialName: "교육청", IsActive: true, PartyKind: "customer", OrgID: model.OrgIDLibrary, Industry: "공공기관"}
	if err := cr.Create(parent); err != nil {
		t.Fatal(err)
	}
	cheonan := &model.Customer{
		OrgName: "천안초등학교", OfficialName: "천안초등학교", IsActive: true, PartyKind: "customer",
		OrgID: model.OrgIDLibrary, Industry: "학교", AddrSido: "충청남도", AddrSigungu: "천안시",
		HasParent: false,
	}
	if err := cr.Create(cheonan); err != nil {
		t.Fatal(err)
	}
	child := &model.Customer{
		OrgName: "계룡고등학교", OfficialName: "계룡고등학교", IsActive: true, PartyKind: "customer",
		OrgID: model.OrgIDLibrary, Industry: "학교", AddrSido: "충남", AddrSigungu: "계룡시",
		HasParent: true, ParentCustomerID: parent.CustomerID,
	}
	if err := cr.Create(child); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.AddSitesFromCustomers(pid, model.OrgIDLibrary, "u", []string{child.CustomerID}); err != nil {
		t.Fatal(err)
	}
	res, err := r.ListSchoolSiteCandidates(pid, model.OrgIDLibrary, "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Fatalf("후보 %d regions=%+v items=%+v", res.Total, res.Regions, res.Items)
	}
	byID := map[string]SchoolSiteCandidate{}
	for _, it := range res.Items {
		byID[it.ID] = it
	}
	if byID[cheonan.CustomerID].Sido != "충남" || byID[cheonan.CustomerID].Sigungu != "천안시" {
		t.Fatalf("천안 정규화 %+v", byID[cheonan.CustomerID])
	}
	if !byID[child.CustomerID].Registered || byID[cheonan.CustomerID].Registered {
		t.Fatalf("등록됨 %+v", byID)
	}
	q, err := r.ListSchoolSiteCandidates(pid, model.OrgIDLibrary, "천안", nil, nil)
	if err != nil || q.Total != 1 || q.Items[0].ID != cheonan.CustomerID {
		t.Fatalf("검색 %+v err=%v", q, err)
	}
}
