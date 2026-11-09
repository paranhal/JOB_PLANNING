package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func seed158UndatedAS(t *testing.T, h *Handler, u *model.User, symptom string) (*model.ASReceipt, *model.WorkTask) {
	t.Helper()
	as := &model.ASReceipt{
		CustomerID: "c-jeongan", ReceiptChannel: "phone", Requester: "학교",
		Symptom: symptom, AssignedTo: u.FullName, AssignedUserID: u.UserID,
		ReceivedBy: u.FullName, ReceiptDatetime: time.Date(2026, 11, 9, 9, 0, 0, 0, time.Local),
		OrgID: u.OrgID,
	}
	if err := h.AS.repo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := h.Workboard.repo.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Workboard.repo.DB().Exec(`UPDATE work_tasks SET org_id=?, work_date='', due_date='' WHERE source_type=? AND source_id=?`,
		u.OrgID, model.WBSourceAS, as.ASID); err != nil {
		t.Fatal(err)
	}
	tk, err := h.Workboard.repo.GetTaskBySource(model.WBSourceAS, as.ASID)
	if err != nil || tk == nil {
		t.Fatalf("undated task: %v", err)
	}
	return as, tk
}

func Test95_158_T1_SameASOneUnplacedCard(t *testing.T) {
	pal := model.WBCard{Kind: model.WBSourceAS, RefID: "AS-1", Category: model.WBSourceAS, SourceNumber: "R2611-019"}
	und := model.WBCard{Kind: "task", RefID: "WT-9", Category: model.WBSourceAS, SourceNumber: "R2611-019", SourceHref: "/as/AS-1/action", TaskID: "WT-9"}
	if cardDedupKey(pal) != "as:AS-1" || cardDedupKey(und) != "as:AS-1" {
		t.Fatalf("keys pal=%s und=%s", cardDedupKey(pal), cardDedupKey(und))
	}
	e, h, si, _ := new95_41App(t)
	as, tk := seed158UndatedAS(t, h, si, "이중카드증상")
	if tk.SourceID != as.ASID {
		t.Fatal("source_id 불일치")
	}
	body := unplacedPanelHTML(board155(t, e, si, "kanban").Body.String())
	pid := `data-pid="as:` + as.ASID + `"`
	if strings.Count(body, pid) != 1 {
		t.Fatalf("같은 AS 카드 %d건\n%s", strings.Count(body, pid), body)
	}
	if !strings.Contains(body, `class="board-cnt">1<`) && !strings.Contains(body, `class="board-cnt">1</span>`) {
		t.Fatal("미배치 건수 배지가 1이 아님")
	}
	if strings.Contains(body, `data-pid="task:`+tk.TaskID+`"`) {
		t.Fatal("일일업무 키가 따로 남았다")
	}
	day := unplacedPanelHTML(board155(t, e, si, "day").Body.String())
	if strings.Count(day, pid) != 1 {
		t.Fatalf("일정표 미배치 %d건", strings.Count(day, pid))
	}
}

func Test95_158_T2_DifferentASStayTwo(t *testing.T) {
	e, h, si, _ := new95_41App(t)
	a, _ := seed158UndatedAS(t, h, si, "접수A증상")
	b, _ := seed158UndatedAS(t, h, si, "접수B증상")
	if a.ASID == b.ASID {
		t.Fatal("같은 AS")
	}
	body := unplacedPanelHTML(board155(t, e, si, "kanban").Body.String())
	if strings.Count(body, `data-pid="as:`+a.ASID+`"`) != 1 {
		t.Fatal("A 없음")
	}
	if strings.Count(body, `data-pid="as:`+b.ASID+`"`) != 1 {
		t.Fatal("B 없음")
	}
}

func Test95_158_T3_DupDropSkipsASPlaceKind(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.Local)
	items := []boardItem{
		{
			PlaceKind: model.WBSourceAS, DeleteKey: "as:AS-KEEP", CanDelete: true,
			Task: model.WorkTask{Title: "같은제목158", TaskID: "AS-KEEP", CreatedAt: now},
			Type: boardTypeStyle{Key: "admin"}, AssigneeName: "SI조직장", CustomerLine: "정안",
		},
		{
			PlaceKind: "task", DeleteKey: "task:WT-DROP", CanDelete: true,
			Task: model.WorkTask{Title: "같은제목158", TaskID: "WT-DROP", CreatedAt: now.Add(time.Minute)},
			Type: boardTypeStyle{Key: "admin"}, AssigneeName: "SI조직장", CustomerLine: "정안",
		},
	}
	annotateUnplacedDupes(items)
	if strings.Contains(items[0].DupDropKeys, "as:") || strings.Contains(items[1].DupDropKeys, "as:") {
		t.Fatalf("AS drop=%q / %q", items[0].DupDropKeys, items[1].DupDropKeys)
	}
	if unplacedDupSkipAS(items[0]) != true {
		t.Fatal("PlaceKind=as 가 제외되지 않음")
	}
	swap := []boardItem{items[1], items[0]}
	swap[0].Task.CreatedAt = now
	swap[1].Task.CreatedAt = now.Add(time.Minute)
	annotateUnplacedDupes(swap)
	if strings.Contains(swap[0].DupDropKeys, "as:") || strings.Contains(swap[1].DupDropKeys, "as:") {
		t.Fatalf("AS 가 drop 됨 %q %q", swap[0].DupDropKeys, swap[1].DupDropKeys)
	}
}

func Test95_158_T4_DuplicateSubmitSavesOnce(t *testing.T) {
	e, repo := newWorkboardServer(t, "wb158.db")
	tok := newWorkSubmitToken()
	form := url.Values{
		"work_type": {"admin"}, "title": {"158중복저장방지"}, "assignee": {"최혜영"},
		"work_date": {"2026-11-20"}, "due_date": {"2026-11-20"},
		"start_time": {"09:00"}, "end_time": {"09:30"},
		"submit_token": {tok}, "redirect": {"/workboard/kanban"},
	}
	first := doForm(t, e, "/workboard/tasks", form)
	if first.Code != http.StatusSeeOther {
		t.Fatalf("1st %d %s", first.Code, first.Body.String())
	}
	second := doForm(t, e, "/workboard/tasks", form)
	if second.Code != http.StatusSeeOther {
		t.Fatalf("2nd %d", second.Code)
	}
	n := 0
	if err := repo.DB().QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE title=?`, "158중복저장방지").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("저장 건수 %d", n)
	}
	form.Set("submit_token", newWorkSubmitToken())
	third := doForm(t, e, "/workboard/tasks", form)
	if third.Code != http.StatusSeeOther {
		t.Fatalf("recent 3rd %d", third.Code)
	}
	if err := repo.DB().QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE title=?`, "158중복저장방지").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("5초 창 후 건수 %d", n)
	}
}

func Test95_158_T5_KeepModalJS(t *testing.T) {
	cmd := exec.Command("node", "keep_modal_test.js")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("js: %v\n%s", err, out)
	}
}

func Test95_158_T6_BrowserUnplacedAndDoubleSave(t *testing.T) {
	if os.Getenv("SKIP_PLAYWRIGHT") == "1" {
		t.Skip("playwright skip")
	}
	e, db, _, _, repo := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p158br.db"))
	oa, err := repository.NewUserRepo(db).GetByUsername("oa01")
	if err != nil || oa == nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO customers (customer_id, org_name, official_name, is_active, org_id)
		VALUES ('c-158','정안초등학교','정안초등학교',1,?)`, oa.OrgID); err != nil {
		t.Fatal(err)
	}
	h := New(db)
	as := &model.ASReceipt{
		CustomerID: "c-158", ReceiptChannel: "phone", Requester: "학교",
		Symptom: "158브라우저AS", AssignedTo: oa.FullName, AssignedUserID: oa.UserID,
		ReceivedBy: oa.FullName, ReceiptDatetime: time.Date(2026, 11, 9, 9, 0, 0, 0, time.Local),
		OrgID: oa.OrgID,
	}
	if err := h.AS.repo.Create(as); err != nil {
		t.Fatal(err)
	}
	if err := repo.SyncASDailyTask(as); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE work_tasks SET org_id=?, work_date='', due_date='' WHERE source_type=? AND source_id=?`,
		oa.OrgID, model.WBSourceAS, as.ASID); err != nil {
		t.Fatal(err)
	}
	staticDir, err := filepath.Abs(filepath.Join(findTemplateRoot(t), "..", "static"))
	if err != nil {
		t.Fatal(err)
	}
	e.Static("/static", staticDir)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	shot := filepath.Join(findTemplateRoot(t), "..", "..", "..", "docs", "리뷰", "screenshots", "95-158")
	shot, _ = filepath.Abs(shot)
	cmd := exec.Command("node", "95_158_unplaced_submit_browser.mjs")
	cmd.Dir = filepath.Join(findTemplateRoot(t), "..", "static", "js")
	cmd.Env = append(os.Environ(),
		"PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1",
		"BASE_URL="+srv.URL,
		"AUTH_COOKIE="+jwtCookieVerified(t, oa).Value,
		"AS_ID="+as.ASID,
		"SCREENSHOT_DIR="+shot,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "Executable doesn't exist") || strings.Contains(s, "browserType.launch") || (strings.Contains(s, "playwright") && strings.Contains(s, "install")) {
			t.Skip(s)
		}
		t.Fatalf("browser: %v\n%s", err, s)
	}
	n := 0
	if err := db.QueryRow(`SELECT COUNT(*) FROM work_tasks WHERE title=?`, "158브라우저중복저장").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n > 1 {
		t.Fatalf("브라우저 더블클릭 저장 %d건", n)
	}
}

func Test95_158_NewWorkFormHasToken(t *testing.T) {
	e, db, me, _, _ := seedPersonalBoard(t, "sqlite", filepath.Join(t.TempDir(), "p158tok.db"))
	_ = db
	body := doGetWith(t, e, "/workboard/new-work/form?kind=admin&date=2026-11-09", jwtCookieUser(t, me)).Body.String()
	if !strings.Contains(body, `name="submit_token"`) {
		t.Fatal("submit_token 없음")
	}
	board := doGetWith(t, e, "/workboard/board?view=day&mine=0&auto=0", jwtCookieUser(t, me)).Body.String()
	if !strings.Contains(board, "keep_modal.js") {
		t.Fatal("보드에 keep_modal 없음")
	}
}
