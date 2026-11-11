package repository

import (
	"strings"
	"testing"

	"customer-support/internal/model"
)

func Test95_167A_SiteSchoolMatchCompanyWide(t *testing.T) {
	r, pid := testEquipRepo(t)
	if _, err := r.db.Exec(`INSERT INTO work_projects(project_id,name,org_id,contract_amount) VALUES('WP-O03','SI사업','O03',1000)`); err != nil {
		t.Fatal(err)
	}
	cr := NewCustomerRepo(r.db)
	parent := &model.Customer{OrgName: model.SchoolParentOfficialName, OfficialName: model.SchoolParentOfficialName, IsActive: true, PartyKind: "customer", OrgID: model.OrgIDLibrary, Industry: "공공기관"}
	if err := cr.Create(parent); err != nil {
		t.Fatal(err)
	}
	school := &model.Customer{
		OrgName: "장곡초등학교", OfficialName: "장곡초등학교", HasParent: true, ParentCustomerID: parent.CustomerID,
		OrgCode: "JG01", IsActive: true, PartyKind: "customer", OrgID: model.OrgIDLibrary, Industry: "학교",
		AddrSido: "충청남도", AddrSigungu: "청양군",
	}
	off := &model.Customer{
		OrgName: "쉬는초등학교", OfficialName: "쉬는초등학교", HasParent: true, ParentCustomerID: parent.CustomerID,
		IsActive: false, PartyKind: "customer", OrgID: model.OrgIDLibrary, Industry: "학교",
	}
	partner := &model.Customer{
		OrgName: "협력초등학교", OfficialName: "협력초등학교", HasParent: true, ParentCustomerID: parent.CustomerID,
		IsActive: true, PartyKind: "partner", OrgID: model.OrgIDLibrary, Industry: "학교",
	}
	lib := &model.Customer{
		OrgName: "도서관동명", OfficialName: "도서관동명", HasParent: true, ParentCustomerID: parent.CustomerID,
		IsActive: true, PartyKind: "customer", OrgID: model.OrgIDLibrary, Industry: "도서관",
	}
	for _, c := range []*model.Customer{school, off, partner, lib} {
		if err := cr.Create(c); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.db.Exec(`UPDATE customers SET org_code='JG01' WHERE customer_id=?`, school.CustomerID); err != nil {
		t.Fatal(err)
	}
	byCode, err := BuildSiteSchoolDataWorkbook([][]string{
		siteSchoolHdr,
		{model.SchoolParentOfficialName, "JG01", "다른이름", "", "", "", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	prev, err := r.PreviewSiteSchool("WP-O03", "O03", byCode, "add", false)
	if err != nil {
		t.Fatal(err)
	}
	if prev.SitesNew != 1 || prev.CustomersNew != 0 || prev.Rows[0].Site == nil || prev.Rows[0].Site.CustomerID != school.CustomerID || prev.Rows[0].Site.OrgID != "O03" {
		t.Fatalf("코드 매칭 %+v", prev.Rows)
	}
	app, err := r.ApplySiteSchool("WP-O03", "O03", "u", "a.xlsx", "add", byCode, false)
	if err != nil || app.Rows[0].Action != "new" {
		t.Fatalf("등록 %+v err=%v", app, err)
	}
	var siteOrg, custOrg string
	if err := r.db.QueryRow(`SELECT org_id FROM project_sites WHERE project_id='WP-O03' AND customer_id=?`, school.CustomerID).Scan(&siteOrg); err != nil || siteOrg != "O03" {
		t.Fatalf("사이트 조직 %s %v", siteOrg, err)
	}
	if err := r.db.QueryRow(`SELECT org_id FROM customers WHERE customer_id=?`, school.CustomerID).Scan(&custOrg); err != nil || custOrg != model.OrgIDLibrary {
		t.Fatalf("고객 조직 %s %v", custOrg, err)
	}
	also, err := r.ApplySiteSchool(pid, model.OrgIDLibrary, "u", "a.xlsx", "add", byCode, false)
	if err != nil || also.Rows[0].Action != "new" {
		t.Fatalf("다른 사업 %+v err=%v", also, err)
	}
	var o03Site string
	if err := r.db.QueryRow(`SELECT id FROM project_sites WHERE project_id='WP-O03' AND customer_id=?`, school.CustomerID).Scan(&o03Site); err != nil {
		t.Fatal(err)
	}
	if err := r.BulkSiteStatus("WP-O03", "u", model.SiteExcluded, "2026-11-11", "범위", []string{o03Site}); err != nil {
		t.Fatal(err)
	}
	var other string
	if err := r.db.QueryRow(`SELECT status FROM project_sites WHERE project_id=? AND customer_id=?`, pid, school.CustomerID).Scan(&other); err != nil || other != model.SitePlanned {
		t.Fatalf("다른 사업 상태 %s %v", other, err)
	}
	if err := r.DeactivateSite(o03Site, "WP-O03"); err != nil {
		t.Fatal(err)
	}
	var still int
	if err := r.db.QueryRow(`SELECT is_active FROM project_sites WHERE project_id=? AND customer_id=?`, pid, school.CustomerID).Scan(&still); err != nil || still != 1 {
		t.Fatalf("다른 사업 삭제 %d %v", still, err)
	}

	dup := &model.Customer{
		OrgName: "장곡초등학교", OfficialName: "장곡초등학교", HasParent: true, ParentCustomerID: parent.CustomerID,
		IsActive: true, PartyKind: "customer", OrgID: "O03", Industry: "학교",
	}
	if err := cr.Create(dup); err != nil {
		t.Fatal(err)
	}
	byName, err := BuildSiteSchoolDataWorkbook([][]string{
		siteSchoolHdr,
		{model.SchoolParentOfficialName, "", "장곡초등학교", "", "", "", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	amb, err := r.PreviewSiteSchool("WP-O03", "O03", byName, "add", true)
	if err != nil {
		t.Fatal(err)
	}
	if amb.Rows[0].Action != "err" || amb.CustomersNew != 0 || !strings.Contains(amb.Rows[0].Msg, "동명 학교가 여러 곳입니다") || !strings.Contains(amb.Rows[0].Msg, school.CustomerID) || !strings.Contains(amb.Rows[0].Msg, dup.CustomerID) {
		t.Fatalf("여러 곳 %+v", amb.Rows[0])
	}
	var nCust int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM customers WHERE org_name='장곡초등학교'`).Scan(&nCust); err != nil || nCust != 2 {
		t.Fatalf("자동 생성 %d %v", nCust, err)
	}

	codeDup := &model.Customer{
		OrgName: "코드중복초", OfficialName: "코드중복초", HasParent: true, ParentCustomerID: parent.CustomerID,
		OrgCode: "DUP1", IsActive: true, PartyKind: "customer", OrgID: "O03", Industry: "학교",
	}
	codeDup2 := &model.Customer{
		OrgName: "코드중복중", OfficialName: "코드중복중", HasParent: true, ParentCustomerID: parent.CustomerID,
		OrgCode: "DUP1", IsActive: true, PartyKind: "customer", OrgID: model.OrgIDLibrary, Industry: "school",
	}
	for _, c := range []*model.Customer{codeDup, codeDup2} {
		if err := cr.Create(c); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.db.Exec(`UPDATE customers SET org_code='DUP1' WHERE customer_id IN (?,?)`, codeDup.CustomerID, codeDup2.CustomerID); err != nil {
		t.Fatal(err)
	}
	byDup, err := BuildSiteSchoolDataWorkbook([][]string{siteSchoolHdr, {model.SchoolParentOfficialName, "DUP1", "코드중복초", "", "", "", ""}})
	if err != nil {
		t.Fatal(err)
	}
	cd, err := r.PreviewSiteSchool("WP-O03", "O03", byDup, "add", true)
	if err != nil || cd.Rows[0].Action != "err" || !strings.Contains(cd.Rows[0].Msg, "같은 기관코드가 여러 곳입니다") || !strings.Contains(cd.Rows[0].Msg, codeDup.CustomerID) {
		t.Fatalf("코드 중복 %+v err=%v", cd, err)
	}

	for _, row := range [][]string{
		{model.SchoolParentOfficialName, "", "쉬는초등학교", "", "", "", ""},
		{model.SchoolParentOfficialName, "", "협력초등학교", "", "", "", ""},
		{model.SchoolParentOfficialName, "", "도서관동명", "", "", "", ""},
	} {
		x, err := BuildSiteSchoolDataWorkbook([][]string{siteSchoolHdr, row})
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.PreviewSiteSchool("WP-O03", "O03", x, "add", false)
		if err != nil || got.Rows[0].Action != "err" || !strings.Contains(got.Rows[0].Msg, "마스터에 없습니다") {
			t.Fatalf("제외 %v %+v err=%v", row[2], got, err)
		}
	}

	missing, err := BuildSiteSchoolDataWorkbook([][]string{
		siteSchoolHdr,
		{model.SchoolParentOfficialName, "", "새초등학교", "", "", "", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	var parentsBefore int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM customers WHERE org_name=?`, model.SchoolParentOfficialName).Scan(&parentsBefore)
	made, err := r.ApplySiteSchool("WP-O03", "O03", "u", "n.xlsx", "add", missing, true)
	if err != nil || made.CustomersNew != 1 || made.Rows[0].Action != "new" || made.Rows[0].Msg != "새로 만들 고객" {
		t.Fatalf("신규 %+v err=%v", made, err)
	}
	var newOrg, newInd, newParent string
	if err := r.db.QueryRow(`SELECT org_id, industry, parent_customer_id FROM customers WHERE org_name='새초등학교'`).Scan(&newOrg, &newInd, &newParent); err != nil {
		t.Fatal(err)
	}
	if newOrg != "O03" || newInd != "학교" || newParent != parent.CustomerID {
		t.Fatalf("신규 고객 org=%s ind=%s parent=%s want %s", newOrg, newInd, newParent, parent.CustomerID)
	}
	var parentsAfter int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM customers WHERE org_name=?`, model.SchoolParentOfficialName).Scan(&parentsAfter)
	if parentsAfter != parentsBefore {
		t.Fatalf("상위 기관 중복 %d→%d", parentsBefore, parentsAfter)
	}
	again, err := r.ApplySiteSchool(pid, model.OrgIDLibrary, "u", "n.xlsx", "add", missing, true)
	if err != nil || again.CustomersNew != 0 || again.Rows[0].Action != "new" {
		t.Fatalf("전사에 있으면 재생성 금지 %+v err=%v", again, err)
	}
	var madeN int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM customers WHERE org_name='새초등학교'`).Scan(&madeN)
	if madeN != 1 {
		t.Fatalf("고객 수 %d", madeN)
	}
}

func Test95_167A_SiteSchoolWorkbookShape(t *testing.T) {
	empty, err := BuildSiteSchoolDataWorkbook([][]string{siteSchoolHdr, {"", "", "", "", "", "", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSiteSchoolWorkbook(empty); err == nil || !strings.Contains(err.Error(), "올릴 행이 없습니다") {
		t.Fatalf("빈 행 %v", err)
	}
	bad, err := BuildSiteSchoolDataWorkbook([][]string{{"잘못된헤더"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSiteSchoolWorkbook(bad); err == nil || !strings.Contains(err.Error(), "헤더") {
		t.Fatalf("헤더 %v", err)
	}
	rows := [][]string{siteSchoolHdr}
	for i := 0; i < model.SchoolImportMaxRows+1; i++ {
		rows = append(rows, []string{"", "", "학교", "", "", "", ""})
	}
	over, err := BuildSiteSchoolDataWorkbook(rows)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSiteSchoolWorkbook(over); err == nil || !strings.Contains(err.Error(), "행까지") {
		t.Fatalf("한도 %v", err)
	}
}
