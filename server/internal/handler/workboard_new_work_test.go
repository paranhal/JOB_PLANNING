package handler

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestNewWorkButtonWriteAndHidden(t *testing.T) {
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "nw-btn.db"))
	ck := jwtCookieUser(t, me)
	for _, view := range []string{"list", "month", "week", "day"} {
		body := doGetWith(t, e, "/workboard/board?view="+view, ck).Body.String()
		if !strings.Contains(body, "+ 업무 등록") {
			t.Fatalf("%s 에 + 업무 등록 없음", view)
		}
		if !strings.Contains(body, `role="radiogroup"`) || !strings.Contains(body, "AS 조치") {
			t.Fatalf("%s 1단계 모달 없음", view)
		}
		if !strings.Contains(body, "고장 접수 및 현장 조치") || !strings.Contains(body, "설비 정기 점검과 기록") {
			t.Fatal("카드 설명 불일치")
		}
		if !strings.Contains(body, "설치·교체·보수 작업") || !strings.Contains(body, "문서·교육·내부 지원") || !strings.Contains(body, "고객 상담 및 계약 활동") {
			t.Fatal("카드 설명 불일치")
		}
		if !strings.Contains(body, "어떤 업무를 등록할까요?") {
			t.Fatal("1단계 제목 없음")
		}
		if !strings.Contains(body, `newWorkKind = 'as'`) && !strings.Contains(body, `newWorkKind: 'as'`) {
			t.Fatal("기본 선택 AS 아님")
		}
	}
	users := repository.NewUserRepo(db)
	ro := &model.User{Username: "ro1", PasswordHash: HashPassword("password10"), FullName: "읽기만", Role: model.RoleTech, IsActive: true, IsReadOnly: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(ro); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByUsername("ro1")
	hidden := doGetWith(t, e, "/workboard/board", jwtCookieUser(t, got)).Body.String()
	if strings.Contains(hidden, "+ 업무 등록") {
		t.Fatal("읽기 전용에 등록 버튼이 있다")
	}
}

func TestNewWorkFormKindsAndAuth(t *testing.T) {
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "nw-form.db"))
	ck := jwtCookieUser(t, me)
	wantType := map[string]string{
		"as": "as", "maintenance": "maintenance", "project_work": "project_work",
		"admin": "admin", "sales": "sales",
	}
	wantField := map[string]string{
		"as": "source_id", "maintenance": "source_id", "project_work": "work_subtype",
		"admin": "title", "sales": "title",
	}
	for kind, wt := range wantType {
		rec := doGetWith(t, e, "/workboard/new-work/form?kind="+kind+"&date=2026-10-24&redirect="+url.QueryEscape("/workboard/board?view=list&date=2026-10-24"), ck)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s → %d %s", kind, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `action="/workboard/tasks"`) {
			t.Fatalf("%s action 없음", kind)
		}
		if !strings.Contains(body, `name="work_type"`) || !strings.Contains(body, `value="`+wt+`"`) {
			t.Fatalf("%s work_type=%s 없음", kind, wt)
		}
		if !strings.Contains(body, `name="`+wantField[kind]+`"`) {
			t.Fatalf("%s 필드 %s 없음", kind, wantField[kind])
		}
		if !strings.Contains(body, "← 업무 유형 선택") {
			t.Fatalf("%s 되돌리기 문구 없음", kind)
		}
	}
	bad := doGetWith(t, e, "/workboard/new-work/form?kind=nope", ck)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("잘못된 kind %d", bad.Code)
	}
	users := repository.NewUserRepo(db)
	ro := &model.User{Username: "ro2", PasswordHash: HashPassword("password10"), FullName: "읽기2", Role: model.RoleTech, IsActive: true, IsReadOnly: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(ro); err != nil {
		t.Fatal(err)
	}
	got, _ := users.GetByUsername("ro2")
	forb := doGetWith(t, e, "/workboard/new-work/form?kind=as", jwtCookieUser(t, got))
	if forb.Code != http.StatusForbidden {
		t.Fatalf("읽기 전용 %d", forb.Code)
	}
}

func TestNewWorkFormOrgIsolation(t *testing.T) {
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "nw-org.db"))
	ts := "2026-10-24 10:00:00"
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-o01','도서관고객','도서관고객',1,'O01'), ('c-o02','미디어고객','미디어고객',1,'O02')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('as-o01','R-O01',?,'c-o01','O01증상','in_progress','김현우','O01',?,?),
		       ('as-o02','R-O02',?,'c-o02','O02증상','in_progress','이다른','O02',?,?)`, ts, ts, ts, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO maintenance_plans (plan_id, plan_year, title, status, org_id) VALUES
		('MP-nw',2026,'정기','approved','O01'),
		('MP-o02',2026,'미디어','approved','O02')`); err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format(dateLayout)
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES ('v-o01','MP-nw',?,'c-o01',0,'normal','김현우','KLAS',0),
		       ('v-o02','MP-nw',?,'c-o02',0,'normal','이다른','KLAS',0),
		       ('v-x02','MP-o02',?,'c-o02',0,'normal','이다른','KLAS',0)`, today, today, today); err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUser(t, me)
	asBody := doGetWith(t, e, "/workboard/new-work/form?kind=as", ck).Body.String()
	if !strings.Contains(asBody, "as-o01") || strings.Contains(asBody, "as-o02") {
		t.Fatal("AS 조각 조직 격리 실패")
	}
	mntBody := doGetWith(t, e, "/workboard/new-work/form?kind=maintenance", ck).Body.String()
	if !strings.Contains(mntBody, "v-o01") || !strings.Contains(mntBody, "v-o02") {
		t.Fatal("같은 계획 조직 방문이 없다")
	}
	if strings.Contains(mntBody, "v-x02") {
		t.Fatal("다른 계획 조직 방문이 있다")
	}
}

func TestNewWorkSaveReturnsToBoard(t *testing.T) {
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "nw-save.db"))
	ck := jwtCookieUser(t, me)
	day := time.Now().Format(dateLayout)
	redir := "/workboard/board?view=day&date=" + day
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-save','저장고객','저장고객',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	ts := day + " 09:00:00"
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('as-save','R-SAVE',?,'c-save','저장증상','in_progress',?,'O01',?,?)`, ts, me.FullName, ts, ts); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO maintenance_plans (plan_id, plan_year, title, status) VALUES ('MP-sv',2026,'정기','approved')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO maintenance_visits (visit_id, plan_id, visit_date, customer_id, sort_order, entry_category, assignee, product_type, completed)
		VALUES ('v-save','MP-sv',?,'c-save',0,'normal',?,'KLAS',0)`, day, me.FullName); err != nil {
		t.Fatal(err)
	}

	posts := []url.Values{
		{"work_type": {"admin"}, "title": {"행정등록95-20A"}, "work_date": {day}, "due_date": {day}, "start_time": {"09:00"}, "end_time": {"09:30"}, "assignee": {me.FullName}, "redirect": {redir}},
		{"work_type": {"sales"}, "title": {"영업등록95-20A"}, "work_date": {day}, "due_date": {day}, "start_time": {"10:00"}, "end_time": {"10:30"}, "assignee": {me.FullName}, "redirect": {redir}},
		{"work_type": {"project_work"}, "title": {"작업등록95-20A"}, "work_subtype": {"other"}, "work_date": {day}, "start_time": {"11:00"}, "end_time": {"11:30"}, "assignee": {me.FullName}, "redirect": {redir}},
		{"work_type": {"as"}, "source_id": {"as-save"}, "work_date": {day}, "due_date": {day}, "start_time": {"12:00"}, "end_time": {"12:30"}, "assignee": {me.FullName}, "redirect": {redir}},
		{"work_type": {"maintenance"}, "source_id": {"v-save"}, "work_date": {day}, "due_date": {day}, "start_time": {"13:00"}, "end_time": {"13:30"}, "assignee": {me.FullName}, "redirect": {redir}},
	}
	for _, form := range posts {
		rec := doFormUser(t, e, "/workboard/tasks", form, ck)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("%s → %d loc=%s body=%s", form.Get("work_type"), rec.Code, rec.Header().Get("Location"), rec.Body.String())
		}
		loc := rec.Header().Get("Location")
		if !strings.HasPrefix(loc, redir) {
			t.Fatalf("복귀 경로 %s want prefix %s", loc, redir)
		}
	}
	page := doGetWith(t, e, redir, ck).Body.String()
	for _, title := range []string{"행정등록95-20A", "영업등록95-20A", "작업등록95-20A"} {
		if !strings.Contains(page, title) {
			t.Fatalf("목록에 %s 없음", title)
		}
	}
}

func TestSafeWorkboardRedirect(t *testing.T) {
	if got := safeWorkboardRedirect("/workboard/board?view=week&date=2026-10-24", "/workboard/kanban"); got != "/workboard/board?view=week&date=2026-10-24" {
		t.Fatalf("허용 경로 %s", got)
	}
	for _, raw := range []string{"https://evil.example/x", "//evil", "/workboard/board\nhttps://x", "http://localhost/workboard/board"} {
		if got := safeWorkboardRedirect(raw, "/workboard/kanban"); got != "/workboard/kanban" {
			t.Fatalf("%q → %s", raw, got)
		}
	}
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("redirect="+url.QueryEscape("https://evil.test")))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	c := e.NewContext(req, httptest.NewRecorder())
	if got := redirectBack(c, "ok=task"); got != "/workboard/kanban?ok=task" {
		t.Fatalf("redirectBack %s", got)
	}
}

func TestNewWorkFormSharesFieldNames(t *testing.T) {
	files := registerParseFiles(t)
	files = append(files, filepath.Join(findTemplateRoot(t), "workboard", "new_work_form.html"))
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		t.Fatal(err)
	}
	kinds := []string{"as", "maintenance", "project_work", "admin"}
	named := map[string]string{
		"as": "wb_form_as", "maintenance": "wb_form_maintenance",
		"project_work": "wb_form_project_work", "admin": "wb_form_admin_sales",
	}
	base := map[string]interface{}{
		"Date": "2026-10-24", "Today": "2026-10-24", "CanWrite": true,
		"ModalRedirect": "/workboard/register", "Assignees": nil, "WorkGroups": nil,
		"ASCards": []model.WBCard{}, "MntCards": []model.WBCard{},
		"Customers": nil, "Projects": nil, "WorkProjects": nil,
	}
	for _, kind := range kinds {
		var a, b bytes.Buffer
		data := map[string]interface{}{}
		for k, v := range base {
			data[k] = v
		}
		data["DailyKind"] = kind
		if err := tmpl.ExecuteTemplate(&a, named[kind], data); err != nil {
			t.Fatalf("%s named: %v", kind, err)
		}
		data["KindBackLabel"] = "← 업무 유형 선택"
		data["ModalRedirect"] = "/workboard/board?view=list&date=2026-10-24"
		if err := tmpl.ExecuteTemplate(&b, "content", data); err != nil {
			t.Fatalf("%s fragment: %v", kind, err)
		}
		if strings.Join(formFieldNames(a.String()), ",") != strings.Join(formFieldNames(b.String()), ",") {
			t.Fatalf("%s name 목록 불일치\n named=%v\n frag=%v", kind, formFieldNames(a.String()), formFieldNames(b.String()))
		}
	}
}

func formFieldNames(html string) []string {
	re := regexp.MustCompile(`name="([^"]+)"`)
	hit := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		hit[m[1]] = true
	}
	var out []string
	for n := range hit {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func TestNewWorkFormQueryCountIndependentOfTasks(t *testing.T) {
	var n int64
	qcount9520N.Store(&n)
	t.Cleanup(func() { qcount9520N.Store(nil) })
	path := filepath.Join(t.TempDir(), "nw-q.db")
	e, _, me, _, repo := seedPersonalBoard(t, sqliteQCount9520, path)
	today := time.Now().Format(dateLayout)
	for i := 0; i < 20; i++ {
		mustCreateBoardTask(t, repo, "nq20", today, "09:00", "10:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	}
	ck := jwtCookieUser(t, me)
	atomic.StoreInt64(&n, 0)
	if rec := doGetWith(t, e, "/workboard/new-work/form?kind=admin", ck); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	q20 := atomic.LoadInt64(&n)
	for i := 0; i < 180; i++ {
		mustCreateBoardTask(t, repo, "nq200", today, "11:00", "12:00", model.OrgIDLibrary, me.FullName, me.UserID, model.WBTaskWaiting, model.WBWorkAdmin)
	}
	atomic.StoreInt64(&n, 0)
	if rec := doGetWith(t, e, "/workboard/new-work/form?kind=admin", ck); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	q200 := atomic.LoadInt64(&n)
	if q200 > q20+4 {
		t.Fatalf("조각 쿼리가 업무 건수에 비례 q20=%d q200=%d", q20, q200)
	}
}
