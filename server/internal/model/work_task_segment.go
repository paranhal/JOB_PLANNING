package model

import (
	"fmt"
	"sort"
	"strings"
)

const (
	SegKindPlan    = "plan"
	SegKindActual  = "actual"
	SegScopeCommon = "common"
	SegScopePerson = "person"

	ItemTodo  = "todo"
	ItemDoing = "doing"
	ItemDone  = "done"
	ItemHold  = "hold"
	ItemNA    = "na"

	ItemKindInstall = "install"
	ItemKindWork    = "work"
	ItemKindAS      = "as"
	ItemKindEtc     = "etc"

	ItemAssetInstall = "install"
	ItemAssetService = "service"
	ItemAssetRef     = "ref"

	CodeGroupItemKind = "wb_item_kind"
	CodeGroupItemUnit = "wb_item_unit"

	ReturnPending     = "pending"
	ReturnReturned    = "returned"
	ReturnNone        = "no_return"
	ReturnResumedNow  = "resumed_now"
	ReturnResumeLater = "resume_later"

	SegSourceManual = "manual"
	SegSourceLive   = "live"

	EndedByUser      = "user"
	EndedByInterrupt = "interrupt"
	EndedByDayEnd    = "day_end"
	EndedByAuto      = "auto"

	MinSegmentMin   = 15
	TravelMinMax    = 180
	WorkdayStartMin = 7 * 60
	WorkdayEndMin   = 20 * 60
)

type WorkTaskSegment struct {
	SegmentID       string
	TaskID          string
	Kind            string
	Scope           string
	Seq             int
	WorkDate        string
	StartTime       string
	EndTime         string
	DurationMin     int
	TravelBeforeMin int
	TravelAfterMin  int
	UserID          string
	Assignee        string
	WorkerUserID    string
	WorkerName      string
	InterruptTaskID     string
	Note                string
	Source              string
	EndedBy             string
	TravelOrigin        string
	TravelDest          string
	TravelPlanDepart    string
	TravelPlanArrive    string
	TravelActualDepart  string
	TravelActualArrive  string
	TravelSource        string
	TravelUnknownReason string
	Virtual             bool
}

type WorkTaskDeparture struct {
	DepartureID     string
	TaskID          string
	UserID          string
	Assignee        string
	WorkDate        string
	StartTime       string
	EndTime         string
	InterruptTaskID string
	ReturnStatus    string
	Reason          string
	NextActionDate  string
	RemainingMin    *int
	ForceReason     string
}

type WorkTaskItem struct {
	ItemID           string
	TaskID           string
	Seq              int
	Title            string
	Status           string
	Note             string
	CopiedFromItemID string
	Kind             string
	ProductName      string
	ModelName        string
	Qty              int
	ASID             string
	ASNumber         string
	ASLocked         bool
	Assignees        []WorkTaskItemAssignee
	AssetIDs         []string
	AssetRole        string
	AssetMissing     bool
	TemplateID       string
	SetMainItemID    string
	Unit             string
	SetIncluded      bool
	ClientKey        string
	SetMainKey       string
	IsSetMain        bool
}

type WorkTaskItemAssignee struct {
	Name   string `json:"Name"`
	UserID string `json:"UserID"`
}

type ItemKindCounts struct {
	Install    int
	Work       int
	AS         int
	Etc        int
	Total      int
	Done       int
	InstallQty int
	UnitQty    map[string]int
	Line       string
	Progress   string
	LineQty    string
}

func NormalizeSegmentKind(s string) string {
	if strings.TrimSpace(s) == SegKindActual {
		return SegKindActual
	}
	return SegKindPlan
}

func NormalizeSegmentScope(s string) string {
	if strings.TrimSpace(s) == SegScopePerson {
		return SegScopePerson
	}
	return SegScopeCommon
}

func NormalizeItemKind(s string) string {
	switch strings.TrimSpace(s) {
	case ItemKindInstall, ItemKindWork, ItemKindAS:
		return strings.TrimSpace(s)
	default:
		return ItemKindEtc
	}
}

func ItemKindLabel(s string) string {
	switch NormalizeItemKind(s) {
	case ItemKindInstall:
		return "장비 장착"
	case ItemKindWork:
		return "공사"
	case ItemKindAS:
		return "AS"
	default:
		return "기타"
	}
}

func NormalizeItemAssetRole(kind, role string) string {
	role = strings.TrimSpace(role)
	switch role {
	case ItemAssetInstall, ItemAssetService, ItemAssetRef:
		return role
	}
	switch NormalizeItemKind(kind) {
	case ItemKindInstall:
		return ItemAssetInstall
	case ItemKindAS:
		return ItemAssetService
	default:
		return ItemAssetRef
	}
}

func ItemStatusFromAS(asStatus string) string {
	switch strings.TrimSpace(asStatus) {
	case "in_progress", "hold", "transfer":
		return ItemDoing
	case "complete":
		return ItemDone
	case "cancelled":
		return ItemNA
	default:
		return ItemTodo
	}
}

func FormatItemKindCounts(c ItemKindCounts) ItemKindCounts {
	var parts []string
	if c.Install > 0 {
		parts = append(parts, fmt.Sprintf("장착 %d건", c.Install))
	}
	if c.Work > 0 {
		parts = append(parts, fmt.Sprintf("공사 %d건", c.Work))
	}
	if c.AS > 0 {
		parts = append(parts, fmt.Sprintf("AS %d건", c.AS))
	}
	if c.Etc > 0 {
		parts = append(parts, fmt.Sprintf("기타 %d건", c.Etc))
	}
	c.Line = strings.Join(parts, " · ")
	if c.Install > 0 && c.InstallQty > 0 {
		c.LineQty = fmt.Sprintf("장착 %d건(%d대)", c.Install, c.InstallQty)
		rest := []string{}
		if c.Work > 0 {
			rest = append(rest, fmt.Sprintf("공사 %d건", c.Work))
		}
		if c.AS > 0 {
			rest = append(rest, fmt.Sprintf("AS %d건", c.AS))
		}
		if c.Etc > 0 {
			rest = append(rest, fmt.Sprintf("기타 %d건", c.Etc))
		}
		if len(rest) > 0 {
			c.LineQty += " · " + strings.Join(rest, " · ")
		}
	} else {
		c.LineQty = c.Line
	}
	if len(c.UnitQty) > 0 {
		keys := make([]string, 0, len(c.UnitQty))
		for u := range c.UnitQty {
			keys = append(keys, u)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, u := range keys {
			parts = append(parts, fmt.Sprintf("%d %s", c.UnitQty[u], u))
		}
		extra := strings.Join(parts, " · ")
		if strings.TrimSpace(c.LineQty) == "" {
			c.LineQty = extra
		} else {
			c.LineQty += " · " + extra
		}
	}
	if c.Total > 0 {
		c.Progress = fmt.Sprintf("항목 완료 %d/%d", c.Done, c.Total)
	}
	return c
}

func SummarizeItemKinds(items []WorkTaskItem) ItemKindCounts {
	var c ItemKindCounts
	for _, it := range items {
		c.Total++
		st := NormalizeItemStatus(it.Status)
		if st == ItemDone || st == ItemNA {
			c.Done++
		}
		q := it.Qty
		if q < 1 {
			q = 1
		}
		unit := strings.TrimSpace(it.Unit)
		switch NormalizeItemKind(it.Kind) {
		case ItemKindInstall:
			c.Install++
			if unit == "" || unit == "대" || unit == ItemUnitDefault {
				c.InstallQty += q
			} else {
				if c.UnitQty == nil {
					c.UnitQty = map[string]int{}
				}
				c.UnitQty[unit] += q
			}
		case ItemKindWork:
			c.Work++
			if unit != "" && unit != ItemUnitDefault {
				if c.UnitQty == nil {
					c.UnitQty = map[string]int{}
				}
				c.UnitQty[unit] += q
			}
		case ItemKindAS:
			c.AS++
		default:
			c.Etc++
		}
	}
	return FormatItemKindCounts(c)
}

func NormalizeItemStatus(s string) string {
	switch strings.TrimSpace(s) {
	case ItemDoing, ItemDone, ItemHold, ItemNA:
		return strings.TrimSpace(s)
	default:
		return ItemTodo
	}
}

func ItemStatusLabel(s string) string {
	switch NormalizeItemStatus(s) {
	case ItemDoing:
		return "진행"
	case ItemDone:
		return "완료"
	case ItemHold:
		return "보류"
	case ItemNA:
		return "해당없음"
	default:
		return "할 일"
	}
}

func ItemIncomplete(s string) bool {
	st := NormalizeItemStatus(s)
	return st == ItemTodo || st == ItemDoing || st == ItemHold
}

func ClampTravelMin(n int) int {
	if n < 0 {
		return 0
	}
	if n > TravelMinMax {
		return TravelMinMax
	}
	return n
}

func SnapDown15(min int) int {
	if min < 0 {
		return 0
	}
	return (min / 15) * 15
}

func SnapUp15(min int) int {
	if min < 0 {
		return 0
	}
	r := min % 15
	if r == 0 {
		return min
	}
	return min + (15 - r)
}

type SplitPlanResult struct {
	FrontStart string
	FrontEnd   string
	BackStart  string
	BackEnd    string
	HasFront   bool
	HasBack    bool
	TravelMin  int
}

func SplitPlanAround(oldStart, oldEnd, newStart, newEnd string, travel int) (SplitPlanResult, error) {
	var out SplitPlanResult
	travel = ClampTravelMin(travel)
	out.TravelMin = travel
	os, oe := ParseHHMMMinutes(oldStart), ParseHHMMMinutes(oldEnd)
	ns, ne := ParseHHMMMinutes(newStart), ParseHHMMMinutes(newEnd)
	if os < 0 || oe <= os || ns < 0 || ne <= ns {
		return out, fmt.Errorf("시간을 확인하세요")
	}
	frontEnd := SnapDown15(ns - travel)
	if frontEnd > WorkdayEndMin {
		frontEnd = WorkdayEndMin
	}
	if frontEnd < WorkdayStartMin {
		frontEnd = WorkdayStartMin
	}
	backStart := SnapUp15(ne + travel)
	if backStart < WorkdayStartMin {
		backStart = WorkdayStartMin
	}
	if backStart > WorkdayEndMin {
		backStart = WorkdayEndMin
	}
	if frontEnd-os >= MinSegmentMin {
		out.HasFront = true
		out.FrontStart = FormatHHMMMinutes(os)
		out.FrontEnd = FormatHHMMMinutes(frontEnd)
	}
	if oe-backStart >= MinSegmentMin {
		out.HasBack = true
		out.BackStart = FormatHHMMMinutes(backStart)
		out.BackEnd = FormatHHMMMinutes(oe)
	}
	if !out.HasFront && !out.HasBack {
		return out, fmt.Errorf("남는 구간이 15분 미만이라 끼워 넣을 수 없습니다")
	}
	return out, nil
}

func VirtualPlanSegment(t WorkTask) WorkTaskSegment {
	st := strings.TrimSpace(t.StartTime)
	en := strings.TrimSpace(t.EndTime)
	if st != "" && len(st) >= 5 {
		st = st[:5]
	}
	if en != "" && len(en) >= 5 {
		en = en[:5]
	}
	dur := t.DurationMin
	if dur <= 0 {
		dur = DurationFromTimes(st, en)
	}
	return WorkTaskSegment{
		SegmentID:   "virtual:" + t.TaskID,
		TaskID:      t.TaskID,
		Kind:        SegKindPlan,
		Scope:       SegScopeCommon,
		Seq:         1,
		WorkDate:    firstNonEmptyStr(t.WorkDate, t.DueDate),
		StartTime:   st,
		EndTime:     en,
		DurationMin: dur,
		Virtual:     true,
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func SourceLinkedASOrMnt(t WorkTask) bool {
	src := strings.TrimSpace(t.SourceType)
	return (src == WBSourceAS || src == WBSourceMaintenance) && strings.TrimSpace(t.SourceID) != ""
}

func ClockHHMM(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 5 {
		return s[:5]
	}
	return s
}

func EnvelopeFromPlanSegments(segs []WorkTaskSegment) (start, end string, dur int) {
	var first, last string
	for _, s := range segs {
		k := NormalizeSegmentKind(s.Kind)
		if k != SegKindPlan && k != SegKindActual && strings.TrimSpace(s.Kind) != "" {
			continue
		}
		st, en := strings.TrimSpace(s.StartTime), strings.TrimSpace(s.EndTime)
		if st == "" {
			continue
		}
		if first == "" || st < first {
			first = st
		}
		if en != "" && en > last {
			last = en
		}
	}
	return first, last, DurationFromTimes(first, last)
}

func RangesOverlapHHMM(aStart, aEnd, bStart, bEnd string) bool {
	as, ae := ParseHHMMMinutes(aStart), ParseHHMMMinutes(aEnd)
	bs, be := ParseHHMMMinutes(bStart), ParseHHMMMinutes(bEnd)
	if as < 0 || ae <= as || bs < 0 || be <= bs {
		return false
	}
	return as < be && bs < ae
}
