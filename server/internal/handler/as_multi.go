package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

func parseExtraSiteReceipts(c echo.Context, base *model.ASReceipt) ([]*model.ASReceipt, string) {
	if c == nil || base == nil {
		return nil, ""
	}
	_ = c.Request().ParseForm()
	ids := c.Request().Form["extra_site_id"]
	syms := c.Request().Form["extra_site_symptom"]
	primary := strings.TrimSpace(base.CustomerID)
	seen := map[string]bool{}
	if primary != "" {
		seen[primary] = true
	}
	var out []*model.ASReceipt
	for i, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		sym := ""
		if i < len(syms) {
			sym = strings.TrimSpace(syms[i])
		}
		cp := *base
		cp.CustomerID = id
		cp.AssetID = ""
		cp.ASID = ""
		cp.ASNumber = ""
		if sym != "" {
			cp.Symptom = sym
		}
		out = append(out, &cp)
	}
	return out, ""
}

func (h *ASHandler) SearchOpenForApply(c echo.Context) error {
	if !canProcessAS(c) {
		return echo.ErrForbidden
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	exclude := strings.TrimSpace(c.QueryParam("exclude"))
	org := currentOrg(c)
	selected := strings.TrimSpace(c.QueryParam("selected"))
	if utf8Len(q) < 2 && selected == "" {
		return c.JSON(http.StatusOK, []repository.WorkTaskSite{})
	}
	var list []repository.WorkTaskSite
	var err error
	if selected != "" && utf8Len(q) < 2 {
		list, err = h.repo.ListASForApplySelected(org, strings.Split(selected, ","), exclude)
	} else {
		list, err = h.repo.SearchOpenASForApply(org, q, exclude, applyASAssigneeFilter(c), 20)
	}
	if err != nil {
		return err
	}
	if list == nil {
		list = []repository.WorkTaskSite{}
	}
	return c.JSON(http.StatusOK, list)
}

func applyASAssigneeFilter(c echo.Context) string {
	role := model.NormalizeRole(currentRole(c))
	if role == model.RoleOrgAdmin || role == model.RoleSupport || isVisionRole(c) {
		return ""
	}
	return strings.TrimSpace(identityUserID(c))
}

func utf8Len(s string) int {
	return len([]rune(strings.TrimSpace(s)))
}

func (h *ASHandler) applyActionCopies(c echo.Context, src *model.ASReceipt, result, prep string, timeSpent int, applyIn *model.ActionApplyInput) (int, []string) {
	if h == nil || src == nil || c == nil {
		return 0, nil
	}
	if strings.TrimSpace(c.FormValue("apply_as_confirm")) != "1" {
		return 0, nil
	}
	_ = c.Request().ParseForm()
	ids := c.Request().Form["extra_as_id"]
	acts := c.Request().Form["extra_action_taken"]
	var skips []string
	n := 0
	seen := map[string]bool{strings.TrimSpace(src.ASID): true}
	for i, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		if n >= 20 {
			skips = append(skips, "사이트는 최대 20곳입니다")
			break
		}
		seen[id] = true
		as, err := h.repo.GetByID(repository.OrgAll, id)
		if err != nil || as == nil {
			skips = append(skips, id+" · 접수를 찾을 수 없습니다")
			continue
		}
		if reason := h.applyCopyBlockReason(c, src, as); reason != "" {
			skips = append(skips, as.ASNumber+" · "+reason)
			continue
		}
		as.ProcessType = src.ProcessType
		as.ProcessTypeReason = src.ProcessTypeReason
		as.WorkPlace = src.WorkPlace
		as.CauseCat1 = src.CauseCat1
		as.CauseCat2 = src.CauseCat2
		as.CauseCat3 = src.CauseCat3
		as.CauseCat1Custom = src.CauseCat1Custom
		as.CauseType = src.CauseType
		as.CauseDetail = src.CauseDetail
		as.VisitDate = src.VisitDate
		as.ResultCode = src.ResultCode
		act := ""
		if i < len(acts) {
			act = strings.TrimSpace(acts[i])
		}
		if act == "" {
			act = src.ActionTaken
		}
		as.ActionTaken = act
		if applyIn != nil && strings.TrimSpace(result) != "" {
			in := *applyIn
			in.CreateRevisitAfterConfirm = false
			if _, err := model.ApplyActionResult(as, in, time.Now()); err != nil {
				skips = append(skips, as.ASNumber+" · "+err.Error())
				continue
			}
		}
		if err := h.repo.Update(as); err != nil {
			skips = append(skips, as.ASNumber+" · 저장 실패")
			continue
		}
		h.syncASPlannedDailyTask(as.ASID)
		if h.processRepo != nil {
			_, _ = h.appendActionProcess(c, as, result, prep, 0)
		}
		n++
	}
	return n, skips
}

func (h *ASHandler) applyCopyBlockReason(c echo.Context, src, as *model.ASReceipt) string {
	org := currentOrg(c)
	if org != repository.OrgAll && strings.TrimSpace(as.OrgID) != "" && as.OrgID != org {
		return "타 조직"
	}
	if isASClosedStatus(as.Status) {
		if h != nil {
			if ok, _, _ := h.unlockActive(c, as.ASID); ok {
				// 잠금 해제된 완료 건만 허용
			} else {
				return "완료·종료"
			}
		} else {
			return "완료·종료"
		}
	}
	role := model.NormalizeRole(currentRole(c))
	if role == model.RoleTech {
		uid := strings.TrimSpace(identityUserID(c))
		if uid != "" && uid != strings.TrimSpace(as.AssignedUserID) {
			return "담당 건이 아님"
		}
		name := strings.TrimSpace(currentUserDisplayName(c))
		if uid == "" && name != "" && name != strings.TrimSpace(as.AssignedTo) {
			return "담당 건이 아님"
		}
	}
	_ = src
	return ""
}
