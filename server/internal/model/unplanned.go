package model

import (
	"strconv"
	"strings"
)

// UnplannedKind §8.1 미계획 유형
const (
	UnplannedNoDate     = "no_date"    // 예정없음
	UnplannedDelayed    = "delayed"    // 예정일경과
	UnplannedNext       = "next"       // 다음일정미정
	UnplannedUnassigned = "unassigned" // 담당자미배정
	UnplannedReview        = "review"         // 미정사유만료
	UnplannedSalesFollowup = "sales_followup" // 영업 후속없음 (§22.1.1 ⑨)
)

// UnplannedNoDateReviewDays 「미정+사유」 재검토 주기(일)
const UnplannedNoDateReviewDays = 14

// ScheduleNoDateReason 미정 사유(§8.2)
const (
	NoDateReasonParts    = "parts"
	NoDateReasonCustomer = "customer"
	NoDateReasonVendor   = "vendor"
	NoDateReasonOther    = "other"
)

func NoDateReasonLabel(code string) string {
	raw := strings.TrimSpace(code)
	switch {
	case raw == NoDateReasonParts || raw == "부품 대기":
		return "부품 대기"
	case raw == NoDateReasonCustomer || raw == "고객 일정 미확정":
		return "고객 일정 미확정"
	case raw == NoDateReasonVendor || raw == "외부 업체 회신 대기":
		return "외부 업체 회신 대기"
	case raw == NoDateReasonOther || strings.HasPrefix(raw, "기타"):
		if strings.HasPrefix(raw, "기타") {
			return raw
		}
		return "기타"
	default:
		return raw
	}
}

func FormatNoDateReason(code, detail string) string {
	code = strings.TrimSpace(code)
	detail = strings.TrimSpace(detail)
	if code == NoDateReasonOther || code == "기타" {
		if detail == "" {
			return "기타"
		}
		return "기타: " + detail
	}
	label := NoDateReasonLabel(code)
	if label == "" {
		return detail
	}
	return label
}

func UnplannedKindLabel(kind string) string {
	switch kind {
	case UnplannedNoDate:
		return "예정없음"
	case UnplannedDelayed:
		return "예정일경과"
	case UnplannedNext:
		return "다음일정미정"
	case UnplannedUnassigned:
		return "담당자미배정"
	case UnplannedReview:
		return "미정사유만료"
	case UnplannedSalesFollowup:
		return "영업 후속없음"
	default:
		return kind
	}
}

// UnplannedBadge 목록 행 뱃지
type UnplannedBadge struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Class string `json:"class"`
}

func UnplannedBadgeOf(kind string, daysOverdue int) UnplannedBadge {
	return UnplannedBadgeOfStarted(kind, daysOverdue, false)
}

// UnplannedBadgeOfStarted §8.2.2 착수한 예정일경과는 노란 「진행중 D+n」.
func UnplannedBadgeOfStarted(kind string, daysOverdue int, started bool) UnplannedBadge {
	switch kind {
	case UnplannedNoDate:
		return UnplannedBadge{Kind: kind, Label: "예정없음", Class: "bg-red-100 text-red-800"}
	case UnplannedDelayed:
		if started {
			label := "진행중"
			if daysOverdue > 0 {
				label = "진행중 D+" + strconv.Itoa(daysOverdue)
			}
			return UnplannedBadge{Kind: kind, Label: label, Class: "bg-amber-100 text-amber-800"}
		}
		label := "예정일경과"
		if daysOverdue > 0 {
			label = "D+" + strconv.Itoa(daysOverdue)
		}
		return UnplannedBadge{Kind: kind, Label: label, Class: "bg-red-100 text-red-800"}
	case UnplannedNext:
		return UnplannedBadge{Kind: kind, Label: "방문함", Class: "bg-amber-100 text-amber-800"}
	case UnplannedUnassigned:
		return UnplannedBadge{Kind: kind, Label: "미배정", Class: "bg-orange-100 text-orange-800"}
	case UnplannedReview:
		return UnplannedBadge{Kind: kind, Label: "재검토", Class: "bg-amber-100 text-amber-900"}
	case UnplannedSalesFollowup:
		return UnplannedBadge{Kind: kind, Label: "영업 후속없음", Class: "bg-orange-100 text-orange-800"}
	default:
		return UnplannedBadge{Kind: kind, Label: kind, Class: "bg-gray-100 text-gray-700"}
	}
}

// UnplannedItem 미계획 업무함 1행
type UnplannedItem struct {
	WorkListItem
	ItemKey       string           `json:"item_key"`
	Kinds         []string         `json:"kinds"`
	Badges        []UnplannedBadge `json:"badges"`
	NoDateReason  string           `json:"no_date_reason"`
	NoDateAt      string           `json:"no_date_at"`
	CanAssignDate bool             `json:"can_assign_date"`
	CanNoDate     bool             `json:"can_no_date"`
	PlanID        string           `json:"plan_id"`
	CustomerID    string           `json:"customer_id"`
	ProductType   string           `json:"product_type"`
	Started       bool             `json:"started"` // §8.2.2 착수(start_datetime 또는 as_processes)
}

func (it UnplannedItem) HasKind(kind string) bool {
	for _, k := range it.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// NeedPlanKind §8.2.1 「계획을 세워야 할 건」 — 예정없음 · 담당자미배정.
// 재검토(미정+사유 만료)는 밀린 건 묶음이므로 예정없음이 있어도 여기 세지 않는다.
func (it UnplannedItem) NeedPlanKind() bool {
	if it.HasKind(UnplannedUnassigned) || it.HasKind(UnplannedSalesFollowup) {
		return true
	}
	return it.HasKind(UnplannedNoDate) && !it.HasKind(UnplannedReview)
}

// OverdueKind §8.2.1 「일정이 지난 건」 — 예정일경과 · 다음일정미정 · 재검토
func (it UnplannedItem) OverdueKind() bool {
	return it.HasKind(UnplannedDelayed) || it.HasKind(UnplannedNext) || it.HasKind(UnplannedReview)
}

// UnplannedKindCounts 유형별 건수(한 건이 여러 유형이면 각각 센다)
type UnplannedKindCounts struct {
	NoDate     int
	Delayed    int
	Next       int
	Unassigned    int
	Review        int
	SalesFollowup int
	NeedPlan      int // 고유: 계획이 없는 건
	Overdue       int // 고유: 밀린 건
	Total         int // 고유 건수
}

func (c *UnplannedKindCounts) AddItem(it UnplannedItem) {
	c.Total++
	if it.NeedPlanKind() {
		c.NeedPlan++
	}
	if it.OverdueKind() {
		c.Overdue++
	}
	for _, k := range it.Kinds {
		switch k {
		case UnplannedNoDate:
			c.NoDate++
		case UnplannedDelayed:
			c.Delayed++
		case UnplannedNext:
			c.Next++
		case UnplannedUnassigned:
			c.Unassigned++
		case UnplannedReview:
			c.Review++
		case UnplannedSalesFollowup:
			c.SalesFollowup++
		}
	}
}

// PlanningCounts §8.4 계획 수립률 분모·분자.
// 분자(Planned)는 예정일 있음 또는 미정+사유. 밀린 건(경과·다음일정·재검토)은 계획이 있으므로 분자에 둔다(§8.2.1).
type PlanningCounts struct {
	Open     int // 전체 미완료
	Planned  int // 계획이 있는 건 (예정일 또는 미정+사유). 담당자미배정은 제외
	NeedPlan int // 계획이 없는 건 (예정없음·담당자미배정)
}

func (p PlanningCounts) HasPlanningRate() bool {
	return HasPlanningRate(p.Open)
}

func (p PlanningCounts) Rate() float64 {
	return PlanningRatePct(p.Planned, p.Open)
}
