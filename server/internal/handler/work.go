package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	workAssigneeNone = "__none__"
	workAllPageSize  = 100
)

type WorkHandler struct {
	repo         *repository.WorkBoardRepo
	asRepo       *repository.ASRepo
	mntRepo      *repository.MaintenanceRepo
	wbRepo       *repository.WBRepo
	userRepo     *repository.UserRepo
	customerRepo *repository.CustomerRepo
	notices      *assignNoticeHook
}

func NewWorkHandler(
	repo *repository.WorkBoardRepo,
	asRepo *repository.ASRepo,
	mntRepo *repository.MaintenanceRepo,
	wbRepo *repository.WBRepo,
	userRepo *repository.UserRepo,
	customerRepo *repository.CustomerRepo,
) *WorkHandler {
	return &WorkHandler{repo: repo, asRepo: asRepo, mntRepo: mntRepo, wbRepo: wbRepo, userRepo: userRepo, customerRepo: customerRepo}
}

func (h *WorkHandler) List(c echo.Context) error {
	role := currentRole(c)
	roleUnresolved := !model.IsKnownRole(role)
	uid := currentUserID(c)
	keys := assigneeKeys(c)

	view := model.ParseDisplay(c.QueryParam("display"), c.QueryParam("view"))
	sortKey, dir := parseWorkTodaySort(c.QueryParam("sort"), c.QueryParam("dir"))

	var users []model.User
	if h.userRepo != nil {
		users, _ = h.userRepo.ListAssignable()
	}

	mineUID, mineKeys := uid, keys
	showPicker := !roleUnresolved && canPickWorkTodayAssignee(c)
	assigneeFilter := currentUserDisplayName(c)
	assigneeQ := ""
	viewingOther := false
	title := "오늘 내 업무"
	scopeOther := ""

	if roleUnresolved {
		mineUID, mineKeys = "", nil
		showPicker = false
		assigneeFilter = ""
	} else if showPicker {
		picked, other := resolveWorkTodayAssignee(c, users)
		if picked != nil {
			mineUID = strings.TrimSpace(picked.UserID)
			mineKeys = repository.MergeAssigneeKeys(picked.FullName, picked.UserID, picked.Username)
			assigneeFilter = strings.TrimSpace(picked.FullName)
			if assigneeFilter == "" {
				assigneeFilter = strings.TrimSpace(picked.Username)
			}
		}
		viewingOther = other
		if viewingOther {
			title = "오늘 · " + assigneeFilter + " 업무"
			scopeOther = assigneeFilter
			assigneeQ = assigneeFilter
		} else if raw := strings.TrimSpace(c.QueryParam("assignee")); raw != "" && !isWorkTodayForbiddenAll(raw) {
			assigneeQ = assigneeFilter
		}
	} else {
		assigneeFilter = ""
	}

	today := time.Now().Format("2006-01-02")
	raw, err := h.repo.ListWorkToday(mineUID, mineKeys, today)
	if err != nil {
		return err
	}

	items := mapWorkListItems(raw)
	stampWorkListOverdue(items, today)
	annotateWorkEdit(c, items)
	if viewingOther {
		lockWorkTodayOtherView(items)
	}
	model.SortWorkListItems(items, sortKey, dir)
	kanban := model.FillWorkKanban(model.ExecKanbanColumnDefs(), items)
	nSched, nProg, nDelay, nDone := countWorkTodayBuckets(items, today)
	avgDelay := avgOverdueDays(items, today)
	reviewMode := strings.TrimSpace(c.QueryParam("review")) == "1"
	if reviewMode {
		items = filterWorkTodayReview(items, today)
		view = "list"
		title = "지연 업무 정리"
		kanban = model.FillWorkKanban(model.ExecKanbanColumnDefs(), items)
	}
	sortHrefs := workTodaySortHrefs(view, sortKey, dir, assigneeQ)

	filterQ := workTodayQuery(view, sortKey, dir, assigneeQ)
	delayWarn := workTodayDelayWarn(nDelay, avgDelay)
	reviewHref := "/work?review=1"
	if assigneeQ != "" {
		reviewHref += "&assignee=" + url.QueryEscape(assigneeQ)
	}
	return c.Render(http.StatusOK, "work/list.html", map[string]interface{}{
		"Title":              title,
		"Active":             NavWork,
		"View":               view,
		"Items":              items,
		"KanbanColumns":      kanban.Columns,
		"KanbanTotal":        kanban.Total,
		"KanbanDrag":         false,
		"KanbanDrop":         "",
		"KanbanHint":         "보류·이관·회신대기는 진행중 열에 뱃지로 구분합니다. 지연은 뱃지로 오늘 예정과 구분합니다.",
		"Total":              len(items),
		"Shown":              len(items),
		"Sort":               sortKey,
		"Dir":                dir,
		"SortHref":           sortHrefs,
		"FilterQ":            filterQ,
		"KanbanHref":         "/work?" + workTodayQuery("kanban", sortKey, dir, assigneeQ),
		"ListHref":           "/work?" + workTodayQuery("list", sortKey, dir, assigneeQ),
		"Role":               role,
		"RoleUnresolved":     roleUnresolved,
		"ScopeNote":          workTodayScopeNote(today, roleUnresolved, scopeOther, nSched, nProg, nDelay, nDone),
		"DelayWarn":          delayWarn,
		"ReviewHref":         reviewHref,
		"ReviewMode":         reviewMode,
		"TodayDate":          today,
		"TomorrowDate":       time.Now().AddDate(0, 0, 1).Format("2006-01-02"),
		"ShowAssignee":       viewingOther,
		"ShowAssigneePicker": showPicker && !reviewMode,
		"Users":              users,
		"AssigneeFilter":     assigneeFilter,
		"CanWrite":           canWriteWorkboard(c) && !viewingOther,
		"SortSelect":         sortSelectOptions(workListSortCols(viewingOther), sortHrefs, sortKey, dir),
	})
}

func (h *WorkHandler) ListAll(c echo.Context) error {
	role := currentRole(c)
	roleUnresolved := !model.IsKnownRole(role)
	view := model.ParseDisplay(c.QueryParam("display"), c.QueryParam("view"))
	sortKey, dir := parseWorkAllSort(c.QueryParam("sort"), c.QueryParam("dir"))
	assignees := queryCSV(c, "assignee")
	kinds := queryCSV(c, "kind")
	statuses := queryCSV(c, "status")
	customers := queryCSV(c, "customer")
	projects := queryCSV(c, "project")
	qtext := strings.TrimSpace(c.QueryParam("q"))
	today := time.Now()
	todayStr := today.Format("2006-01-02")
	from, to, period, rangeParam := parseWorkAllPeriod(c, today)
	skipRange := false
	for _, s := range statuses {
		if s == workStatusDelayed {
			skipRange = true
			break
		}
	}
	if !skipRange && len(assignees) == 1 && assignees[0] == workAssigneeNone {
		skipRange = true
	}

	raw, err := h.repo.ListUnified("", nil, true, from, to)
	if err != nil {
		return err
	}
	items := mapWorkListItems(raw)
	if !skipRange {
		items = filterWorkItemsInRange(items, from, to)
	}
	universe := len(items)
	items = filterWorkItemsAssignees(items, assignees)
	items = filterWorkItemsKinds(items, kinds)
	items = filterWorkItemsStatuses(items, statuses, todayStr)
	custNames := map[string]string{}
	var custList []model.Customer
	if h.customerRepo != nil {
		custList, _ = h.customerRepo.ListForReceipt()
		for _, cu := range custList {
			custNames[cu.CustomerID] = cu.OrgName
		}
	}
	items = filterWorkItemsCustomers(items, customers, custNames)
	items = filterWorkItemsProjects(items, projects)
	var asIDs map[string]bool
	if qtext != "" && h.asRepo != nil {
		asIDs = h.asRepo.MatchIDs(qtext)
	}
	items = filterWorkItemsKeyword(items, qtext, asIDs)
	annotateWorkEdit(c, items)
	model.SortWorkListItems(items, sortKey, dir)

	total := len(items)
	offset := parseWorkOffset(c.QueryParam("offset"))
	shown, hasMore := paginateWorkItems(items, offset, workAllPageSize)
	kanban := model.FillWorkKanban(model.ExecKanbanColumnDefs(), shown)

	var users []model.User
	if h.userRepo != nil {
		users, _ = h.userRepo.ListAssignable()
	}
	var projList []model.WorkProject
	if h.wbRepo != nil {
		projList, _ = h.wbRepo.ListProjects(true)
	}

	q := workAllQuery{
		View: view, Sort: sortKey, Dir: dir,
		Assignees: assignees, Kinds: kinds, Statuses: statuses,
		Customers: customers, Projects: projects, Q: qtext,
		Period: period, Range: rangeParam, From: from, To: to,
	}
	moreHref := ""
	if hasMore {
		moreQ := q
		moreQ.Offset = offset + len(shown)
		moreHref = "/work/all?" + moreQ.Encode()
	}
	assigneeFilter := ""
	if len(assignees) == 1 {
		assigneeFilter = assignees[0]
	}
	statusFilter := ""
	if len(statuses) == 1 {
		statusFilter = statuses[0]
	}
	return c.Render(http.StatusOK, "work/list.html", map[string]interface{}{
		"Title":           "전체 업무",
		"Active":          NavWorkAll,
		"AllWork":         true,
		"View":            view,
		"Items":           shown,
		"KanbanColumns":   kanban.Columns,
		"KanbanTotal":     kanban.Total,
		"KanbanDrag":      false,
		"KanbanDrop":      "",
		"KanbanHint":      "보류·이관·회신대기는 진행중 열에 뱃지로 구분합니다. 행정업무 칸반과 같은 분류입니다.",
		"Total":           total,
		"Universe":        universe,
		"Shown":           len(shown),
		"HasMore":         hasMore,
		"MoreHref":        moreHref,
		"From":            from,
		"To":              to,
		"Period":          period,
		"Range":           rangeParam,
		"Q":               qtext,
		"Sort":            sortKey,
		"Dir":             dir,
		"SortHref":        workAllSortHrefs(q),
		"FilterQ":         q.Encode(),
		"KanbanHref":      "/work/all?" + q.withView("kanban").Encode(),
		"ListHref":        "/work/all?" + q.withView("list").Encode(),
		"ResetHref":       workAllResetHref(view, sortKey, dir),
		"Role":            role,
		"RoleUnresolved":  roleUnresolved,
		"ScopeNote":       workAllScopeNote(from, to, assigneeFilter, statusFilter, roleUnresolved),
		"AssigneeFilter":  assigneeFilter,
		"StatusFilter":    statusFilter,
		"Assignees":       users,
		"Customers":       custList,
		"Projects":        projList,
		"FilterChips":     workFilterChips(q, users, custList, projList),
		"HasFilters":      workHasExtraFilters(q) || (period != "" && period != "3m"),
		"SelAssignee":     selectedSet(assignees),
		"SelKind":         selectedSet(kinds),
		"SelStatus":       selectedSet(statuses),
		"SelCustomer":     selectedSet(customers),
		"SelProject":      selectedSet(projects),
		"CustomersJSON":   jsonJS(workCustomerOpts(custList)),
		"SelCustomerJSON": jsonJS(customers),
		"ShowAssignee":    true,
		"CanWrite":        canWriteWorkboard(c),
		"SortSelect":      sortSelectOptions(workListSortCols(true), workAllSortHrefs(q), sortKey, dir),
	})
}

func mapWorkListItems(raw []model.WorkListItem) []model.WorkListItem {
	var items []model.WorkListItem
	for _, it := range raw {
		if !it.ApplyKanbanMapping() {
			continue
		}
		items = append(items, it)
	}
	return items
}

// annotateWorkEdit §37.3 남의 담당자는 굵게, 읽기 전용은 자물쇠.
func annotateWorkEdit(c echo.Context, items []model.WorkListItem) {
	for i := range items {
		items[i].EditLocked = !canEditTask(c, items[i].Assignee, "")
		items[i].AssigneeOther = strings.TrimSpace(items[i].Assignee) != "" && !assigneeIsMine(c, items[i].Assignee, "")
	}
}

func parseWorkTodaySort(sort, dir string) (string, string) {
	if strings.TrimSpace(sort) == "" && strings.TrimSpace(dir) == "" {
		return "due_date", "asc"
	}
	if strings.TrimSpace(sort) == "prefix" {
		if strings.TrimSpace(dir) != "desc" {
			dir = "asc"
		}
		return "prefix", dir
	}
	return model.NormalizeAdminWorkSort(sort, dir)
}

func parseWorkAllSort(sort, dir string) (string, string) {
	if strings.TrimSpace(sort) == "" && strings.TrimSpace(dir) == "" {
		return "due_date", "desc"
	}
	if strings.TrimSpace(sort) == "prefix" {
		if strings.TrimSpace(dir) != "desc" {
			dir = "asc"
		}
		return "prefix", dir
	}
	return model.NormalizeAdminWorkSort(sort, dir)
}

func parseWorkOffset(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func paginateWorkItems(items []model.WorkListItem, offset, limit int) ([]model.WorkListItem, bool) {
	if limit <= 0 {
		return items, false
	}
	if offset > len(items) {
		offset = len(items)
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], end < len(items)
}

func filterWorkItemsInRange(items []model.WorkListItem, from, to string) []model.WorkListItem {
	if from == "" || to == "" {
		return items
	}
	var out []model.WorkListItem
	for _, it := range items {
		d := model.WorkItemScheduledDate(it)
		if d == "" {
			d = model.NormalizeAppDate(it.CompleteDate)
		}
		if d == "" || (d >= from && d <= to) {
			out = append(out, it)
		}
	}
	return out
}

func stampWorkListOverdue(items []model.WorkListItem, today string) {
	today = strings.TrimSpace(today)
	for i := range items {
		if items[i].DaysOverdue > 0 {
			continue
		}
		sched := model.WorkItemScheduledDate(items[i])
		if sched != "" && today != "" && sched < today {
			d := model.DaysBetweenDates(sched, today)
			if d < 1 {
				d = 1
			}
			items[i].DaysOverdue = d
		}
	}
}

func countWorkTodayBuckets(items []model.WorkListItem, today string) (sched, progress, delayed, done int) {
	today = strings.TrimSpace(today)
	for _, it := range items {
		if it.MappedStatus == model.WBTaskComplete || strings.TrimSpace(it.CompleteDate) == today {
			done++
			continue
		}
		d := model.WorkItemScheduledDate(it)
		overdue := d != "" && today != "" && d < today
		if overdue && !it.IsWaiting() {
			delayed++
			continue
		}
		if workItemInProgress(it) {
			progress++
			continue
		}
		if d == today {
			sched++
		}
	}
	return
}

func workItemInProgress(it model.WorkListItem) bool {
	if it.Bucket == model.WBTaskInProgress || it.Bucket == model.WorkBucketInProgress {
		return true
	}
	switch it.MappedStatus {
	case model.WBTaskInProgress, model.WBTaskHold, model.WBTaskTransfer, model.WBTaskReview, model.WBTaskWaitingFor:
		return true
	}
	return false
}

func avgOverdueDays(items []model.WorkListItem, today string) int {
	n, sum := 0, 0
	today = strings.TrimSpace(today)
	for _, it := range items {
		if it.MappedStatus == model.WBTaskComplete || strings.TrimSpace(it.CompleteDate) == today {
			continue
		}
		if it.IsWaiting() {
			continue
		}
		d := model.WorkItemScheduledDate(it)
		if d == "" || today == "" || d >= today {
			continue
		}
		days := it.DaysOverdue
		if days < 1 {
			days = model.DaysBetweenDates(d, today)
		}
		if days < 1 {
			days = 1
		}
		n++
		sum += days
	}
	if n == 0 {
		return 0
	}
	avg := int(float64(sum)/float64(n) + 0.5)
	if avg < 1 {
		avg = 1
	}
	return avg
}

func filterWorkTodayReview(items []model.WorkListItem, today string) []model.WorkListItem {
	today = strings.TrimSpace(today)
	var out []model.WorkListItem
	for _, it := range items {
		if it.MappedStatus == model.WBTaskComplete || strings.TrimSpace(it.CompleteDate) == today {
			continue
		}
		if it.IsWaiting() {
			continue
		}
		d := model.WorkItemScheduledDate(it)
		if d != "" && today != "" && d < today {
			out = append(out, it)
		}
	}
	return out
}

func workTodayDelayWarn(n, avg int) string {
	if n <= 0 {
		return ""
	}
	if avg < 1 {
		avg = 1
	}
	return strconv.Itoa(n) + "건이 평균 " + strconv.Itoa(avg) + "일 밀려 있습니다"
}

func workTodayScopeNote(today string, unresolved bool, otherName string, sched, progress, delayed, done int) string {
	scope := "내 업무"
	if unresolved {
		scope = "팀 전체"
	} else if n := strings.TrimSpace(otherName); n != "" {
		scope = n + " 업무"
	}
	return today + " · " + scope + " · 오늘 예정 " + strconv.Itoa(sched) + " · 진행중 " + strconv.Itoa(progress) + " · 지연 " + strconv.Itoa(delayed) + " · 오늘 완료 " + strconv.Itoa(done)
}

func workAllScopeNote(from, to, assignee, status string, unresolved bool) string {
	parts := []string{"팀 전체", from + " ~ " + to}
	if unresolved {
		parts = []string{"역할을 확인하지 못해 팀 전체로 표시합니다", from + " ~ " + to}
	}
	if assignee == workAssigneeNone {
		parts = append(parts, "미배정")
	} else if assignee != "" {
		parts = append(parts, assignee+" 배정")
	}
	if status == "delayed" {
		parts = append(parts, "지연")
	} else if status == "complete" {
		parts = append(parts, "완료")
	}
	return strings.Join(parts, " · ")
}

func workTodayQuery(view, sortKey, dir, assignee string) string {
	v := workTodayFilterQuery(view, assignee)
	if sortKey == "prefix" {
		if dir != "desc" {
			dir = "asc"
		}
	} else {
		sortKey, dir = model.NormalizeAdminWorkSort(sortKey, dir)
	}
	if sortKey != "due_date" || dir != "asc" {
		v.Set("sort", sortKey)
		v.Set("dir", dir)
	}
	return v.Encode()
}

func workTodayFilterQuery(view, assignee string) url.Values {
	v := url.Values{}
	if view == "kanban" {
		v.Set("display", "kanban")
		v.Set("view", "kanban")
	} else if view == "list" {
		v.Set("display", "list")
		v.Set("view", "list")
	}
	if a := strings.TrimSpace(assignee); a != "" && !isWorkTodayForbiddenAll(a) {
		v.Set("assignee", a)
	}
	return v
}

func workTodaySortHrefs(view, curSort, curDir, assignee string) map[string]string {
	if curSort == "prefix" {
		if curDir != "desc" {
			curDir = "asc"
		}
	} else {
		curSort, curDir = model.NormalizeAdminWorkSort(curSort, curDir)
	}
	return sortLinkHrefs("/work", workTodayFilterQuery(view, assignee),
		[]string{"task_id", "prefix", "title", "customer", "assignee", "work_date", "due_date", "status"},
		curSort, curDir)
}

type workAllQuery struct {
	View, Sort, Dir            string
	Assignees, Kinds, Statuses []string
	Customers, Projects        []string
	Q, Period, Range, From, To string
	Offset                     int
}

func (q workAllQuery) withView(view string) workAllQuery {
	q.View = view
	return q
}

// workFilterQuery 검색·필터만 남긴다. 정렬·페이지는 붙이지 않는다. §35.7 · §37.4
func workFilterQuery(q workAllQuery) url.Values {
	v := url.Values{}
	if q.View == "kanban" {
		v.Set("display", "kanban")
		v.Set("view", "kanban")
	} else if q.View == "list" {
		v.Set("display", "list")
		v.Set("view", "list")
	}
	addCSV(v, "assignee", q.Assignees)
	addCSV(v, "kind", q.Kinds)
	addCSV(v, "status", q.Statuses)
	addCSV(v, "customer", q.Customers)
	addCSV(v, "project", q.Projects)
	if strings.TrimSpace(q.Q) != "" {
		v.Set("q", q.Q)
	}
	switch q.Period {
	case "today":
		v.Set("range", "1d")
	case "week":
		v.Set("range", "1w")
	case "month":
		v.Set("range", "1m")
	case "year", "custom":
		if q.From != "" {
			v.Set("from", q.From)
		}
		if q.To != "" {
			v.Set("to", q.To)
		}
		if q.Period == "year" {
			v.Set("period", "year")
		}
	default:
		// 최근 3개월(기본)은 URL에 안 남긴다
	}
	return v
}

func (q workAllQuery) Encode() string {
	v := workFilterQuery(q)
	sortKey, dir := parseWorkAllSort(q.Sort, q.Dir)
	if sortKey != "due_date" || dir != "desc" {
		v.Set("sort", sortKey)
		v.Set("dir", dir)
	}
	if q.Offset > 0 {
		v.Set("offset", strconv.Itoa(q.Offset))
	}
	return v.Encode()
}

func workAllSortHrefs(q workAllQuery) map[string]string {
	curSort, curDir := parseWorkAllSort(q.Sort, q.Dir)
	return sortLinkHrefs("/work/all", workFilterQuery(q),
		[]string{"task_id", "prefix", "title", "customer", "assignee", "work_date", "due_date", "status"},
		curSort, curDir)
}

func workAllResetHref(view, sortKey, dir string) string {
	q := workAllQuery{View: view, Sort: sortKey, Dir: dir, Period: "3m", Range: "3m"}
	enc := q.Encode()
	if enc == "" {
		return "/work/all"
	}
	return "/work/all?" + enc
}

func workAllURL(assignee, status string) string {
	q := workAllQuery{}
	if assignee != "" {
		q.Assignees = []string{assignee}
	}
	if status != "" {
		q.Statuses = []string{status}
	}
	s := q.Encode()
	if s == "" {
		return "/work/all"
	}
	return "/work/all?" + s
}

func planUnplannedURL(mine bool, role, kind string) string {
	return planUnplannedURLDisp(mine, role, kind, "")
}

func planUnplannedURLDisp(mine bool, role, kind, display string) string {
	return planUnplannedURLFull(mine, role, kind, display, "", "")
}

func unplannedFilterQuery(mine bool, role, kind, display string) url.Values {
	v := url.Values{}
	if role == model.RoleTech {
		if mine {
			v.Set("mine", "1")
		} else {
			v.Set("mine", "0")
		}
	} else if mine {
		v.Set("mine", "1")
	}
	if kind != "" {
		v.Set("kind", kind)
	}
	if model.ParseDisplay(display, "") == "kanban" {
		v.Set("display", "kanban")
	}
	return v
}

func planUnplannedURLFull(mine bool, role, kind, display, sort, dir string) string {
	v := unplannedFilterQuery(mine, role, kind, display)
	if sort != "" {
		v.Set("sort", sort)
		if dir != "" {
			v.Set("dir", dir)
		}
	}
	s := v.Encode()
	if s == "" {
		return "/plan/unplanned"
	}
	return "/plan/unplanned?" + s
}
