package repository

import (
	"path/filepath"
	"testing"

	"customer-support/internal/model"
)

func TestListContractUnionSources(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cu.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	prepo := NewProjectRepo(db)
	srepo := NewSalesRepo(db)

	onlyWork := &model.WorkProject{Name: "사업관리만", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 1000}
	if err := prepo.Create(onlyWork); err != nil {
		t.Fatal(err)
	}

	linkedS := &model.SalesProject{Name: "연결영업", ProspectName: "고객A"}
	if err := srepo.Create(linkedS); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, contract_amount=?, contracted_at=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, 5000, "2026-09-01", linkedS.SalesID); err != nil {
		t.Fatal(err)
	}
	linkedW := &model.WorkProject{Name: "연결된사업", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 5000, SalesProjectID: linkedS.SalesID}
	if err := prepo.Create(linkedW); err != nil {
		t.Fatal(err)
	}

	openS := &model.SalesProject{Name: "진행중"}
	if err := srepo.Create(openS); err != nil {
		t.Fatal(err)
	}

	lost := &model.SalesProject{Name: "실주"}
	if err := srepo.Create(lost); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseLost, lost.SalesID); err != nil {
		t.Fatal(err)
	}

	zero := &model.SalesProject{Name: "금액0계약", ProspectName: "고객B", ExpectedAmount: 165000}
	if err := srepo.Create(zero); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, contract_amount=0, contracted_at=?, expected_amount=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, "2026-09-02", 165000, zero.SalesID); err != nil {
		t.Fatal(err)
	}

	otherOrg := &model.SalesProject{Name: "타조직계약", OrgID: "O99"}
	if err := srepo.Create(otherOrg); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, org_id=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, "O99", otherOrg.SalesID); err != nil {
		t.Fatal(err)
	}

	rows, err := prepo.ListContractUnion(ContractUnionFilter{OrgID: OrgAll})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, r := range rows {
		names[r.Name] = r.Source
	}
	if names["사업관리만"] != model.ContractSourceWork {
		t.Fatal("사업관리만 없음")
	}
	if names["연결된사업"] != model.ContractSourceWork || names["연결영업"] != "" {
		t.Fatalf("연결 건이 중복이다 %+v", names)
	}
	if names["금액0계약"] != model.ContractSourceSales {
		t.Fatal("미연결 영업 행 없음")
	}
	if names["진행중"] != "" || names["실주"] != "" {
		t.Fatal("진행·실주가 들어갔다")
	}
	var zeroRow model.ContractRow
	for _, r := range rows {
		if r.Name == "금액0계약" {
			zeroRow = r
		}
	}
	if !zeroRow.AmountMissing || zeroRow.ExpectedAmount != 165000 {
		t.Fatalf("금액 미입력 %+v", zeroRow)
	}
	kpi := model.BuildContractUnionKPI(rows, "2026-10-19")
	if kpi.Amount < 1000+5000 {
		t.Fatalf("금액 합계 %d", kpi.Amount)
	}
	if kpi.Unpromoted < 1 {
		t.Fatal("미전환 건수 0")
	}

	st := ContractUnionFilter{OrgID: OrgAll, Status: model.WBProjectActive}
	only, err := prepo.ListContractUnion(st)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range only {
		if r.Source == model.ContractSourceSales {
			t.Fatal("상태 필터에 ② 행이 있다")
		}
	}

	o99, err := prepo.ListContractUnion(ContractUnionFilter{OrgID: "O01"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range o99 {
		if r.Name == "타조직계약" {
			t.Fatal("타 조직 영업 계약이 노출됐다")
		}
	}

	found := doGetContractHref(rows, "금액0계약")
	if found != "/sales/"+zero.SalesID {
		t.Fatalf("href %s", found)
	}

	y2025, err := prepo.ListContractUnion(ContractUnionFilter{OrgID: OrgAll, Year: 2025})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range y2025 {
		if r.Name == "금액0계약" {
			t.Fatal("연도 필터에 ② 행이 잘못 들어갔다")
		}
	}

	q, err := prepo.ListContractUnion(ContractUnionFilter{OrgID: OrgAll, Search: "금액0"})
	if err != nil {
		t.Fatal(err)
	}
	if doGetContractHref(q, "금액0계약") == "" {
		t.Fatal("검색에 ② 행이 없다")
	}
}

func TestListContractUnionLinkSuggest(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "cu_link.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	prepo := NewProjectRepo(db)
	srepo := NewSalesRepo(db)

	s1 := &model.SalesProject{Name: "제천 RFID 구축"}
	if err := srepo.Create(s1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, contract_amount=?, sales_no=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, 17869000, "S2609-032", s1.SalesID); err != nil {
		t.Fatal(err)
	}
	w1 := &model.WorkProject{Name: "제천RFID구축", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 17689000}
	if err := prepo.Create(w1); err != nil {
		t.Fatal(err)
	}

	partial := &model.SalesProject{Name: "제천 RFID"}
	if err := srepo.Create(partial); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, contract_amount=? WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, 100, partial.SalesID); err != nil {
		t.Fatal(err)
	}

	sDupA := &model.SalesProject{Name: "동명이인"}
	sDupB := &model.SalesProject{Name: "동명이인"}
	if err := srepo.Create(sDupA); err != nil {
		t.Fatal(err)
	}
	if err := srepo.Create(sDupB); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, contract_amount=1 WHERE sales_id IN (?,?)`,
		model.SalesStage4Closed, model.SalesCloseContracted, sDupA.SalesID, sDupB.SalesID); err != nil {
		t.Fatal(err)
	}
	wDup := &model.WorkProject{Name: "동명이인", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 1}
	if err := prepo.Create(wDup); err != nil {
		t.Fatal(err)
	}

	linkedS := &model.SalesProject{Name: "중복연결영업"}
	if err := srepo.Create(linkedS); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sales_projects SET stage=?, close_reason=?, contract_amount=9 WHERE sales_id=?`,
		model.SalesStage4Closed, model.SalesCloseContracted, linkedS.SalesID); err != nil {
		t.Fatal(err)
	}
	wA := &model.WorkProject{Name: "중복A", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 9, SalesProjectID: linkedS.SalesID}
	if err := prepo.Create(wA); err != nil {
		t.Fatal(err)
	}
	wB := &model.WorkProject{Name: "중복B", PlanYear: 2026, Status: model.WBProjectActive, ContractAmount: 9, SalesProjectID: linkedS.SalesID}
	if err := prepo.Create(wB); err == nil {
		t.Fatal("같은 영업 사업에 사업관리 두 건 연결이 허용됐다")
	}

	rows, err := prepo.ListContractUnion(ContractUnionFilter{OrgID: OrgAll})
	if err != nil {
		t.Fatal(err)
	}
	var seed, amb model.ContractRow
	names := map[string]string{}
	for _, r := range rows {
		names[r.Name] = r.Source
		if r.ProjectID == w1.ProjectID {
			seed = r
		}
		if r.ProjectID == wDup.ProjectID {
			amb = r
		}
	}
	if seed.LinkNeed != "S2609-032" || seed.LinkSalesID != s1.SalesID {
		t.Fatalf("연결 제안 %+v", seed)
	}
	if !seed.AmountMismatch {
		t.Fatal("금액 불일치 없음")
	}
	if names["제천 RFID 구축"] != "" {
		t.Fatal("제안 대상이 ② 행으로 나왔다")
	}
	if names["제천 RFID"] != model.ContractSourceSales {
		t.Fatal("부분 일치 영업이 제안으로 빠지거나 ②에 없다")
	}
	if !amb.LinkAmbiguous || amb.LinkNeed != "" {
		t.Fatalf("동명이인 확인 필요 %+v", amb)
	}

	kpi := model.BuildContractUnionKPI(rows, "2026-10-19")
	if kpi.NeedLink < 1 {
		t.Fatal("연결 필요 건수 0")
	}

	if err := prepo.LinkSalesProject(w1.ProjectID, s1.SalesID, "tester"); err != nil {
		t.Fatal(err)
	}
	got, err := prepo.Get(w1.ProjectID)
	if err != nil || got.SalesProjectID != s1.SalesID {
		t.Fatalf("연결 저장 %+v %v", got, err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM data_change_logs WHERE table_name='work_projects' AND entity_id=?`, w1.ProjectID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("변경 로그 없음")
	}

	after, err := prepo.ListContractUnion(ContractUnionFilter{OrgID: OrgAll})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range after {
		if r.ProjectID == w1.ProjectID && r.AmountMismatch && r.SalesAmount != 17869000 {
			t.Fatalf("연결 후 영업 금액 %+v", r)
		}
		if r.Name == "제천 RFID 구축" && r.Source == model.ContractSourceSales {
			t.Fatal("연결 후에도 ② 행")
		}
	}
}

func TestContractUnionIsSharedContractQuery(t *testing.T) {
	// 73-E 계약 탭은 이 함수를 공유한다. 별도 판정 함수를 두지 않는다.
	var r *ProjectRepo
	_ = r.ListContractUnion
}

func doGetContractHref(rows []model.ContractRow, name string) string {
	for _, r := range rows {
		if r.Name == name {
			return r.Href()
		}
	}
	return ""
}
