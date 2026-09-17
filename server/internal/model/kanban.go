package model

import (
	"sort"
	"strings"
	"time"
)

const (
	KanbanLayoutASList        = "as_list"       // §12.8·§35.1 대기·진행중·검토·완료
	MeetingKanbanPrevComplete = "prev_complete" // §35.4 전일 완료
)

func UnplannedKanbanColumnDefs() []KanbanColumnDef {
	return []KanbanColumnDef{
		{Key: UnplannedNoDate, Title: "예정없음", Border: "border-red-200"},
		{Key: UnplannedDelayed, Title: "예정일경과", Border: "border-rose-200"},
		{Key: UnplannedNext, Title: "다음일정미정", Border: "border-amber-200"},
		{Key: UnplannedUnassigned, Title: "담당자미배정", Border: "border-orange-200"},
		{Key: UnplannedReview, Title: "미정사유만료", Border: "border-yellow-200"},
	}
}

func AssetKanbanColumnDefs() []KanbanColumnDef {
	return []KanbanColumnDef{
		{Key: AssetOpOperating, Title: "운영중", Border: "border-emerald-200"},
		{Key: AssetOpMaintenance, Title: "점검중", Border: "border-sky-200"},
		{Key: AssetOpFault, Title: "장애", Border: "border-red-200"},
		{Key: AssetOpRetired, Title: "철수", Border: "border-slate-200"},
		{Key: AssetOpDisposed, Title: "폐기", Border: "border-gray-300"},
	}
}

// AssignUnplannedKanbanColumn §8.1 표 순. 영업 후속없음은 담당자미배정 열.
func AssignUnplannedKanbanColumn(it UnplannedItem) string {
	if it.HasKind(UnplannedNoDate) {
		return UnplannedNoDate
	}
	if it.HasKind(UnplannedDelayed) {
		return UnplannedDelayed
	}
	if it.HasKind(UnplannedNext) {
		return UnplannedNext
	}
	if it.HasKind(UnplannedUnassigned) || it.HasKind(UnplannedSalesFollow) || it.HasKind(UnplannedSalesUnsigned) {
		return UnplannedUnassigned
	}
	if it.HasKind(UnplannedAssetSerial) {
		return UnplannedNoDate
	}
	if it.HasKind(UnplannedReview) {
		return UnplannedReview
	}
	return UnplannedUnassigned
}

// AssignAssetKanbanColumn 운영상태 코드. 비거나 모르는 값은 운영중(기본값).
func AssignAssetKanbanColumn(status string) string {
	switch strings.TrimSpace(status) {
	case AssetOpOperating, AssetOpMaintenance, AssetOpFault, AssetOpRetired, AssetOpDisposed:
		return strings.TrimSpace(status)
	default:
		return AssetOpOperating
	}
}

func FillUnplannedKanban(items []UnplannedItem) KanbanView {
	cols := emptyKanbanColumns(UnplannedKanbanColumnDefs())
	idx := indexKanbanColumns(cols)
	seen := map[string]bool{}
	for i := range items {
		it := items[i]
		k := strings.TrimSpace(it.ItemKey)
		if k == "" {
			k = WorkListItemKey(it.WorkListItem)
		}
		if k == "" || k == ":" || seen[k] {
			continue
		}
		b := AssignUnplannedKanbanColumn(it)
		j, ok := idx[b]
		if !ok {
			continue
		}
		seen[k] = true
		card := KanbanCardFromUnplanned(it)
		card.Bucket = b
		cols[j].Items = append(cols[j].Items, card)
	}
	return finishKanbanView(cols)
}

func FillAssetKanban(items []Asset) KanbanView {
	cols := emptyKanbanColumns(AssetKanbanColumnDefs())
	idx := indexKanbanColumns(cols)
	seen := map[string]bool{}
	for i := range items {
		a := items[i]
		id := strings.TrimSpace(a.AssetID)
		if id == "" || seen[id] {
			continue
		}
		b := AssignAssetKanbanColumn(a.OperationStatus)
		j, ok := idx[b]
		if !ok {
			continue
		}
		seen[id] = true
		card := KanbanCardFromAsset(a)
		card.Bucket = b
		cols[j].Items = append(cols[j].Items, card)
	}
	return finishKanbanView(cols)
}

func MeetingKanbanColumnDefs() []KanbanColumnDef {
	return []KanbanColumnDef{
		{Key: MeetingKanbanPrevComplete, Title: "전일 완료", Border: "border-emerald-200"},
		{Key: WorkBucketCompletedToday, Title: "오늘 완료", Border: "border-teal-200"},
		{Key: WorkBucketInProgress, Title: "진행중", Border: "border-yellow-200"},
		{Key: WorkBucketToday, Title: "오늘 예정", Border: "border-sky-200"},
		{Key: WorkBucketUnplanned, Title: "미계획", Border: "border-rose-200"},
	}
}

// FillMeetingKanban §38.6. 산식은 호출 측이 채운다. 배타 순서만 적용한다.
// 전일 완료 → 오늘 완료 → 진행중 → 오늘 예정 → 미계획.
func FillMeetingKanban(prevComplete, todayComplete, inProgress, todaySched, unplanned []WorkListItem) KanbanView {
	cols := emptyKanbanColumns(MeetingKanbanColumnDefs())
	idx := indexKanbanColumns(cols)
	seen := map[string]bool{}
	add := func(items []WorkListItem, bucket string) {
		j, ok := idx[bucket]
		if !ok {
			return
		}
		for i := range items {
			it := items[i]
			k := WorkListItemKey(it)
			if k == ":" || seen[k] {
				continue
			}
			seen[k] = true
			it.Bucket = bucket
			if it.MappedStatus == "" {
				it.MappedStatus = MapASStatusToWB(it.Status)
			}
			cols[j].Items = append(cols[j].Items, KanbanCardFromWork(it))
		}
	}
	add(prevComplete, MeetingKanbanPrevComplete)
	add(todayComplete, WorkBucketCompletedToday)
	add(inProgress, WorkBucketInProgress)
	add(todaySched, WorkBucketToday)
	add(unplanned, WorkBucketUnplanned)
	return finishKanbanView(cols)
}

// MeetingKanbanScheduledSum 오늘 예정 + 오늘 완료. §38.6 요약 「예정」과 같아야 한다.
func MeetingKanbanScheduledSum(view KanbanView) int {
	n := 0
	for _, c := range view.Columns {
		if c.Key == WorkBucketToday || c.Key == WorkBucketCompletedToday {
			n += c.Count
		}
	}
	return n
}

// KanbanColumnDef 화면이 넘기는 열 정의. 판정은 AssignKanbanColumn.
type KanbanColumnDef struct {
	Key    string
	Title  string
	Border string
}

// KanbanColumn 공통 칸반 한 열. 빈 열도 Count=0 으로 유지한다. §35.3
type KanbanColumn struct {
	Key       string
	Title     string
	Border    string
	Count     int
	CountUnit string
	Subtitle  string
	Items     []KanbanCard
	Sort      int
	IsLost    bool
}

// KanbanCard 공통 카드. 화면별 추가 필드는 비워 둔다.
type KanbanCard struct {
	ID                string
	Kind              string
	ItemKey           string
	RefID             string
	RefNumber         string
	Title             string
	Href              string
	ActionHref        string
	EditHref          string
	OrgName           string
	Assignee          string
	Prefix            string
	PrefixLabel       string
	WorkType          string
	TypeLabel         string
	MappedStatus      string
	Bucket            string
	LeftStyle         string
	SortDate          string
	DueDate           string
	DelayBadge        string
	DelayClass        string
	Urgent            bool
	UrgentNote        string
	Grouped           bool
	DaysOverdue       int
	Symptom           string
	Extra             string
	Tentative         bool
	Stage             string
	AmountUnconfirmed bool
	LastActivity      string
	AmountDesc        int
	EditLocked        bool
	AssigneeOther     bool
}

// KanbanView 화면이 열 정의와 카드를 넘기면 배타 배정·건수·정렬을 채운다. §35.1
type KanbanView struct {
	Columns []KanbanColumn
	Total   int
}

// ParseDisplay §35.3 ?display=list|kanban. view= 는 예전 링크 호환.
func ParseDisplay(display, view string) string {
	for _, v := range []string{display, view} {
		switch strings.TrimSpace(v) {
		case "kanban":
			return "kanban"
		case "list":
			return "list"
		}
	}
	return "list"
}

func ExecKanbanColumnDefs() []KanbanColumnDef {
	return []KanbanColumnDef{
		{Key: WBTaskWaiting, Title: "진행 예정", Border: "border-slate-200"},
		{Key: WBTaskInProgress, Title: "진행중", Border: "border-yellow-200"},
		{Key: WBTaskComplete, Title: "완료", Border: "border-emerald-200"},
	}
}

func AdminKanbanColumnDefs() []KanbanColumnDef {
	return []KanbanColumnDef{
		{Key: WBTaskWaiting, Title: "할 일", Border: "border-slate-200"},
		{Key: WBTaskInProgress, Title: "진행중", Border: "border-yellow-200"},
		{Key: WBTaskComplete, Title: "완료", Border: "border-emerald-200"},
	}
}

func ASListKanbanColumnDefs() []KanbanColumnDef {
	return []KanbanColumnDef{
		{Key: WBTaskWaiting, Title: "대기", Border: "border-slate-200"},
		{Key: WBTaskInProgress, Title: "진행중", Border: "border-amber-200"},
		{Key: WBTaskReview, Title: "검토", Border: "border-orange-200"},
		{Key: WBTaskComplete, Title: "완료", Border: "border-emerald-200"},
	}
}

func PlanKanbanColumnDefs(selectedDate, today string) []KanbanColumnDef {
	return []KanbanColumnDef{
		{Key: WorkBucketUnplanned, Title: PlanKanbanColumnTitle(WorkBucketUnplanned, selectedDate, today), Border: "border-slate-200"},
		{Key: WorkBucketToday, Title: PlanKanbanColumnTitle(WorkBucketToday, selectedDate, today), Border: "border-sky-200"},
		{Key: WorkBucketInProgress, Title: PlanKanbanColumnTitle(WorkBucketInProgress, selectedDate, today), Border: "border-yellow-200"},
		{Key: WorkBucketDelayed, Title: PlanKanbanColumnTitle(WorkBucketDelayed, selectedDate, today), Border: "border-red-200"},
		{Key: WorkBucketCompletedToday, Title: PlanKanbanColumnTitle(WorkBucketCompletedToday, selectedDate, today), Border: "border-emerald-200"},
	}
}

// AssignASListColumn §33.9.1 매핑 후 4열 묶음. 검토만 별도 열, 보류·이관·회신대기는 진행중.
func AssignASListColumn(mapped string) string {
	if mapped == "" {
		return ""
	}
	if mapped == WBTaskReview {
		return WBTaskReview
	}
	return WBKanbanBucket(mapped)
}

func emptyKanbanColumns(defs []KanbanColumnDef) []KanbanColumn {
	cols := make([]KanbanColumn, len(defs))
	for i, d := range defs {
		cols[i] = KanbanColumn{
			Key: d.Key, Title: d.Title, Border: d.Border,
			Items: []KanbanCard{},
		}
	}
	return cols
}

func indexKanbanColumns(cols []KanbanColumn) map[string]int {
	m := make(map[string]int, len(cols))
	for i := range cols {
		m[cols[i].Key] = i
	}
	return m
}

func finishKanbanView(cols []KanbanColumn) KanbanView {
	total := 0
	for i := range cols {
		SortKanbanCards(cols[i].Items)
		cols[i].Count = len(cols[i].Items)
		if cols[i].Items == nil {
			cols[i].Items = []KanbanCard{}
		}
		total += cols[i].Count
	}
	return KanbanView{Columns: cols, Total: total}
}

// FillWorkKanban 이미 Bucket 이 채워진 업무를 배타 배정한다. 한 키는 한 열.
func FillWorkKanban(defs []KanbanColumnDef, items []WorkListItem) KanbanView {
	cols := emptyKanbanColumns(defs)
	idx := indexKanbanColumns(cols)
	seen := map[string]bool{}
	for i := range items {
		it := items[i]
		k := WorkListItemKey(it)
		if k == ":" || seen[k] {
			continue
		}
		seen[k] = true
		j, ok := idx[it.Bucket]
		if !ok || it.Bucket == "" {
			continue
		}
		cols[j].Items = append(cols[j].Items, KanbanCardFromWork(it))
	}
	return finishKanbanView(cols)
}

func FillAdminKanban(defs []KanbanColumnDef, items []WorkTask) KanbanView {
	cols := emptyKanbanColumns(defs)
	idx := indexKanbanColumns(cols)
	seen := map[string]bool{}
	for i := range items {
		t := items[i]
		id := strings.TrimSpace(t.TaskID)
		if id == "" || seen[id] {
			continue
		}
		b := WBKanbanBucket(t.Status)
		j, ok := idx[b]
		if !ok || b == "" {
			continue
		}
		seen[id] = true
		cols[j].Items = append(cols[j].Items, KanbanCardFromAdminTask(t))
	}
	return finishKanbanView(cols)
}

func FillASListKanban(defs []KanbanColumnDef, items []ASListItem) KanbanView {
	cols := emptyKanbanColumns(defs)
	idx := indexKanbanColumns(cols)
	seen := map[string]bool{}
	for i := range items {
		it := items[i]
		id := strings.TrimSpace(it.ASID)
		if id == "" || seen[id] {
			continue
		}
		mapped := MapASStatusToWB(it.Status)
		b := AssignASListColumn(mapped)
		j, ok := idx[b]
		if !ok || b == "" {
			continue
		}
		seen[id] = true
		card := KanbanCardFromAS(it)
		card.MappedStatus = mapped
		card.Bucket = b
		cols[j].Items = append(cols[j].Items, card)
	}
	return finishKanbanView(cols)
}

func KanbanCardFromWork(it WorkListItem) KanbanCard {
	days := it.DaysOverdue
	if days <= 0 {
		if sched := WorkItemScheduledDate(it); sched != "" {
			today := time.Now().Format("2006-01-02")
			if sched < today {
				days = DaysBetweenDates(sched, today)
				if days < 1 {
					days = 1
				}
			}
		}
	}
	inProg := it.Bucket == WorkBucketInProgress || it.Bucket == WBTaskInProgress
	delay, delayClass := PlanDelayBadge(days, inProg)
	if it.IsWaiting() {
		delay, delayClass = WaitingBadge(it.BlockedReason)
	}
	return KanbanCard{
		ID:            WorkListItemKey(it),
		RefID:         it.RefID,
		RefNumber:     it.RefNumber,
		Title:         it.Title,
		Href:          it.Href,
		OrgName:       it.OrgName,
		Assignee:      it.Assignee,
		Prefix:        it.Prefix,
		PrefixLabel:   WorkPrefixLabel(it.Prefix),
		MappedStatus:  it.MappedStatus,
		Bucket:        it.Bucket,
		LeftStyle:     WorkCardColorStyle(it.Assignee, it.ProductType, it.Prefix),
		SortDate:      WorkItemScheduledDate(it),
		DueDate:       firstNonEmpty(it.DueDate, it.ScheduledDate),
		DelayBadge:    delay,
		DelayClass:    delayClass,
		Urgent:        WorkItemUrgent(it.Urgency),
		Grouped:       strings.TrimSpace(it.ReceiptGroupID) != "",
		DaysOverdue:   days,
		Tentative:     it.Tentative,
		EditLocked:    it.EditLocked,
		AssigneeOther: it.AssigneeOther,
	}
}

func KanbanCardFromAdminTask(t WorkTask) KanbanCard {
	return KanbanCard{
		ID:           t.TaskID,
		Kind:         "task",
		ItemKey:      t.TaskID,
		RefID:        t.TaskID,
		RefNumber:    t.TaskID,
		Title:        t.Title,
		Href:         "/admin-work/" + t.TaskID,
		OrgName:      t.CustomerLabel(),
		Assignee:     t.Assignee,
		WorkType:     t.WorkType,
		TypeLabel:    WBWorkTypeLabel(t.WorkType),
		MappedStatus: t.Status,
		Bucket:       WBKanbanBucket(t.Status),
		LeftStyle:    WorkCardColorStyle(t.Assignee, "", t.WorkType),
		SortDate:     firstNonEmpty(t.DueDate, t.WorkDate),
		DueDate:      t.DueDate,
	}
}

func KanbanCardFromUnplanned(it UnplannedItem) KanbanCard {
	c := KanbanCardFromWork(it.WorkListItem)
	if strings.TrimSpace(it.ItemKey) != "" {
		c.ID = it.ItemKey
		c.ItemKey = it.ItemKey
	}
	if it.Href != "" {
		c.Href = it.Href
	}
	c.MappedStatus = ""
	if len(it.Badges) > 0 {
		c.DelayBadge = it.Badges[0].Label
		c.DelayClass = it.Badges[0].Class
	}
	c.Extra = it.NoDateReason
	return c
}

func KanbanCardFromAsset(a Asset) KanbanCard {
	return KanbanCard{
		ID:        a.AssetID,
		RefID:     a.AssetID,
		RefNumber: a.AssetID,
		Title:     a.ProductName,
		Href:      "/assets/" + a.AssetID,
		EditHref:  "/assets/" + a.AssetID + "/edit",
		OrgName:   a.OrgName,
		LeftStyle: WorkCardColorStyle("", a.ProductType, ""),
		SortDate:  a.InstallDate,
		DueDate:   a.InstallDate,
		Extra:     a.SerialNumber,
		Bucket:    AssignAssetKanbanColumn(a.OperationStatus),
	}
}

func KanbanCardFromAS(it ASListItem) KanbanCard {
	return KanbanCard{
		ID:          it.ASID,
		RefID:       it.ASID,
		RefNumber:   it.ASNumber,
		Title:       it.OrgName,
		Href:        "/as/" + it.ASID,
		OrgName:     it.OrgName,
		Assignee:    it.AssignedTo,
		Prefix:      WorkPrefixAS,
		PrefixLabel: WorkPrefixLabel(WorkPrefixAS),
		LeftStyle:   WorkCardColorStyle(it.AssignedTo, it.ProductName, WorkPrefixAS),
		SortDate:    it.VisitScheduledDate,
		Urgent:      WorkItemUrgent(it.Urgency) || it.Urgency == "high",
		Grouped:     it.GroupSize >= 2,
		DaysOverdue: it.VisitDaysOverdue,
		Symptom:     it.Symptom,
	}
}

func SortKanbanCards(items []KanbanCard) {
	sort.SliceStable(items, func(i, j int) bool {
		di, dj := items[i].SortDate, items[j].SortDate
		if di == "" {
			di = "9999-99-99"
		}
		if dj == "" {
			dj = "9999-99-99"
		}
		if di != dj {
			return di < dj
		}
		if items[i].AmountDesc != items[j].AmountDesc {
			return items[i].AmountDesc > items[j].AmountDesc
		}
		ni, nj := items[i].RefNumber, items[j].RefNumber
		if ni == "" {
			ni = items[i].ID
		}
		if nj == "" {
			nj = items[j].ID
		}
		return ni < nj
	})
}
