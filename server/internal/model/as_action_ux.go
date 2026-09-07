package model

import "strings"

// CauseCategory 원인분류 계층. §34.3.3
type CauseCategory struct {
	Code         string `json:"code"`
	Level        int    `json:"level"`
	ParentCode   string `json:"parent_code"`
	Label        string `json:"label"`
	SortOrder    int    `json:"sort_order"`
	IsActive     bool   `json:"is_active"`
	CauseTypeMap string `json:"cause_type_map"`
	IsFault      bool   `json:"is_fault"`
}

// CauseTypeFromCat2 2차 코드의 cause_type_map. 매핑이 없으면 빈값. §34.3.3
func CauseTypeFromCat2(cats []CauseCategory, cat2 string) string {
	cat2 = strings.TrimSpace(cat2)
	if cat2 == "" {
		return ""
	}
	for _, c := range cats {
		if c.Code == cat2 {
			return strings.TrimSpace(c.CauseTypeMap)
		}
	}
	return ""
}

// CauseCat2HasChildren 3차가 있는 2차인가. 있으면 3차 필수. §34.3.3
func CauseCat2HasChildren(cats []CauseCategory, cat2 string) bool {
	cat2 = strings.TrimSpace(cat2)
	if cat2 == "" {
		return false
	}
	for _, c := range cats {
		if c.Level == 3 && c.ParentCode == cat2 && c.IsActive {
			return true
		}
	}
	return false
}

const (
	ProcessTypeRemote  = "remote"
	ProcessTypeVisit   = "visit"
	ProcessTypeInquiry = "inquiry"
)

// ProcessTypesForWorkPlace 근무구분에 허용되는 처리유형. 첫 항목이 기본값. §34.3.2
func ProcessTypesForWorkPlace(place string) []string {
	switch NormalizeWorkPlace(place) {
	case WorkPlaceOffice:
		return []string{ProcessTypeRemote, ProcessTypeInquiry}
	case WorkPlaceField:
		return []string{ProcessTypeVisit, ProcessTypeInquiry}
	default:
		return nil
	}
}

// ProcessTypeAllowed 근무구분·처리유형 조합이 맞는가.
func ProcessTypeAllowed(place, proc string) bool {
	proc = strings.TrimSpace(proc)
	if proc == "" {
		return false
	}
	for _, p := range ProcessTypesForWorkPlace(place) {
		if p == proc {
			return true
		}
	}
	return false
}

// ProcessTypePlaces 드롭다운 data-places. remote=내근, visit=외근, inquiry=둘 다.
func ProcessTypePlaces(code string) string {
	switch strings.TrimSpace(code) {
	case ProcessTypeRemote:
		return WorkPlaceOffice
	case ProcessTypeVisit:
		return WorkPlaceField
	case ProcessTypeInquiry:
		return WorkPlaceOffice + " " + WorkPlaceField
	default:
		return ""
	}
}

// IsSelectableActionResult 조치 화면에서 고를 수 있는 결과. §34.3.4
func IsSelectableActionResult(code string) bool {
	switch strings.TrimSpace(code) {
	case ResultDone, ResultPartial, ResultTransfer:
		return true
	default:
		return false
	}
}
