package repository

import (
	"fmt"
	"testing"

	"customer-support/internal/model"
)

func Test95_167_SiteCandidatesIgnoreCustomerOrg(t *testing.T) {
	r, pid := testEquipRepo(t)
	if _, err := r.db.Exec(`INSERT INTO work_projects(project_id,name,org_id,contract_amount) VALUES('WP-O03','SI사업','O03',1000)`); err != nil {
		t.Fatal(err)
	}
	cr := NewCustomerRepo(r.db)
	parentA := &model.Customer{OrgName: "교육청A", OfficialName: "교육청A", IsActive: true, PartyKind: "customer", OrgID: "O01", Industry: "공공기관"}
	parentB := &model.Customer{OrgName: "교육청B", OfficialName: "교육청B", IsActive: true, PartyKind: "customer", OrgID: "O01", Industry: "공공기관"}
	if err := cr.Create(parentA); err != nil {
		t.Fatal(err)
	}
	if err := cr.Create(parentB); err != nil {
		t.Fatal(err)
	}
	o01 := make([]*model.Customer, 0, 825)
	for i := 0; i < 825; i++ {
		sido, sigungu := "충청남도", "천안시"
		if i == 1 {
			sido, sigungu = "경기도", "수원시"
		}
		c := &model.Customer{
			OrgName: fmt.Sprintf("오원학교%04d", i), OfficialName: fmt.Sprintf("오원학교%04d", i),
			IsActive: true, PartyKind: "customer", OrgID: "O01", Industry: "school",
			AddrSido: sido, AddrSigungu: sigungu, HasParent: true, ParentCustomerID: parentA.CustomerID,
		}
		if err := cr.Create(c); err != nil {
			t.Fatal(err)
		}
		o01 = append(o01, c)
	}
	o03 := &model.Customer{
		OrgName: "장곡초등학교", OfficialName: "장곡초등학교", IsActive: true, PartyKind: "customer",
		OrgID: "O03", Industry: "학교", AddrSido: "충청남도", AddrSigungu: "청양군",
	}
	off := &model.Customer{OrgName: "쉬는학교", OfficialName: "쉬는학교", IsActive: false, PartyKind: "customer", OrgID: "O01", Industry: "학교"}
	partner := &model.Customer{OrgName: "협력학교", OfficialName: "협력학교", IsActive: true, PartyKind: "partner", OrgID: "O01", Industry: "학교"}
	for _, c := range []*model.Customer{o03, off, partner} {
		if err := cr.Create(c); err != nil {
			t.Fatal(err)
		}
	}
	res, err := r.ListSiteCandidates("WP-O03", "O03", "school", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 826 {
		t.Fatalf("후보 %d", res.Total)
	}
	seenO01, seenO03 := false, false
	for _, it := range res.Items {
		if it.ID == o01[0].CustomerID {
			seenO01 = true
		}
		if it.ID == o03.CustomerID {
			seenO03 = true
		}
		if it.ID == off.CustomerID || it.ID == partner.CustomerID {
			t.Fatalf("제외 대상이 후보에 있음 %s", it.Name)
		}
	}
	if !seenO01 || !seenO03 {
		t.Fatal("O01 또는 O03 학교가 후보에 없다")
	}
	sido, err := r.ListSiteCandidates("WP-O03", "O03", "school", "", []string{"경기"}, nil)
	if err != nil || sido.Total != 1 || sido.Items[0].ID != o01[1].CustomerID {
		t.Fatalf("시도 필터 %+v err=%v", sido, err)
	}
	q, err := r.ListSiteCandidates("WP-O03", "O03", "school", "장곡", nil, nil)
	if err != nil || q.Total != 1 || q.Items[0].ID != o03.CustomerID {
		t.Fatalf("검색 %+v err=%v", q, err)
	}

	added, err := r.AddSitesFromCustomers("WP-O03", "O03", "u", []string{o01[0].CustomerID, o03.CustomerID})
	if err != nil || added.Added != 2 {
		t.Fatalf("등록 %+v err=%v", added, err)
	}
	var siteOrg, custOrg string
	if err := r.db.QueryRow(`SELECT org_id FROM project_sites WHERE project_id='WP-O03' AND customer_id=?`, o01[0].CustomerID).Scan(&siteOrg); err != nil || siteOrg != "O03" {
		t.Fatalf("사이트 조직 %s %v", siteOrg, err)
	}
	if err := r.db.QueryRow(`SELECT org_id FROM customers WHERE customer_id=?`, o01[0].CustomerID).Scan(&custOrg); err != nil || custOrg != "O01" {
		t.Fatalf("고객 조직 %s %v", custOrg, err)
	}
	again, err := r.ListSiteCandidates("WP-O03", "O03", "school", o01[0].OrgName, nil, nil)
	if err != nil || again.Total != 1 || !again.Items[0].Registered {
		t.Fatalf("등록 표시 %+v err=%v", again, err)
	}
	dup, err := r.AddSitesFromCustomers("WP-O03", "O03", "u", []string{o01[0].CustomerID})
	if err != nil || dup.Added != 0 || dup.Skipped != 1 || dup.SkippedItems[0].Reason != "이미 이 사업에 등록됨" {
		t.Fatalf("재등록 %+v err=%v", dup, err)
	}

	also, err := r.AddSitesFromCustomers(pid, model.OrgIDLibrary, "u", []string{o01[0].CustomerID})
	if err != nil || also.Added != 1 {
		t.Fatalf("다른 사업 등록 %+v err=%v", also, err)
	}
	var otherOrg, otherStatus string
	if err := r.db.QueryRow(`SELECT org_id, status FROM project_sites WHERE project_id=? AND customer_id=?`, pid, o01[0].CustomerID).Scan(&otherOrg, &otherStatus); err != nil || otherOrg != model.OrgIDLibrary || otherStatus != model.SitePlanned {
		t.Fatalf("O01 사업 행 %s %s %v", otherOrg, otherStatus, err)
	}
	var o03Site string
	if err := r.db.QueryRow(`SELECT id FROM project_sites WHERE project_id='WP-O03' AND customer_id=?`, o01[0].CustomerID).Scan(&o03Site); err != nil {
		t.Fatal(err)
	}
	if err := r.BulkSiteStatus("WP-O03", "u", model.SiteExcluded, "2026-11-11", "범위", []string{o03Site}); err != nil {
		t.Fatal(err)
	}
	if err := r.db.QueryRow(`SELECT status FROM project_sites WHERE project_id=? AND customer_id=?`, pid, o01[0].CustomerID).Scan(&otherStatus); err != nil || otherStatus != model.SitePlanned {
		t.Fatalf("다른 사업 상태 %s %v", otherStatus, err)
	}
	pO03, err := r.SiteProgress("WP-O03")
	if err != nil {
		t.Fatal(err)
	}
	pO01, err := r.SiteProgress(pid)
	if err != nil {
		t.Fatal(err)
	}
	if pO03.Excluded != 1 || pO03.Denom != 1 || pO01.Excluded != 0 || pO01.Denom != 1 {
		t.Fatalf("집계 O03=%+v O01=%+v", pO03, pO01)
	}
	if err := r.DeactivateSite(o03Site, "WP-O03"); err != nil {
		t.Fatal(err)
	}
	var still int
	if err := r.db.QueryRow(`SELECT is_active FROM project_sites WHERE project_id=? AND customer_id=?`, pid, o01[0].CustomerID).Scan(&still); err != nil || still != 1 {
		t.Fatalf("다른 사업 삭제 영향 %d %v", still, err)
	}

	sameA := &model.Customer{
		OrgName: "동명초등학교", OfficialName: "동명초등학교", IsActive: true, PartyKind: "customer",
		OrgID: "O01", Industry: "학교", AddrSido: "충청남도", AddrSigungu: "공주시",
		HasParent: true, ParentCustomerID: parentA.CustomerID,
	}
	sameB := &model.Customer{
		OrgName: "동명초등학교", OfficialName: "동명초등학교", IsActive: true, PartyKind: "customer",
		OrgID: "O03", Industry: "학교", AddrSido: "충청남도", AddrSigungu: "공주시",
		HasParent: true, ParentCustomerID: parentB.CustomerID,
	}
	var more []*model.Customer
	for i := 0; i < 3; i++ {
		more = append(more, &model.Customer{
			OrgName: "동명초등학교", OfficialName: "동명초등학교", IsActive: true, PartyKind: "customer",
			OrgID: "O01", Industry: "학교", AddrSido: "충청남도", AddrSigungu: "공주시",
			HasParent: true, ParentCustomerID: parentA.CustomerID,
		})
	}
	for _, c := range append([]*model.Customer{sameA, sameB}, more...) {
		if err := cr.Create(c); err != nil {
			t.Fatal(err)
		}
	}
	ids := []string{sameA.CustomerID, sameB.CustomerID}
	for _, c := range more {
		ids = append(ids, c.CustomerID)
	}
	named, err := r.AddSitesFromCustomers("WP-O03", "O03", "u", ids)
	if err != nil {
		t.Fatal(err)
	}
	if named.Added != 4 || named.Skipped != 1 || named.SkippedItems[0].Reason != "같은 이름 사이트가 있음" || named.SkippedItems[0].CustomerID != more[2].CustomerID {
		t.Fatalf("동명 %+v", named)
	}
	var nA, nB int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id='WP-O03' AND customer_id=?`, sameA.CustomerID).Scan(&nA); err != nil || nA != 1 {
		t.Fatalf("교육청A 쪽 %d %v", nA, err)
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM project_sites WHERE project_id='WP-O03' AND customer_id=?`, sameB.CustomerID).Scan(&nB); err != nil || nB != 1 {
		t.Fatalf("교육청B 쪽 %d %v", nB, err)
	}
}
