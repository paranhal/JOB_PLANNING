package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func new83(t *testing.T) (*echo.Echo, *sql.DB, *Handler, *http.Cookie, *repository.WBRepo) {
	t.Helper()
	e, db, h, ck, repo := new82(t)
	g := e.Group("")
	g.Use(h.Auth.AuthMiddleware)
	g.GET("/work-item-templates", h.ItemTemplate.Page)
	g.POST("/work-item-templates", h.ItemTemplate.Create)
	g.POST("/work-item-templates/:id", h.ItemTemplate.Update)
	g.POST("/work-item-templates/:id/delete", h.ItemTemplate.Delete)
	g.POST("/work-item-templates/:id/deactivate", h.ItemTemplate.Deactivate)
	g.GET("/work-item-templates.json", h.ItemTemplate.ListJSON)
	return e, db, h, ck, repo
}

func seed83Pipe(t *testing.T, h *Handler) (pipe, dig, back, fence *model.ItemTemplate) {
	t.Helper()
	r := h.ItemTemplate.repo
	dig = &model.ItemTemplate{Name: "터파기", Kind: model.ItemKindWork, DefaultQty: 1, Unit: "식"}
	if err := r.Create(dig, nil); err != nil {
		t.Fatal(err)
	}
	back = &model.ItemTemplate{Name: "되메우기·복구", Kind: model.ItemKindWork, DefaultQty: 1, Unit: "식"}
	if err := r.Create(back, nil); err != nil {
		t.Fatal(err)
	}
	fence = &model.ItemTemplate{Name: "안전펜스", Kind: model.ItemKindWork, DefaultQty: 1, Unit: "식"}
	if err := r.Create(fence, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.Update(dig, []model.ItemTemplateLink{{ChildTemplateID: fence.TemplateID, DefaultChecked: true}}); err != nil {
		t.Fatal(err)
	}
	pipe = &model.ItemTemplate{Name: "야외 배관 작업", Kind: model.ItemKindWork, DefaultQty: 1, Unit: "식"}
	if err := r.Create(pipe, []model.ItemTemplateLink{
		{ChildTemplateID: dig.TemplateID, DefaultChecked: true, SortOrder: 1},
		{ChildTemplateID: back.TemplateID, DefaultChecked: false, SortOrder: 2},
	}); err != nil {
		t.Fatal(err)
	}
	return pipe, dig, back, fence
}

func itemRow83(title, kind, qty, unit, tpl, key, setMain, included, status, assignees string) url.Values {
	return url.Values{
		"item_title": {title}, "item_product": {title}, "item_kind": {kind}, "item_qty": {qty},
		"item_unit": {unit}, "item_status": {status}, "item_assignees": {assignees},
		"item_id": {""}, "item_note": {""}, "item_model": {""}, "item_as_id": {""},
		"item_template_id": {tpl}, "item_client_key": {key}, "item_set_main_key": {setMain},
		"item_included": {included},
	}
}

func mergeItemRows83(base url.Values, rows ...url.Values) url.Values {
	out := cloneItemForm82(base)
	for _, r := range rows {
		for k, vs := range r {
			out[k] = append(out[k], vs...)
		}
	}
	return out
}

func Test95_83_MasterCRUDPermissionAndMenu(t *testing.T) {
	e, _, h, _, _ := new83(t)
	vision := jwtCookieVision(t)
	page := doGetWith(t, e, "/work-item-templates", vision)
	if page.Code != http.StatusOK {
		t.Fatalf("관리 화면 %d %s", page.Code, page.Body.String())
	}
	body := page.Body.String()
	if !strings.Contains(body, "작업 항목 관리") || !strings.Contains(body, "열 너비 초기화") {
		t.Fatal("목록 렌더 부족")
	}
	if !strings.Contains(body, `data-table-cols="work_item_templates"`) {
		t.Fatal("95-68 열 너비 없음")
	}
	if !strings.Contains(body, "함께 하는 항목") {
		t.Fatal("등록 모달 세트 없음")
	}

	create := doFormWith(t, e, "/work-item-templates", url.Values{
		"name": {"야외 배관 작업"}, "kind": {"work"}, "default_qty": {"1"}, "unit": {"식"}, "org_id": {""},
	}, vision)
	if create.Code != http.StatusSeeOther {
		t.Fatalf("등록 %d %s", create.Code, create.Body.String())
	}
	dup := doFormWith(t, e, "/work-item-templates", url.Values{
		"name": {"야외 배관 작업"}, "kind": {"work"}, "default_qty": {"1"}, "unit": {"식"},
	}, vision)
	if loc := dup.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("중복 loc=%s", loc)
	}
	zero := doFormWith(t, e, "/work-item-templates", url.Values{
		"name": {"수량0"}, "kind": {"work"}, "default_qty": {"0"}, "unit": {"건"},
	}, vision)
	if loc := zero.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("수량0 loc=%s", loc)
	}
	badU := doFormWith(t, e, "/work-item-templates", url.Values{
		"name": {"단위이상"}, "kind": {"work"}, "default_qty": {"1"}, "unit": {"kg"},
	}, vision)
	if loc := badU.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("단위 loc=%s", loc)
	}
	badK := doFormWith(t, e, "/work-item-templates", url.Values{
		"name": {"구분이상"}, "kind": {"nope"}, "default_qty": {"1"}, "unit": {"건"},
	}, vision)
	if loc := badK.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("구분 loc=%s", loc)
	}

	dig := doFormWith(t, e, "/work-item-templates", url.Values{
		"name": {"터파기"}, "kind": {"work"}, "default_qty": {"1"}, "unit": {"식"},
	}, vision)
	if dig.Code != http.StatusSeeOther {
		t.Fatal("터파기")
	}
	list, _ := h.ItemTemplate.repo.List("", "", "", true, true)
	var pipeID, digID string
	for _, it := range list {
		if it.Name == "야외 배관 작업" {
			pipeID = it.TemplateID
		}
		if it.Name == "터파기" {
			digID = it.TemplateID
		}
	}
	self := doFormWith(t, e, "/work-item-templates/"+pipeID, url.Values{
		"name": {"야외 배관 작업"}, "kind": {"work"}, "default_qty": {"1"}, "unit": {"식"},
		"link_child": {pipeID}, "link_checked": {pipeID},
	}, vision)
	if loc := self.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("자기연결 loc=%s", loc)
	}
	okLink := doFormWith(t, e, "/work-item-templates/"+pipeID, url.Values{
		"name": {"야외 배관 작업"}, "kind": {"work"}, "default_qty": {"1"}, "unit": {"식"},
		"link_child": {digID}, "link_checked": {digID},
	}, vision)
	if okLink.Code != http.StatusSeeOther {
		t.Fatalf("세트 저장 %d", okLink.Code)
	}
	cycle := doFormWith(t, e, "/work-item-templates/"+digID, url.Values{
		"name": {"터파기"}, "kind": {"work"}, "default_qty": {"1"}, "unit": {"식"},
		"link_child": {pipeID}, "link_checked": {pipeID},
	}, vision)
	if loc := cycle.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("순환 loc=%s", loc)
	}

	oa, err := repository.NewUserRepo(h.Workboard.repo.DB()).GetByUsername("oa03")
	if err != nil || oa == nil {
		t.Fatal("oa03")
	}
	oaCK := jwtCookieUser(t, oa)
	oaPage := doGetWith(t, e, "/work-item-templates", oaCK)
	if oaPage.Code != http.StatusOK {
		t.Fatalf("조직관리자 화면 %d", oaPage.Code)
	}
	if strings.Contains(oaPage.Body.String(), `editId='`+pipeID+`'`) {
		t.Fatal("조직관리자가 전사 항목 수정 가능")
	}
	forbid := doFormWith(t, e, "/work-item-templates/"+pipeID, url.Values{
		"name": {"야외 배관 작업"}, "kind": {"work"}, "default_qty": {"1"}, "unit": {"식"},
	}, oaCK)
	if forbid.Code != http.StatusForbidden {
		t.Fatalf("전사 수정 %d", forbid.Code)
	}
	own := doFormWith(t, e, "/work-item-templates", url.Values{
		"name": {"SI전용굴착"}, "kind": {"work"}, "default_qty": {"1"}, "unit": {"식"},
	}, oaCK)
	if own.Code != http.StatusSeeOther {
		t.Fatalf("조직 전용 등록 %d %s", own.Code, own.Body.String())
	}

	tech := jwtCookieOrg(t, model.RoleTech, "O03")
	if rec := doGetWith(t, e, "/work-item-templates", tech); rec.Code != http.StatusForbidden {
		t.Fatalf("기술 접근 %d", rec.Code)
	}

	js := jsonGET82(t, e, "/work-item-templates.json", oaCK)
	if js.Code != http.StatusOK {
		t.Fatalf("json %d", js.Code)
	}
	if !strings.Contains(js.Body.String(), "야외 배관 작업") || !strings.Contains(js.Body.String(), "SI전용굴착") {
		t.Fatal("자기 조직+전사 JSON 부족")
	}
	techJS := jsonGET82(t, e, "/work-item-templates.json", jwtCookieOrg(t, model.RoleTech, "O04"))
	if strings.Contains(techJS.Body.String(), "SI전용굴착") {
		t.Fatal("타 조직 항목이 선택 목록에 있다")
	}

	res := FilterNav(MenuCtx{Role: model.RoleOrgAdmin, LoginRole: model.RoleOrgAdmin, Perms: model.DefaultPermissions(model.RoleOrgAdmin)}, NavWorkItemTemplates, NavBadges{})
	hrefs := collectNavHrefs(res.Groups)
	if !containsHref(hrefs, "/work-item-templates") {
		t.Fatal("조직관리자 메뉴에 작업 항목 관리 없음")
	}
	if !containsHref(hrefs, "/product-groups") && true {
		// 제품군은 vision only — 작업 항목은 그 근처(설정·관리)에 둔다.
	}
	vnav := FilterNav(MenuCtx{Role: model.RoleVisionAdmin, LoginRole: model.RoleVisionAdmin, Perms: model.DefaultPermissions(model.RoleVisionAdmin)}, NavWorkItemTemplates, NavBadges{})
	vhrefs := collectNavHrefs(vnav.Groups)
	if !containsHref(vhrefs, "/work-item-templates") || !containsHref(vhrefs, "/product-groups") {
		t.Fatal("비전 메뉴 위치 확인 실패")
	}
}

func Test95_83_SetExpandSnapshotSummaryCopy(t *testing.T) {
	e, db, h, ck, repo := new83(t)
	pipe, dig, back, fence := seed83Pipe(t, h)
	id := pwCreateTask(t, e, repo, "배관세트작업")
	nw := doGetWith(t, e, "/workboard/new-work/form?kind=project_work&date=2026-11-02", ck)
	if nw.Code != http.StatusOK {
		t.Fatalf("새업무 %d", nw.Code)
	}
	if strings.Contains(nw.Body.String(), "직접 입력 항목") {
		t.Fatal("새 업무 등록에 작업 항목 선택이 남아 있다")
	}
	edit := doGetWith(t, e, "/workboard/tasks/"+id+"/edit", ck)
	if edit.Code != http.StatusOK {
		t.Fatalf("수정 %d", edit.Code)
	}
	nb := edit.Body.String()
	if !strings.Contains(nb, "직접 입력 항목") || !strings.Contains(nb, "야외 배관 작업") {
		t.Fatal("선택 목록 렌더 없음")
	}
	if !strings.Contains(nb, "터파기") || !strings.Contains(nb, "되메우기·복구") {
		t.Fatal("세트 JSON 렌더 없음")
	}
	if strings.Count(nb, fence.Name) > strings.Count(nb, pipe.Name)+2 {
		t.Fatal("한 단계 전개 범위를 넘는 렌더")
	}
	js := jsonGET82(t, e, "/work-item-templates.json", ck)
	var payload struct {
		Templates []struct {
			Name  string
			Links []struct {
				Name           string
				DefaultChecked bool
			}
		}
	}
	if err := json.Unmarshal(js.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var pipeLinks []string
	for _, t0 := range payload.Templates {
		if t0.Name != "야외 배관 작업" {
			continue
		}
		for _, l := range t0.Links {
			pipeLinks = append(pipeLinks, l.Name)
			if l.Name == "터파기" && !l.DefaultChecked {
				t.Fatal("터파기 기본 포함이어야 함")
			}
			if l.Name == "되메우기·복구" && l.DefaultChecked {
				t.Fatal("되메우기 기본 제외이어야 함")
			}
			if l.Name == "안전펜스" {
				t.Fatal("연관의 연관이 전개됐다")
			}
		}
	}
	if !(containsName83(pipeLinks, "터파기") && containsName83(pipeLinks, "되메우기·복구") && !containsName83(pipeLinks, "안전펜스")) {
		t.Fatalf("링크 %v", pipeLinks)
	}

	base := url.Values{
		"title": {"배관세트작업"}, "work_date": {"2026-10-21"}, "due_date": {"2026-10-21"},
		"assignee": {"관리자"}, "work_type": {"project_work"}, "work_subtype": {"other"},
	}
	form := mergeItemRows83(base,
		itemRow83("야외 배관 작업", "work", "1", "식", pipe.TemplateID, "k-main", "", "1", "todo", "관리자"),
		itemRow83("터파기", "work", "1", "식", dig.TemplateID, "k-dig", "k-main", "1", "todo", "관리자"),
		itemRow83("되메우기·복구", "work", "1", "식", back.TemplateID, "k-back", "k-main", "0", "todo", "관리자"),
		itemRow83("직접추가", "etc", "1", "건", "", "k-free", "", "1", "todo", "관리자"),
		itemRow83("야외 배관 작업", "work", "1", "식", pipe.TemplateID, "k-dup", "", "1", "todo", "관리자"),
	)
	if rec := doFormWith(t, e, "/workboard/tasks/"+id+"/update", form, ck); rec.Code != http.StatusSeeOther {
		t.Fatalf("저장 %d %s", rec.Code, rec.Body.String())
	}
	got, err := repo.ListItems(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("제외 후 저장 %d %+v", len(got), got)
	}
	names := map[string]int{}
	for _, it := range got {
		names[it.Title]++
		if it.Title == "터파기" && it.SetMainItemID == "" {
			t.Fatal("터파기 세트 연결 없음")
		}
		if it.Title == "되메우기·복구" {
			t.Fatal("제외 행이 저장됐다")
		}
	}
	if names["야외 배관 작업"] != 2 || names["직접추가"] != 1 {
		t.Fatalf("이름 %v", names)
	}
	sum := model.SummarizeItemKinds(got)
	if sum.Work != 3 || sum.Etc != 1 {
		t.Fatalf("집계 %+v", sum)
	}

	dropDig := cloneItemForm82(base)
	for _, it := range got {
		inc := "1"
		if it.Title == "터파기" {
			inc = "0"
		}
		addSavedItem83(dropDig, it, inc)
	}
	if rec := doFormWith(t, e, "/workboard/tasks/"+id+"/update", dropDig, ck); rec.Code != http.StatusSeeOther {
		t.Fatalf("터파기 제외 %d", rec.Code)
	}
	got, _ = repo.ListItems(id)
	for _, it := range got {
		if it.Title == "터파기" {
			t.Fatal("체크 해제 후 터파기가 남음")
		}
	}

	keepChild := mergeItemRows83(base,
		itemRow83("터파기", "work", "1", "식", dig.TemplateID, "k-dig", "", "1", "todo", "관리자"),
		itemRow83("직접추가", "etc", "1", "건", "", "k-free", "", "1", "todo", "관리자"),
	)
	if rec := doFormWith(t, e, "/workboard/tasks/"+id+"/update", keepChild, ck); rec.Code != http.StatusSeeOther {
		t.Fatalf("주항목 삭제 %d", rec.Code)
	}
	got, _ = repo.ListItems(id)
	if len(got) != 2 {
		t.Fatalf("주항목 삭제 후 %d", len(got))
	}
	for _, it := range got {
		if it.Title == "터파기" && it.SetMainItemID != "" {
			t.Fatal("세트 연결이 남음")
		}
	}

	if err := h.ItemTemplate.repo.Update(pipe, nil); err != nil {
		t.Fatal(err)
	}
	_ = h.ItemTemplate.repo.Deactivate(pipe.TemplateID)
	got, _ = repo.ListItems(id)
	foundFree := false
	for _, it := range got {
		if it.Title == "직접추가" {
			foundFree = true
		}
		if it.Title == "야외 배관 작업" {
			t.Fatal("스냅샷이 주항목 삭제 전 이름에 묶여 있으면 안 됨(이미 삭제됨)")
		}
	}
	if !foundFree {
		t.Fatal("직접 입력 항목 사라짐")
	}
	snapID := pwCreateTask(t, e, repo, "스냅샷작업")
	snapForm := mergeItemRows83(url.Values{
		"title": {"스냅샷작업"}, "work_date": {"2026-10-21"}, "due_date": {"2026-10-21"},
		"assignee": {"관리자"}, "work_type": {"project_work"}, "work_subtype": {"other"},
	}, itemRow83("터파기", "work", "30", "m", dig.TemplateID, "k1", "", "1", "todo", "관리자"))
	if rec := doFormWith(t, e, "/workboard/tasks/"+snapID+"/update", snapForm, ck); rec.Code != http.StatusSeeOther {
		t.Fatalf("스냅샷 저장 %d", rec.Code)
	}
	if err := h.ItemTemplate.repo.Update(&model.ItemTemplate{
		TemplateID: dig.TemplateID, Name: "터파기(개명)", Kind: model.ItemKindWork, DefaultQty: 1, Unit: "식", OrgID: dig.OrgID,
	}, nil); err != nil {
		t.Fatal(err)
	}
	_ = h.ItemTemplate.repo.Deactivate(dig.TemplateID)
	items, _ := repo.ListItems(snapID)
	if len(items) != 1 || items[0].Title != "터파기" || items[0].Unit != "m" {
		t.Fatalf("스냅샷 %+v", items)
	}
	js2 := jsonGET82(t, e, "/work-item-templates.json", ck)
	if strings.Contains(js2.Body.String(), "터파기(개명)") || strings.Contains(js2.Body.String(), "야외 배관 작업") {
		t.Fatal("비활성 항목이 선택 목록에 남음")
	}

	show := doGetWith(t, e, "/workboard/tasks/"+snapID, ck)
	if !strings.Contains(show.Body.String(), "터파기") {
		t.Fatal("상세 스냅샷 이름 없음")
	}
	if !strings.Contains(show.Body.String(), "공사 1건") {
		t.Fatal("집계 없음")
	}

	if err := repo.CopyIncompleteItems(snapID, id); err != nil {
		t.Fatal(err)
	}
	copied, _ := repo.ListItems(id)
	var copiedDig bool
	for _, it := range copied {
		if it.Title == "터파기" {
			copiedDig = true
			if it.SetMainItemID != "" {
				t.Fatal("복사에 세트 연결이 남음")
			}
		}
	}
	if !copiedDig {
		t.Fatal("포함된 행 복사 안 됨")
	}

	lockID := pwCreateTask(t, e, repo, "잠금작업")
	lockForm := mergeItemRows83(url.Values{
		"title": {"잠금작업"}, "work_date": {"2026-10-21"}, "due_date": {"2026-10-21"},
		"assignee": {"관리자"}, "work_type": {"project_work"}, "work_subtype": {"other"},
	}, itemRow83("터파기", "work", "1", "식", "", "k-lock", "", "1", "done", "관리자"))
	if rec := doFormWith(t, e, "/workboard/tasks/"+lockID+"/update", lockForm, ck); rec.Code != http.StatusSeeOther {
		t.Fatal("잠금 저장")
	}
	locked, _ := repo.ListItems(lockID)
	excl := mergeItemRows83(url.Values{
		"title": {"잠금작업"}, "work_date": {"2026-10-21"}, "due_date": {"2026-10-21"},
		"assignee": {"관리자"}, "work_type": {"project_work"}, "work_subtype": {"other"},
	}, itemRow83("터파기", "work", "1", "식", "", "k-lock", "", "0", "done", "관리자"))
	excl.Del("item_id")
	excl.Add("item_id", locked[0].ItemID)
	loc := doFormWith(t, e, "/workboard/tasks/"+lockID+"/update", excl, ck).Header().Get("Location")
	if !strings.Contains(loc, "err=item") {
		t.Fatalf("완료 제외 loc=%s", loc)
	}

	inUseDel := doFormWith(t, e, "/work-item-templates/"+back.TemplateID+"/delete", url.Values{}, jwtCookieVision(t))
	if inUseDel.Code != http.StatusSeeOther {
		t.Fatalf("미사용 삭제 %d", inUseDel.Code)
	}
	used := doFormWith(t, e, "/work-item-templates/"+dig.TemplateID+"/delete", url.Values{}, jwtCookieVision(t))
	if loc := used.Header().Get("Location"); !strings.Contains(loc, "err=") {
		t.Fatalf("사용중 삭제 loc=%s", loc)
	}

	root := findTemplateRoot(t)
	src := string(mustRead(t, filepath.Join(root, "workboard/_work_items.html")))
	if !strings.Contains(src, "직접 입력 항목") || !strings.Contains(src, "addFromSelect") || !strings.Contains(src, "data-set-check") {
		t.Fatal("공용 템플릿 세트 UI 없음")
	}
	for _, f := range []string{"workboard/task_show.html", "workboard/project_work_action.html"} {
		b := string(mustRead(t, filepath.Join(root, f)))
		if !strings.Contains(b, `template "work_task_items"`) {
			t.Fatalf("%s 공용 템플릿 없음", f)
		}
	}
	css, err := os.ReadFile(filepath.Join(root, "..", "static", "vendor", "tailwind.css"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(css)
	for _, cls := range []string{"opacity-50", "z-[90]", "max-h-64"} {
		if !strings.Contains(s, cls) && !strings.Contains(s, strings.ReplaceAll(strings.ReplaceAll(cls, "[", "\\["), "]", "\\]")) {
			t.Fatalf("번들에 %s 없음", cls)
		}
	}
	_ = db
}

func addSavedItem83(v url.Values, it model.WorkTaskItem, included string) {
	v.Add("item_title", it.Title)
	v.Add("item_product", it.Title)
	v.Add("item_kind", it.Kind)
	qty := it.Qty
	if qty < 1 {
		qty = 1
	}
	v.Add("item_qty", strconv.Itoa(qty))
	v.Add("item_unit", it.Unit)
	v.Add("item_status", it.Status)
	asg := ""
	for i, a := range it.Assignees {
		if i > 0 {
			asg += ","
		}
		asg += a.Name
	}
	v.Add("item_assignees", asg)
	v.Add("item_id", it.ItemID)
	v.Add("item_note", "")
	v.Add("item_model", "")
	v.Add("item_as_id", it.ASID)
	v.Add("item_template_id", it.TemplateID)
	v.Add("item_client_key", it.ItemID)
	v.Add("item_set_main_key", it.SetMainItemID)
	v.Add("item_included", included)
}

func containsName83(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
