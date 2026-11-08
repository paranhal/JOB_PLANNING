package handler

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const (
	flashOKMntSaved = "mnt_saved"
	flashOKMntDone  = "mnt_done"
)

type mntActionPopup struct {
	Kind      string
	Title     string
	Site      string
	Product   string
	Equipment string
	Assignee  string
	VisitDate string
	TimeRange string
	Duration  string
	Status    string
	Warning   string
	BackURL   string
	Continue  bool
	Embed     bool
}

func canEditVisitAction(c echo.Context, completed bool) bool {
	if !canEditMaintenanceSchedule(c) {
		return false
	}
	if !completed {
		return true
	}
	switch currentRole(c) {
	case model.RoleOrgAdmin, model.RoleSupport, model.RoleAdmin, model.RoleVisionAdmin:
		return true
	default:
		return false
	}
}

func (h *MaintenanceHandler) loadVisitAction(vid string) (*model.MaintenanceVisit, error) {
	v, err := h.repo.GetVisit(vid)
	if err != nil || v == nil {
		return nil, err
	}
	if people, err := h.repo.ListVisitAssignees(vid); err == nil {
		v.People = people
	}
	if targets, err := h.repo.ListVisitTargets(vid); err == nil {
		v.Targets = targets
	}
	return v, nil
}

// VisitAction 정기점검 방문 조치 화면 — 해당 방문만 완료·비고 처리.
func (h *MaintenanceHandler) VisitAction(c echo.Context) error {
	return h.renderVisitAction(c, nil, "")
}

func (h *MaintenanceHandler) ShowVisit(c echo.Context) error {
	return h.VisitAction(c)
}

func (h *MaintenanceHandler) renderVisitAction(c echo.Context, overlay *model.MaintenanceVisit, formErr string) error {
	vid := c.Param("visit_id")
	if overlay == nil {
		v, err := h.loadVisitAction(vid)
		if err != nil || v == nil {
			return echo.ErrNotFound
		}
		overlay = v
	} else if overlay.VisitID == "" {
		overlay.VisitID = vid
	}
	v := overlay
	assignees := listAssignableForPicker(h.userRepo, c)
	var projects []model.WorkProject
	if h.wbRepo != nil {
		projects, _ = h.wbRepo.ListProjects(true)
	}
	name := v.ShortName
	if name == "" {
		name = v.OrgName
	}
	back := strings.TrimSpace(c.QueryParam("back"))
	if back == "" {
		back = strings.TrimSpace(c.FormValue("back"))
	}
	if back == "" {
		back = workboardHomeURL("")
	}
	groups, groupJSON := h.workGroupPicksForVisit(c)
	var assets []model.Asset
	if h.assetRepo != nil {
		assets, _ = h.assetRepo.ListByCustomer(v.CustomerID)
	}
	embed := c.QueryParam("embed") == "1" || c.FormValue("embed") == "1"
	stored := map[string]string{}
	var extraIDs []string
	for _, p := range v.People {
		if id := strings.TrimSpace(p.UserID); id != "" {
			stored[id] = p.AssigneeName
			extraIDs = append(extraIDs, id)
		}
	}
	if oid := visitOwnerUserID(*v); oid != "" {
		stored[oid] = v.Assignee
		extraIDs = append(extraIDs, oid)
	}
	assignees = ensurePickerUsersNamed(h.userRepo, assignees, stored, extraIDs...)
	data := map[string]interface{}{
		"Title":               "정기점검 조치 · " + name,
		"Active":              workboardNavActive(back),
		"Visit":               v,
		"SiteName":            name,
		"Assignees":           assignees,
		"PickerNames":         overlayPickerStoredNames(h.userRepo, pickerNamesFor(h.userRepo, assignees, extraIDs...), stored),
		"WorkGroups":          groups,
		"WorkGroupMembers":    groupJSON,
		"Projects":            projects,
		"Assets":              assets,
		"CanEdit":             canEditVisitAction(c, v.Completed),
		"AssigneeRequired":    requiresAssigneeChoice(c),
		"Today":               time.Now().Format("2006-01-02"),
		"BackURL":             back,
		"BackLabel":           workboardBackLabel(back),
		"PlanHref":            mntVisitHref(*v),
		"SignatureDataURL":    signatureDataURL(signaturePNGForName(h.userRepo, v.Assignee)),
		"Embed":               embed,
		"HideNav":             embed,
		"OrgProducts":         h.orgProductList(h.repo.CustomerOrgID(v.CustomerID)),
		"UseOrgProductLegend": h.useOrgProductLegend(h.repo.CustomerOrgID(v.CustomerID)),
		"ProductLocked":       strings.TrimSpace(v.ProductType) != "",
		"EquipmentLine":       model.FormatVisitTargetsLine(v.Targets),
		"PickerTarget":        "action",
		"PeopleIDs":           visitPeopleIDsCSV(*v),
		"GroupIDs":            visitGroupIDsCSV(*v),
		"OwnerUserID":         visitOwnerUserID(*v),
		"PickerUserIDs":       extraIDs,
		"PickerOwnerID":       visitOwnerUserID(*v),
		"DisplayAssignee":     userDisplayName(h.userRepo, visitOwnerUserID(*v), v.Assignee),
		"CanPlanEdit":         false,
		"PlanEditHref":        "",
		"TargetSelected":      visitTargetSelected(v.Targets),
		"CustomTargets":       visitCustomTargets(v.Targets),
	}
	errCode := strings.TrimSpace(formErr)
	if errCode == "" {
		errCode = strings.TrimSpace(c.QueryParam("err"))
	}
	if h.wbRepo != nil {
		if daily, err := h.wbRepo.GetTaskBySource(model.WBSourceMaintenance, v.VisitID); err == nil && daily != nil {
			data["CanPlanEdit"] = canPlanEditTask(c, daily)
			data["PlanEditHref"] = planEditHref(daily.TaskID)
			items, _ := h.wbRepo.ListItems(daily.TaskID)
			data["WorkItems"] = items
			attachWorkSegData(data, h.wbRepo, daily, errCode, mntVisitActionHref(*v), !canEditVisitAction(c, v.Completed))
			pst, pen, _ := planClockOf(h.wbRepo, daily)
			data["PlanStart"] = pst
			data["PlanEnd"] = pen
			if pst != "" {
				if pen != "" {
					data["PlanTimeLabel"] = pst + "~" + pen
				} else {
					data["PlanTimeLabel"] = pst
				}
			}
		} else {
			attachWorkSegData(data, h.wbRepo, nil, errCode, mntVisitActionHref(*v), !canEditVisitAction(c, v.Completed))
		}
	} else {
		attachWorkSegData(data, nil, nil, errCode, mntVisitActionHref(*v), !canEditVisitAction(c, v.Completed))
	}
	if formErr != "" && formErr != "seg_merge" && formErr != "seg_shift" {
		data["FlashPopup"] = formErr
	} else if q := strings.TrimSpace(c.QueryParam("err")); q != "" {
		if q == errAssigneeRequired {
			data["FlashPopup"] = msgAssigneeRequired
		} else {
			data["FlashPopup"] = q
		}
	}
	if popup := h.consumeMntActionPopup(c, *v, name, back, embed); popup != nil {
		data["MntSavePopup"] = popup
	}
	data["VisitTimeMissing"] = visitActionTimeMissing(*v)
	matchAssets, otherAssets, storedOrphans := splitVisitActionAssets(assets, v.ProductType, v.Targets)
	data["MatchAssets"] = matchAssets
	data["OtherAssets"] = otherAssets
	data["StoredOrphanAssets"] = storedOrphans
	data["MatchAssetIDs"] = visitAssetIDs(matchAssets)
	if h.wbRepo != nil {
		attachProjectPicker(c, pickerFromRepo(h.wbRepo), data, ProjectPickerOpts{
			Selected: v.ProjectID, SelectedName: v.ProjectName,
			PreferYear: visitPlanYear(v.VisitDate), AlwaysInclude: v.ProjectID,
			EmptyListHint: siteProjectEmptyHint, ShowSiteNote: true,
			UseSiteProjects: true, SiteCustomerID: v.CustomerID,
		})
	}
	return c.Render(http.StatusOK, "maintenance/visit_action.html", data)
}

func (h *MaintenanceHandler) consumeMntActionPopup(c echo.Context, v model.MaintenanceVisit, site, back string, embed bool) *mntActionPopup {
	kind, values := readFlashOKCookie(c)
	if kind != flashOKMntSaved && kind != flashOKMntDone {
		return nil
	}
	clearFlashOKCookie(c)
	title := "저장되었습니다."
	if kind == flashOKMntDone {
		title = "점검이 완료되었습니다."
	}
	warn := ""
	if len(values) > 1 {
		warn = strings.TrimSpace(values[1])
	}
	status := "예정"
	if v.Completed {
		status = "방문 완료"
	}
	equip := model.FormatVisitTargetsLine(v.Targets)
	if equip == "" {
		equip = strings.TrimSpace(v.ProductType)
	}
	if equip == "" {
		equip = "—"
	}
	return &mntActionPopup{
		Kind:      kind,
		Title:     title,
		Site:      site,
		Product:   strings.TrimSpace(v.ProductType),
		Equipment: equip,
		Assignee:  v.AssigneeSummary(),
		VisitDate: v.VisitDate,
		TimeRange: model.FormatVisitClockRange(v.StartTime, v.EndTime),
		Duration:  model.FormatDurationLabel(v.DurationMin),
		Status:    status,
		Warning:   warn,
		BackURL:   back,
		Continue:  kind == flashOKMntSaved,
		Embed:     embed,
	}
}

// UpdateVisitAction 방문 조치 저장(완료·실제방문일·비고·담당). 연간 목록으로 보내지 않는다.
func (h *MaintenanceHandler) UpdateVisitAction(c echo.Context) error {
	vid := c.Param("visit_id")
	existing, err := h.loadVisitAction(vid)
	if err != nil || existing == nil {
		return echo.ErrNotFound
	}
	if !canEditVisitAction(c, existing.Completed) {
		return echo.ErrForbidden
	}
	v := *existing
	v.Notes = strings.TrimSpace(c.FormValue("notes"))
	v.ProjectID = postedFormValue(c, "project_id", existing.ProjectID)
	if err := requireVisitSiteProject(h.wbRepo, v.CustomerID, v.ProjectID, existing.ProjectID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if d := strings.TrimSpace(c.FormValue("visit_date")); d != "" {
		parsed, err := model.ParseAppDate(d)
		if err != nil {
			return h.renderVisitAction(c, &v, "방문일을 확인해 주세요.")
		}
		if parsed != "" {
			v.VisitDate = parsed
		}
	}
	if strings.TrimSpace(existing.ProductType) == "" {
		v.ProductType = strings.TrimSpace(c.FormValue("product_type"))
	} else {
		v.ProductType = existing.ProductType
	}
	if err := h.applyVisitAssigneesFromForm(c, &v); err != nil {
		return h.renderVisitAction(c, &v, err.Error())
	}
	if strings.TrimSpace(c.FormValue("assignee_multi")) != "1" && strings.TrimSpace(c.FormValue("assignee")) != "" {
		v.People = []model.MaintenanceVisitAssignee{{
			AssigneeName: v.Assignee, Role: model.WBMemberOwner,
		}}
		v.WorkGroupID = ""
	}
	if requiresAssigneeChoice(c) && !assigneeFilled(v.Assignee, "", v.WorkGroupID) && len(v.People) == 0 {
		return h.renderVisitAction(c, &v, msgAssigneeRequired)
	}

	start, end, travel, dur, err := parseVisitActionTimes(c)
	if err != nil {
		v.StartTime, v.EndTime, v.TravelMin, v.DurationMin = start, end, travel, dur
		return h.renderVisitAction(c, &v, err.Error())
	}
	v.StartTime, v.EndTime, v.TravelMin, v.DurationMin = start, end, travel, dur
	if h.wbRepo != nil {
		if daily, _ := h.wbRepo.GetTaskBySource(model.WBSourceMaintenance, vid); daily != nil {
			n, _ := h.wbRepo.CountStoredPlanSegments(daily.TaskID)
			clockChanged := model.ClockHHMM(start) != model.ClockHHMM(existing.StartTime) ||
				model.ClockHHMM(end) != model.ClockHHMM(existing.EndTime)
			if n > 1 && clockChanged && strings.TrimSpace(c.FormValue("confirm_merge_segments")) != "1" {
				return h.renderVisitAction(c, &v, "seg_merge")
			}
			if n > 1 && clockChanged {
				_ = h.wbRepo.CollapsePlanToSingle(daily, v.VisitDate, start, end)
			}
			if err := savePostedActionSegments(c, h.wbRepo, daily); err != nil {
				if errors.Is(err, errSegShift) {
					return h.renderVisitAction(c, &v, "seg_shift")
				}
				return err
			}
			if actual, _ := h.wbRepo.ListActualSegments(daily.TaskID); len(actual) > 0 {
				st, en, d := model.EnvelopeFromPlanSegments(actual)
				if strings.TrimSpace(st) != "" {
					v.StartTime, v.EndTime = model.ClockHHMM(st), model.ClockHHMM(en)
					if d > 0 {
						v.DurationMin = d
					}
				}
			}
		}
	}

	status := strings.TrimSpace(c.FormValue("status"))
	markDone := c.FormValue("mark_done") == "1"
	if markDone {
		status = "done"
	}
	done := status == "done" || (status == "" && c.FormValue("completed") == "1")
	if status == "open" {
		done = false
	}
	v.Completed = done
	if v.Completed {
		v.CompletedDate = strings.TrimSpace(c.FormValue("completed_date"))
		if v.CompletedDate == "" {
			if markDone {
				v.CompletedDate = time.Now().Format("2006-01-02")
			} else {
				v.CompletedDate = v.VisitDate
			}
		}
	} else {
		v.CompletedDate = ""
	}

	targets, err := h.parseVisitActionTargets(c, v.CustomerID)
	if err != nil {
		v.Targets = targets
		return h.renderVisitAction(c, &v, err.Error())
	}
	v.Targets = targets

	if err := h.repo.UpdateVisit(v); err != nil {
		return h.renderVisitAction(c, &v, err.Error())
	}
	orgID := h.repo.CustomerOrgID(v.CustomerID)
	if err := h.repo.ReplaceVisitTargets(vid, orgID, targets); err != nil {
		return h.renderVisitAction(c, &v, err.Error())
	}
	uid, name, on := stampOnBehalf(c, visitOwnerUserID(v), v.Assignee)
	_ = h.repo.StampVisitActedBy(vid, uid, name, on)
	h.syncVisitTask(vid, v.Completed)
	h.repo.TouchPlanUpdated(v.PlanID)
	h.recordVisitAssign(c, vid, v.Assignee, v.PlanID, v.CustomerID, v.ProductType, v.VisitDate)

	warn := h.visitTimeOverlapWarning(v)
	kind := flashOKMntSaved
	if markDone {
		kind = flashOKMntDone
	}
	vals := []string{vid}
	if warn != "" {
		vals = append(vals, warn)
	}
	setFlashOKCookie(c, kind, vals)

	loc := "/maintenance/visits/" + vid + "/action"
	q := url.Values{}
	if back := strings.TrimSpace(c.FormValue("back")); back != "" {
		q.Set("back", back)
	}
	if c.FormValue("embed") == "1" {
		q.Set("embed", "1")
	}
	if enc := q.Encode(); enc != "" {
		loc += "?" + enc
	}
	return c.Redirect(http.StatusSeeOther, loc)
}

func parseVisitActionTimes(c echo.Context) (start, end string, travel, dur int, err error) {
	var ok bool
	start, ok = model.NormalizeVisitClock(c.FormValue("start_time"))
	if !ok {
		return "", "", 0, 0, fmtVisitTimeErr("방문 시각을 확인해 주세요.")
	}
	end, ok = model.NormalizeVisitClock(c.FormValue("end_time"))
	if !ok {
		return start, "", 0, 0, fmtVisitTimeErr("종료 시각을 확인해 주세요.")
	}
	if raw := strings.TrimSpace(c.FormValue("travel_min")); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 0 {
			return start, end, 0, 0, fmtVisitTimeErr("이동 시간을 확인해 주세요.")
		}
		travel = n
	}
	if raw := strings.TrimSpace(c.FormValue("duration_min")); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 0 {
			return start, end, travel, 0, fmtVisitTimeErr("소요 시간을 확인해 주세요.")
		}
		dur = n
	}
	site, ok := model.VisitSiteDurationMin(start, end)
	if !ok {
		return start, end, travel, dur, fmtVisitTimeErr("종료 시각이 방문 시각보다 빠르거나 같습니다. 시간을 확인해 주세요.")
	}
	if start != "" && end != "" {
		dur = site
	}
	return start, end, travel, dur, nil
}

func fmtVisitTimeErr(msg string) error {
	return &visitActionErr{msg: msg}
}

type visitActionErr struct{ msg string }

func (e *visitActionErr) Error() string { return e.msg }

func (h *MaintenanceHandler) parseVisitActionTargets(c echo.Context, customerID string) ([]model.MaintenanceVisitTarget, error) {
	var out []model.MaintenanceVisitTarget
	seenAsset := map[string]bool{}
	for _, id := range uniqueTrimmed(formStringSlice(c, "target_asset_id")) {
		name, ok := h.repo.AssetBelongsToCustomer(id, customerID)
		if !ok {
			return out, fmtVisitTimeErr("이 사이트의 설치자산만 선택할 수 있습니다.")
		}
		if seenAsset[id] {
			continue
		}
		seenAsset[id] = true
		out = append(out, model.MaintenanceVisitTarget{AssetID: id, Name: name})
	}
	names := formStringSlice(c, "target_custom_name")
	memos := formStringSlice(c, "target_custom_memo")
	for i, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		memo := ""
		if i < len(memos) {
			memo = strings.TrimSpace(memos[i])
		}
		out = append(out, model.MaintenanceVisitTarget{Name: name, Memo: memo})
	}
	if len(out) > model.MaxVisitTargets {
		return out, fmtVisitTimeErr("점검 장비는 최대 50개까지입니다.")
	}
	return out, nil
}

func (h *MaintenanceHandler) visitTimeOverlapWarning(v model.MaintenanceVisit) string {
	if h == nil || h.wbRepo == nil {
		return ""
	}
	start, end := strings.TrimSpace(v.StartTime), strings.TrimSpace(v.EndTime)
	if start == "" || end == "" {
		return ""
	}
	date := v.VisitDate
	if v.Completed && strings.TrimSpace(v.CompletedDate) != "" {
		date = v.CompletedDate
	}
	exclude := ""
	if task, err := h.wbRepo.GetTaskBySource(model.WBSourceMaintenance, v.VisitID); err == nil && task != nil {
		exclude = task.TaskID
	}
	names := map[string]bool{}
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n != "" {
			names[n] = true
		}
	}
	add(v.Assignee)
	for _, p := range v.People {
		add(p.AssigneeName)
	}
	for name := range names {
		ok, err := h.wbRepo.AssigneeTimeOverlaps(date, name, start, end, exclude)
		if err == nil && ok {
			return "같은 날 같은 담당자의 다른 일정과 시간이 겹칩니다. 저장은 되었습니다."
		}
	}
	return ""
}

var _ template.JS

func visitActionTimeMissing(v model.MaintenanceVisit) bool {
	return v.Completed && strings.TrimSpace(v.StartTime) == "" && strings.TrimSpace(v.EndTime) == ""
}

func requireVisitSiteProject(wb *repository.WBRepo, customerID, projectID, existingID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	if projectID == strings.TrimSpace(existingID) {
		return nil
	}
	if wb == nil || wb.DB() == nil {
		return fmt.Errorf("이 사이트가 등록되지 않은 사업입니다")
	}
	ok, err := repository.NewProjectRepo(wb.DB()).CustomerOnActiveSite(customerID, projectID)
	if err != nil || !ok {
		return fmt.Errorf("이 사이트가 등록되지 않은 사업입니다")
	}
	return nil
}

func visitAssetMatchesProduct(a model.Asset, product string) bool {
	p := compactVisitProduct(product)
	if p == "" {
		return true
	}
	blob := compactVisitProduct(strings.Join([]string{a.ProductName, a.ProductType, a.ProductCategory, a.ModelName}, " "))
	return strings.Contains(blob, p)
}

func compactVisitProduct(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

func splitVisitActionAssets(all []model.Asset, product string, targets []model.MaintenanceVisitTarget) (match, other []model.Asset, orphans []model.Asset) {
	inList := map[string]bool{}
	for _, a := range all {
		id := strings.TrimSpace(a.AssetID)
		if id != "" {
			inList[id] = true
		}
		if visitAssetMatchesProduct(a, product) {
			match = append(match, a)
		} else {
			other = append(other, a)
		}
	}
	for _, t := range targets {
		id := strings.TrimSpace(t.AssetID)
		if id == "" || inList[id] {
			continue
		}
		name := strings.TrimSpace(t.Name)
		if name == "" {
			name = id
		}
		orphans = append(orphans, model.Asset{AssetID: id, ProductName: name})
	}
	if match == nil {
		match = []model.Asset{}
	}
	if other == nil {
		other = []model.Asset{}
	}
	if orphans == nil {
		orphans = []model.Asset{}
	}
	return match, other, orphans
}

func visitAssetIDs(items []model.Asset) []string {
	out := []string{}
	for _, a := range items {
		if id := strings.TrimSpace(a.AssetID); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func visitTargetSelected(items []model.MaintenanceVisitTarget) map[string]bool {
	out := map[string]bool{}
	for _, it := range items {
		if id := strings.TrimSpace(it.AssetID); id != "" {
			out[id] = true
		}
	}
	return out
}

func visitCustomTargets(items []model.MaintenanceVisitTarget) []map[string]string {
	var out []map[string]string
	for _, it := range items {
		if strings.TrimSpace(it.AssetID) != "" {
			continue
		}
		name := strings.TrimSpace(it.Name)
		if name == "" {
			continue
		}
		out = append(out, map[string]string{"name": name, "memo": strings.TrimSpace(it.Memo)})
	}
	if out == nil {
		out = []map[string]string{}
	}
	return out
}
