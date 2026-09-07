package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// WorkPrefix 업무 리스트 접두어
const (
	WorkPrefixAS           = "as"           // [AS]
	WorkPrefixMaintenance  = "maintenance"  // [정기점검]
	WorkPrefixConfirm      = "confirm"      // [확인]
	WorkPrefixGeneral      = "general"      // [일반업무]
	WorkPrefixSales        = "sales"        // [영업]
)

func WorkPrefixLabel(p string) string {
	switch p {
	case WorkPrefixAS:
		return "AS"
	case WorkPrefixMaintenance:
		return "정기점검"
	case WorkPrefixConfirm:
		return "확인"
	case WorkPrefixGeneral:
		return "일반업무"
	case WorkPrefixSales:
		return "영업"
	default:
		return p
	}
}

// WorkPrefixCounts 접두어별 건수
type WorkPrefixCounts struct {
	AS          int `json:"as"`
	Maintenance int `json:"maintenance"`
	Confirm     int `json:"confirm"`
	General     int `json:"general"`
	Total       int `json:"total"`
}

func (c *WorkPrefixCounts) Add(prefix string, n int) {
	switch prefix {
	case WorkPrefixAS:
		c.AS += n
	case WorkPrefixMaintenance:
		c.Maintenance += n
	case WorkPrefixConfirm:
		c.Confirm += n
	case WorkPrefixGeneral:
		c.General += n
	}
	c.Total += n
}

// WorkDashStats 담당자/관리자 홈 대시보드 지표
type WorkDashStats struct {
	OpenTotal            int                   `json:"open_total"`
	OpenByAssignee       []AssigneePrefixStats `json:"open_by_assignee"`
	TodayScheduled       WorkPrefixCounts      `json:"today_scheduled"`
	TodayByAssignee      []AssigneePrefixStats `json:"today_by_assignee"`
	Delayed              int                   `json:"delayed"`
	DelayedByAssignee    []AssigneePrefixStats `json:"delayed_by_assignee"`
	TodayCompleted       WorkPrefixCounts      `json:"today_completed"`
	CompletedByAssignee  []AssigneePrefixStats `json:"completed_by_assignee"`
	SchedulePending      int                   `json:"schedule_pending"`
	PendingByAssignee    []AssigneePrefixStats `json:"pending_by_assignee"`
	Unassigned           int                   `json:"unassigned"`
	UnassignedPrefixLine string                `json:"unassigned_prefix_line"` // 미배정은 접두어만: "[AS] 2 / [정기점검] 1"
	Unplanned            int                   `json:"unplanned"`              // §8 미계획(유형 합산, 건 중복 제거)
	UpcomingOcc          int                   `json:"upcoming_occ"`           // §13.15.12 다가오는 실행 (당일 제외 N일)
	OverdueDeadline      int                   `json:"overdue_deadline"`       // §13.15.12 상위 마감 경과
}

// AssigneePrefixStats 담당자별 접두어 집계 (대시보드 카드용)
type AssigneePrefixStats struct {
	Name   string           `json:"name"`
	Counts WorkPrefixCounts `json:"counts"`
	Line   string           `json:"line"` // 표시용: "양기현: [AS] 3 / [정기점검] 1"
}

// WorkListItem 통합 업무 리스트 1행
type WorkListItem struct {
	Prefix         string `json:"prefix"`
	RefID          string `json:"ref_id"`
	RefNumber      string `json:"ref_number"`
	Title          string `json:"title"`
	OrgName        string `json:"org_name"`
	Assignee       string `json:"assignee"`
	ScheduledDate  string `json:"scheduled_date"`
	WorkDate       string `json:"work_date,omitempty"`
	DueDate        string `json:"due_date,omitempty"`
	CompleteDate   string `json:"complete_date,omitempty"`
	Status         string `json:"status"`
	StatusLabel    string `json:"status_label"`
	MappedStatus   string `json:"mapped_status,omitempty"` // MapASStatusToWB 결과
	Bucket         string `json:"bucket,omitempty"`        // 3열 또는 5열 배정 결과
	Href           string `json:"href"`
	DaysOverdue    int    `json:"days_overdue"`
	SubLabel       string `json:"sub_label"` // 재방문 등 보조 라벨
	NeedPlan       bool   `json:"need_plan,omitempty"`
	Urgency        string `json:"urgency,omitempty"`
	ReceiptGroupID string `json:"receipt_group_id,omitempty"`
	ProductType    string `json:"product_type,omitempty"`
}

const (
	KanbanLayoutExec = "exec" // §33.9 오늘 내 업무 3열
	KanbanLayoutPlan = "plan" // §7.8 일일 업무 등록 5열
)

// ApplyKanbanMapping 원본 상태를 §33.5.2·§33.9.1 3열로 맞춘다. 취소 등은 false.
func (it *WorkListItem) ApplyKanbanMapping() bool {
	return it.applyKanbanMapping(KanbanLayoutExec, "")
}

// ApplyPlanKanbanMapping §7.8 5열 배타 배정. 판정은 ApplyKanbanMapping 과 같은 매핑을 쓴다.
func (it *WorkListItem) ApplyPlanKanbanMapping(selectedDate string) bool {
	return it.applyKanbanMapping(KanbanLayoutPlan, selectedDate)
}

func (it *WorkListItem) applyKanbanMapping(layout, selectedDate string) bool {
	if it == nil {
		return false
	}
	it.MappedStatus = MapASStatusToWB(it.Status)
	if it.MappedStatus == "" {
		it.Bucket = ""
		return false
	}
	it.Bucket = AssignKanbanColumn(*it, selectedDate, layout)
	if layout == KanbanLayoutPlan && it.Bucket == WorkBucketInProgress {
		if sched := WorkItemScheduledDate(*it); sched != "" && selectedDate != "" && sched < selectedDate {
			it.DaysOverdue = DaysBetweenDates(sched, selectedDate)
		}
	}
	return it.Bucket != ""
}

// AssignKanbanColumn §33.9.1 매핑을 공유해 3열·5열 묶음을 낸다. 열 구성만 다르다.
func AssignKanbanColumn(it WorkListItem, selectedDate, layout string) string {
	mapped := it.MappedStatus
	if mapped == "" {
		mapped = MapASStatusToWB(it.Status)
	}
	if mapped == "" {
		return ""
	}
	if layout == KanbanLayoutASList {
		return AssignASListColumn(mapped)
	}
	if layout != KanbanLayoutPlan {
		return WBKanbanBucket(mapped)
	}
	return assignPlanKanbanColumn(it, mapped, selectedDate)
}

func assignPlanKanbanColumn(it WorkListItem, mapped, selectedDate string) string {
	selectedDate = NormalizeAppDate(selectedDate)
	sched := WorkItemScheduledDate(it)
	completeOn := NormalizeAppDate(it.CompleteDate)

	// 1 오늘 완료
	if mapped == WBTaskComplete {
		if completeOn != "" && completeOn == selectedDate {
			return WorkBucketCompletedToday
		}
		return ""
	}
	// 2 미계획업무 — §8.2.1 계획을 세워야 할 건 / 예정일 없음·담당자 미배정
	if it.NeedPlan || sched == "" || strings.TrimSpace(it.Assignee) == "" {
		return WorkBucketUnplanned
	}
	// 3 진행중 — 3열과 같은 WBKanbanBucket 판정
	if WBKanbanBucket(mapped) == WBTaskInProgress {
		return WorkBucketInProgress
	}
	// 4 지연
	if sched != "" && selectedDate != "" && sched < selectedDate {
		return WorkBucketDelayed
	}
	// 5 오늘 예정
	if sched != "" && sched == selectedDate {
		return WorkBucketToday
	}
	// 예정일 > 선택일
	return ""
}

func WorkItemScheduledDate(it WorkListItem) string {
	for _, s := range []string{it.ScheduledDate, it.WorkDate, it.DueDate} {
		if d := NormalizeAppDate(s); d != "" {
			return d
		}
	}
	return ""
}

func WorkListItemKey(it WorkListItem) string {
	if it.RefID != "" {
		return it.Prefix + ":" + it.RefID
	}
	return it.Prefix + ":" + it.RefNumber
}

func PlanKanbanColumnTitle(col, selectedDate, today string) string {
	selectedDate = NormalizeAppDate(selectedDate)
	today = NormalizeAppDate(today)
	md := FormatMonthDay(selectedDate)
	isToday := selectedDate != "" && selectedDate == today
	switch col {
	case WorkBucketUnplanned:
		return "미계획업무"
	case WorkBucketToday:
		if isToday {
			return "오늘 예정"
		}
		return md + " 예정"
	case WorkBucketInProgress:
		return "진행중"
	case WorkBucketDelayed:
		return "지연"
	case WorkBucketCompletedToday:
		if isToday {
			return "오늘 완료"
		}
		return md + " 완료"
	default:
		return col
	}
}

func FormatMonthDay(date string) string {
	t, err := time.ParseInLocation("2006-01-02", NormalizeAppDate(date), time.Local)
	if err != nil {
		return date
	}
	return fmt.Sprintf("%d/%d", int(t.Month()), t.Day())
}

func DaysBetweenDates(from, to string) int {
	a, err1 := time.ParseInLocation("2006-01-02", NormalizeAppDate(from), time.Local)
	b, err2 := time.ParseInLocation("2006-01-02", NormalizeAppDate(to), time.Local)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}

func PlanDelayBadge(daysOverdue int, inProgress bool) (label, class string) {
	if daysOverdue <= 0 {
		return "", ""
	}
	if inProgress {
		b := UnplannedBadgeOfStarted(UnplannedDelayed, daysOverdue, true)
		return b.Label, b.Class
	}
	return "D+" + strconv.Itoa(daysOverdue), "bg-red-100 text-red-800"
}

func WorkItemUrgent(urgency string) bool {
	switch strings.TrimSpace(urgency) {
	case "상", "urgent", "high", "긴급":
		return true
	default:
		return false
	}
}

// SortWorkListItems §33.5.3 과 같은 정렬. 기본 = 종료일 오름차순.
func SortWorkListItems(items []WorkListItem, sortKey, dir string) {
	sortKey, dir = NormalizeAdminWorkSort(sortKey, dir)
	desc := dir == "desc"
	val := func(it WorkListItem) string {
		switch sortKey {
		case "task_id":
			return strings.ToLower(it.RefNumber)
		case "title":
			return strings.ToLower(it.Title)
		case "customer":
			return strings.ToLower(it.OrgName)
		case "assignee":
			return strings.ToLower(it.Assignee)
		case "work_date":
			if it.WorkDate != "" {
				return it.WorkDate
			}
			return it.ScheduledDate
		case "status":
			return it.MappedStatus
		default: // due_date
			if it.DueDate != "" {
				return it.DueDate
			}
			if it.ScheduledDate != "" {
				return it.ScheduledDate
			}
			return "9999-99-99"
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := val(items[i]), val(items[j])
		if a != b {
			if desc {
				return a > b
			}
			return a < b
		}
		return items[i].RefNumber < items[j].RefNumber
	})
}

// WorkBucket 리스트 필터 버킷
const (
	WorkBucketOpen            = "open"
	WorkBucketToday           = "today"
	WorkBucketDelayed         = "delayed"
	WorkBucketCompletedToday  = "completed_today"
	WorkBucketInProgress      = "in_progress" // §7.8 칸반 열. ListBucket 재사용
	WorkBucketUnplanned       = "unplanned"   // §7.8 표시 열. 조회는 ListUnplanned
	WorkBucketSchedulePending = "schedule_pending"
	WorkBucketUnassigned      = "unassigned"
)
