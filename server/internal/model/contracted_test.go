package model

import (
	"strings"
	"testing"
)

func TestIsContractedEachTerm(t *testing.T) {
	if (WorkProject{ContractAmount: 1}).IsContracted() != true {
		t.Fatal("금액")
	}
	if (WorkProject{ContractNo: "C-1"}).IsContracted() != true {
		t.Fatal("번호")
	}
	linked := WorkProject{SalesProjectID: "SP-1", SalesStage: SalesStage4Closed, SalesCloseReason: SalesCloseContracted}
	if !linked.IsContracted() {
		t.Fatal("영업 연결")
	}
	none := WorkProject{Name: "없음"}
	if none.IsContracted() {
		t.Fatal("모두 없음")
	}
	open := WorkProject{SalesProjectID: "SP-1", SalesStage: SalesStage4Bid, SalesCloseReason: ""}
	if open.IsContracted() {
		t.Fatal("영업 미계약 연결")
	}
}

func TestContractRunStatus(t *testing.T) {
	today := "2026-10-19"
	prog := WorkProject{Status: WBProjectActive, StartDate: "2026-01-01", EndDate: "2026-12-31"}
	if prog.ContractRunStatus(today) != RunProgress {
		t.Fatal("진행")
	}
	wait := WorkProject{Status: WBProjectActive, StartDate: "2026-11-01"}
	if wait.ContractRunStatus(today) != RunWait {
		t.Fatal("대기")
	}
	need := WorkProject{Status: WBProjectActive, EndDate: "2026-09-01"}
	if need.ContractRunStatus(today) != RunNeedComplete {
		t.Fatal("완료 확인 필요")
	}
	done := WorkProject{Status: WBProjectComplete, EndDate: "2026-09-01"}
	if done.ContractRunStatus(today) != RunDone {
		t.Fatal("완료")
	}
	if need.ContractRunStatus(today) == RunDone {
		t.Fatal("종료일 경과로 자동 완료되면 안 된다")
	}
}

func TestContractedSQLSharedFragment(t *testing.T) {
	s := ContractedSQL("p", "sp")
	for _, p := range []string{"contract_amount", "contract_no", "close_reason"} {
		if !strings.Contains(s, p) {
			t.Fatal(s)
		}
	}
}
