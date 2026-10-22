package handler

import (
	"database/sql"
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

func seedWorkGroupForBoard(t *testing.T, db *sql.DB, users *repository.UserRepo) (*model.WorkGroup, *model.User, *model.User) {
	t.Helper()
	tech := &model.User{Username: "gtech", PasswordHash: "x", FullName: "그룹장원", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	off := &model.User{Username: "goff", PasswordHash: "x", FullName: "그룹원", Role: model.RoleOffice, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	for _, u := range []*model.User{tech, off} {
		if err := users.Create(u); err != nil {
			t.Fatal(err)
		}
	}
	wg := repository.NewWorkGroupRepo(db)
	admin, err := users.GetByUsername("admin")
	if err != nil || admin == nil {
		t.Fatal("admin")
	}
	g := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "등록그룹"}
	a := model.WorkGroupActor{UserID: admin.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleOrgAdmin}
	if err := wg.Create(a, g); err != nil {
		t.Fatal(err)
	}
	_ = wg.AddMember(a, g.GroupID, tech.UserID, model.WorkGroupMember, "")
	_ = wg.AddMember(a, g.GroupID, off.UserID, model.WorkGroupMember, "")
	if err := wg.SetLeader(a, g.GroupID, tech.UserID); err != nil {
		t.Fatal(err)
	}
	return g, tech, off
}

func taskGroupID(t *testing.T, db *sql.DB, taskID string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT COALESCE(work_group_id,'') FROM work_tasks WHERE task_id=?`, taskID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestWorkboardRegisterUsesWorkGroupFourKinds(t *testing.T) {
	e, repo, users, db := newProjectWorkHarness(t)
	g, tech, off := seedWorkGroupForBoard(t, db, users)

	page := doGet(t, e, "/workboard/register?view=day")
	if page.Code != http.StatusOK {
		t.Fatalf("register %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "등록그룹") || !strings.Contains(body, "name=\"work_group_id\"") {
		t.Fatal("등록 폼에 업무 그룹 없음")
	}

	adminOK := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":     {"admin"},
		"title":         {"그룹행정"},
		"work_date":     {"2026-10-21"},
		"start_time":    {"09:00"},
		"end_time":      {"09:30"},
		"assignee":      {tech.FullName},
		"work_group_id": {g.GroupID},
		"member_name":   {off.FullName},
		"redirect":      {"/workboard/register"},
	})
	if adminOK.Code != http.StatusSeeOther {
		t.Fatalf("admin %d %s", adminOK.Code, adminOK.Body.String())
	}

	pwOK := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":     {"project_work"},
		"title":         {"그룹작업"},
		"work_subtype":  {"build"},
		"work_date":     {"2026-10-21"},
		"assignee":      {tech.FullName},
		"start_time":    {"10:00"},
		"end_time":      {"10:30"},
		"work_group_id": {g.GroupID},
		"member_name":   {off.FullName},
		"redirect":      {"/workboard/register"},
	})
	if pwOK.Code != http.StatusSeeOther {
		t.Fatalf("pw %d %s", pwOK.Code, pwOK.Body.String())
	}

	if _, err := db.Exec(`INSERT INTO as_receipts (as_id, as_number, receipt_datetime, customer_id, symptom, status, assigned_to, org_id)
		VALUES ('AS-g13','A-g13','2026-10-21 09:00','C-high','증상','received',?,?)`, tech.FullName, model.OrgIDLibrary); err != nil {
		t.Fatal(err)
	}
	asOK := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":     {"as"},
		"source_id":     {"AS-g13"},
		"work_date":     {"2026-10-21"},
		"start_time":    {"11:00"},
		"end_time":      {"11:30"},
		"assignee":      {tech.FullName},
		"work_group_id": {g.GroupID},
		"member_name":   {off.FullName},
		"redirect":      {"/workboard/register"},
	})
	if asOK.Code != http.StatusSeeOther {
		t.Fatalf("as %d %s", asOK.Code, asOK.Body.String())
	}

	seedVisit(t, db, "V-g13", "2026-10-21", tech.FullName, "KLAS", "계룡고")
	mntOK := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type":     {"maintenance"},
		"source_id":     {"V-g13"},
		"work_date":     {"2026-10-21"},
		"start_time":    {"13:00"},
		"end_time":      {"13:30"},
		"assignee":      {tech.FullName},
		"work_group_id": {g.GroupID},
		"member_name":   {off.FullName},
		"redirect":      {"/workboard/register"},
	})
	if mntOK.Code != http.StatusSeeOther {
		t.Fatalf("mnt %d %s", mntOK.Code, mntOK.Body.String())
	}

	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	var adminID, asID string
	for _, tk := range tasks {
		gid := taskGroupID(t, db, tk.TaskID)
		if gid != g.GroupID {
			continue
		}
		kinds[tk.WorkType] = true
		if tk.WorkType == model.WBWorkAdmin {
			adminID = tk.TaskID
		}
		if tk.WorkType == model.WBWorkAS || tk.SourceType == model.WBSourceAS {
			asID = tk.TaskID
		}
		ms, _ := repo.ListMembers(tk.TaskID)
		hasOff := false
		for _, m := range ms {
			if m.Assignee == off.FullName {
				hasOff = true
			}
		}
		if !hasOff {
			t.Fatalf("%s 참여자 없음", tk.TaskID)
		}
	}
	for _, k := range []string{model.WBWorkAdmin, model.WBWorkProjectWork, model.WBWorkAS, model.WBWorkMaintenance} {
		if !kinds[k] {
			t.Fatalf("종류 미저장 %s %+v", k, kinds)
		}
	}

	closed := &model.WorkGroup{OrgID: model.OrgIDLibrary, Name: "끝난그룹"}
	adminU, _ := users.GetByUsername("admin")
	a := model.WorkGroupActor{UserID: adminU.UserID, OrgID: model.OrgIDLibrary, Role: model.RoleOrgAdmin}
	wg := repository.NewWorkGroupRepo(db)
	if err := wg.Create(a, closed); err != nil {
		t.Fatal(err)
	}
	_ = wg.Close(a, closed.GroupID)
	bad := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"거부"}, "work_date": {"2026-10-21"},
		"start_time": {"14:00"}, "end_time": {"14:30"}, "assignee": {tech.FullName},
		"work_group_id": {closed.GroupID}, "redirect": {"/workboard/register"},
	})
	if bad.Code == http.StatusSeeOther && strings.Contains(bad.Header().Get(echo.HeaderLocation), "ok=") {
		t.Fatal("종료 그룹 저장됨")
	}

	o3 := &model.WorkGroup{OrgID: "O03", Name: "SI그룹"}
	a3 := model.WorkGroupActor{UserID: adminU.UserID, OrgID: "O03", Role: model.RoleVisionAdmin}
	if err := wg.Create(a3, o3); err != nil {
		t.Fatal(err)
	}
	cross := doForm(t, e, "/workboard/tasks", url.Values{
		"work_type": {"admin"}, "title": {"타조직"}, "work_date": {"2026-10-21"},
		"start_time": {"15:00"}, "end_time": {"15:30"}, "assignee": {tech.FullName},
		"work_group_id": {o3.GroupID}, "redirect": {"/workboard/register"},
	})
	if cross.Code == http.StatusSeeOther && strings.Contains(cross.Header().Get(echo.HeaderLocation), "ok=") {
		t.Fatal("타 조직 그룹 저장됨")
	}

	techCK := jwtCookieUserClaims(t, tech)
	techPage := doGetWith(t, e, "/workboard/register?view=day", techCK)
	if techPage.Code != http.StatusOK {
		t.Fatalf("tech register %d", techPage.Code)
	}
	if !strings.Contains(techPage.Body.String(), "등록그룹") {
		t.Fatal("소속 그룹이 안 보임")
	}

	if adminID == "" {
		t.Fatal("행정 업무 없음")
	}
	show := doGet(t, e, "/workboard/tasks/"+adminID)
	if !strings.Contains(show.Body.String(), "그룹: 등록그룹") {
		t.Fatal("상세 칩 없음")
	}
	if !strings.Contains(show.Body.String(), "그룹 구성원으로 다시 채우기") {
		t.Fatal("다시 채우기 없음")
	}
	cal := doGet(t, e, "/workboard/register?view=day&date=2026-10-21")
	if !strings.Contains(cal.Body.String(), "그룹: 등록그룹") {
		t.Fatal("일정표 칩 없음")
	}
	filt := doGet(t, e, "/workboard/register?view=day&date=2026-10-21&work_group_id="+g.GroupID)
	if filt.Code != http.StatusOK {
		t.Fatalf("filter %d", filt.Code)
	}

	_ = wg.AddMember(a, g.GroupID, adminU.UserID, model.WorkGroupMember, "")
	ms, _ := repo.ListMembers(adminID)
	for _, m := range ms {
		if m.Assignee == "관리자" {
			t.Fatal("구성원 추가가 기존 업무에 소급됨")
		}
	}
	refill := doForm(t, e, "/workboard/tasks/"+adminID+"/refill-group", url.Values{"redirect": {"/workboard/tasks/" + adminID}})
	if refill.Code != http.StatusSeeOther {
		t.Fatalf("refill %d", refill.Code)
	}
	ms2, _ := repo.ListMembers(adminID)
	foundAdmin := false
	for _, m := range ms2 {
		if m.Assignee == "관리자" {
			foundAdmin = true
		}
	}
	if !foundAdmin {
		t.Fatal("다시 채우기 후 구성원 없음")
	}

	if asID == "" {
		t.Fatal("AS 업무 없음")
	}
	var src string
	_ = db.QueryRow(`SELECT COALESCE(assignee_source,'') FROM work_tasks WHERE task_id=?`, asID).Scan(&src)
	if src != model.WBAssigneeSourceManual {
		t.Fatalf("그룹 지정 담당자가 manual 아님 %q", src)
	}
	as, err := repository.NewASRepo(db).GetByID(model.OrgIDLibrary, "AS-g13")
	if err != nil || as == nil {
		t.Fatal(err)
	}
	as.AssignedTo = "다른사람"
	as.AssignedUserID = ""
	if err := repo.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.GetTask(asID)
	if got.Assignee != tech.FullName {
		t.Fatalf("AS 동기화가 덮어씀 %q", got.Assignee)
	}
}

func TestWorkboardGroupModalMarkup(t *testing.T) {
	root := findTemplateRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "workboard", "_modals.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	n := strings.Count(s, `name="work_group_id"`) + strings.Count(s, `template "wb_work_group_field"`)
	if n < 4 {
		t.Fatalf("그룹 선택 %d", n)
	}
	js, err := os.ReadFile(filepath.Join(findStaticRoot(t), "js", "work-group-task.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "work_group_id") || !strings.Contains(string(js), "memberRows") {
		t.Fatal("그룹 선택 시 담당자 채움 스크립트 없음")
	}
}
