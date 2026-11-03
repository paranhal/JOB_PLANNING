package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func defaultItemKinds(codeRepo *repository.CodeRepo) []model.Code {
	if codeRepo != nil {
		if list, err := codeRepo.ActiveByGroup(model.CodeGroupItemKind); err == nil && len(list) > 0 {
			return list
		}
	}
	return []model.Code{
		{CodeValue: model.ItemKindInstall, CodeName: "장비 장착"},
		{CodeValue: model.ItemKindWork, CodeName: "공사"},
		{CodeValue: model.ItemKindAS, CodeName: "AS"},
		{CodeValue: model.ItemKindEtc, CodeName: "기타"},
	}
}

func itemMemberChips(h *WorkboardHandler, t *model.WorkTask) []model.WorkTaskItemAssignee {
	var out []model.WorkTaskItemAssignee
	seen := map[string]bool{}
	add := func(name, uid string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, model.WorkTaskItemAssignee{Name: name, UserID: strings.TrimSpace(uid)})
	}
	if t != nil {
		add(t.Assignee, t.AssigneeUserID)
		if mems, err := h.repo.ListMembers(t.TaskID); err == nil {
			for _, m := range mems {
				add(m.Assignee, m.UserID)
			}
		}
	}
	return out
}

func defaultItemUnits(codeRepo *repository.CodeRepo) []model.Code {
	if codeRepo != nil {
		if list, err := codeRepo.ActiveByGroup(model.CodeGroupItemUnit); err == nil && len(list) > 0 {
			return list
		}
	}
	return []model.Code{
		{CodeValue: "건", CodeName: "건"},
		{CodeValue: "대", CodeName: "대"},
		{CodeValue: "m", CodeName: "m"},
		{CodeValue: "식", CodeName: "식"},
	}
}

func attachWorkItemView(data map[string]interface{}, h *WorkboardHandler, t *model.WorkTask, readOnly bool) {
	if data == nil {
		return
	}
	data["ItemKinds"] = defaultItemKinds(h.codeRepo)
	data["ItemUnits"] = defaultItemUnits(h.codeRepo)
	data["WorkItemsReadOnly"] = readOnly
	org := ""
	if v, ok := data["ItemOrgID"].(string); ok {
		org = strings.TrimSpace(v)
	}
	if t != nil && strings.TrimSpace(t.OrgID) != "" {
		org = strings.TrimSpace(t.OrgID)
	}
	if h != nil && h.itemTpl != nil {
		list, _ := h.itemTpl.ListActiveForOrg(org)
		data["ItemTemplates"] = list
		data["ItemTemplatesJSON"] = repository.ItemTemplatePickJSON(list)
	}
	if t != nil {
		data["ItemMembers"] = itemMemberChips(h, t)
		if items, ok := data["WorkItems"].([]model.WorkTaskItem); ok {
			data["WorkItemSummary"] = model.SummarizeItemKinds(items)
		} else if items, err := h.repo.ListItems(t.TaskID); err == nil {
			data["WorkItems"] = items
			data["WorkItemSummary"] = model.SummarizeItemKinds(items)
		}
		data["ItemTaskID"] = t.TaskID
		data["ItemCustomerID"] = strings.TrimSpace(t.CustomerID)
		data["ItemWorkDate"] = strings.TrimSpace(t.WorkDate)
		if data["ItemWorkDate"] == "" {
			data["ItemWorkDate"] = strings.TrimSpace(t.DueDate)
		}
	} else {
		if data["ItemMembers"] == nil {
			data["ItemMembers"] = []model.WorkTaskItemAssignee{}
		}
		if data["WorkItems"] == nil {
			data["WorkItems"] = []model.WorkTaskItem{}
		}
	}
}

func stampBoardItemKinds(h *WorkboardHandler, items []boardItem) {
	if h == nil || h.repo == nil || len(items) == 0 {
		return
	}
	var ids []string
	for _, it := range items {
		if id := strings.TrimSpace(it.Task.TaskID); id != "" {
			ids = append(ids, id)
		}
	}
	sum, err := h.repo.ItemKindSummaries(ids)
	if err != nil {
		return
	}
	union, _ := h.repo.ItemAssigneeUnion(ids)
	for i := range items {
		id := strings.TrimSpace(items[i].Task.TaskID)
		if s := sum[id]; s.Line != "" || s.Total > 0 {
			items[i].ItemKindLine = s.Line
			items[i].ItemProgress = s.Progress
			items[i].RemainItemN = s.Total - s.Done
		}
		if names := union[id]; len(names) > 0 {
			items[i].AssigneeTitle = strings.Join(names, ", ")
			items[i].AssigneeLine = strings.Join(names, ", ")
			items[i].AssigneeName = names[0]
			if len(names) > 1 {
				items[i].AssigneeExtra = fmt.Sprintf("외 %d", len(names)-1)
			}
		}
	}
}

func (h *WorkboardHandler) ItemSiteAssets(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return err
	}
	cid := strings.TrimSpace(t.CustomerID)
	assets := []model.Asset{}
	hidden := 0
	if cid != "" {
		ar := repository.NewAssetRepo(h.repo.DB())
		all, _ := ar.ListByCustomer(cid)
		assets = all
	}
	type row struct {
		AssetID  string `json:"asset_id"`
		Name     string `json:"name"`
		Serial   string `json:"serial"`
		Category string `json:"category"`
		CatLabel string `json:"cat_label"`
		Location string `json:"location"`
	}
	cats := map[string]int{}
	var rows []row
	for _, a := range assets {
		cat := strings.TrimSpace(a.ProductCategory)
		if cat == "" {
			cat = "기타"
		}
		cats[cat]++
		loc := strings.TrimSpace(strings.TrimSpace(a.BuildingName + " " + a.FloorName + " " + a.RoomName + " " + a.InstallLocation))
		rows = append(rows, row{
			AssetID: a.AssetID, Name: a.ProductName, Serial: a.SerialNumber,
			Category: cat, CatLabel: cat, Location: loc,
		})
	}
	if cid != "" {
		_ = h.repo.DB().QueryRow(`SELECT COUNT(*) FROM assets WHERE TRIM(COALESCE(customer_id,''))!=?`, cid).Scan(&hidden)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"assets": rows, "categories": cats, "total": len(rows), "hidden": hidden, "customer_id": cid,
	})
}

func (h *WorkboardHandler) RegisterItemAssets(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanPlanEdit(c, t); err != nil {
		return err
	}
	itemID := strings.TrimSpace(c.FormValue("item_id"))
	if itemID == "" {
		return itemJSONErr(c, "항목이 없습니다")
	}
	mgmt := strings.TrimSpace(c.FormValue("management_type"))
	if !model.IsSelectableManagementType(mgmt) {
		return itemJSONErr(c, "관리유형을 선택하세요")
	}
	product := strings.TrimSpace(c.FormValue("product_name"))
	if product == "" {
		product = "장비"
	}
	modelName := strings.TrimSpace(c.FormValue("model_name"))
	mfr := strings.TrimSpace(c.FormValue("manufacturer"))
	installDate := strings.TrimSpace(c.FormValue("install_date"))
	if installDate == "" {
		installDate = strings.TrimSpace(t.WorkDate)
	}
	cid := strings.TrimSpace(c.FormValue("customer_id"))
	if cid == "" {
		cid = strings.TrimSpace(t.CustomerID)
	}
	if cid == "" {
		return itemJSONErr(c, "기관을 선택하세요")
	}
	_ = c.Request().ParseForm()
	serials := c.Request().PostForm["asset_serial"]
	locs := c.Request().PostForm["asset_location"]
	notes := c.Request().PostForm["asset_note"]
	n := len(serials)
	if n < 1 {
		n = 1
		serials = []string{""}
	}
	seen := map[string]bool{}
	ar := repository.NewAssetRepo(h.repo.DB())
	for i := 0; i < n; i++ {
		sn := ""
		if i < len(serials) {
			sn = strings.TrimSpace(serials[i])
		}
		if sn == "" {
			continue
		}
		if seen[sn] {
			return itemJSONErr(c, "일련번호가 중복됩니다")
		}
		seen[sn] = true
		var cnt int
		_ = h.repo.DB().QueryRow(`SELECT COUNT(*) FROM assets WHERE customer_id=? AND TRIM(COALESCE(serial_number,''))=?`, cid, sn).Scan(&cnt)
		if cnt > 0 {
			return itemJSONErr(c, "일련번호가 이미 있습니다: "+sn)
		}
	}
	var created []string
	for i := 0; i < n; i++ {
		sn := ""
		if i < len(serials) {
			sn = strings.TrimSpace(serials[i])
		}
		loc := ""
		if i < len(locs) {
			loc = strings.TrimSpace(locs[i])
		}
		note := ""
		if i < len(notes) {
			note = strings.TrimSpace(notes[i])
		}
		name := product
		if sn == "" {
			name = fmt.Sprintf("%s %d", product, i+1)
		}
		a := &model.Asset{
			CustomerID: cid, ProductName: name, ModelName: modelName, Manufacturer: mfr,
			SerialNumber: sn, InstallDate: installDate, ManagementType: mgmt,
			InstallLocation: loc, Notes: note, OperationStatus: model.AssetOpOperating,
			ProjectID: t.ProjectID, IsManaged: mgmt != model.ManagementTypeThirdParty,
		}
		if err := ar.Create(a); err != nil {
			for _, aid := range created {
				_, _ = h.repo.DB().Exec(`DELETE FROM assets WHERE asset_id=?`, aid)
			}
			return itemJSONErr(c, "자산 등록에 실패했습니다")
		}
		created = append(created, a.AssetID)
	}
	if err := h.repo.ReplaceItemAssets(itemID, model.ItemAssetInstall, created); err != nil {
		return itemJSONErr(c, err.Error())
	}
	if wantsJSONError(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "asset_ids": created})
	}
	return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t.TaskID)
}

func (h *WorkboardHandler) LinkItemAssets(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanPlanEdit(c, t); err != nil {
		return err
	}
	itemID := strings.TrimSpace(c.FormValue("item_id"))
	role := strings.TrimSpace(c.FormValue("role"))
	_ = c.Request().ParseForm()
	ids := c.Request().PostForm["asset_id"]
	if s := strings.TrimSpace(c.FormValue("asset_ids")); s != "" {
		ids = append(ids, strings.Split(s, ",")...)
	}
	cid := strings.TrimSpace(t.CustomerID)
	ar := repository.NewAssetRepo(h.repo.DB())
	var okIDs []string
	for _, aid := range ids {
		aid = strings.TrimSpace(aid)
		if aid == "" {
			continue
		}
		a, err := ar.GetByID(aid)
		if err != nil || a == nil {
			return itemJSONErr(c, "자산을 찾을 수 없습니다")
		}
		if cid != "" && strings.TrimSpace(a.CustomerID) != cid {
			return itemJSONErr(c, "이 작업 기관의 자산만 선택할 수 있습니다")
		}
		okIDs = append(okIDs, aid)
	}
	if strings.TrimSpace(c.FormValue("kind")) == model.ItemKindAS && len(okIDs) == 0 {
		return itemJSONErr(c, "AS 대상 자산을 선택하세요")
	}
	if err := h.repo.ReplaceItemAssets(itemID, role, okIDs); err != nil {
		return itemJSONErr(c, err.Error())
	}
	if wantsJSONError(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true})
	}
	return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t.TaskID)
}

func (h *WorkboardHandler) LinkItemAS(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyUnlessCanPlanEdit(c, t); err != nil {
		return err
	}
	itemID := strings.TrimSpace(c.FormValue("item_id"))
	mode := strings.TrimSpace(c.FormValue("mode"))
	if mode == "existing" {
		asID := strings.TrimSpace(c.FormValue("as_id"))
		as, err := h.asRepo.GetByID(t.OrgID, asID)
		if err != nil || as == nil {
			return itemJSONErr(c, "접수를 찾을 수 없습니다")
		}
		if cid := strings.TrimSpace(t.CustomerID); cid != "" && strings.TrimSpace(as.CustomerID) != cid {
			return itemJSONErr(c, "같은 기관의 접수만 연결할 수 있습니다")
		}
		if err := h.repo.SetItemAS(itemID, as.ASID); err != nil {
			return itemJSONErr(c, err.Error())
		}
		_ = h.repo.SyncASDailyTask(as)
		if wantsJSONError(c) {
			return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "as_id": as.ASID, "as_number": as.ASNumber})
		}
		return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t.TaskID)
	}
	items, _ := h.repo.ListItems(t.TaskID)
	var it *model.WorkTaskItem
	for i := range items {
		if items[i].ItemID == itemID {
			it = &items[i]
			break
		}
	}
	if it == nil {
		return itemJSONErr(c, "항목이 없습니다")
	}
	if len(it.AssetIDs) == 0 {
		return itemJSONErr(c, "대상 자산을 먼저 선택하세요")
	}
	who := t.Assignee
	uid := t.AssigneeUserID
	if len(it.Assignees) > 0 {
		who = it.Assignees[0].Name
		uid = it.Assignees[0].UserID
	}
	day := strings.TrimSpace(t.WorkDate)
	if day == "" {
		day = strings.TrimSpace(t.DueDate)
	}
	if day == "" {
		day = time.Now().Format(dateLayout)
	}
	as := &model.ASReceipt{
		CustomerID: t.CustomerID, AssetID: it.AssetIDs[0],
		Symptom:    strings.TrimSpace(c.FormValue("symptom")),
		AssignedTo: who, AssignedUserID: uid,
		VisitScheduledDate: day, ScheduleConfirmed: false,
		OrgID: t.OrgID, WorkGroupID: t.WorkGroupID,
		Urgency: "normal", Priority: "normal",
	}
	if as.Symptom == "" {
		as.Symptom = it.Title
	}
	if err := h.asRepo.Create(as); err != nil {
		return itemJSONErr(c, err.Error())
	}
	if err := h.repo.SetItemAS(itemID, as.ASID); err != nil {
		return itemJSONErr(c, err.Error())
	}
	_ = h.repo.SyncASDailyTask(as)
	if wantsJSONError(c) {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "as_id": as.ASID, "as_number": as.ASNumber})
	}
	return c.Redirect(http.StatusSeeOther, "/workboard/tasks/"+t.TaskID)
}

func (h *WorkboardHandler) ListOpenASForItem(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	t, err := h.repo.GetTask(id)
	if err != nil || t == nil {
		return echo.ErrNotFound
	}
	if err := denyOtherOrgTask(c, t); err != nil {
		return err
	}
	cid := strings.TrimSpace(t.CustomerID)
	rows, err := h.repo.DB().Query(`
		SELECT as_id, COALESCE(as_number,''), COALESCE(symptom,''), COALESCE(status,'')
		FROM as_receipts WHERE customer_id=? AND COALESCE(status,'') NOT IN ('complete','cancelled')
		ORDER BY receipt_datetime DESC LIMIT 80`, cid)
	if err != nil {
		return itemJSONErr(c, err.Error())
	}
	defer rows.Close()
	var list []map[string]string
	for rows.Next() {
		var id, num, sy, st string
		if rows.Scan(&id, &num, &sy, &st) != nil {
			continue
		}
		list = append(list, map[string]string{"as_id": id, "as_number": num, "symptom": sy, "status": st})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"items": list})
}

func itemJSONErr(c echo.Context, msg string) error {
	if wantsJSONError(c) {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "item", "message": msg})
	}
	setFlashErrCookie(c, msg)
	return c.Redirect(http.StatusSeeOther, redirectBack(c, "err=item"))
}
