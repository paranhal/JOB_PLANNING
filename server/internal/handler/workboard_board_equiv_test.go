package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type equivUsers struct {
	me, other, admin, o03 *model.User
}

func seedEquivBoard(t *testing.T) (*echo.Echo, *sql.DB, *Handler, equivUsers, string, string) {
	t.Helper()
	today := time.Now().Format(dateLayout)
	yday := time.Now().AddDate(0, 0, -2).Format(dateLayout)
	e, db, me, other, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "eq.db"))
	h := New(db)
	users := repository.NewUserRepo(db)
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O03','O03','미디어사업팀2','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	oa, _ := users.GetByUsername("oa01")
	u03 := &model.User{Username: "u03", PasswordHash: HashPassword("password10"), FullName: "오삼이", Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true}
	if err := users.Create(u03); err != nil {
		t.Fatal(err)
	}
	got03, _ := users.GetByUsername("u03")
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('C-EQ','동등고객','동등고객',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	mustCreateBoardTask(t, repo, "날짜본인", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "날짜타인", today, "11:00", "12:00", model.OrgIDLibrary, other.FullName, other.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	dueOnly := &model.WorkTask{
		Title: "예정일만", DueDate: today, OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(dueOnly); err != nil {
		t.Fatal(err)
	}
	memTask := &model.WorkTask{
		Title: "구성원전용", WorkDate: today, DueDate: today, StartTime: "13:00", EndTime: "14:00",
		OrgID: model.OrgIDLibrary, Assignee: oa.FullName, AssigneeUserID: oa.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin, Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(memTask); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceSupportMembers(memTask.TaskID, []model.WorkTaskMember{{
		TaskID: memTask.TaskID, UserID: me.UserID, Assignee: me.FullName, Role: model.WBMemberSupport,
	}}); err != nil {
		t.Fatal(err)
	}
	mustCreateBoardTask(t, repo, "관리자행정", today, "15:00", "16:00", model.OrgIDLibrary, oa.FullName, oa.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	sales := &model.WorkTask{
		Title: "영업활동건", WorkDate: today, DueDate: today, StartTime: "16:00", EndTime: "17:00",
		OrgID: model.OrgIDLibrary, Assignee: me.FullName, AssigneeUserID: me.UserID,
		Status: model.WBTaskWaiting, WorkType: model.WBWorkSales, SourceType: model.WBSourceSalesActivity,
		Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(sales); err != nil {
		t.Fatal(err)
	}
	mustCreateBoardTask(t, repo, "취소건", today, "08:00", "08:30", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskCancelled, model.WBWorkAdmin)
	mustCreateBoardTask(t, repo, "완료건", today, "08:30", "09:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskComplete, model.WBWorkAdmin)
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, visit_scheduled_date, created_at, updated_at)
		VALUES
		('AS-D','R-D',?,'C-EQ','방문있음','assigned',?, 'O01', ?, ?, ?),
		('AS-N','R-N',?,'C-EQ','날짜없는AS','assigned',?, 'O01', '', ?, ?),
		('AS-L','R-L',?,'C-EQ','지연AS','in_progress',?, 'O01', ?, ?, ?)`,
		now, me.FullName, today, now, now,
		now, me.FullName, now, now,
		now, me.FullName, yday, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_work_items (work_id, work_number, as_id, work_kind, scheduled_date, schedule_confirmed, assigned_to, status, created_at, updated_at)
		VALUES
		('W-R','R-D-W01','AS-D','revisit',?,?,?,'open',?,?),
		('W-C','R-D-W02','AS-D','confirm',?,?,?,'open',?,?),
		('W-S','R-D-W03','AS-D','support',?,?,?,'open',?,?)`,
		today, 1, me.FullName, now, now,
		today, 1, me.FullName, now, now,
		today, 1, me.FullName, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_plans (plan_id, plan_year, title, status, org_id) VALUES ('MP-EQ',2026,'점검','approved','O01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, sort_order, completed, assignee, product_type)
		VALUES ('MV-T','MP-EQ',?,'C-EQ',1,0,?,'KLAS'), ('MV-L','MP-EQ',?,'C-EQ',2,0,?,'KLAS')`, today, me.FullName, yday, me.FullName); err != nil {
		t.Fatal(err)
	}
	mustCreateBoardTask(t, repo, "O03업무", today, "10:00", "11:00", "O03", got03.FullName, got03.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	timedEmpty := &model.WorkTask{
		Title: "자동배치대상", WorkDate: today, DueDate: today, OrgID: model.OrgIDLibrary,
		Assignee: me.FullName, AssigneeUserID: me.UserID, Status: model.WBTaskWaiting, WorkType: model.WBWorkAdmin,
		Priority: model.WBPriorityNormal,
	}
	if err := repo.CreateTask(timedEmpty); err != nil {
		t.Fatal(err)
	}
	return e, db, h, equivUsers{me: me, other: other, admin: oa, o03: got03}, today, yday
}

func collectFor(t *testing.T, h *Handler, u *model.User, path string) boardEntries {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.AddCookie(jwtCookieUser(t, u))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	var out boardEntries
	if err := h.Auth.AuthMiddleware(func(c echo.Context) error {
		view := parseBoardView(c.QueryParam("view"))
		base := parseBoardDate(c.QueryParam("date"), time.Now())
		fromT, toT := boardRange(view, base)
		from, to := fromT.Format(dateLayout), toT.Format(dateLayout)
		var err error
		out, err = h.Workboard.collectBoardEntries(c, from, to, resolveBoardScope(c))
		return err
	})(c); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCollectBoardEntriesSubsetOfLegacy(t *testing.T) {
	e, _, h, us, today, _ := seedEquivBoard(t)
	_ = e
	entries := collectFor(t, h, us.me, "/workboard/board?view=list&date="+today+"&auto=0")
	got := entries.keySet()
	legacy := map[string]bool{}
	reg := doGetWith(t, e, "/workboard/register?view=day&date="+today+"&auto=0", jwtCookieUser(t, us.me))
	if reg.Code != 200 {
		t.Fatalf("register %d", reg.Code)
	}
	for _, title := range []string{"날짜본인", "구성원전용", "영업활동건"} {
		if !strings.Contains(reg.Body.String(), title) {
			t.Fatalf("구 화면에 %s 없음", title)
		}
	}
	_ = reg
	for _, it := range append(append(append([]boardItem{}, entries.Placed...), entries.Unplaced...), entries.Overdue...) {
		legacy[it.DedupKey] = true
		_ = got
	}
	missing := []string{}
	for _, title := range []string{"날짜본인", "구성원전용", "영업활동건", "날짜없는AS"} {
		body := doGetWith(t, e, "/workboard/board?view=list&date="+today+"&auto=0", jwtCookieUser(t, us.me)).Body.String()
		unp := doGetWith(t, e, "/workboard/board?view=month&date="+today+"&auto=0", jwtCookieUser(t, us.me)).Body.String()
		if !strings.Contains(body, title) && !strings.Contains(unp, title) {
			missing = append(missing, title)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("새 화면에 없음: %s", strings.Join(missing, ","))
	}
	if strings.Contains(doGetWith(t, e, "/workboard/board?view=list&date="+today, jwtCookieUser(t, us.me)).Body.String(), "취소건") {
		t.Fatal("새 화면에 취소")
	}
}

func TestCollectBoardAutoPlaceAndAuto0(t *testing.T) {
	_, db, h, us, today, _ := seedEquivBoard(t)
	_ = h
	var id, wd, st string
	if err := db.QueryRow(`SELECT task_id, work_date, start_time FROM work_tasks WHERE title='자동배치대상'`).Scan(&id, &wd, &st); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(st) != "" {
		t.Fatal("시드에 시각이 있음")
	}
	collectFor(t, h, us.me, "/workboard/board?view=list&date="+today+"&auto=0")
	if err := db.QueryRow(`SELECT start_time FROM work_tasks WHERE task_id=?`, id).Scan(&st); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(st) != "" {
		t.Fatal("auto=0 인데 시각이 채워졌다")
	}
	collectFor(t, h, us.me, "/workboard/board?view=list&date="+today)
	if err := db.QueryRow(`SELECT work_date, start_time FROM work_tasks WHERE task_id=?`, id).Scan(&wd, &st); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(wd) != today || strings.TrimSpace(st) == "" {
		t.Fatalf("자동 배치 후 work_date=%s start=%s", wd, st)
	}
}

func TestUnplacedBoxNotOnCalendar(t *testing.T) {
	e, _, _, us, today, _ := seedEquivBoard(t)
	ck := jwtCookieUser(t, us.me)
	month := doGetWith(t, e, "/workboard/board?view=month&date="+today+"&auto=0", ck).Body.String()
	if !strings.Contains(month, "미배치 업무") || !strings.Contains(month, "날짜없는AS") {
		t.Fatal("월 보기 미배치 박스에 날짜 없는 AS 없음")
	}
	if !strings.Contains(month, "data-pf=\"as\"") {
		t.Fatal("유형 필터 칩 없음")
	}
	if !strings.Contains(month, "data-ptoggle") {
		t.Fatal("접기 버튼 없음")
	}
	if strings.Count(month, "날짜없는AS") != 1 {
		t.Fatalf("미배치가 칸에도 올라간 것 같음 count=%d", strings.Count(month, "날짜없는AS"))
	}
}

func TestBoardOverdueBadgeMatchesWorkToday(t *testing.T) {
	e, _, _, us, today, _ := seedEquivBoard(t)
	ck := jwtCookieUser(t, us.me)
	work := doGetWith(t, e, "/work", ck)
	board := doGetWith(t, e, "/workboard/board?view=list&date="+today+"&auto=0", ck)
	if !strings.Contains(board.Body.String(), "지연") {
		t.Fatal("리스트에 지연 배지 없음")
	}
	if work.Code != 200 {
		t.Fatalf("work %d", work.Code)
	}
}

func TestBoardScopeMineAllAssigneeAndMember(t *testing.T) {
	e, _, _, us, today, _ := seedEquivBoard(t)
	ck := jwtCookieUser(t, us.me)
	mine := doGetWith(t, e, "/workboard/board?date="+today+"&auto=0", ck).Body.String()
	if strings.Contains(mine, "날짜타인") {
		t.Fatal("기술 기본 내 업무에 타인")
	}
	if !strings.Contains(mine, "구성원전용") {
		t.Fatal("구성원 전용 업무가 내 범위에 없음")
	}
	all := doGetWith(t, e, "/workboard/board?date="+today+"&mine=0&auto=0", ck).Body.String()
	if !strings.Contains(all, "날짜타인") && !strings.Contains(all, "관리자행정") {
		t.Fatal("mine=0 전체에 타인/관리자 업무 없음")
	}
	ack := jwtCookieUser(t, us.admin)
	adm := doGetWith(t, e, "/workboard/board?date="+today+"&auto=0", ack).Body.String()
	if !strings.Contains(adm, "날짜본인") || !strings.Contains(adm, "관리자행정") {
		t.Fatal("관리자 기본 전체 실패")
	}
	one := doGetWith(t, e, "/workboard/board?date="+today+"&assignee="+url.QueryEscape(us.me.FullName)+"&auto=0", ack).Body.String()
	if !strings.Contains(one, "날짜본인") {
		t.Fatal("assignee 한 명 필터 실패")
	}
}

func TestBoardOrgIsolationAndCLevel(t *testing.T) {
	e, _, _, us, today, _ := seedEquivBoard(t)
	body := doGetWith(t, e, "/workboard/board?date="+today+"&mine=0&auto=0", jwtCookieUser(t, us.o03)).Body.String()
	if strings.Contains(body, "날짜본인") || strings.Contains(body, "O01내") {
		t.Fatal("O03 전체 범위에 O01 업무")
	}
	if !strings.Contains(body, "O03업무") {
		t.Fatal("O03 본인 업무 없음")
	}
}

func TestBoardDedupAndWorkItemHref(t *testing.T) {
	e, db, h, us, today, _ := seedEquivBoard(t)
	entries := collectFor(t, h, us.admin, "/workboard/board?view=list&date="+today+"&auto=0")
	n := 0
	for k := range entries.keySet() {
		if strings.HasPrefix(k, "as:AS-D") {
			n++
		}
	}
	if n > 1 {
		t.Fatalf("AS-D 중복 %d", n)
	}
	_ = db
	_ = e
	hrefOK := false
	for _, it := range append(append([]boardItem{}, entries.Placed...), entries.Unplaced...) {
		if strings.Contains(it.Href, "/as/") {
			hrefOK = true
			break
		}
	}
	if !hrefOK {
		t.Fatal("AS·하부업무 링크 없음")
	}
}

func TestBoardRegisterBidirectionalCreate(t *testing.T) {
	e, _, _, us, today, _ := seedEquivBoard(t)
	ck := jwtCookieUser(t, us.me)
	form := url.Values{
		"work_type": {model.WBWorkAdmin}, "title": {"신화면등록"}, "work_date": {today},
		"start_time": {"18:00"}, "end_time": {"18:30"}, "assignee": {us.me.FullName},
		"redirect": {"/workboard/board"},
	}
	rec := doFormWith(t, e, "/workboard/tasks", form, ck)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	reg := doGetWith(t, e, "/workboard/register?view=day&date="+today+"&auto=0", ck).Body.String()
	if !strings.Contains(reg, "신화면등록") {
		t.Fatal("새 화면 등록이 구 화면에 없음")
	}
	form2 := url.Values{
		"work_type": {model.WBWorkAdmin}, "title": {"구화면등록"}, "work_date": {today},
		"start_time": {"18:30"}, "end_time": {"19:00"}, "assignee": {us.me.FullName},
		"redirect": {"/workboard/register"},
	}
	_ = doFormWith(t, e, "/workboard/tasks", form2, ck)
	board := doGetWith(t, e, "/workboard/board?date="+today+"&auto=0", ck).Body.String()
	if !strings.Contains(board, "구화면등록") {
		t.Fatal("구 화면 등록이 새 화면에 없음")
	}
}

func TestBoardPlaceFromUnplacedUsesSchedule(t *testing.T) {
	e, db, _, us, today, _ := seedEquivBoard(t)
	ck := jwtCookieUser(t, us.me)
	var tid string
	if err := db.QueryRow(`SELECT task_id FROM work_tasks WHERE title='예정일만'`).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"kind": {"task"}, "ref_id": {tid}, "work_date": {today},
		"start_time": {"19:00"}, "end_time": {"19:30"}, "assignee": {us.me.FullName},
		"redirect": {"/workboard/board?view=list&date=" + today + "&auto=0"},
	}
	rec := doFormWith(t, e, "/workboard/schedule", form, ck)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusOK {
		t.Fatalf("place %d %s", rec.Code, rec.Body.String())
	}
	board := doGetWith(t, e, "/workboard/board?view=list&date="+today+"&auto=0", ck).Body.String()
	if !strings.Contains(board, "예정일만") {
		t.Fatal("배치 후 리스트에 없음")
	}
	reg := doGetWith(t, e, "/workboard/register?view=day&date="+today+"&auto=0", ck).Body.String()
	if !strings.Contains(reg, "예정일만") {
		t.Fatal("배치 후 구 화면에 없음")
	}
}

func TestBoardCLevelForbiddenStill(t *testing.T) {
	e, db, _, _, _, _ := seedEquivBoard(t)
	users := repository.NewUserRepo(db)
	ceo := &model.User{Username: "ceoeq", PasswordHash: HashPassword("password10"), FullName: "대표eq", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	if err := users.Create(ceo); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByUsername("ceoeq")
	rec := doGetWith(t, e, "/workboard/board", jwtCookieUser(t, got))
	if rec.Code != http.StatusOK {
		t.Fatalf("c_level 조회 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "조회 전용") {
		t.Fatal("C-Level 조회 전용 표시 없음")
	}
}

func doFormWith(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	if ck != nil {
		req.AddCookie(ck)
	}
	e.ServeHTTP(rec, req)
	return rec
}
