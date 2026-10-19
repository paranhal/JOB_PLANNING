package model

import "strings"

const (
	RunProgress     = "progress"
	RunWait         = "wait"
	RunNeedComplete = "need_complete"
	RunDone         = "done"
)

// ContractedSQL 계약된 사업. 사업 대시보드·계약 관리·목록·집계 공유. 83-A.
func ContractedSQL(pAlias, salesAlias string) string {
	if pAlias == "" {
		pAlias = "p"
	}
	if salesAlias == "" {
		salesAlias = "sp"
	}
	return ` (
			COALESCE(` + pAlias + `.contract_amount,0) > 0
			OR TRIM(COALESCE(` + pAlias + `.contract_no,'')) != ''
			OR (
				TRIM(COALESCE(` + pAlias + `.sales_project_id,'')) != ''
				AND COALESCE(` + salesAlias + `.stage,'')='closed'
				AND COALESCE(` + salesAlias + `.close_reason,'')='contracted'
			)
		)`
}

func (p WorkProject) IsContracted() bool {
	if p.ContractAmount > 0 {
		return true
	}
	if strings.TrimSpace(p.ContractNo) != "" {
		return true
	}
	return strings.TrimSpace(p.SalesProjectID) != "" &&
		strings.TrimSpace(p.SalesStage) == SalesStage4Closed &&
		strings.TrimSpace(p.SalesCloseReason) == SalesCloseContracted
}

func (p WorkProject) ContractRunStatus(today string) string {
	if strings.TrimSpace(p.Status) == WBProjectComplete {
		return RunDone
	}
	today = strings.TrimSpace(today)
	end := strings.TrimSpace(p.EndDate)
	start := strings.TrimSpace(p.StartDate)
	if end != "" && today != "" && end < today {
		return RunNeedComplete
	}
	if start != "" && today != "" && start > today {
		return RunWait
	}
	return RunProgress
}

func ContractRunLabel(st string) string {
	switch st {
	case RunWait:
		return "대기"
	case RunNeedComplete:
		return "완료 확인 필요"
	case RunDone:
		return "완료"
	default:
		return "진행"
	}
}

func ProjectContractHref(projectID, salesID, source string) string {
	if source == ContractSourceSales || (strings.TrimSpace(projectID) == "" && strings.TrimSpace(salesID) != "") {
		return "/sales/" + salesID
	}
	return "/contracts/" + projectID
}
