package handler

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func newGTDServer(t *testing.T) (*echo.Echo, *repository.WBRepo) {
	t.Helper()
	dir := t.TempDir()
	db, err := repository.InitDB(filepath.Join(dir, "gtd_http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	repository.NewUserRepo(db).EnsureAdmin(HashPassword("admin"))

	e := echo.New()
	e.Renderer = NewRenderer()
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	aw := g.Group("/admin-work")
	aw.GET("", h.AdminWork.List)
	aw.GET("/stats", h.AdminWork.Stats)
	aw.GET("/new", h.AdminWork.New)
	aw.POST("", h.AdminWork.Create)
	aw.POST("/inbox", h.AdminWork.CreateInbox)
	aw.POST("/:id/classify", h.AdminWork.Classify)
	aw.POST("/:id/move", h.AdminWork.MoveKanban)
	aw.GET("/:id", h.AdminWork.Show)
	wb := g.Group("/workboard")
	wb.GET("/tasks/:id/edit", h.Workboard.EditTask)
	wb.GET("/tasks/:id", h.Workboard.ShowTask)
	wb.POST("/tasks/:id/update", h.Workboard.UpdateTask)
	wb.POST("/tasks/:id/actions", h.Workboard.CreateAction)
	wb.POST("/tasks/:id/actions/:aid/update", h.Workboard.UpdateAction)
	wb.POST("/tasks/:id/activities", h.Workboard.CreateActivity)
	return e, repository.NewWBRepo(db)
}

func TestAdminWorkQuickRegisterAndStats(t *testing.T) {
	e, repo := newGTDServer(t)

	list := doGet(t, e, "/admin-work")
	body := list.Body.String()
	if list.Code != http.StatusOK || !strings.Contains(body, "빠른 등록") {
		t.Fatalf("목록 빠른 등록 폼 없음 status=%d", list.Code)
	}
	if strings.Contains(body, "수집함 등록") || strings.Contains(body, "수집함은") {
		t.Fatal("수집함 문구가 남아 있음")
	}

	rec := doForm(t, e, "/admin-work/inbox", url.Values{"title": {"월간 실적 자료 요청"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "ok=waiting") {
		t.Fatalf("quick: status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	items, err := repo.ListAdminWork("waiting", "")
	if err != nil || len(items) != 1 || items[0].Status != model.WBTaskWaiting {
		t.Fatalf("waiting items %+v err=%v", items, err)
	}

	stats := doGet(t, e, "/admin-work/stats")
	sbody := stats.Body.String()
	if stats.Code != http.StatusOK || strings.Contains(sbody, "수집함") {
		t.Fatalf("현황에 수집함이 남음 status=%d", stats.Code)
	}
	if !strings.Contains(sbody, "회신 대기") || !strings.Contains(sbody, "할 일") {
		t.Fatalf("현황 카드 문구 없음")
	}
	if !strings.Contains(sbody, ">보류<") || !strings.Contains(sbody, ">이관<") {
		t.Fatal("현황 카드에 보류·이관 분리가 없음")
	}
	if strings.Contains(sbody, "보류·이관") {
		t.Fatal("보류·이관 합침 카드가 남아 있음")
	}
}

func TestAdminWorkClassifyRejectsWaiting(t *testing.T) {
	e, repo := newGTDServer(t)
	rec := doForm(t, e, "/admin-work/inbox", url.Values{"title": {"분류할 메모"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("quick status=%d", rec.Code)
	}
	items, err := repo.ListAdminWork("waiting", "")
	if err != nil || len(items) != 1 {
		t.Fatalf("waiting %+v err=%v", items, err)
	}
	id := items[0].TaskID

	bad := doForm(t, e, "/admin-work/"+id+"/classify", url.Values{
		"status": {"waiting"},
	})
	if bad.Code != http.StatusSeeOther || !strings.Contains(bad.Header().Get("Location"), "err=classify") {
		t.Fatalf("classify loc=%q", bad.Header().Get("Location"))
	}
	got, _ := repo.GetTask(id)
	if got == nil || got.Status != model.WBTaskWaiting {
		t.Fatalf("분류 거부 후 상태 %+v", got)
	}
}

func TestAdminWorkCancelRequiresReason(t *testing.T) {
	e, _ := newGTDServer(t)
	rec := doForm(t, e, "/admin-work", url.Values{
		"work_type": {"admin"},
		"title":     {"취소 테스트"},
		"due_date":  {"2026-08-20"},
		"status":    {"cancelled"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=cancel_reason") {
		t.Fatalf("취소 필수값 loc=%q", rec.Header().Get("Location"))
	}
}

func TestAdminWorkCompleteBlockedByRequiredAction(t *testing.T) {
	e, repo := newGTDServer(t)

	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "계획서", DueDate: "2026-08-20",
		WorkDate: "2026-08-20", StartTime: "09:00", EndTime: "09:30", Status: model.WBTaskInProgress}
	if err := repo.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAction(&model.WorkAction{TaskID: task.TaskID, Title: "초안", Required: true}); err != nil {
		t.Fatal(err)
	}

	rec := doForm(t, e, "/workboard/tasks/"+task.TaskID+"/update", url.Values{
		"title":         {task.Title},
		"due_date":      {task.DueDate},
		"work_date":     {task.WorkDate},
		"start_time":    {task.StartTime},
		"end_time":      {task.EndTime},
		"status":        {"complete"},
		"complete_note": {"발송 완료"},
		"work_type":     {"admin"},
		"priority":      {"normal"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=complete_block") {
		t.Fatalf("완료 차단 실패 loc=%q", rec.Header().Get("Location"))
	}

	show := doGet(t, e, "/workboard/tasks/"+task.TaskID+"?back=/admin-work")
	if show.Code != http.StatusOK {
		t.Fatalf("상세 status=%d", show.Code)
	}
	body := show.Body.String()
	if strings.Contains(body, "다음 행동") {
		t.Fatal("조치 화면에 다음 행동이 남아 있음")
	}
	if !strings.Contains(body, "조치 이력") {
		t.Fatal("조치 이력 없음")
	}
}

func TestAdminWorkHoldRequiresReason(t *testing.T) {
	e, _ := newGTDServer(t)
	rec := doForm(t, e, "/admin-work", url.Values{
		"work_type": {"admin"},
		"title":     {"보류 테스트"},
		"due_date":  {"2026-08-20"},
		"status":    {"hold"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=hold_required") {
		t.Fatalf("보류 필수값 loc=%q", rec.Header().Get("Location"))
	}
}

func TestAdminWorkWaitingForAndActivity(t *testing.T) {
	e, repo := newGTDServer(t)
	rec := doForm(t, e, "/admin-work", url.Values{
		"work_type":       {"admin"},
		"title":           {"견적 요청"},
		"due_date":        {"2026-08-20"},
		"status":          {"waiting_for"},
		"wait_party_kind": {"vendor"},
		"wait_party":      {"외부업체",},
		"wait_request":    {"견적서 회신"},
		"reply_due_date":  {"2026-08-25"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("회신 대기 등록 loc=%q", rec.Header().Get("Location"))
	}
	items, err := repo.ListAdminWork("waiting_for", "")
	if err != nil || len(items) != 1 {
		t.Fatalf("waiting_for list len=%d err=%v", len(items), err)
	}
	actions, _ := repo.ListActions(items[0].TaskID)
	if len(actions) != 1 || actions[0].Status != model.WBActionWaiting {
		t.Fatalf("대기 행동 %+v", actions)
	}

	act := doForm(t, e, "/workboard/tasks/"+items[0].TaskID+"/activities", url.Values{
		"activity_type":    {"send"},
		"activity_content": {"견적 요청 메일 발송"},
		"spent_minutes":    {"15"},
		"back":             {"/admin-work"},
	})
	if act.Code != http.StatusSeeOther || !strings.Contains(act.Header().Get("Location"), "ok=activity") {
		t.Fatalf("이력 loc=%q", act.Header().Get("Location"))
	}
	hist, _ := repo.ListActivities(items[0].TaskID)
	if len(hist) != 1 || hist[0].Content != "견적 요청 메일 발송" {
		t.Fatalf("이력 %+v", hist)
	}
}

func TestIRMCaseNextActionsAndTimeline(t *testing.T) {
	e, repo := newGTDServer(t)
	rec := doForm(t, e, "/admin-work", url.Values{
		"work_type":     {"admin"},
		"customer_name": {"충남교육청통합도서관"},
		"title":         {"행안부 IRM 평가지표 세부 레벨 작성"},
		"due_date":      {"2026-08-13"},
		"work_date":     {"2026-08-13"},
		"status":        {"in_progress"},
		"assignee":      {"관리자"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("업무 등록 loc=%q", rec.Header().Get("Location"))
	}
	items, err := repo.ListAdminWork("in_progress", "")
	if err != nil || len(items) != 1 {
		t.Fatalf("list %+v err=%v", items, err)
	}
	id := items[0].TaskID

	bad := doForm(t, e, "/workboard/tasks/"+id+"/actions", url.Values{
		"action_title":    {"홈페이지 부문 요청"},
		"action_status":   {"waiting"},
		"action_required": {"1"},
		"back":            {"/admin-work"},
	})
	if bad.Code != http.StatusSeeOther || !strings.Contains(bad.Header().Get("Location"), "err=waiting_for") {
		t.Fatalf("대기 필수값 loc=%q", bad.Header().Get("Location"))
	}

	vendors := [][]string{
		{"홈페이지 부문 요청", "vendor", "OO소프트 김OO", "IRM 지표 3-1~3-4", "2026-08-13", "2026-08-15"},
		{"자료관리 부문 요청", "vendor", "△△시스템 이OO", "자료관리 부문 지표", "2026-08-13", "2026-08-15"},
		{"전자도서관 부문 요청", "vendor", "□□정보 박OO", "전자도서관 부문 지표", "2026-08-13", "2026-08-15"},
	}
	for _, v := range vendors {
		rec = doForm(t, e, "/workboard/tasks/"+id+"/actions", url.Values{
			"action_title":        {v[0]},
			"action_status":       {"waiting"},
			"action_required":     {"1"},
			"wait_party_kind":     {v[1]},
			"wait_party":          {v[2]},
			"wait_request":        {v[3]},
			"reply_due_date":      {v[4]},
			"next_check_date":     {v[5]},
			"back":                {"/admin-work"},
		})
		if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
			t.Fatalf("행동 %s loc=%q", v[0], rec.Header().Get("Location"))
		}
	}
	actions, _ := repo.ListActions(id)
	if len(actions) != 3 {
		t.Fatalf("다음 행동 %d", len(actions))
	}
	done, req, err := repo.CountActionProgress(id)
	if err != nil || done != 0 || req != 3 {
		t.Fatalf("progress %d/%d err=%v", done, req, err)
	}
	got, _ := repo.GetTask(id)
	if got.Progress != 0 {
		t.Fatalf("진행률=%d want 0", got.Progress)
	}

	doForm(t, e, "/workboard/tasks/"+id+"/actions/"+actions[0].ActionID+"/update", url.Values{
		"action_status": {"complete"}, "confirmed": {"1"}, "back": {"/admin-work"},
	})
	got, _ = repo.GetTask(id)
	if got.Progress < 33 || got.Progress > 34 {
		t.Fatalf("1/3 진행률=%d", got.Progress)
	}

	for _, a := range actions[1:] {
		doForm(t, e, "/workboard/tasks/"+id+"/actions/"+a.ActionID+"/update", url.Values{
			"action_status": {"complete"}, "confirmed": {"1"}, "back": {"/admin-work"},
		})
	}
	got, _ = repo.GetTask(id)
	if got.Progress != 100 {
		t.Fatalf("3/3 진행률=%d", got.Progress)
	}

	doForm(t, e, "/workboard/tasks/"+id+"/activities", url.Values{
		"activity_type": {"write"}, "activity_content": {"3사 자료 취합·수정"}, "spent_minutes": {"90"}, "back": {"/admin-work"},
	})
	doForm(t, e, "/workboard/tasks/"+id+"/activities", url.Values{
		"activity_type": {"send"}, "activity_content": {"최종본 메일 발송"}, "spent_minutes": {"20"}, "back": {"/admin-work"},
	})
	doForm(t, e, "/workboard/tasks/"+id+"/activities", url.Values{
		"activity_type": {"complete"}, "activity_content": {"고객 완료 확인 회신 받음"}, "spent_minutes": {"10"}, "back": {"/admin-work"},
	})

	show := doGet(t, e, "/workboard/tasks/"+id+"?back=/admin-work")
	if show.Code != http.StatusOK {
		t.Fatalf("상세 status=%d", show.Code)
	}
	body := show.Body.String()
	for _, want := range []string{
		"충남교육청통합도서관", "행안부 IRM 평가지표 세부 레벨 작성",
		"조치 이력",
		"3사 자료 취합·수정", "90분", "최종본 메일 발송", "20분",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("상세에 %q 없음", want)
		}
	}
	if strings.Contains(body, "다음 행동") || strings.Contains(body, "이력 추가") {
		t.Error("조치 화면에 다음 행동·이력 폼이 남아 있음")
	}

	list := doGet(t, e, "/admin-work")
	if !strings.Contains(list.Body.String(), "회신 대기") {
		t.Fatal("목록에 회신 대기 없음")
	}
}

func TestForceCompleteRecordsReason(t *testing.T) {
	e, repo := newGTDServer(t)
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "강제완료", DueDate: "2026-08-20",
		WorkDate: "2026-08-20", Status: model.WBTaskInProgress}
	if err := repo.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAction(&model.WorkAction{TaskID: task.TaskID, Title: "필수", Required: true}); err != nil {
		t.Fatal(err)
	}
	rec := doForm(t, e, "/workboard/tasks/"+task.TaskID+"/update", url.Values{
		"title":          {task.Title},
		"due_date":       {task.DueDate},
		"work_date":      {task.WorkDate},
		"status":         {"complete"},
		"complete_note":  {"마감 처리"},
		"force_complete": {"1"},
		"force_reason":   {"일정 마감으로 잔여 행동 종결"},
		"work_type":      {"admin"},
		"priority":       {"normal"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("강제 완료 loc=%q", rec.Header().Get("Location"))
	}
	got, _ := repo.GetTask(task.TaskID)
	if got == nil || got.Status != model.WBTaskComplete {
		t.Fatalf("상태 %+v", got)
	}
	hist, _ := repo.ListActivities(task.TaskID)
	found := false
	for _, a := range hist {
		if strings.Contains(a.Content, "일정 마감으로 잔여 행동 종결") {
			found = true
		}
	}
	if !found {
		t.Fatalf("강제 완료 사유가 이력에 없음 %+v", hist)
	}
}

func TestMissingNextActionWarning(t *testing.T) {
	e, repo := newGTDServer(t)
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "진행만", DueDate: "2026-08-20",
		WorkDate: "2026-08-20", Status: model.WBTaskInProgress}
	if err := repo.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	show := doGet(t, e, "/workboard/tasks/"+task.TaskID+"?back=/admin-work")
	if strings.Contains(show.Body.String(), "다음 행동") {
		t.Fatal("조치 화면에 다음 행동이 남아 있음")
	}
}

func TestActivitySpentMinutesRequired(t *testing.T) {
	e, repo := newGTDServer(t)
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "소요시간", DueDate: "2026-08-20", Status: model.WBTaskInProgress}
	if err := repo.CreateTask(task); err != nil {
		t.Fatal(err)
	}
	rec := doForm(t, e, "/workboard/tasks/"+task.TaskID+"/activities", url.Values{
		"activity_type":    {"write"},
		"activity_content": {"초안"},
		"back":             {"/admin-work"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "err=spent") {
		t.Fatalf("소요시간 필수 loc=%q", rec.Header().Get("Location"))
	}
	zero := doForm(t, e, "/workboard/tasks/"+task.TaskID+"/activities", url.Values{
		"activity_type":    {"write"},
		"activity_content": {"초안"},
		"spent_minutes":    {"0"},
		"back":             {"/admin-work"},
	})
	if zero.Code != http.StatusSeeOther || !strings.Contains(zero.Header().Get("Location"), "err=spent") {
		t.Fatalf("0분 거부 loc=%q", zero.Header().Get("Location"))
	}
}

func TestAdminWorkKanbanMoveAndListSort(t *testing.T) {
	e, repo := newGTDServer(t)
	early := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "가나다 마감먼저", DueDate: "2026-08-10", Status: model.WBTaskWaiting}
	late := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "하하하 마감나중", DueDate: "2026-08-30", Status: model.WBTaskWaiting}
	if err := repo.CreateTask(early); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(late); err != nil {
		t.Fatal(err)
	}

	list := doGet(t, e, "/admin-work")
	body := list.Body.String()
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d", list.Code)
	}
	if !strings.Contains(body, `href="`) || !strings.Contains(body, "sort=due_date") {
		t.Fatal("종료일 정렬 링크 없음")
	}
	ei := strings.Index(body, "가나다 마감먼저")
	li := strings.Index(body, "하하하 마감나중")
	if ei < 0 || li < 0 || ei > li {
		t.Fatalf("기본 정렬이 종료일 오름차순이 아님 ei=%d li=%d", ei, li)
	}

	byTitle := doGet(t, e, "/admin-work?sort=title&dir=desc")
	tb := byTitle.Body.String()
	if !strings.Contains(tb, "sort=title") || !strings.Contains(tb, `name="dir"`) || !strings.Contains(tb, `value="desc"`) {
		t.Fatal("?sort= 유지 안 됨")
	}
	if strings.Index(tb, "하하하 마감나중") > strings.Index(tb, "가나다 마감먼저") {
		t.Fatal("제목 내림차순이 아님")
	}

	kanban := doGet(t, e, "/admin-work?view=kanban")
	kb := kanban.Body.String()
	if kanban.Code != http.StatusOK || !strings.Contains(kb, "할 일") || !strings.Contains(kb, "진행중") || !strings.Contains(kb, "완료") {
		t.Fatal("칸반 3열 없음")
	}
	if strings.Contains(kb, ">검토<") {
		t.Fatal("칸반에 검토 열이 있음")
	}

	noNote := doForm(t, e, "/admin-work/"+early.TaskID+"/move", url.Values{"status": {"complete"}})
	if noNote.Code != http.StatusSeeOther || !strings.Contains(noNote.Header().Get("Location"), "err=complete_note") {
		t.Fatalf("완료 검증 loc=%q", noNote.Header().Get("Location"))
	}
	prog := doForm(t, e, "/admin-work/"+early.TaskID+"/move", url.Values{"status": {"in_progress"}})
	if prog.Code != http.StatusSeeOther || strings.Contains(prog.Header().Get("Location"), "err=") {
		t.Fatalf("진행중 이동 loc=%q", prog.Header().Get("Location"))
	}
	got, _ := repo.GetTask(early.TaskID)
	if got == nil || got.Status != model.WBTaskInProgress {
		t.Fatalf("진행중 %+v", got)
	}
	done := doForm(t, e, "/admin-work/"+early.TaskID+"/move", url.Values{
		"status": {"complete"}, "complete_note": {"자료 제출 완료"},
	})
	if done.Code != http.StatusSeeOther || strings.Contains(done.Header().Get("Location"), "err=") {
		t.Fatalf("완료 이동 loc=%q", done.Header().Get("Location"))
	}
	got, _ = repo.GetTask(early.TaskID)
	if got == nil || got.Status != model.WBTaskComplete {
		t.Fatalf("완료 %+v", got)
	}
}

func TestSection336ActionVsEdit(t *testing.T) {
	e, repo := newGTDServer(t)
	task := &model.WorkTask{WorkType: model.WBWorkAdmin, Title: "구분고정", DueDate: "2026-08-20",
		WorkDate: "2026-08-20", StartTime: "09:00", EndTime: "09:30", Status: model.WBTaskInProgress,
		CustomerName: "충남교육청"}
	if err := repo.CreateTask(task); err != nil {
		t.Fatal(err)
	}

	show := doGet(t, e, "/workboard/tasks/"+task.TaskID)
	if show.Code != http.StatusOK {
		t.Fatalf("조치 status=%d", show.Code)
	}
	body := show.Body.String()
	if !strings.Contains(body, "등록 정보") || !strings.Contains(body, "일정표") {
		t.Fatal("조치에 등록 정보·일정표가 없다")
	}
	if strings.Contains(body, "기간 내 반복 실행") || strings.Contains(body, "다음 행동") || strings.Contains(body, "이력 추가") {
		t.Fatal("조치에 반복 설정·다음 행동·이력 폼이 남아 있다")
	}
	if strings.Contains(body, `name="work_type"`) {
		i := strings.Index(body, `name="scope" value="action"`)
		if i < 0 {
			t.Fatal("조치 저장 폼이 없다")
		}
		chunk := body[i:]
		if j := strings.Index(chunk, "조치 이력"); j > 0 {
			chunk = chunk[:j]
		}
		if strings.Contains(chunk, `name="work_type"`) {
			t.Fatal("조치에서 업무 구분을 바꿀 수 있다")
		}
	}
	if !strings.Contains(body, "영업 활동으로 만들기") || !strings.Contains(body, "from_task=") {
		t.Fatal("영업 활동으로 만들기 버튼이 없다")
	}

	edit := doGet(t, e, "/workboard/tasks/"+task.TaskID+"/edit")
	if edit.Code != http.StatusOK {
		t.Fatalf("수정 status=%d", edit.Code)
	}
	ebody := edit.Body.String()
	if !strings.Contains(ebody, "기간 내 반복 실행") || !strings.Contains(ebody, `name="work_type"`) {
		t.Fatal("수정 화면에 반복·업무 구분이 없다")
	}
	if strings.Contains(ebody, "다음 행동") {
		t.Fatal("수정 화면에 다음 행동이 있다")
	}

	rec := doForm(t, e, "/workboard/tasks/"+task.TaskID+"/update", url.Values{
		"scope": {"action"}, "status": {"in_progress"}, "work_type": {"support"},
		"title": {"바뀌면안됨"}, "priority": {"normal"},
	})
	if rec.Code != http.StatusSeeOther || strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("조치 저장 loc=%q", rec.Header().Get("Location"))
	}
	got, _ := repo.GetTask(task.TaskID)
	if got == nil || got.WorkType != model.WBWorkAdmin || got.Title != "구분고정" {
		t.Fatalf("조치에서 등록 정보가 바뀜 %+v", got)
	}
	hist, _ := repo.ListActivities(task.TaskID)
	if len(hist) == 0 {
		t.Fatal("조치 저장 시 이력이 안 쌓였다")
	}

	rec = doForm(t, e, "/workboard/tasks/"+task.TaskID+"/update", url.Values{
		"scope": {"register"}, "work_type": {"admin"}, "title": {"등록에서수정"},
		"due_date": {"2026-08-20"}, "work_date": {"2026-08-20"},
		"start_time": {"09:00"}, "end_time": {"09:30"},
	})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/edit") {
		t.Fatalf("등록 저장 loc=%q", rec.Header().Get("Location"))
	}
	got, _ = repo.GetTask(task.TaskID)
	if got == nil || got.Title != "등록에서수정" {
		t.Fatalf("등록 수정 실패 %+v", got)
	}
}
