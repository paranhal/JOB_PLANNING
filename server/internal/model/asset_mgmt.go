package model

import "strings"

const (
	ManagementTypeFault     = "fault"
	ManagementTypePeriodic  = "periodic"
	ManagementTypeDirect    = "direct"
	ManagementTypeOnDemand  = "on_demand"
	ManagementTypeReference = "reference"

	MaintContractPaid = "paid"
	MaintContractFree = "free"
	MaintContractNone = "none"
	MaintCycleCall    = "call"
)

func IsSelectableManagementType(s string) bool {
	switch strings.TrimSpace(s) {
	case ManagementTypeFault, ManagementTypePeriodic, ManagementTypeThirdParty:
		return true
	default:
		return false
	}
}

func ManagementTypeLabel(s string) string {
	switch strings.TrimSpace(s) {
	case ManagementTypeDirect:
		return "직접유지보수"
	case ManagementTypeFault:
		return "장애대응"
	case ManagementTypePeriodic:
		return "정기점검"
	case ManagementTypeOnDemand:
		return "요청시지원"
	case ManagementTypeReference:
		return "참고관리"
	case ManagementTypeThirdParty:
		return "타사장비"
	case "":
		return "—"
	default:
		return s
	}
}

// MapBulkManagementType 093 일괄 정리 §3-A. 바꿀 필요 없으면 입력을 그대로 둔다.
func MapBulkManagementType(mgmt, contract string) string {
	m := strings.TrimSpace(mgmt)
	c := strings.ToLower(strings.TrimSpace(contract))
	switch strings.ToLower(m) {
	case ManagementTypeDirect, ManagementTypeOnDemand, ManagementTypeReference:
		return ManagementTypePeriodic
	case "":
		switch c {
		case MaintContractPaid, MaintContractFree:
			return ManagementTypePeriodic
		case MaintContractNone:
			return ManagementTypeFault
		default:
			return ""
		}
	default:
		return m
	}
}

// MapBulkMaintCycle 093 일괄 정리 §3-B. 미계약이면서 주기가 있으면 call. 빈 값은 비움. 다른 계약은 그대로.
func MapBulkMaintCycle(contract, cycle string) string {
	c := strings.ToLower(strings.TrimSpace(contract))
	if c != MaintContractNone {
		return cycle
	}
	if strings.TrimSpace(cycle) == "" {
		return strings.TrimSpace(cycle)
	}
	return MaintCycleCall
}

func CanonicalMaintCycleCode(s string) string {
	t := strings.TrimSpace(s)
	if strings.EqualFold(t, MaintCycleCall) {
		return MaintCycleCall
	}
	return t
}

func MaintCycleDisplayLabel(s string) string {
	t := strings.TrimSpace(s)
	if strings.EqualFold(t, MaintCycleCall) {
		return "CALL"
	}
	switch t {
	case "monthly":
		return "월"
	case "quarterly":
		return "분기"
	case "semi":
		return "반기"
	case "odd_bimonthly":
		return "홀수격월"
	case "even_bimonthly":
		return "짝수격월"
	case "yearly":
		return "년1회"
	case "custom":
		return "직접입력"
	case "none":
		return "없음"
	default:
		return t
	}
}

// ApplyNoneContractCycleDefault 저장 경로: 미계약이고 주기가 비면 CALL. 마이그레이션(빈 값 유지)과 다르다.
func ApplyNoneContractCycleDefault(a *Asset) {
	if a == nil {
		return
	}
	a.MaintCycle = CanonicalMaintCycleCode(a.MaintCycle)
	if strings.ToLower(strings.TrimSpace(a.MaintContractType)) == MaintContractNone &&
		strings.TrimSpace(a.MaintCycle) == "" {
		a.MaintCycle = MaintCycleCall
	}
}
