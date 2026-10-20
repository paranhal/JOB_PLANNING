package model

import "strings"

// SalesActTypeStage 영업 단계(4단계) ↔ 활동 유형 기본 대응. 어느 단계도 아니면 "".
func SalesActTypeStage(activityType string) string {
	switch strings.TrimSpace(activityType) {
	case SalesActTypeResearch:
		return SalesStage4Discover
	case SalesActTypeMaterial, SalesActTypeQuote, SalesActTypeRequirement, SalesActTypeRFPReceived:
		return SalesStage4Propose
	case SalesActTypeProposal, SalesActTypeRFP:
		return SalesStage4Bid
	default:
		return ""
	}
}

// SalesActTypesForStage 해당 단계의 기본 유형. 알 수 없으면 기타.
func SalesActTypesForStage(stage string) []string {
	switch strings.TrimSpace(stage) {
	case SalesStage4Discover:
		return []string{SalesActTypeResearch}
	case SalesStage4Propose:
		return []string{SalesActTypeMaterial, SalesActTypeQuote, SalesActTypeRequirement, SalesActTypeRFPReceived}
	case SalesStage4Bid:
		return []string{SalesActTypeProposal}
	default:
		return []string{SalesActTypeOther}
	}
}

func SalesActStageLabel(stage string) string {
	switch strings.TrimSpace(stage) {
	case SalesStage4Discover:
		return "발굴"
	case SalesStage4Propose:
		return "제안"
	case SalesStage4Bid:
		return "입찰"
	default:
		return "기타"
	}
}

func IsActiveSalesActType(code string) bool {
	switch strings.TrimSpace(code) {
	case SalesActTypeResearch, SalesActTypeMaterial, SalesActTypeQuote,
		SalesActTypeRequirement, SalesActTypeRFPReceived, SalesActTypeProposal, SalesActTypeOther:
		return true
	default:
		return false
	}
}

func IsActiveSalesActMethod(code string) bool {
	switch strings.TrimSpace(code) {
	case SalesActMethodVisit, SalesActMethodCall, SalesActMethodMail,
		SalesActMethodOnline, SalesActMethodInternal, SalesActMethodOther:
		return true
	default:
		return false
	}
}

func IsContactSalesActMethod(code string) bool {
	switch strings.TrimSpace(code) {
	case SalesActMethodVisit, SalesActMethodCall, SalesActMethodMail, SalesActMethodOnline:
		return true
	default:
		return false
	}
}

func ValidateSalesActTypeMethod(typ, method string) error {
	typ = strings.TrimSpace(typ)
	method = strings.TrimSpace(method)
	if typ == "" {
		return errSalesAct("활동 유형을 선택하세요")
	}
	if !IsActiveSalesActType(typ) {
		return errSalesAct("예전 활동 유형입니다. 내용과 방법을 다시 선택하세요")
	}
	if method == "" {
		return errSalesAct("방법을 선택하세요")
	}
	if !IsActiveSalesActMethod(method) {
		return errSalesAct("방법을 선택하세요")
	}
	return nil
}

type salesActErr string

func (e salesActErr) Error() string { return string(e) }

func errSalesAct(s string) error { return salesActErr(s) }

func SalesActStageMismatchWarning(projectStage, activityType string) string {
	want := SalesActTypeStage(activityType)
	proj := strings.TrimSpace(projectStage)
	if want == "" || proj == "" || want == proj {
		return ""
	}
	return "이 사업은 " + SalesActStageLabel(proj) + " 단계입니다. " +
		SalesActTypeDisplayName(activityType) + "은(는) " + SalesActStageLabel(want) + " 단계 활동입니다."
}

func SalesActTypeDisplayName(code string) string {
	switch strings.TrimSpace(code) {
	case SalesActTypeResearch:
		return "정보 수집"
	case SalesActTypeMaterial:
		return "자료송부"
	case SalesActTypeQuote:
		return "견적제출"
	case SalesActTypeRequirement:
		return "요구사항 파악"
	case SalesActTypeRFPReceived:
		return "RFP 접수"
	case SalesActTypeProposal, SalesActTypeRFP:
		return "제안서 제출"
	case SalesActTypeOther:
		return "기타"
	default:
		return code
	}
}

func SalesActMethodDisplayName(code string) string {
	switch strings.TrimSpace(code) {
	case SalesActMethodVisit:
		return "방문"
	case SalesActMethodCall:
		return "전화"
	case SalesActMethodMail:
		return "이메일"
	case SalesActMethodOnline:
		return "온라인미팅"
	case SalesActMethodInternal:
		return "내부회의"
	case SalesActMethodOther:
		return "기타"
	default:
		return code
	}
}

func (a SalesActivity) NeedsTypeReview() bool {
	return strings.TrimSpace(a.ActivityType) == SalesActTypeOther && IsContactSalesActMethod(a.ActivityMethod)
}

type SalesActTypeGroup struct {
	Stage       string
	Label       string
	Recommended bool
	Types       []Code
}

func SalesActTypeGroups(projectStage string, types []Code) []SalesActTypeGroup {
	by := map[string][]Code{}
	for _, t := range types {
		st := SalesActTypeStage(t.CodeValue)
		if t.CodeValue == SalesActTypeOther || st == "" {
			st = ""
		}
		by[st] = append(by[st], t)
	}
	order := []string{SalesStage4Discover, SalesStage4Propose, SalesStage4Bid, ""}
	proj := strings.TrimSpace(projectStage)
	if proj != "" {
		front := []string{proj}
		for _, s := range order {
			if s != proj {
				front = append(front, s)
			}
		}
		order = front
	}
	var out []SalesActTypeGroup
	for _, st := range order {
		items := by[st]
		if len(items) == 0 {
			continue
		}
		out = append(out, SalesActTypeGroup{
			Stage: st, Label: SalesActStageLabel(st),
			Recommended: st != "" && st == proj,
			Types:       items,
		})
	}
	return out
}
