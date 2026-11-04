package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// 95-99 T2 화면군. 새 조회 화면을 main.go 에 넣으면 이 목록에도 넣어야 한다.
var policyMatrixGroups = []string{
	"/projects", "/sales", "/as", "/meeting", "/workboard",
	"/maintenance", "/stats", "/customers", "/attachments", "/quotes",
}

func Test95_99_T2_RouteCoverage(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "cmd", "server", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, g := range policyMatrixGroups {
		if !strings.Contains(body, `"`+g) && !strings.Contains(body, `"`+g+`"`) {
			if g == "/attachments" && !strings.Contains(body, "/attachments") {
				t.Errorf("main.go 에 %s 없음", g)
			}
		}
		if g != "/attachments" && !strings.Contains(body, g) {
			t.Errorf("main.go 라우트 누락 %s", g)
		}
	}
}

type policyUserSet struct {
	vis, cl, oa01, oa02, tech, sales, support, obs *model.User
	asO01, asO02, attID                            string
}

func newPolicyMatrixApp(t *testing.T) (*echo.Echo, policyUserSet) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "p99.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order)
		VALUES('O02','O02','미디어사업팀','business',1,20)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	_ = users.EnsureAdmin(HashPassword("adminadmin"))
	mk := func(un, name, role, org string, ro bool) *model.User {
		u := &model.User{
			Username: un, PasswordHash: HashPassword("password10"), FullName: name,
			Role: role, IsActive: true, OrgID: org, ProfileDone: true, IsReadOnly: ro,
		}
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
		got, _ := users.GetByUsername(un)
		return got
	}
	out := policyUserSet{
		vis:     mk("vis99", "비전", model.RoleVisionAdmin, model.OrgIDLibrary, false),
		cl:      mk("cl99", "씨레벨", model.RoleCLevel, "O08", true),
		oa01:    mk("oa99a", "도관리", model.RoleOrgAdmin, model.OrgIDLibrary, false),
		oa02:    mk("oa99b", "미관리", model.RoleOrgAdmin, "O02", false),
		tech:    mk("tech99", "기술", model.RoleTech, model.OrgIDLibrary, false),
		sales:   mk("sales99", "영업", model.RoleSales, model.OrgIDLibrary, false),
		support: mk("sup99", "지원", model.RoleSupport, model.OrgIDLibrary, false),
		obs:     mk("obs99", "옵저버", model.RoleObserver, model.OrgIDLibrary, true),
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active) VALUES (?,?,?,1)`,
		"cust99", "매트릭스도서관", "매트릭스도서관"); err != nil {
		t.Fatal(err)
	}
	asRepo := repository.NewASRepo(db)
	a1 := &model.ASReceipt{
		CustomerID: "cust99", ReceiptChannel: "phone", Requester: "홍",
		Symptom: "O01증상", Urgency: "normal", Priority: "normal",
		AssignedTo: "기술", ReceiptDatetime: time.Date(2026, 8, 10, 9, 0, 0, 0, time.Local), Status: "received",
		OrgID: model.OrgIDLibrary,
	}
	if err := asRepo.Create(a1); err != nil {
		t.Fatal(err)
	}
	a2 := &model.ASReceipt{
		CustomerID: "cust99", ReceiptChannel: "phone", Requester: "김",
		Symptom: "O02증상고유", Urgency: "normal", Priority: "normal",
		AssignedTo: "이다른", ReceiptDatetime: time.Date(2026, 8, 10, 9, 0, 0, 0, time.Local), Status: "received",
		OrgID: "O02",
	}
	if err := asRepo.Create(a2); err != nil {
		t.Fatal(err)
	}
	out.asO01, out.asO02 = a1.ASID, a2.ASID
	fp := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(fp, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	att := &model.Attachment{RefType: model.RefTypeAS, RefID: a2.ASID, FileName: "f.txt", FilePath: fp, MIMEType: "text/plain"}
	if err := repository.NewAttachmentRepo(db).Create(att); err != nil {
		t.Fatal(err)
	}
	out.attID = att.AttachmentID

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.Use(h.Auth.GuardCLevelHidden)
	g.Use(h.Auth.RequireActiveRole)
	g.GET("/as", h.AS.List)
	g.GET("/as/new", h.AS.New)
	g.GET("/as/:id", h.AS.Show)
	g.GET("/as/:id/action", h.AS.Action)
	g.POST("/as/:id/update", h.AS.Update)
	g.GET("/meeting", h.Meeting.Show)
	g.GET("/meeting/board", h.Meeting.Board)
	g.GET("/workboard/board", h.Workboard.Board)
	g.GET("/work", h.Work.List)
	g.GET("/customers", h.Customer.List)
	g.GET("/stats", h.Stats.List)
	g.GET("/attachments/:id", h.Attachment.Download)
	g.GET("/projects", h.Project.List)
	g.GET("/sales", h.Sales.List)
	g.GET("/maintenance", h.Maintenance.ListPlans)
	g.GET("/quotes", h.Quotes.List)
	return e, out
}

func policyGET(t *testing.T, e *echo.Echo, path string, u *model.User) *httptest.ResponseRecorder {
	t.Helper()
	return doGetWith(t, e, path, jwtCookieUser(t, u))
}

func viewAllowed(code int) bool {
	return code == http.StatusOK || code == http.StatusFound || code == http.StatusSeeOther || code == http.StatusMovedPermanently
}

func Test95_99_T2_MatrixIsolation(t *testing.T) {
	e, u := newPolicyMatrixApp(t)
	type row struct {
		name string
		user *model.User
		path string
		want string // allow | hide | write403
	}
	rows := []row{
		{"비전 타조직 상세", u.vis, "/as/" + u.asO02 + "/action", "allow"},
		{"C-Level 타조직 상세", u.cl, "/as/" + u.asO02 + "/action", "allow"},
		{"조직관리자 자기", u.oa01, "/as/" + u.asO01 + "/action", "allow"},
		{"조직관리자 타조직", u.oa01, "/as/" + u.asO02 + "/action", "hide"},
		{"기술 타조직", u.tech, "/as/" + u.asO02 + "/action", "hide"},
		{"영업 타조직", u.sales, "/as/" + u.asO02 + "/action", "hide"},
		{"지원 타조직", u.support, "/as/" + u.asO02 + "/action", "hide"},
		{"옵저버 타조직", u.obs, "/as/" + u.asO02 + "/action", "hide"},
		{"기술 첨부 타조직", u.tech, "/attachments/" + u.attID, "hide"},
		{"C-Level 첨부", u.cl, "/attachments/" + u.attID, "allow"},
		{"C-Level 목록 AS", u.cl, "/as", "allow"},
		{"C-Level 회의", u.cl, "/meeting", "allow"},
		{"C-Level 일일업무", u.cl, "/workboard/board", "allow"},
		{"C-Level 고객(전사)", u.cl, "/customers", "allow"},
		{"C-Level 통계", u.cl, "/stats", "allow"},
		{"C-Level 사업", u.cl, "/projects", "allow"},
		{"C-Level 영업", u.cl, "/sales", "allow"},
		{"C-Level 점검", u.cl, "/maintenance", "allow"},
		{"C-Level 견적", u.cl, "/quotes", "allow"},
		{"기술 자기 목록", u.tech, "/as", "allow"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			rec := policyGET(t, e, r.path, r.user)
			switch r.want {
			case "allow":
				if !viewAllowed(rec.Code) {
					t.Fatalf("%s %d", r.path, rec.Code)
				}
			case "hide":
				if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
					t.Fatalf("차단이어야 함 %s %d", r.path, rec.Code)
				}
			}
		})
	}
}

func Test95_99_T3_CLevelReadNoWrite(t *testing.T) {
	e, u := newPolicyMatrixApp(t)
	ck := jwtCookieUser(t, u.cl)
	for _, path := range []string{"/workboard/board", "/meeting", "/as", "/as/" + u.asO02 + "/action"} {
		rec := policyGET(t, e, path, u.cl)
		if !viewAllowed(rec.Code) {
			t.Fatalf("GET %s %d", path, rec.Code)
		}
	}
	board := policyGET(t, e, "/workboard/board", u.cl)
	if board.Code == http.StatusOK && !strings.Contains(board.Body.String(), "조회 전용") {
		t.Fatal("일일업무 조회 전용 없음")
	}
	post := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/as/"+u.asO01+"/update", strings.NewReader("status=in_progress"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(ck)
	e.ServeHTTP(post, req)
	if post.Code != http.StatusForbidden {
		t.Fatalf("C-Level POST %d", post.Code)
	}
}

func Test95_99_T4_TechListHasNoOtherOrg(t *testing.T) {
	e, u := newPolicyMatrixApp(t)
	rec := policyGET(t, e, "/as", u.tech)
	if rec.Code != http.StatusOK {
		t.Fatalf("/as %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "O02증상고유") {
		t.Fatal("기술 목록에 타 조직 AS")
	}
	hide := policyGET(t, e, "/as/"+u.asO02+"/action", u.tech)
	if hide.Code != http.StatusNotFound {
		t.Fatalf("직접 주소 %d want 404", hide.Code)
	}
}

func Test95_99_T6_MeetingNot403(t *testing.T) {
	e, u := newPolicyMatrixApp(t)
	rec := policyGET(t, e, "/meeting", u.cl)
	if rec.Code == http.StatusForbidden {
		t.Fatal("/meeting 403")
	}
	if !viewAllowed(rec.Code) {
		t.Fatalf("/meeting %d", rec.Code)
	}
}

// Test95_90_CLevelMeetingGETNot403 95-90 회귀: C-Level GET /meeting 이 GuardCLevelHidden 으로 403 이던 문제.
func Test95_90_CLevelMeetingGETNot403(t *testing.T) {
	e, u := newPolicyMatrixApp(t)
	for _, path := range []string{"/meeting", "/meeting/board"} {
		rec := policyGET(t, e, path, u.cl)
		if rec.Code == http.StatusForbidden {
			t.Fatalf("95-90 C-Level GET %s → 403 (풀려야 함)", path)
		}
		if !viewAllowed(rec.Code) {
			t.Fatalf("95-90 C-Level GET %s → %d", path, rec.Code)
		}
	}
	techMeet := policyGET(t, e, "/meeting", u.tech)
	if techMeet.Code == http.StatusForbidden {
		t.Fatal("일반 사용자 /meeting 403")
	}
	oaMeet := policyGET(t, e, "/meeting", u.oa01)
	if oaMeet.Code == http.StatusForbidden {
		t.Fatal("조직관리자 /meeting 403")
	}
	visMeet := policyGET(t, e, "/meeting", u.vis)
	if visMeet.Code == http.StatusForbidden {
		t.Fatal("비전관리자 /meeting 403")
	}
}

func Test95_99_T8_RoleAllowBlockTable(t *testing.T) {
	e, u := newPolicyMatrixApp(t)
	type row struct {
		role, who, path, want string
		user                  *model.User
		method                string
	}
	rows := []row{
		{"비전관리자", "vis", "/as/" + u.asO02 + "/action", "allow", u.vis, http.MethodGet},
		{"비전관리자", "vis", "/meeting", "allow", u.vis, http.MethodGet},
		{"비전관리자", "vis", "/as/" + u.asO01 + "/action", "allow", u.vis, http.MethodGet},
		{"조직관리자", "oa01", "/as/" + u.asO01 + "/action", "allow", u.oa01, http.MethodGet},
		{"조직관리자", "oa01", "/as/" + u.asO02 + "/action", "hide", u.oa01, http.MethodGet},
		{"조직관리자", "oa01", "/meeting", "allow", u.oa01, http.MethodGet},
		{"조직관리자", "oa02", "/as/" + u.asO02 + "/action", "allow", u.oa02, http.MethodGet},
		{"조직관리자", "oa02", "/as/" + u.asO01 + "/action", "hide", u.oa02, http.MethodGet},
		{"일반(기술)", "tech", "/as/" + u.asO01 + "/action", "allow", u.tech, http.MethodGet},
		{"일반(기술)", "tech", "/as/" + u.asO02 + "/action", "hide", u.tech, http.MethodGet},
		{"일반(기술)", "tech", "/meeting", "allow", u.tech, http.MethodGet},
		{"일반(기술)", "tech", "/attachments/" + u.attID, "hide", u.tech, http.MethodGet},
		{"C-Level", "cl", "/meeting", "allow", u.cl, http.MethodGet},
		{"C-Level", "cl", "/as/" + u.asO02 + "/action", "allow", u.cl, http.MethodGet},
		{"C-Level", "cl", "/as/new", "write403", u.cl, http.MethodGet},
		{"C-Level", "cl", "/as/" + u.asO01 + "/update", "write403", u.cl, http.MethodPost},
	}
	for _, r := range rows {
		t.Run(r.role+" "+r.method+" "+r.path, func(t *testing.T) {
			var rec *httptest.ResponseRecorder
			if r.method == http.MethodPost {
				rec = httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, r.path, strings.NewReader("status=in_progress"))
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
				req.AddCookie(jwtCookieUser(t, r.user))
				e.ServeHTTP(rec, req)
			} else {
				rec = policyGET(t, e, r.path, r.user)
			}
			switch r.want {
			case "allow":
				if !viewAllowed(rec.Code) {
					t.Fatalf("want allow got %d", rec.Code)
				}
			case "hide":
				if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
					t.Fatalf("want hide got %d", rec.Code)
				}
			case "write403":
				if rec.Code != http.StatusForbidden {
					t.Fatalf("want 403 got %d", rec.Code)
				}
			}
		})
	}
}
