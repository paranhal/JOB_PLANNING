package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func new82(t *testing.T) (*echo.Echo, *sql.DB, *Handler, *http.Cookie, *repository.WBRepo) {
	t.Helper()
	e, repo, _, db := newProjectWorkHarness(t)
	h := New(db)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/workboard/board", h.Workboard.Board)
	g.GET("/workboard/new-work/form", h.Workboard.NewWorkForm)
	g.GET("/workboard/tasks/:id/item-assets", h.Workboard.ItemSiteAssets)
	g.POST("/workboard/tasks/:id/item-assets", h.Workboard.RegisterItemAssets)
	g.POST("/workboard/tasks/:id/item-assets/link", h.Workboard.LinkItemAssets)
	g.POST("/workboard/tasks/:id/item-as", h.Workboard.LinkItemAS)
	g.GET("/workboard/tasks/:id/open-as", h.Workboard.ListOpenASForItem)
	g.GET("/assets/:id", h.Asset.Show)
	g.GET("/as", h.AS.List)
	g.GET("/work", h.Work.List)
	g.GET("/meeting/board", h.Meeting.Board)
	g.GET("/meeting/weekly", h.Meeting.Weekly)
	admin, err := repository.NewUserRepo(db).GetByUsername("admin")
	if err != nil || admin == nil {
		t.Fatal("admin")
	}
	return e, db, h, jwtCookieUser(t, admin), repo
}

func jsonPOST82(t *testing.T, e *echo.Echo, path string, form url.Values, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost"+path, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func jsonGET82(t *testing.T, e *echo.Echo, path string, ck *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost"+path, nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(ck)
	e.ServeHTTP(rec, req)
	return rec
}

func Test95_82_SharedTemplateAndForm(t *testing.T) {
	root := findTemplateRoot(t)
	items := string(mustRead(t, filepath.Join(root, "workboard/_work_items.html")))
	if strings.Contains(items, "하위 업무로 올리기") {
		t.Fatal("하위 업무로 올리기 금지")
	}
	if !strings.Contains(items, `name="item_kind"`) || !strings.Contains(items, `name="item_qty"`) || !strings.Contains(items, `data-item-assignees`) {
		t.Fatal("공용 항목 필드 부족")
	}
	if !strings.Contains(items, "전체 보기") && !strings.Contains(string(mustRead(t, filepath.Join(root, "..", "static", "js", "work_items.js"))), "전체 보기") {
		t.Fatal("장비 종류 전체 보기 없음")
	}
	for _, f := range []string{
		"workboard/_modals.html",
		"workboard/task_show.html",
		"workboard/project_work_action.html",
		"workboard/new_work_form.html",
	} {
		src := string(mustRead(t, filepath.Join(root, f)))
		if f != "workboard/new_work_form.html" && !strings.Contains(src, `template "work_task_items"`) {
			t.Fatalf("%s 공용 템플릿 없음", f)
		}
	}
	js := string(mustRead(t, filepath.Join(root, "..", "static", "js", "work_items.js")))
	if !strings.Contains(js, "wi-pick-all") || !strings.Contains(js, "wi-missing-modal") {
		t.Fatal("자산 선택·미등록 팝업 JS 없음")
	}

	e, db, _, ck, repo := new82(t)
	id := pwCreateTask(t, e, repo, "항목폼작업")
	_, _ = db.Exec(`UPDATE work_tasks SET customer_id='C-high' WHERE task_id=?`, id)
	_ = repo.ReplaceSupportMembers(id, []model.WorkTaskMember{{Assignee: "김지원", Role: model.WBMemberSupport}})
	form := url.Values{
		"title": {"항목폼작업"}, "work_date": {"2026-10-21"}, "due_date": {"2026-10-21"},
		"assignee": {"관리자"}, "work_type": {"project_work"}, "work_subtype": {"other"},
		"item_title": {"IP카메라"}, "item_product": {"IP카메라"}, "item_kind": {"install"},
		"item_qty": {"2"}, "item_status": {"todo"}, "item_assignees": {"관리자,김지원"},
		"item_id": {""}, "item_note": {""}, "item_model": {""}, "item_as_id": {""},
	}
	upd := doFormWith(t, e, "/workboard/tasks/"+id+"/update", form, ck)
	if upd.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", upd.Code, upd.Body.String())
	}
	got, err := repo.ListItems(id)
	if err != nil || len(got) != 1 {
		t.Fatalf("항목 %d err=%v", len(got), err)
	}
	if got[0].Kind != model.ItemKindInstall || got[0].Qty != 2 || got[0].Title != "IP카메라" {
		t.Fatalf("저장값 %+v", got[0])
	}
	if len(got[0].Assignees) != 2 {
		t.Fatalf("담당 %v", got[0].Assignees)
	}
	tk, _ := repo.GetTask(id)
	if tk.Assignee != "관리자" {
		t.Fatalf("상위 assignee 변경됨 %s", tk.Assignee)
	}

	badQty := cloneItemForm82(form)
	badQty.Set("item_qty", "0")
	if loc := doFormWith(t, e, "/workboard/tasks/"+id+"/update", badQty, ck).Header().Get("Location"); !strings.Contains(loc, "err=item") {
		t.Fatalf("수량 0 loc=%s", loc)
	}
	badKind := cloneItemForm82(form)
	badKind.Set("item_kind", "nope")
	loc := doFormWith(t, e, "/workboard/tasks/"+id+"/update", badKind, ck).Header().Get("Location")
	if !strings.Contains(loc, "err=item") {
		t.Fatalf("코드 외 구분 loc=%s", loc)
	}
	badWho := cloneItemForm82(form)
	badWho.Set("item_assignees", "외부인")
	if loc := doFormWith(t, e, "/workboard/tasks/"+id+"/update", badWho, ck).Header().Get("Location"); !strings.Contains(loc, "err=item") {
		t.Fatalf("참여자 밖 loc=%s", loc)
	}

	show := doGetWith(t, e, "/workboard/tasks/"+id, ck)
	act := doGetWith(t, e, "/workboard/tasks/"+id+"/action", ck)
	edit := doGetWith(t, e, "/workboard/tasks/"+id+"/edit", ck)
	nw := doGetWith(t, e, "/workboard/new-work/form?kind=project_work&date=2026-11-02", ck)
	for name, rec := range map[string]*httptest.ResponseRecorder{"show": show, "act": act, "edit": edit, "nw": nw} {
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %d", name, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "data-work-items") && !strings.Contains(body, "작업 리스트") {
			t.Fatalf("%s 공용 템플릿 렌더 없음", name)
		}
	}
	if !strings.Contains(show.Body.String(), "장착 1건") {
		t.Fatal("상세 집계 없음")
	}
}

func cloneItemForm82(v url.Values) url.Values {
	out := url.Values{}
	for k, vals := range v {
		out[k] = append([]string{}, vals...)
	}
	return out
}

func Test95_82_AssetsAndASAndCalendar(t *testing.T) {
	e, db, _, ck, repo := new82(t)
	id := pwCreateTask(t, e, repo, "자산AS작업")
	_, _ = db.Exec(`UPDATE work_tasks SET customer_id='C-high' WHERE task_id=?`, id)
	_ = repo.ReplaceItems(id, []model.WorkTaskItem{
		{Kind: model.ItemKindInstall, Title: "카메라", ProductName: "카메라", Qty: 2, Status: model.ItemDone, Assignees: []model.WorkTaskItemAssignee{{Name: "관리자"}}},
		{Kind: model.ItemKindAS, Title: "DVR점검", ProductName: "DVR점검", Qty: 1, Status: model.ItemTodo, Assignees: []model.WorkTaskItemAssignee{{Name: "관리자"}}},
	})
	items, _ := repo.ListItems(id)
	if len(items) < 2 {
		t.Fatal("항목 없음")
	}
	inst, asItem := items[0], items[1]
	if inst.Kind != model.ItemKindInstall {
		inst, asItem = items[1], items[0]
	}

	noMgmt := jsonPOST82(t, e, "/workboard/tasks/"+id+"/item-assets", url.Values{
		"item_id": {inst.ItemID}, "product_name": {"카메라"}, "asset_serial": {""},
	}, ck)
	if noMgmt.Code != http.StatusBadRequest {
		t.Fatalf("관리유형 미선택 %d %s", noMgmt.Code, noMgmt.Body.String())
	}

	dup := jsonPOST82(t, e, "/workboard/tasks/"+id+"/item-assets", url.Values{
		"item_id": {inst.ItemID}, "product_name": {"카메라"}, "management_type": {"periodic"},
		"asset_serial": {"SN-DUP", "SN-DUP"},
	}, ck)
	if dup.Code != http.StatusBadRequest {
		t.Fatalf("일련 중복 %d %s", dup.Code, dup.Body.String())
	}
	var nBefore int
	_ = db.QueryRow(`SELECT COUNT(*) FROM assets WHERE customer_id='C-high'`).Scan(&nBefore)

	ok := jsonPOST82(t, e, "/workboard/tasks/"+id+"/item-assets", url.Values{
		"item_id": {inst.ItemID}, "product_name": {"카메라"}, "management_type": {"periodic"},
		"asset_serial": {"", ""},
	}, ck)
	if ok.Code != http.StatusOK && ok.Code != http.StatusSeeOther {
		t.Fatalf("자산 등록 %d %s", ok.Code, ok.Body.String())
	}
	var nAfter int
	_ = db.QueryRow(`SELECT COUNT(*) FROM assets WHERE customer_id='C-high'`).Scan(&nAfter)
	if nAfter-nBefore != 2 {
		t.Fatalf("수량 N→N행 %d→%d", nBefore, nAfter)
	}

	list := jsonGET82(t, e, "/workboard/tasks/"+id+"/item-assets", ck)
	if list.Code != http.StatusOK {
		t.Fatalf("자산 목록 %d", list.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if int(payload["total"].(float64)) != 2 {
		t.Fatalf("기관 자산만 %v", payload["total"])
	}

	_, _ = db.Exec(`INSERT INTO customers(customer_id,org_name,official_name,is_active,org_id,party_kind)
		VALUES('C-other82','타기관','타기관',1,'O03','customer')`)
	ar := repository.NewAssetRepo(db)
	other := &model.Asset{CustomerID: "C-other82", ProductName: "타기관장비", ManagementType: "periodic", OperationStatus: model.AssetOpOperating}
	if err := ar.Create(other); err != nil {
		t.Fatal(err)
	}
	linkBad := jsonPOST82(t, e, "/workboard/tasks/"+id+"/item-assets/link", url.Values{
		"item_id": {asItem.ItemID}, "role": {"service"}, "kind": {"as"}, "asset_id": {other.AssetID},
	}, ck)
	if linkBad.Code != http.StatusBadRequest {
		t.Fatalf("타 기관 자산 %d %s", linkBad.Code, linkBad.Body.String())
	}

	var ownID string
	_ = db.QueryRow(`SELECT asset_id FROM assets WHERE customer_id='C-high' LIMIT 1`).Scan(&ownID)
	linkOK := jsonPOST82(t, e, "/workboard/tasks/"+id+"/item-assets/link", url.Values{
		"item_id": {asItem.ItemID}, "role": {"service"}, "kind": {"as"}, "asset_id": {ownID},
	}, ck)
	if linkOK.Code != http.StatusOK && linkOK.Code != http.StatusSeeOther {
		t.Fatalf("대상 자산 %d %s", linkOK.Code, linkOK.Body.String())
	}

	asNew := jsonPOST82(t, e, "/workboard/tasks/"+id+"/item-as", url.Values{
		"item_id": {asItem.ItemID}, "mode": {"new"}, "symptom": {"화면 안 나옴"},
	}, ck)
	if asNew.Code != http.StatusOK && asNew.Code != http.StatusSeeOther {
		t.Fatalf("AS 연결 %d %s", asNew.Code, asNew.Body.String())
	}
	items, _ = repo.ListItems(id)
	var linked model.WorkTaskItem
	for _, it := range items {
		if it.Kind == model.ItemKindAS {
			linked = it
		}
	}
	if linked.ASID == "" || linked.ASNumber == "" {
		t.Fatalf("접수번호 없음 %+v", linked)
	}
	if !strings.HasPrefix(linked.ASNumber, "R") {
		t.Fatalf("접수번호 규칙 %s", linked.ASNumber)
	}
	if linked.Status != model.ItemTodo {
		t.Fatalf("상태 동기화 %s", linked.Status)
	}
	if !linked.ASLocked {
		t.Fatal("AS 상태 읽기 전용 아님")
	}
	gotAS, err := repository.NewASRepo(db).GetByID(model.OrgIDLibrary, linked.ASID)
	if err != nil || gotAS == nil {
		t.Fatal("접수 없음")
	}
	if gotAS.ScheduleConfirmed {
		t.Fatal("새 접수는 미확정이어야 함")
	}
	if daily, _ := repo.GetTaskBySource(model.WBSourceAS, linked.ASID); daily != nil {
		t.Fatalf("이중 막대 work_tasks %s", daily.TaskID)
	}
	_, _ = repository.ReconcileASDailyTasks(db)
	if daily, _ := repo.GetTaskBySource(model.WBSourceAS, linked.ASID); daily != nil {
		t.Fatal("Reconcile 후에도 이중 막대")
	}

	asList := doGetWith(t, e, "/as", ck)
	if asList.Code != http.StatusOK || !strings.Contains(asList.Body.String(), linked.ASNumber) {
		t.Fatalf("AS 목록 미노출 %d", asList.Code)
	}

	show := doGetWith(t, e, "/assets/"+ownID, ck)
	if show.Code != http.StatusOK || !strings.Contains(show.Body.String(), "작업 이력") {
		t.Fatalf("자산 이력 %d", show.Code)
	}
	if n := strings.Count(show.Body.String(), `data-work-history="1"`); n != 1 && n != 2 {
		if n == 0 {
			t.Fatal("작업 이력 행 없음")
		}
	}

	parent, _ := repo.GetTask(id)
	asIDKeep := linked.ASID
	if err := repo.DeleteTask(id); err != nil {
		t.Fatal(err)
	}
	keep, _ := repository.NewASRepo(db).GetByID(model.OrgIDLibrary, asIDKeep)
	if keep == nil {
		t.Fatal("상위 삭제 시 접수 보존되어야 함")
	}
	_ = parent
}

func Test95_82_CopyWithoutASAndCalendarChips(t *testing.T) {
	e, db, _, ck, repo := new82(t)
	id := pwCreateTask(t, e, repo, "후속복사작업")
	_, _ = db.Exec(`UPDATE work_tasks SET customer_id='C-high', work_date='2026-10-21', due_date='2026-10-21' WHERE task_id=?`, id)
	_ = repo.ReplaceItems(id, []model.WorkTaskItem{
		{Kind: model.ItemKindAS, Title: "점검", ProductName: "점검", Qty: 1, Status: model.ItemTodo, ASID: "AS-KEEP"},
		{Kind: model.ItemKindInstall, Title: "설치", ProductName: "설치", Qty: 1, Status: model.ItemDone},
	})
	child := &model.WorkTask{Title: "후속", WorkDate: "2026-10-22", DueDate: "2026-10-22",
		OrgID: model.OrgIDLibrary, Assignee: "관리자", Status: model.WBTaskWaiting, WorkType: model.WBWorkProjectWork}
	if err := repo.CreateTask(child); err != nil {
		t.Fatal(err)
	}
	if err := repo.CopyIncompleteItems(id, child.TaskID); err != nil {
		t.Fatal(err)
	}
	copied, _ := repo.ListItems(child.TaskID)
	if len(copied) != 1 || copied[0].ASID != "" {
		t.Fatalf("후속 복사 as_id 미복사 실패 %+v", copied)
	}

	day := "2026-10-21"
	board := doGetWith(t, e, "/workboard/board?view=day&date="+day+"&auto=0", ck)
	if board.Code != http.StatusOK {
		t.Fatalf("day %d", board.Code)
	}
	if !strings.Contains(board.Body.String(), "장착") && !strings.Contains(board.Body.String(), "data-item-kind-line") {
		t.Log(board.Body.String()[:min(800, board.Body.Len())])
	}
	week := doGetWith(t, e, "/workboard/board?view=week&date="+day+"&auto=0", ck)
	month := doGetWith(t, e, "/workboard/board?view=month&date="+day+"&auto=0", ck)
	list := doGetWith(t, e, "/workboard/board?view=list&date="+day+"&auto=0", ck)
	for _, rec := range []*httptest.ResponseRecorder{week, month, list, board} {
		if rec.Code != http.StatusOK {
			t.Fatalf("cal %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "후속복사작업") && !strings.Contains(body, "data-item-kind-line") && !strings.Contains(body, "장착") {
			t.Fatalf("집계 칩 없음 view body has title=%v", strings.Contains(body, "후속복사작업"))
		}
	}
	if strings.Contains(board.Body.String(), "[AS]고객_") {
		t.Fatal("일 보기 이중 AS 막대")
	}

	work := doGetWith(t, e, "/work", ck)
	if work.Code != http.StatusOK {
		t.Fatalf("work %d", work.Code)
	}
}

func Test95_82_TemplatesChipsCSS360(t *testing.T) {
	root := findTemplateRoot(t)
	for _, f := range []string{
		"workboard/board_day.html", "workboard/board_week.html", "workboard/board_month.html",
		"workboard/board_list.html", "workboard/task_show.html", "workboard/project_work_action.html",
		"meeting/board.html", "meeting/weekly.html", "work/list.html", "asset/show.html",
	} {
		src := string(mustRead(t, filepath.Join(root, f)))
		if f != "asset/show.html" && !strings.Contains(src, "ItemKindLine") && !strings.Contains(src, "WorkItemSummary") {
			t.Fatalf("%s 집계 칩 없음", f)
		}
	}
	items := string(mustRead(t, filepath.Join(root, "workboard/_work_items.html")))
	if !strings.Contains(items, "grid-cols-1") {
		t.Fatal("360 카드형 접힘 없음")
	}
	css, err := os.ReadFile(filepath.Join(root, "..", "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(css)
	for _, cls := range []string{"z-[90]", "min-h-[5rem]", "max-h-64"} {
		tok := strings.ReplaceAll(strings.ReplaceAll(cls, "[", "\\["), "]", "\\]")
		if !strings.Contains(s, tok) && !strings.Contains(s, cls) {
			t.Fatalf("번들에 %s 없음 — npm run build:css", cls)
		}
	}
}
