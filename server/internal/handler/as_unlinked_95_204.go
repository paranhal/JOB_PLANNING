package handler

import (
	"net/http"
	"net/url"
	"strings"

	"customer-support/internal/model"
	"customer-support/internal/repository"

	"github.com/labstack/echo/v4"
)

// 95-204: 자산 연결이 없는 목록의 「일괄 연결」. 후보 자산이 정확히 1개인 건만, 사용자가 고른 건만 연결한다.
// 화면의 값은 믿지 않고 요청 시점에 서버가 대상·권한을 다시 판정한다(건마다 건너뜀 사유를 돌려준다).

type unlinkedSkip struct{ Title, Reason string }

type unlinkedLinkResult struct {
	Kind    string
	Linked  int
	Skipped []unlinkedSkip
	More    int // 화면에 다 적지 못한 건너뜀 수
}

const unlinkedSkipShown = 10

// SkippedN 건너뛴 전체 건수(화면에 적은 것 + 적지 못한 것).
func (r *unlinkedLinkResult) SkippedN() int { return len(r.Skipped) + r.More }

func (r *unlinkedLinkResult) skip(title, code string) {
	if len(r.Skipped) >= unlinkedSkipShown {
		r.More++
		return
	}
	r.Skipped = append(r.Skipped, unlinkedSkip{Title: title, Reason: repository.LinkSkipText(code)})
}

// asRowLink AS 목록 한 줄의 연결 가능 여부와 안내(툴팁). 쓰기 권한은 조치 저장과 같은 함수로 판정한다.
func (h *ASHandler) asRowLink(c echo.Context, it repository.UnlinkedAS) (bool, string) {
	if code := repository.LinkEligibility(it.Candidates, "", false); code != "" {
		return false, repository.LinkSkipText(code)
	}
	if code := h.asLinkPermission(c, it.OrgID, &model.ASReceipt{Status: it.Status, AssignedTo: it.AssignedTo, AssignedUserID: it.AssignedUserID,
		CreatedByUserID: it.CreatedByUserID, WorkGroupID: it.WorkGroupID}); code != "" {
		return false, repository.LinkSkipText(code)
	}
	return true, ""
}

// asLinkPermission 조치(접수 수정)와 같은 규칙: 조직 쓰기 권한 + 접수자·담당·그룹 업무 권한, 닫힌 건은 잠금 해제 규칙. 막히면 사유 코드.
func (h *ASHandler) asLinkPermission(c echo.Context, orgID string, as *model.ASReceipt) string {
	if isReadOnly(c) || viewAsActive(c) || !CanWriteOrg(c, orgID) || !canEditASReceipt(c, as) {
		return repository.LinkSkipDenied
	}
	if isASClosedStatus(as.Status) && !asUnlockAllowsClosed(c, as, false) {
		return repository.LinkSkipClosed
	}
	return ""
}

func (h *ASHandler) mntRowLink(c echo.Context, it repository.UnlinkedMnt) (bool, string) {
	if code := repository.LinkEligibility(it.Candidates, it.ProductType, true); code != "" {
		return false, repository.LinkSkipText(code)
	}
	if code := mntLinkPermission(c, it.OrgID, it.Completed); code != "" {
		return false, repository.LinkSkipText(code)
	}
	return true, ""
}

func mntLinkPermission(c echo.Context, orgID string, completed bool) string {
	if !canEditMaintenanceSchedule(c) || !CanWriteOrg(c, orgID) {
		return repository.LinkSkipDenied
	}
	if !canEditVisitAction(c, completed) {
		return repository.LinkSkipClosed
	}
	return ""
}

// bulkQS 일괄 연결 요청 주소에 싣는 목록 상태(탭·필터·정렬·쪽·범위). 처리 뒤 같은 목록을 다시 그린다.
func bulkQS(c echo.Context, p unlinkedParams, tab string) string {
	q := p.values("", tab, p.Page)
	for k, vs := range scopeQuery(c) {
		q[k] = vs
	}
	return q.Encode()
}

// UnlinkedLinkSingle POST /as/unlinked/link-single — 폼: kind=as|mnt, ids(최대 100). 처리 뒤 해당 탭 목록 조각을 결과 요약과 함께 돌려준다.
func (h *ASHandler) UnlinkedLinkSingle(c echo.Context) error {
	if !requireCSRF(c) {
		return c.String(http.StatusForbidden, "요청을 확인할 수 없습니다. 새로고침 후 다시 시도하세요")
	}
	if isReadOnly(c) || viewAsActive(c) {
		return c.String(http.StatusForbidden, "읽기 전용 계정은 연결할 수 없습니다")
	}
	kind := strings.TrimSpace(c.FormValue("kind"))
	if kind != "as" && kind != "mnt" {
		return c.String(http.StatusBadRequest, "종류가 올바르지 않습니다")
	}
	if kind == "mnt" && !canEditMaintenanceSchedule(c) {
		return c.String(http.StatusForbidden, "점검 일정을 수정할 권한이 없습니다")
	}
	form, err := c.FormParams()
	if err != nil {
		return c.String(http.StatusBadRequest, "요청을 읽을 수 없습니다")
	}
	var ids []string
	seen := map[string]bool{}
	for _, v := range form["ids"] {
		if v = strings.TrimSpace(v); v != "" && !seen[v] {
			seen[v] = true
			ids = append(ids, v)
		}
	}
	if len(ids) == 0 {
		return c.String(http.StatusBadRequest, "연결할 건을 선택하세요")
	}
	if len(ids) > repository.LinkSingleMax {
		return c.String(http.StatusBadRequest, "한 번에 100건까지 연결할 수 있습니다")
	}
	org := currentOrg(c)
	res := &unlinkedLinkResult{Kind: kind}
	if kind == "as" {
		targets, err := h.repo.LinkSingleTargetsAS(org, ids)
		if err != nil {
			return c.String(http.StatusInternalServerError, "연결 대상을 확인하지 못했습니다")
		}
		for _, id := range ids {
			t, ok := targets[id]
			switch {
			case !ok:
				res.skip(id, repository.LinkSkipNotFound)
				continue
			case t.Skip != "":
				res.skip(t.Title, t.Skip)
				continue
			}
			if code := h.asLinkPermission(c, t.OrgID, &model.ASReceipt{Status: t.Status, AssignedTo: t.AssignedTo, AssignedUserID: t.AssignedUserID,
				CreatedByUserID: t.CreatedBy, WorkGroupID: t.WorkGroup}); code != "" {
				res.skip(t.Title, code)
				continue
			}
			linked, err := h.repo.LinkSingleAS(org, id, t.AssetID, t.Title)
			switch {
			case err != nil:
				res.skip(t.Title, repository.LinkSkipError)
			case !linked:
				res.skip(t.Title, repository.LinkSkipNotFound)
			default:
				res.Linked++
			}
		}
	} else {
		targets, err := h.repo.LinkSingleTargetsMnt(org, ids)
		if err != nil {
			return c.String(http.StatusInternalServerError, "연결 대상을 확인하지 못했습니다")
		}
		for _, id := range ids {
			t, ok := targets[id]
			switch {
			case !ok:
				res.skip(id, repository.LinkSkipNotFound)
				continue
			case t.Skip != "":
				res.skip(t.Title, t.Skip)
				continue
			}
			if code := mntLinkPermission(c, t.OrgID, t.Closed); code != "" {
				res.skip(t.Title, code)
				continue
			}
			linked, err := h.repo.LinkSingleMnt(org, id, t.AssetID)
			switch {
			case err != nil:
				res.skip(t.Title, repository.LinkSkipError)
			case !linked:
				res.skip(t.Title, repository.LinkSkipNotFound)
			default:
				res.Linked++
			}
		}
	}
	repository.LogLinkSingleBulk(kind, res.Linked, len(ids)-res.Linked, ids)
	c.Set("ul_result", res)
	if kind == "as" {
		return h.UnlinkedAS(c)
	}
	return h.UnlinkedMnt(c)
}

// unlinkedResultData 결과 요약과 탭 건수 칩 갱신(OOB)용 값.
func (h *ASHandler) unlinkedResultData(c echo.Context, data map[string]interface{}) {
	r, _ := c.Get("ul_result").(*unlinkedLinkResult)
	if r == nil {
		return
	}
	data["Result"] = r
	scope := h.unlinkedScope(c)
	if n, err := h.repo.CountUnlinkedAS(scope); err == nil {
		data["ASN"] = n
	}
	if n, err := h.repo.CountUnlinkedMnt(scope); err == nil {
		data["MntN"] = n
	}
}

var _ = url.Values{}

// unlinkedBulkData 일괄 연결 영역(전체 선택·요청 주소)과 결과 요약을 목록 조각 데이터에 더한다.
func (h *ASHandler) unlinkedBulkData(c echo.Context, p unlinkedParams, tab string, data map[string]interface{}, _ int) {
	n := 0
	switch rows := data["Rows"].(type) {
	case []unlinkedASRow:
		for _, r := range rows {
			if r.LinkOK {
				n++
			}
		}
	case []unlinkedMntRow:
		for _, r := range rows {
			if r.LinkOK {
				n++
			}
		}
	}
	data["LinkableN"], data["BulkQS"], data["CSRFToken"] = n, bulkQS(c, p, tab), ensureCSRFToken(c)
	h.unlinkedResultData(c, data)
}
