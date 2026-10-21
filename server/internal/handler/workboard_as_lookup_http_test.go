package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newASLookupHTTP(t *testing.T) (*echo.Echo, *http.Cookie, *http.Cookie, *http.Cookie, *http.Cookie) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "aslookuph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	oa := &model.User{Username: "oa01l", PasswordHash: "x", FullName: "도서관장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	o3 := &model.User{Username: "oa03l", PasswordHash: "x", FullName: "SI장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	cl := &model.User{Username: "cl97", PasswordHash: "x", FullName: "씨레벨", Role: model.RoleCLevel, IsActive: true, IsReadOnly: true, ProfileDone: true}
	vs := &model.User{Username: "vs97", PasswordHash: "x", FullName: "비젼", Role: model.RoleVisionAdmin, IsActive: true, ProfileDone: true}
	for _, u := range []*model.User{oa, o3, cl, vs} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-seoch','서천도서관','서천도서관',1,'O01'),
		       ('c-none','없는고객사','없는고객사',1,'O01'),
		       ('c-si','SI학교','SI학교',1,'O03')`); err != nil {
		t.Fatal(err)
	}
	ts := "2026-10-19 10:00:00"
	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id, created_at, updated_at)
		VALUES ('as-open','R2610-001',?,'c-seoch','문 안 열림','in_progress','도서관담당A','O01',?,?),
		       ('as-place','R2610-075',?,'c-seoch','서천 점검','in_progress','도서관담당A','O01',?,?),
		       ('as-done','R2610-099',?,'c-seoch','완료증상','completed','도서관담당A','O01',?,?),
		       ('as-si','R2610-SI1',?,'c-si','SI증상','in_progress','SI담당C','O03',?,?)`,
		ts, ts, ts, ts, ts, ts, ts, ts, ts, ts, ts, ts); err != nil {
		t.Fatal(err)
	}
	wb := repository.NewWBRepo(db)
	if err := wb.CreateTask(&model.WorkTask{
		WorkType: model.WBWorkAS, Title: "서천배치", OrgID: model.OrgIDLibrary,
		Status: model.WBTaskWaiting, SourceType: model.WBSourceAS, SourceID: "as-place",
		WorkDate: "2026-10-19", StartTime: "09:00", EndTime: "09:30", Assignee: "도서관담당A",
	}); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/register", h.Workboard.Register)
	g.GET("/workboard/as-lookup", h.Workboard.LookupRegisterAS)
	g.GET("/workboard/tasks/:id", h.Workboard.ShowTask)
	g.GET("/as/new", h.AS.New)
	return e, jwtCookieUserClaims(t, oa), jwtCookieUserClaims(t, o3), jwtCookieUserClaims(t, cl), jwtCookieUserClaims(t, vs)
}

type asLookupJSON struct {
	Kind      string `json:"kind"`
	Hint      string `json:"hint"`
	CanCreate bool   `json:"can_create"`
	NewHref   string `json:"new_href"`
	Items     []struct {
		ASID     string `json:"as_id"`
		Number   string `json:"as_number"`
		Kind     string `json:"kind"`
		Status   string `json:"status"`
		TaskHref string `json:"task_href"`
		ASHref   string `json:"as_href"`
		Placed   bool   `json:"placed"`
	} `json:"items"`
}

func getASLookup(t *testing.T, e *echo.Echo, q string, ck *http.Cookie) asLookupJSON {
	t.Helper()
	rec := doGetWith(t, e, "/workboard/as-lookup?q="+url.QueryEscape(q), ck)
	if rec.Code != http.StatusOK {
		t.Fatalf("lookup %q %d %s", q, rec.Code, rec.Body.String())
	}
	var out asLookupJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestWorkboardASLookupHTTP(t *testing.T) {
	e, oa, o3, cl, _ := newASLookupHTTP(t)

	page := doGetWith(t, e, "/workboard/register?view=day&date=2026-10-19&auto=0", oa)
	if page.Code != http.StatusOK {
		t.Fatalf("register %d", page.Code)
	}
	if !strings.Contains(page.Body.String(), `id="as-lookup-status"`) || !strings.Contains(page.Body.String(), `id="as-register-q"`) {
		t.Fatal("안내 영역 없음")
	}
	if !strings.Contains(page.Body.String(), "대기 중인 AS가 없습니다") && !strings.Contains(page.Body.String(), "R2610-001") {
		t.Fatal("열린 AS 선택 목록 회귀")
	}

	short := getASLookup(t, e, "서", oa)
	if short.Kind != "short" || !strings.Contains(short.Hint, "두 글자") || len(short.Items) != 0 {
		t.Fatalf("1글자 %+v", short)
	}
	wild := getASLookup(t, e, "%%", oa)
	if wild.Kind != "missing" {
		t.Fatalf("와일드카드 %+v", wild)
	}

	ok := getASLookup(t, e, "R2610-001", oa)
	if ok.Kind != "ok" {
		t.Fatalf("열림 %+v", ok)
	}

	placed := getASLookup(t, e, "R2610-075", oa)
	if placed.Kind != "placed" || !strings.Contains(placed.Hint, "이미 일일업무로 등록") {
		t.Fatalf("배치 %+v", placed)
	}
	if len(placed.Items) != 1 || !strings.Contains(placed.Items[0].TaskHref, "/workboard/tasks/") {
		t.Fatalf("업무 링크 %+v", placed.Items)
	}

	done := getASLookup(t, e, "R2610-099", oa)
	if done.Kind != "closed" || len(done.Items) != 1 || done.Items[0].ASHref != "/as/as-done" {
		t.Fatalf("완료 %+v", done)
	}

	miss := getASLookup(t, e, "없는고객사", oa)
	if miss.Kind != "missing" || miss.Hint != msgASLookupNone || !miss.CanCreate {
		t.Fatalf("없음 %+v", miss)
	}
	if miss.NewHref != "/as/new?customer_id=c-none" {
		t.Fatalf("추가 링크 %s", miss.NewHref)
	}
	unknown := getASLookup(t, e, "존재하지않는번호XYZ", oa)
	if unknown.Kind != "missing" || unknown.NewHref != "/as/new" {
		t.Fatalf("미특정 %+v", unknown)
	}

	clMiss := getASLookup(t, e, "없는고객사", cl)
	if clMiss.CanCreate {
		t.Fatal("C-Level 추가 노출")
	}

	hidden := getASLookup(t, e, "R2610-075", o3)
	if hidden.Kind != "missing" || hidden.Hint != msgASLookupNone {
		t.Fatalf("타조직 번호 %+v", hidden)
	}
	if strings.Contains(hidden.Hint, "서천") || len(hidden.Items) != 0 {
		t.Fatal("타조직 정보 노출")
	}
	nameHide := getASLookup(t, e, "서천도서관", o3)
	if nameHide.Kind != "missing" || nameHide.NewHref != "/as/new" {
		t.Fatalf("타조직 이름 %+v", nameHide)
	}
	own := getASLookup(t, e, "R2610-SI1", o3)
	if own.Kind != "ok" {
		t.Fatalf("O03 자기 건 %+v", own)
	}
}
