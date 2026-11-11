package handler

import (
	"database/sql"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func new172(t *testing.T) (*echo.Echo, *sql.DB, *repository.WBRepo, *model.User, *model.User) {
	t.Helper()
	e, db, me, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "d172.db"))
	oa, err := repository.NewUserRepo(db).GetByUsername("oa01")
	if err != nil || oa == nil {
		t.Fatal("조직관리자 없음")
	}
	return e, db, repo, me, oa
}

func add172User(t *testing.T, db *sql.DB, username, name, role, org string) *model.User {
	t.Helper()
	u := &model.User{
		Username: username, PasswordHash: HashPassword("password10"), FullName: name,
		Role: role, IsActive: true, OrgID: org, ProfileDone: true,
	}
	users := repository.NewUserRepo(db)
	if err := users.Create(u); err != nil {
		t.Fatal(err)
	}
	got, err := users.GetByUsername(username)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	return got
}

func seed172AS(t *testing.T, db *sql.DB, id, assignee string) {
	t.Helper()
	nows := time.Now().Format("2006-01-02 15:04:05")
	if _, err := db.Exec(`INSERT OR IGNORE INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES (?,?,?,1,?)`, "C"+id, "천우물초", "천우물초", model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, customer_id, receipt_datetime, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id, "R-"+id, "C"+id, nows, "배치시험", "received", assignee, model.OrgIDLibrary, nows, nows); err != nil {
		t.Fatal(err)
	}
}

func post172(t *testing.T, e *echo.Echo, ck *http.Cookie, form url.Values) *http.Response {
	t.Helper()
	rec := doFormWith(t, e, "/workboard/schedule", form, ck)
	return rec.Result()
}

func form172(day, kind, ref, assignee, start string) url.Values {
	v := url.Values{
		"kind": {kind}, "ref_id": {ref}, "work_date": {day},
		"redirect": {"/workboard/board?view=day&date=" + day + "&auto=0"},
	}
	if assignee != "" {
		v.Set("assignee", assignee)
	}
	if start != "" {
		v.Set("start_time", start)
		v.Set("end_time", "10:00")
		if start != "09:00" {
			v.Set("end_time", "11:30")
		}
	}
	return v
}

func task172(t *testing.T, repo *repository.WBRepo, kind, ref string) *model.WorkTask {
	t.Helper()
	got, err := repo.GetTaskBySource(kind, ref)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func Test95_172_EmptyASAssigneeNotSilent(t *testing.T) {
	e, db, repo, me, oa := new172(t)
	day := time.Now().AddDate(0, 0, 9).Format("2006-01-02")
	seed172AS(t, db, "AS172A", "")
	seed172AS(t, db, "AS172B", "")

	meRes := post172(t, e, jwtCookieUser(t, me), form172(day, model.WBSourceAS, "AS172A", "", "09:00"))
	if meRes.StatusCode != http.StatusSeeOther || strings.Contains(meRes.Header.Get("Location"), "assignee_required") {
		t.Fatalf("일반 사용자 본인 보충 %d %s", meRes.StatusCode, meRes.Header.Get("Location"))
	}
	got := task172(t, repo, model.WBSourceAS, "AS172A")
	if got == nil || strings.TrimSpace(got.Assignee) != me.FullName {
		t.Fatalf("본인으로 안 채워짐 %+v", got)
	}
	var assigned string
	if err := db.QueryRow(`SELECT TRIM(COALESCE(assigned_to,'')) FROM as_receipts WHERE as_id='AS172A'`).Scan(&assigned); err != nil {
		t.Fatal(err)
	}
	if assigned != "" {
		t.Fatalf("접수 담당자를 배치가 덮음 %q", assigned)
	}

	oaRes := post172(t, e, jwtCookieUser(t, oa), form172(day, model.WBSourceAS, "AS172B", "", "11:00"))
	loc := oaRes.Header.Get("Location")
	if oaRes.StatusCode != http.StatusSeeOther || !strings.Contains(loc, "assignee_required") {
		t.Fatalf("조직관리자 빈 담당자 %d %s", oaRes.StatusCode, loc)
	}
	if task172(t, repo, model.WBSourceAS, "AS172B") != nil {
		t.Fatal("오류인데 업무가 생김")
	}
	page := doGetWith(t, e, loc, jwtCookieUser(t, oa))
	if !strings.Contains(page.Body.String(), "담당자를 선택해 주세요.") {
		t.Fatal("안내 문구 없음")
	}
}

func Test95_172_OrgAdminPicksMemberAndSupport(t *testing.T) {
	e, db, repo, me, oa := new172(t)
	extra := add172User(t, db, "park172", "박지원", model.RoleTech, model.OrgIDLibrary)
	day := time.Now().AddDate(0, 0, 9).Format("2006-01-02")
	seed172AS(t, db, "AS172C", "")
	res := post172(t, e, jwtCookieUser(t, oa), url.Values{
		"kind": {model.WBSourceAS}, "ref_id": {"AS172C"}, "work_date": {day},
		"start_time": {"13:00"}, "end_time": {"14:00"},
		"assignee_multi": {"1"}, "owner_user_id": {me.UserID},
		"assignee_user_id": {me.UserID, extra.UserID},
		"redirect":         {"/workboard/board?view=day&date=" + day},
	})
	if res.StatusCode != http.StatusSeeOther || strings.Contains(res.Header.Get("Location"), "err=") {
		t.Fatalf("선택 배치 %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	got := task172(t, repo, model.WBSourceAS, "AS172C")
	if got == nil || got.Assignee != me.FullName {
		t.Fatalf("주담당 %+v", got)
	}
	members, err := repo.ListMembers(got.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	var support bool
	for _, m := range members {
		if m.IsSupport() && (m.Assignee == extra.FullName || m.UserID == extra.UserID) {
			support = true
		}
	}
	if !support {
		t.Fatalf("지원 없음 %+v", members)
	}
	body := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&mine=0&auto=0", jwtCookieUser(t, oa)).Body.String()
	if !strings.Contains(body, me.FullName) {
		t.Fatal("일정 열에 주담당이 없음")
	}
}

func Test95_172_UnassignedTaskAndVisit(t *testing.T) {
	e, db, repo, me, oa := new172(t)
	day := time.Now().AddDate(0, 0, 9).Format("2006-01-02")
	mustCreateBoardTask(t, repo, "미배정업무172", day, "", "", model.OrgIDLibrary, "", "", model.WBTaskWaiting, model.WBWorkAdmin)
	task := taskByTitle(t, repo, "미배정업무172")
	meRes := post172(t, e, jwtCookieUser(t, me), form172(day, "task", task.TaskID, "", "14:00"))
	if meRes.StatusCode != http.StatusSeeOther || strings.Contains(meRes.Header.Get("Location"), "assignee_required") {
		t.Fatalf("업무 본인 보충 %d %s", meRes.StatusCode, meRes.Header.Get("Location"))
	}
	again := taskByTitle(t, repo, "미배정업무172")
	if again == nil || again.Assignee != me.FullName || !strings.HasPrefix(again.StartTime, "14:00") {
		t.Fatalf("업무 배치 %+v", again)
	}

	mustCreateBoardTask(t, repo, "관리자빈업무172", day, "", "", model.OrgIDLibrary, "", "", model.WBTaskWaiting, model.WBWorkAdmin)
	empty := taskByTitle(t, repo, "관리자빈업무172")
	oaRes := post172(t, e, jwtCookieUser(t, oa), form172(day, "task", empty.TaskID, "", "15:00"))
	if oaRes.StatusCode != http.StatusSeeOther || !strings.Contains(oaRes.Header.Get("Location"), "assignee_required") {
		t.Fatalf("관리자 빈 업무 %d %s", oaRes.StatusCode, oaRes.Header.Get("Location"))
	}
	still := taskByTitle(t, repo, "관리자빈업무172")
	if still == nil || strings.TrimSpace(still.Assignee) != "" || strings.TrimSpace(still.StartTime) != "" {
		t.Fatalf("빈 업무가 배치됨 %+v", still)
	}

	seedVisit(t, db, "V172", day, "", "KLAS", "천우물도서관")
	vRes := post172(t, e, jwtCookieUser(t, oa), form172(day, model.WBSourceMaintenance, "V172", "", "16:00"))
	if vRes.StatusCode != http.StatusSeeOther || !strings.Contains(vRes.Header.Get("Location"), "assignee_required") {
		t.Fatalf("점검 빈 담당자 %d %s", vRes.StatusCode, vRes.Header.Get("Location"))
	}
	if task172(t, repo, model.WBSourceMaintenance, "V172") != nil {
		t.Fatal("점검 업무가 생김")
	}
	ok := post172(t, e, jwtCookieUser(t, me), form172(day, model.WBSourceMaintenance, "V172", "", "16:00"))
	if ok.StatusCode != http.StatusSeeOther || strings.Contains(ok.Header.Get("Location"), "assignee_required") {
		t.Fatalf("점검 본인 보충 %d %s", ok.StatusCode, ok.Header.Get("Location"))
	}
	visitTask := task172(t, repo, model.WBSourceMaintenance, "V172")
	if visitTask == nil || visitTask.Assignee != me.FullName {
		t.Fatalf("점검 담당 %+v", visitTask)
	}
	var visitAssignee string
	if err := db.QueryRow(`SELECT TRIM(COALESCE(assignee,'')) FROM maintenance_visits WHERE visit_id='V172'`).Scan(&visitAssignee); err != nil {
		t.Fatal(err)
	}
	if visitAssignee != "" {
		t.Fatalf("방문 원본 담당자를 배치가 덮음 %q", visitAssignee)
	}
}

func Test95_172_ExistingAssigneeAndRejects(t *testing.T) {
	e, db, repo, me, oa := new172(t)
	other := add172User(t, db, "park172b", "박지원", model.RoleTech, model.OrgIDLibrary)
	sup := add172User(t, db, "sup172", "서지우", model.RoleSupport, model.OrgIDLibrary)
	day := time.Now().AddDate(0, 0, 9).Format("2006-01-02")
	mustCreateBoardTask(t, repo, "있는담당172", day, "", "", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	task := taskByTitle(t, repo, "있는담당172")
	same := post172(t, e, jwtCookieUser(t, me), form172(day, "task", task.TaskID, me.FullName, "09:00"))
	if same.StatusCode != http.StatusSeeOther || strings.Contains(same.Header.Get("Location"), "err=") {
		t.Fatalf("그대로 제출 %d %s", same.StatusCode, same.Header.Get("Location"))
	}
	if got := taskByTitle(t, repo, "있는담당172"); got == nil || got.Assignee != me.FullName {
		t.Fatalf("담당 유지 %+v", got)
	}
	deny := post172(t, e, jwtCookieUser(t, other), form172(day, "task", task.TaskID, other.FullName, "09:30"))
	if deny.StatusCode != http.StatusForbidden && deny.StatusCode != http.StatusSeeOther {
		t.Fatalf("권한 %d", deny.StatusCode)
	}
	if got := taskByTitle(t, repo, "있는담당172"); got == nil || got.Assignee != me.FullName {
		t.Fatalf("권한 없이 바뀜 %+v", got)
	}
	chg := post172(t, e, jwtCookieUser(t, oa), form172(day, "task", task.TaskID, other.FullName, "09:00"))
	if chg.StatusCode != http.StatusSeeOther || strings.Contains(chg.Header.Get("Location"), "err=") {
		t.Fatalf("관리자 변경 %d %s", chg.StatusCode, chg.Header.Get("Location"))
	}
	if got := taskByTitle(t, repo, "있는담당172"); got == nil || got.Assignee != other.FullName {
		t.Fatalf("변경 반영 %+v", got)
	}

	seed172AS(t, db, "AS172D", "")
	foreign := post172(t, e, jwtCookieUser(t, oa), form172(day, model.WBSourceAS, "AS172D", "이다른", "11:00"))
	if foreign.StatusCode < 400 && !strings.Contains(foreign.Header.Get("Location"), "err=") {
		t.Fatalf("타조직이 통과 %d %s", foreign.StatusCode, foreign.Header.Get("Location"))
	}
	if task172(t, repo, model.WBSourceAS, "AS172D") != nil {
		t.Fatal("타조직 담당으로 업무가 생김")
	}
	self := post172(t, e, jwtCookieUser(t, oa), form172(day, model.WBSourceAS, "AS172D", oa.FullName, "11:00"))
	if self.StatusCode < 400 {
		t.Fatalf("조직관리자 본인 지정 %d %s", self.StatusCode, self.Header.Get("Location"))
	}
	if task172(t, repo, model.WBSourceAS, "AS172D") != nil {
		t.Fatal("본인 지정으로 업무가 생김")
	}

	seed172AS(t, db, "AS172E", "")
	supRes := post172(t, e, jwtCookieUser(t, sup), form172(day, model.WBSourceAS, "AS172E", me.FullName, "12:00"))
	if supRes.StatusCode != http.StatusSeeOther || strings.Contains(supRes.Header.Get("Location"), "err=") {
		t.Fatalf("지원 배정 %d %s", supRes.StatusCode, supRes.Header.Get("Location"))
	}
	if got := task172(t, repo, model.WBSourceAS, "AS172E"); got == nil || got.Assignee != me.FullName {
		t.Fatalf("지원이 고른 담당 %+v", got)
	}
	seed172AS(t, db, "AS172F", "")
	supEmpty := post172(t, e, jwtCookieUser(t, sup), form172(day, model.WBSourceAS, "AS172F", "", "12:30"))
	if supEmpty.StatusCode != http.StatusSeeOther || !strings.Contains(supEmpty.Header.Get("Location"), "assignee_required") {
		t.Fatalf("지원 빈 값 %d %s", supEmpty.StatusCode, supEmpty.Header.Get("Location"))
	}
}

func Test95_172_OverlapUsesChosenAssignee(t *testing.T) {
	e, db, repo, me, oa := new172(t)
	day := time.Now().AddDate(0, 0, 9).Format("2006-01-02")
	mustCreateBoardTask(t, repo, "겹침기준172", day, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	seed172AS(t, db, "AS172G", "")
	hit := post172(t, e, jwtCookieUser(t, oa), form172(day, model.WBSourceAS, "AS172G", me.FullName, "09:00"))
	if hit.StatusCode != http.StatusSeeOther && hit.StatusCode != http.StatusConflict {
		t.Fatalf("겹침 %d", hit.StatusCode)
	}
	if task172(t, repo, model.WBSourceAS, "AS172G") != nil {
		t.Fatal("겹치는 시각에 업무가 생김")
	}
	if !strings.Contains(hit.Header.Get("Location"), "err=") && hit.StatusCode != http.StatusConflict {
		t.Fatal("겹침 안내 없음")
	}
	free := post172(t, e, jwtCookieUser(t, oa), form172(day, model.WBSourceAS, "AS172G", me.FullName, ""))
	if free.StatusCode != http.StatusSeeOther || strings.Contains(free.Header.Get("Location"), "err=") {
		t.Fatalf("빈 자리 %d %s", free.StatusCode, free.Header.Get("Location"))
	}
	got := task172(t, repo, model.WBSourceAS, "AS172G")
	if got == nil || got.Assignee != me.FullName || strings.HasPrefix(got.StartTime, "09:00") || strings.TrimSpace(got.StartTime) == "" {
		t.Fatalf("그 사람 빈 자리 %+v", got)
	}
}

func Test95_172_PlaceModalRendersPicker(t *testing.T) {
	e, _, repo, me, _ := new172(t)
	day := time.Now().AddDate(0, 0, 9).Format("2006-01-02")
	mustCreateBoardTask(t, repo, "카드172", "", "", "", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	for _, view := range []string{"day", "week", "month"} {
		body := doGetWith(t, e, "/workboard/board?view="+view+"&date="+day+"&auto=0", jwtCookieUser(t, me)).Body.String()
		form := body
		if i := strings.Index(body, `id="board-place-form"`); i >= 0 {
			form = body[i:]
			if j := strings.Index(form, "</form>"); j >= 0 {
				form = form[:j]
			}
		} else {
			t.Fatalf("%s 배치 폼 없음", view)
		}
		if !strings.Contains(form, `data-assignee-picker="1"`) {
			t.Fatalf("%s 선택기 없음", view)
		}
		if strings.Contains(form, `id="board-pW"`) {
			t.Fatalf("%s 비활성 담당자 칸이 남음", view)
		}
		if !strings.Contains(body, "담당자를 선택해 주세요.") || !strings.Contains(body, "setPlaceAssignee") {
			t.Fatalf("%s 초기화 스크립트 없음", view)
		}
		if !strings.Contains(body, `data-assignee="`+me.FullName+`"`) {
			t.Fatalf("%s 카드 담당자 없음", view)
		}
	}
	src := string(mustRead(t, filepath.Join(findTemplateRoot(t), "workboard", "_board_unplaced.html")))
	if !strings.Contains(src, `{{template "wb_assignee_picker" .}}`) {
		t.Fatal("공통 선택기 템플릿을 안 씀")
	}
}
