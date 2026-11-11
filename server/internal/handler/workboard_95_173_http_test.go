package handler

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func Test95_173_OrgAdminSeesUnlockOnClosedProjectWork(t *testing.T) {
	e, repo, users, db := newProjectWorkHarness(t)
	if err := repository.NewSettingsRepo(db).EnsureASCompletedEditPassword(HashPassword); err != nil {
		t.Fatal(err)
	}
	id := pwCreateTask(t, e, repo, "잠금해제173")
	done := doForm(t, e, "/workboard/tasks/"+id+"/action", pwActionForm())
	if done.Code != http.StatusSeeOther {
		t.Fatalf("완료 저장 %d %s", done.Code, done.Header().Get("Location"))
	}
	page := doGet(t, e, "/workboard/tasks/"+id+"/action")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "완료 건 수정 잠금 해제") {
		t.Fatal("조직관리자에게 잠금 해제 메뉴가 없다")
	}
	bad := doForm(t, e, "/workboard/tasks/"+id+"/action/unlock-edit", url.Values{"unlock_password": {"nope"}})
	if bad.Code != http.StatusSeeOther || !strings.Contains(bad.Header().Get("Location"), "err=") {
		t.Fatalf("잘못된 비밀번호 %d %s", bad.Code, bad.Header().Get("Location"))
	}
	ok := doForm(t, e, "/workboard/tasks/"+id+"/action/unlock-edit", url.Values{"unlock_password": {"as-edit"}})
	if ok.Code != http.StatusSeeOther || strings.Contains(ok.Header().Get("Location"), "err=") {
		t.Fatalf("잠금 해제 %d %s", ok.Code, ok.Header().Get("Location"))
	}
	saved := doForm(t, e, "/workboard/tasks/"+id+"/action", pwActionForm())
	if saved.Code != http.StatusSeeOther || strings.Contains(saved.Header().Get("Location"), "err=locked") {
		t.Fatalf("해제 후 저장 %d %s", saved.Code, saved.Header().Get("Location"))
	}

	vis := doGetWith(t, e, "/workboard/tasks/"+id+"/action", jwtCookieRole(t, model.RoleVisionAdmin))
	if vis.Code != http.StatusOK || !strings.Contains(vis.Body.String(), "완료 건 수정 잠금 해제") {
		t.Fatal("비전관리자 메뉴가 없다")
	}
	away, err := users.GetByUsername("oa03")
	if err != nil || away == nil {
		t.Fatal(err)
	}
	other := doGetWith(t, e, "/workboard/tasks/"+id+"/action", jwtCookieUser(t, away))
	if other.Code == http.StatusOK && strings.Contains(other.Body.String(), "완료 건 수정 잠금 해제") {
		t.Fatal("다른 조직 관리자에게 메뉴가 보인다")
	}
	sup := &model.User{Username: "sup173", PasswordHash: HashPassword("password10"), FullName: "지원173", Role: model.RoleSupport, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	tech := &model.User{Username: "tech173", PasswordHash: HashPassword("password10"), FullName: "기술173", Role: model.RoleTech, IsActive: true, OrgID: model.OrgIDLibrary, ProfileDone: true}
	if err := users.Create(sup); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(tech); err != nil {
		t.Fatal(err)
	}
	sup, _ = users.GetByUsername("sup173")
	tech, _ = users.GetByUsername("tech173")
	for _, u := range []*model.User{sup, tech} {
		body := doGetWith(t, e, "/workboard/tasks/"+id+"/action", jwtCookieUser(t, u)).Body.String()
		if strings.Contains(body, "완료 건 수정 잠금 해제") {
			t.Fatalf("%s 에게 메뉴가 보인다", u.Role)
		}
		res := doFormUser(t, e, "/workboard/tasks/"+id+"/action/unlock-edit", url.Values{"unlock_password": {"as-edit"}}, jwtCookieUser(t, u))
		if res.Code != http.StatusForbidden {
			t.Fatalf("%s 잠금 해제 %d", u.Role, res.Code)
		}
	}
}

func Test95_173_SegmentDeleteMergeOnTaskDetail(t *testing.T) {
	e, repo, users := newProjectWorkServer(t)
	day := time.Now().AddDate(0, 0, 4).Format("2006-01-02")
	mustCreateBoardTask(t, repo, "완료구간173", day, "09:00", "11:00", model.OrgIDLibrary, "관리자", "admin-id", model.WBTaskComplete, model.WBWorkProjectWork)
	id := taskIDByTitle173(t, repo, "완료구간173")
	if err := repo.ReplaceActualSegments(id, []model.WorkTaskSegment{
		{Kind: model.SegKindActual, WorkDate: day, StartTime: "09:00", EndTime: "10:00", WorkerName: "관리자", DurationMin: 60},
		{Kind: model.SegKindActual, WorkDate: day, StartTime: "10:00", EndTime: "11:00", WorkerName: "김실무", DurationMin: 60, InterruptTaskID: "WT-INT173"},
	}); err != nil {
		t.Fatal(err)
	}
	page := doGet(t, e, "/workboard/tasks/"+id)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `data-seg-delete="1"`) || !strings.Contains(body, "앞 구간과 합치기") {
		t.Fatal("완료 건 상세에 삭제·합치기가 없다")
	}
	if !strings.Contains(body, "끼워 넣은 업무") {
		t.Fatal("끼워넣기 안내가 없다")
	}

	ro := &model.User{Username: "ro173", PasswordHash: HashPassword("password10"), FullName: "읽기173", Role: model.RoleOrgAdmin, IsActive: true, IsReadOnly: true, OrgID: model.OrgIDLibrary, Email: "ro173@ex.com", ProfileDone: true}
	if err := users.Create(ro); err != nil {
		t.Fatal(err)
	}
	ro, _ = users.GetByUsername("ro173")
	roPage := doGetWith(t, e, "/workboard/tasks/"+id, jwtCookieUser(t, ro)).Body.String()
	if strings.Contains(roPage, `data-seg-delete=`) {
		t.Fatal("읽기 전용에 삭제 버튼이 있다")
	}
	roPost := doFormUser(t, e, "/workboard/tasks/"+id+"/segments/delete", url.Values{"seg_index": {"1"}}, jwtCookieUser(t, ro))
	if roPost.Code != http.StatusForbidden {
		t.Fatalf("읽기 전용 삭제 %d", roPost.Code)
	}

	mix := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	if err := repo.ReplaceActualSegments(id, []model.WorkTaskSegment{
		{Kind: model.SegKindActual, WorkDate: day, StartTime: "09:00", EndTime: "10:00", WorkerName: "관리자", DurationMin: 60},
		{Kind: model.SegKindActual, WorkDate: mix, StartTime: "10:00", EndTime: "11:00", WorkerName: "김실무", DurationMin: 60},
	}); err != nil {
		t.Fatal(err)
	}
	badDate := doForm(t, e, "/workboard/tasks/"+id+"/segments/merge", url.Values{"seg_index": {"1"}})
	if badDate.Code != http.StatusSeeOther || !strings.Contains(badDate.Header().Get("Location"), "seg_date_mix") {
		t.Fatalf("날짜 다름 %d %s", badDate.Code, badDate.Header().Get("Location"))
	}

	if err := repo.ReplaceActualSegments(id, []model.WorkTaskSegment{
		{Kind: model.SegKindActual, WorkDate: day, StartTime: "09:00", EndTime: "10:00", WorkerName: "관리자", DurationMin: 60},
		{Kind: model.SegKindActual, WorkDate: day, StartTime: "10:00", EndTime: "11:00", WorkerName: "김실무", DurationMin: 60, InterruptTaskID: "WT-INT173"},
	}); err != nil {
		t.Fatal(err)
	}
	merged := doForm(t, e, "/workboard/tasks/"+id+"/segments/merge", url.Values{"seg_index": {"1"}})
	if merged.Code != http.StatusSeeOther || !strings.Contains(merged.Header().Get("Location"), "ok=seg") {
		t.Fatalf("합치기 %d %s", merged.Code, merged.Header().Get("Location"))
	}
	got, _ := repo.ListActualSegments(id)
	if len(got) != 1 || got[0].StartTime != "09:00" || got[0].EndTime != "11:00" || got[0].WorkerName != "관리자" {
		t.Fatalf("합친 구간 %+v", got)
	}
	if strings.TrimSpace(got[0].InterruptTaskID) != "WT-INT173" {
		t.Fatalf("끼워넣기 연결 %+v", got[0].InterruptTaskID)
	}
	hist := doGet(t, e, "/workboard/tasks/"+id).Body.String()
	if !strings.Contains(hist, "구간 변경") || !strings.Contains(hist, "09:00~11:00") {
		t.Fatal("이력이 없다")
	}
	if !strings.Contains(hist, `data-seg-delete-off="1"`) || strings.Contains(hist, `data-seg-delete="`) {
		t.Fatal("마지막 구간 삭제가 꺼지지 않았다")
	}
	board := doGet(t, e, "/workboard/board?view=day&date="+day+"&mine=0&auto=0").Body.String()
	if !strings.Contains(board, "완료구간173") {
		t.Fatal("일정표에 합친 업무가 없다")
	}

	if err := repo.ReplaceActualSegments(id, []model.WorkTaskSegment{
		{Kind: model.SegKindActual, WorkDate: day, StartTime: "09:00", EndTime: "10:00", WorkerName: "관리자", DurationMin: 60},
		{Kind: model.SegKindActual, WorkDate: day, StartTime: "10:00", EndTime: "", WorkerName: "관리자", Source: model.SegSourceLive},
	}); err != nil {
		t.Fatal(err)
	}
	live := doForm(t, e, "/workboard/tasks/"+id+"/segments/delete", url.Values{"seg_index": {"0"}})
	if live.Code != http.StatusSeeOther || !strings.Contains(live.Header().Get("Location"), "seg_live") {
		t.Fatalf("진행 중 %d %s", live.Code, live.Header().Get("Location"))
	}
	if left, _ := repo.ListActualSegments(id); len(left) != 2 {
		t.Fatalf("진행 중인데 지워짐 %d", len(left))
	}
}

func Test95_173_PlanSegmentsStayPlan(t *testing.T) {
	e, repo, _ := newProjectWorkServer(t)
	day := time.Now().AddDate(0, 0, 6).Format("2006-01-02")
	mustCreateBoardTask(t, repo, "계획구간173", day, "09:00", "11:00", model.OrgIDLibrary, "관리자", "admin-id", model.WBTaskWaiting, model.WBWorkAdmin)
	id := taskIDByTitle173(t, repo, "계획구간173")
	if err := repo.ReplacePlanSegments(id, []model.WorkTaskSegment{
		{Kind: model.SegKindPlan, WorkDate: day, StartTime: "09:00", EndTime: "10:00", DurationMin: 60},
		{Kind: model.SegKindPlan, WorkDate: day, StartTime: "10:00", EndTime: "11:00", DurationMin: 60},
	}); err != nil {
		t.Fatal(err)
	}
	rec := doForm(t, e, "/workboard/tasks/"+id+"/segments/delete", url.Values{"seg_index": {"1"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=seg") {
		t.Fatalf("계획 삭제 %d %s", rec.Code, rec.Header().Get("Location"))
	}
	actual, _ := repo.ListActualSegments(id)
	if len(actual) != 0 {
		t.Fatalf("실제 구간이 생김 %+v", actual)
	}
	plan, _ := repo.ListSegments(id)
	var plans []model.WorkTaskSegment
	for _, s := range plan {
		if s.Kind == model.SegKindPlan {
			plans = append(plans, s)
		}
	}
	if len(plans) != 1 || plans[0].StartTime != "09:00" || plans[0].EndTime != "10:00" {
		t.Fatalf("계획 구간 %+v", plans)
	}
	tk, err := repo.GetTask(id)
	if err != nil || tk == nil || tk.StartTime != "09:00" || tk.EndTime != "10:00" || tk.DurationMin != 60 {
		t.Fatalf("업무 시간 %+v", tk)
	}
	last := doForm(t, e, "/workboard/tasks/"+id+"/segments/delete", url.Values{"seg_index": {"0"}})
	if last.Code != http.StatusSeeOther || !strings.Contains(last.Header().Get("Location"), "seg_last") {
		t.Fatalf("마지막 삭제 %d %s", last.Code, last.Header().Get("Location"))
	}
}

func Test95_173_ASUnlockKeepsOwnerRule(t *testing.T) {
	e, db, h, ck, _ := new78A(t)
	if _, err := db.Exec(`INSERT INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c173','학교173','학교173',1,'O01')`); err != nil {
		t.Fatal(err)
	}
	other := &model.ASReceipt{
		CustomerID: "c173", ReceiptChannel: "phone", Requester: "교사", Symptom: "잠금",
		AssignedTo: "다른담당", Status: "completed", ReceiptDatetime: time.Now(), OrgID: "O01",
	}
	if err := h.AS.repo.Create(other); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE as_receipts SET status='completed' WHERE as_id=?`, other.ASID); err != nil {
		t.Fatal(err)
	}
	page := doGetWith(t, e, "/as/"+other.ASID+"/action", ck)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `id="btn-unlock-edit"`) {
		t.Fatal("조직관리자에게 AS 잠금 해제 버튼이 없다")
	}
	mine := &model.ASReceipt{
		CustomerID: "c173", ReceiptChannel: "phone", Requester: "교사", Symptom: "본인잠금",
		AssignedTo: "관리자", AssignedUserID: "admin-id", Status: "completed",
		ReceiptDatetime: time.Now(), OrgID: "O01",
	}
	if err := h.AS.repo.Create(mine); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE as_receipts SET status='completed', assigned_to='관리자', assigned_user_id='admin-id' WHERE as_id=?`, mine.ASID); err != nil {
		t.Fatal(err)
	}
	owner := doGetWith(t, e, "/as/"+mine.ASID+"/action", ck).Body.String()
	if strings.Contains(owner, `id="btn-unlock-edit"`) {
		t.Fatal("본인 접수에 잠금 해제 버튼이 보인다")
	}
}

func taskIDByTitle173(t *testing.T, repo *repository.WBRepo, title string) string {
	t.Helper()
	tasks, err := repo.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range tasks {
		if tk.Title == title {
			return tk.TaskID
		}
	}
	t.Fatalf("업무 없음 %s", title)
	return ""
}
