package handler

import (
	"bytes"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func TestMergeVisitAssigneeIDsGroupExcludeAndExtra(t *testing.T) {
	got := mergeVisitAssigneeIDs([]string{"u1", "u2", "u3"}, []string{"u4"}, []string{"u2"})
	want := []string{"u1", "u3", "u4"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
	if n := mergeVisitAssigneeIDs([]string{"u1", "u2"}, nil, nil); strings.Join(n, ",") != "u1,u2" {
		t.Fatalf("그룹만 %v", n)
	}
}

func newMnt9514AServer(t *testing.T) (*echo.Echo, *sqlOrRepo14a) {
	t.Helper()
	db, err := repository.InitDB(filepath.Join(t.TempDir(), "h14a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT OR IGNORE INTO orgs(org_id,org_no,org_name,kind,is_active,sort_order) VALUES
		('O03','O03','SI사업팀','business',1,30)`); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepo(db)
	users.EnsureAdmin(HashPassword("admin"))
	oa := &model.User{Username: "oa14a", PasswordHash: HashPassword("x"), FullName: "도서관장", Role: model.RoleOrgAdmin, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	si := &model.User{Username: "si14a", PasswordHash: HashPassword("x"), FullName: "에스아이", Role: model.RoleOrgAdmin, IsActive: true, OrgID: "O03", ProfileDone: true}
	a := &model.User{Username: "a14a", PasswordHash: "x", FullName: "김서준", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	b := &model.User{Username: "b14a", PasswordHash: "x", FullName: "박지훈", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	c := &model.User{Username: "c14a", PasswordHash: "x", FullName: "최유진", Role: model.RoleOffice, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	siU := &model.User{Username: "su14a", PasswordHash: "x", FullName: "에스원", Role: model.RoleTech, IsActive: true, OrgID: "O03", ProfileDone: true}
	for _, u := range []*model.User{oa, si, a, b, c, siU} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c14a','해미도서관','해미도서관',1,?),('c14b','SI학교','SI학교',1,'O03')`, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewMaintenanceRepo(db)
	_ = repo.UpsertSiteConfig(&model.MaintenanceSiteConfig{
		CustomerID: "c14a", ShortName: "해미", HasKlas: true, InspectionCycle: "monthly", OrgID: model.OrgIDLibrary,
	})
	wg := repository.NewWorkGroupRepo(db)
	gLib := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "수도권 점검팀"}
	if err := wg.CreateWithMembers(model.WorkGroupActor{UserID: oa.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleOrgAdmin}, gLib, []string{a.UserID, b.UserID, c.UserID}, a.UserID); err != nil {
		t.Fatal(err)
	}
	gOff := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "닫힌팀"}
	if err := wg.CreateWithMembers(model.WorkGroupActor{UserID: oa.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleOrgAdmin}, gOff, []string{b.UserID}, b.UserID); err != nil {
		t.Fatal(err)
	}
	if err := wg.Close(model.WorkGroupActor{UserID: oa.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleOrgAdmin}, gOff.GroupID); err != nil {
		t.Fatal(err)
	}
	gSI := &model.WorkGroup{OrgID: "O03", Name: "SI점검팀"}
	if err := wg.CreateWithMembers(model.WorkGroupActor{UserID: si.UserID, OrgID: "O03", Role: model.RoleOrgAdmin}, gSI, []string{siU.UserID}, siU.UserID); err != nil {
		t.Fatal(err)
	}
	gMid := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "중부권 점검팀"}
	if err := wg.CreateWithMembers(model.WorkGroupActor{UserID: oa.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleOrgAdmin}, gMid, []string{a.UserID, c.UserID}, a.UserID); err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	e.Renderer = NewRenderer()
	AttachHTTPErrorHandler(e)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/maintenance/:id", h.Maintenance.ShowPlan)
	g.POST("/maintenance/:id/visits", h.Maintenance.AddVisit)
	g.POST("/maintenance/visits/:visit_id/update", h.Maintenance.UpdateVisit)
	g.POST("/maintenance/:id/assign-slot", h.Maintenance.AssignUnassignedSlot)
	return e, &sqlOrRepo14a{repo: repo, users: users, oa: oa, a: a, b: b, c: c, siU: siU, gLib: gLib.GroupID, gSI: gSI.GroupID, gOff: gOff.GroupID, gMid: gMid.GroupID}
}

type sqlOrRepo14a struct {
	repo             *repository.MaintenanceRepo
	users            *repository.UserRepo
	oa, a, b, c, siU *model.User
	gLib, gSI, gOff, gMid string
}

func TestMnt95_14AShowWorkGroupsOrgIsolation(t *testing.T) {
	e, fx := newMnt9514AServer(t)
	p, err := fx.repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	oaC := jwtCookieUserClaims(t, fx.oa)
	rec := doGetWith(t, e, "/maintenance/"+p.PlanID+"?view=calendar&month=10", oaC)
	if rec.Code != http.StatusOK {
		t.Fatalf("show %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "수도권 점검팀 (3명)") {
		t.Fatal("자기 조직 활성 그룹")
	}
	if strings.Contains(body, "SI점검팀") {
		t.Fatal("타 조직 그룹")
	}
	if strings.Contains(body, "닫힌팀") {
		t.Fatal("비활성 그룹")
	}
	if strings.Contains(body, `<select name="assignee"`) {
		t.Fatal("단일 담당 select 가 남아 있음")
	}
	if strings.Contains(body, `<select name="work_group_id"`) {
		t.Fatal("별도 업무 그룹 select")
	}
	if !strings.Contains(body, `name="assignee_user_id"`) || !strings.Contains(body, `name="assignee_group_id"`) || !strings.Contains(body, "이름 검색") {
		t.Fatal("체크박스 다중 선택 컴포넌트")
	}
	if !strings.Contains(body, "keydown.escape") {
		t.Fatal("ESC 닫힘")
	}
}

func TestMnt95_14AAddVisitThreeAssignees(t *testing.T) {
	e, fx := newMnt9514AServer(t)
	p, err := fx.repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	rec := doFormUser(t, e, "/maintenance/"+p.PlanID+"/visits", url.Values{
		"visit_date":       {"2026-10-28"},
		"customer_id":      {"c14a"},
		"product_type":     {"KLAS"},
		"assignee_multi":   {"1"},
		"assignee_user_id": {fx.a.UserID, fx.b.UserID, fx.c.UserID},
		"owner_user_id":    {fx.a.UserID},
	}, jwtCookieUserClaims(t, fx.oa))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("add %d %s", rec.Code, rec.Body.String())
	}
	visits, _ := fx.repo.ListVisits(p.PlanID)
	if len(visits) != 1 {
		t.Fatalf("visits %d", len(visits))
	}
	got, err := fx.repo.ListVisitAssignees(visits[0].VisitID)
	if err != nil || len(got) != 3 {
		t.Fatalf("assignees %v %v", got, err)
	}
	owners := 0
	for _, it := range got {
		if it.Role == model.WBMemberOwner {
			owners++
			if it.UserID != fx.a.UserID {
				t.Fatalf("주담당 %s", it.UserID)
			}
		}
	}
	if owners != 1 {
		t.Fatalf("owner n=%d", owners)
	}
	fresh, _ := fx.repo.GetVisit(visits[0].VisitID)
	if fresh == nil || fresh.Assignee != fx.a.FullName {
		t.Fatalf("미러 assignee=%v", fresh)
	}
}

func TestMnt95_14AAddVisitGroupOnlyAndExclude(t *testing.T) {
	e, fx := newMnt9514AServer(t)
	p, err := fx.repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUserClaims(t, fx.oa)
	only := doFormUser(t, e, "/maintenance/"+p.PlanID+"/visits", url.Values{
		"visit_date":        {"2026-10-28"},
		"customer_id":       {"c14a"},
		"product_type":      {"KLAS"},
		"assignee_group_id": {fx.gLib},
		"assignee_multi":    {"1"},
	}, ck)
	if only.Code != http.StatusSeeOther {
		t.Fatalf("group only %d %s", only.Code, only.Body.String())
	}
	visits, _ := fx.repo.ListVisits(p.PlanID)
	got, _ := fx.repo.ListVisitAssignees(visits[0].VisitID)
	if len(got) != 3 {
		t.Fatalf("그룹 전원 %d %+v", len(got), got)
	}
	ownerOK := false
	for _, it := range got {
		if it.Role == model.WBMemberOwner && it.UserID == fx.a.UserID {
			ownerOK = true
		}
	}
	if !ownerOK {
		t.Fatal("그룹장 owner")
	}
	if visits[0].WorkGroupID != fx.gLib {
		t.Fatalf("대표 그룹 %s", visits[0].WorkGroupID)
	}
	for _, it := range got {
		if it.WorkGroupID != fx.gLib {
			t.Fatalf("행 그룹 %s", it.WorkGroupID)
		}
	}

	ex := doFormUser(t, e, "/maintenance/"+p.PlanID+"/visits", url.Values{
		"visit_date":               {"2026-10-29"},
		"customer_id":              {"c14a"},
		"product_type":             {"앤로보틱스"},
		"assignee_group_id":        {fx.gLib},
		"assignee_multi":           {"1"},
		"assignee_user_id":         {fx.a.UserID, fx.c.UserID},
		"assignee_exclude_user_id": {fx.b.UserID},
		"owner_user_id":            {fx.a.UserID},
		"round_no":                 {"1"},
	}, ck)
	if ex.Code != http.StatusSeeOther {
		t.Fatalf("exclude %d loc=%s body=%s", ex.Code, ex.Header().Get("Location"), ex.Body.String())
	}
	visits, _ = fx.repo.ListVisits(p.PlanID)
	var second *model.MaintenanceVisit
	for i := range visits {
		if visits[i].ProductType == "앤로보틱스" {
			second = &visits[i]
		}
	}
	if second == nil {
		t.Fatal("제외 방문 없음")
	}
	got, _ = fx.repo.ListVisitAssignees(second.VisitID)
	if len(got) != 2 {
		t.Fatalf("제외 후 %d %+v", len(got), got)
	}
	for _, it := range got {
		if it.UserID == fx.b.UserID {
			t.Fatal("해제한 사람이 저장됨")
		}
		if it.WorkGroupID != fx.gLib {
			t.Fatalf("행 그룹 %s", it.WorkGroupID)
		}
	}
}

func TestMnt95_14ARejectForeignUserAndGroup(t *testing.T) {
	e, fx := newMnt9514AServer(t)
	p, err := fx.repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUserClaims(t, fx.oa)
	badUser := doFormUser(t, e, "/maintenance/"+p.PlanID+"/visits", url.Values{
		"visit_date":       {"2026-10-28"},
		"customer_id":      {"c14a"},
		"product_type":     {"KLAS"},
		"assignee_multi":   {"1"},
		"assignee_user_id": {fx.siU.UserID},
	}, ck)
	if badUser.Code != http.StatusBadRequest {
		t.Fatalf("타 조직 사용자 %d %s", badUser.Code, badUser.Body.String())
	}
	badG := doFormUser(t, e, "/maintenance/"+p.PlanID+"/visits", url.Values{
		"visit_date":        {"2026-10-28"},
		"customer_id":       {"c14a"},
		"product_type":      {"KLAS"},
		"assignee_group_id": {fx.gSI},
		"assignee_multi":    {"1"},
	}, ck)
	if badG.Code != http.StatusBadRequest {
		t.Fatalf("타 조직 그룹 %d %s", badG.Code, badG.Body.String())
	}
	visits, _ := fx.repo.ListVisits(p.PlanID)
	if len(visits) != 0 {
		t.Fatalf("위변조 저장됨 %d", len(visits))
	}
}

func TestMnt95_14AUpdateVisitDropsOneAssignee(t *testing.T) {
	e, fx := newMnt9514AServer(t)
	p, err := fx.repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	ck := jwtCookieUserClaims(t, fx.oa)
	_ = doFormUser(t, e, "/maintenance/"+p.PlanID+"/visits", url.Values{
		"visit_date":       {"2026-10-28"},
		"customer_id":      {"c14a"},
		"product_type":     {"KLAS"},
		"assignee_multi":   {"1"},
		"assignee_user_id": {fx.a.UserID, fx.b.UserID},
		"owner_user_id":    {fx.a.UserID},
	}, ck)
	visits, _ := fx.repo.ListVisits(p.PlanID)
	if len(visits) != 1 {
		t.Fatal(len(visits))
	}
	upd := doFormUser(t, e, "/maintenance/visits/"+visits[0].VisitID+"/update", url.Values{
		"plan_id":          {p.PlanID},
		"visit_date":       {"2026-10-28"},
		"customer_id":      {"c14a"},
		"product_type":     {"KLAS"},
		"assignee_multi":   {"1"},
		"assignee_user_id": {fx.a.UserID},
		"owner_user_id":    {fx.a.UserID},
	}, ck)
	if upd.Code != http.StatusSeeOther {
		t.Fatalf("update %d %s", upd.Code, upd.Body.String())
	}
	got, _ := fx.repo.ListVisitAssignees(visits[0].VisitID)
	if len(got) != 1 || got[0].UserID != fx.a.UserID {
		t.Fatalf("수정 반영 %+v", got)
	}
}

func TestMnt95_14AAddVisitTwoGroupsAndOverlap(t *testing.T) {
	e, fx := newMnt9514AServer(t)
	p, err := fx.repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	rec := doFormUser(t, e, "/maintenance/"+p.PlanID+"/visits", url.Values{
		"visit_date":        {"2026-10-30"},
		"customer_id":       {"c14a"},
		"product_type":      {"KLAS"},
		"assignee_multi":    {"1"},
		"assignee_group_id": {fx.gLib, fx.gMid},
		"assignee_user_id":  {fx.a.UserID, fx.b.UserID},
		"owner_user_id":     {fx.a.UserID},
	}, jwtCookieUserClaims(t, fx.oa))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("two groups %d %s", rec.Code, rec.Body.String())
	}
	visits, _ := fx.repo.ListVisits(p.PlanID)
	if len(visits) != 1 {
		t.Fatalf("visits %d", len(visits))
	}
	if visits[0].WorkGroupID != fx.gLib {
		t.Fatalf("대표 그룹 %s", visits[0].WorkGroupID)
	}
	got, _ := fx.repo.ListVisitAssignees(visits[0].VisitID)
	if len(got) != 3 {
		t.Fatalf("겹침 중복 %d %+v", len(got), got)
	}
	by := map[string]model.MaintenanceVisitAssignee{}
	for _, it := range got {
		by[it.UserID] = it
	}
	if by[fx.a.UserID].WorkGroupID != fx.gLib {
		t.Fatalf("겹친 구성원 그룹 %s", by[fx.a.UserID].WorkGroupID)
	}
	if by[fx.b.UserID].WorkGroupID != fx.gLib {
		t.Fatalf("수도권만 %s", by[fx.b.UserID].WorkGroupID)
	}
	if by[fx.c.UserID].WorkGroupID == "" {
		t.Fatal("중부/수도권 구성원 그룹 없음")
	}
}

func TestMnt95_14AAssignSlotMultiAssignees(t *testing.T) {
	e, fx := newMnt9514AServer(t)
	p, err := fx.repo.CreatePlanForOrg(model.OrgIDLibrary, 2026, "t")
	if err != nil {
		t.Fatal(err)
	}
	rec := doFormUser(t, e, "/maintenance/"+p.PlanID+"/assign-slot", url.Values{
		"visit_date":       {"2026-10-28"},
		"customer_id":      {"c14a"},
		"product_type":     {"KLAS"},
		"assignee_multi":   {"1"},
		"assignee_user_id": {fx.a.UserID, fx.b.UserID},
		"owner_user_id":    {fx.a.UserID},
	}, jwtCookieUserClaims(t, fx.oa))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("slot %d %s", rec.Code, rec.Body.String())
	}
	visits, _ := fx.repo.ListVisits(p.PlanID)
	if len(visits) != 1 {
		t.Fatalf("visits %d", len(visits))
	}
	got, _ := fx.repo.ListVisitAssignees(visits[0].VisitID)
	if len(got) != 2 {
		t.Fatalf("slot assignees %d", len(got))
	}
}

func TestMnt95_14APlanShowAssigneeComponentRender(t *testing.T) {
	root := findTemplateRoot(t)
	files := []string{
		filepath.Join(root, "layout", "base.html"),
		filepath.Join(root, "layout", "_nav_tree.html"),
		filepath.Join(root, "maintenance", "plan_show.html"),
		filepath.Join(root, "maintenance", "_assignee_multi.html"),
	}
	tmpl, err := template.New("").Funcs(funcMap()).ParseFiles(files...)
	if err != nil {
		t.Fatal(err)
	}
	data := planViewData(mntViewCalendar, 10, sampleVisits())
	data["Assignees"] = []model.User{{UserID: "u1", FullName: "김서준", Role: model.RoleTech}}
	data["WorkGroups"] = []model.WorkGroup{{GroupID: "g1", Name: "수도권 점검팀", MemberN: 3}}
	data["CanEdit"] = true
	data["Unassigned"] = []model.UnassignedMonthSlot{{CustomerID: "c1", ShortName: "대기", ProductType: "KLAS"}}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, `<select name="assignee"`) {
		t.Fatal("단일 select name=assignee")
	}
	if strings.Contains(out, `<select name="work_group_id"`) {
		t.Fatal("별도 select name=work_group_id")
	}
	if !strings.Contains(out, `type="checkbox"`) || !strings.Contains(out, "이름 검색") {
		t.Fatal("체크박스·검색")
	}
	if !strings.Contains(out, "수도권 점검팀 (3명)") {
		t.Fatal("그룹 옵션")
	}
	if strings.Count(out, ">업무 그룹<") < 1 || strings.Count(out, ">담당자<") < 1 {
		t.Fatal("업무 그룹·담당자 섹션")
	}
	if strings.Count(out, `data-mnt-assignee-multi`) < 2 {
		t.Fatal("추가·수정에 공통 컴포넌트")
	}
}

func TestMnt95_14AAssigneePartialUILintZero(t *testing.T) {
	root := findTemplateRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "maintenance", "_assignee_multi.html"))
	if err != nil {
		t.Fatal(err)
	}
	got := countUILint(string(b))
	if !got.zero() {
		t.Fatalf("ui_lint %+v", got)
	}
}
