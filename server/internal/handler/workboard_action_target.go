package handler

import (
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// taskActionDest 업무 조치 화면의 주소·라벨·권한. ShowTask·캘린더 바·칸반이 같이 쓴다.
type taskActionDest struct {
	Href  string
	Label string
	Can   bool
}

func hrefWithBack(href, back string) string {
	href = strings.TrimSpace(href)
	back = strings.TrimSpace(back)
	if href == "" || back == "" {
		return href
	}
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	q := u.Query()
	if q.Get("back") == "" {
		q.Set("back", back)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func hrefPathOnly(href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		if i := strings.IndexAny(href, "?#"); i >= 0 {
			return href[:i]
		}
		return href
	}
	return u.Path
}

// taskActionTarget 유형별 조치 화면. 문자열을 화면마다 조립하지 않는다.
func (h *WorkboardHandler) taskActionTarget(c echo.Context, t *model.WorkTask) taskActionDest {
	out := taskActionDest{Label: "조치"}
	if t == nil {
		return out
	}
	canWrite := c != nil && canEditTask(c, t.Assignee, t.AssigneeUserID)
	switch t.SourceType {
	case model.WBSourceASWork:
		if sid := strings.TrimSpace(t.SourceID); sid != "" {
			out.Href = "/as/work/" + sid + "/action"
			out.Can = canWrite && (canProcessAS(c) || canReceiveAS(c) || canWriteWorkboard(c))
			if t.Status == model.WBTaskComplete {
				out.Label = "조치 완료"
			}
		}
	case model.WBSourceAS:
		if sid := strings.TrimSpace(t.SourceID); sid != "" {
			out.Href = asOpenHref(sid)
			out.Can = canWrite && (canProcessAS(c) || canReceiveAS(c) || canWriteWorkboard(c))
			if h != nil && h.asRepo != nil {
				if as, _ := h.asRepo.GetByID(repository.OrgAll, sid); as != nil && isASClosedStatus(as.Status) {
					out.Label = "조치 완료"
				}
			}
		}
	case model.WBSourceMaintenance:
		if h != nil && h.mntRepo != nil {
			if v, _ := h.mntRepo.GetVisit(strings.TrimSpace(t.SourceID)); v != nil {
				out.Href = mntVisitActionHref(*v)
				out.Can = canWrite && (canEditMaintenanceSchedule(c) || canWriteWorkboard(c))
				if v.Completed {
					out.Label = "조치 완료"
				}
			}
		}
	case model.WBSourceSalesActivity:
		if h != nil && h.repo != nil {
			if sid := h.repo.SalesIDByActivity(t.SourceID); sid != "" {
				out.Href = "/sales/" + sid
				out.Label = "영업 사업"
				out.Can = canViewSales(c)
			}
		}
	}
	if t.WorkType == model.WBWorkProjectWork {
		id := strings.TrimSpace(t.TaskID)
		if id != "" && !strings.HasPrefix(id, "ext:") {
			out.Href = projectWorkActionPath(id)
			out.Label = "조치 화면 열기"
			out.Can = true
		}
	}
	if strings.TrimSpace(out.Href) != "" {
		return out
	}
	id := strings.TrimSpace(t.TaskID)
	if id != "" && !strings.HasPrefix(id, "ext:") {
		out.Href = "/workboard/tasks/" + id
		out.Can = canWrite
		return out
	}
	out.Href = t.DetailHref()
	if out.Href == "#" {
		out.Href = ""
	}
	out.Can = canWrite && out.Href != ""
	return out
}
