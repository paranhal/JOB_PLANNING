package handler

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

type MeetingHandler struct {
	work  *repository.WorkBoardRepo
	stats *repository.StatsRepo
}

func NewMeetingHandler(work *repository.WorkBoardRepo, stats *repository.StatsRepo) *MeetingHandler {
	return &MeetingHandler{work: work, stats: stats}
}

// Show GET /meeting?date=YYYY-MM-DD&mine=0|1
func (h *MeetingHandler) Show(c echo.Context) error {
	role := currentRole(c)
	uid := currentUserID(c)
	keys := assigneeKeys(c)

	today := parseMeetingAnchor(c.QueryParam("date"))
	dateStr := today.Format("2006-01-02")
	prevStr := today.AddDate(0, 0, -1).Format("2006-01-02")

	mineParam := c.QueryParam("mine")
	mineUID, mineKeys := "", []string(nil)
	scopeAll := false
	if role == "tech" {
		if mineParam == "0" {
			scopeAll = true
		} else {
			mineUID, mineKeys = uid, keys
		}
	} else if mineParam == "1" {
		mineUID, mineKeys = uid, keys
	} else {
		scopeAll = true
	}

	yesterday, err := h.work.ListCompletedOn(prevStr, mineUID, mineKeys, 300)
	if err != nil {
		return err
	}
	todayItems, err := h.work.ListScheduledOn(dateStr, mineUID, mineKeys, 300)
	if err != nil {
		return err
	}
	unplanned, _, err := h.work.ListUnplanned(mineUID, mineKeys, "")
	if err != nil {
		return err
	}

	cols := repository.BuildStatsPeriodColumns(model.StatsViewDay, today)
	filter := repository.ParseMeetingFilter(model.StatsScopeTeam, "", "")
	if mineUID != "" {
		name := strings.TrimSpace(ctxString(c, "user_name"))
		if name == "" && len(mineKeys) > 0 {
			name = strings.TrimSpace(mineKeys[0])
		}
		if name != "" {
			filter = repository.ParseMeetingFilter(model.StatsScopeAssignee, name, "")
		}
	}
	_ = h.stats.FillPeriodOverview(cols, filter)

	var prevCol, curCol *model.StatsPeriodColumn
	for i := range cols {
		switch cols[i].Key {
		case "prev":
			prevCol = &cols[i]
		case "current":
			curCol = &cols[i]
		}
	}

	showAssignee := role == "admin" || role == "receipt" || scopeAll
	return c.Render(http.StatusOK, "meeting/show.html", map[string]interface{}{
		"Title":          "일일 업무 회의",
		"Active":         "meeting",
		"Date":           dateStr,
		"PrevDate":       prevStr,
		"NextDate":       today.AddDate(0, 0, 1).Format("2006-01-02"),
		"Yesterday":      yesterday,
		"TodayItems":     todayItems,
		"YesterdayN":     len(yesterday),
		"TodayN":         len(todayItems),
		"UnplannedN":     len(unplanned),
		"UnplannedHref":  planUnplannedURL(mineUID != "", role, ""),
		"PrevCol":        prevCol,
		"CurCol":         curCol,
		"ShowAssignee":   showAssignee,
		"Role":           role,
		"Mine":           mineUID != "",
		"ScopeNote":      meetingScopeNote(role, scopeAll),
		"MineQ":          meetingMineQuery(role, mineUID != ""),
		"HolidayBanner":  meetingHolidayBanner(dateStr),
	})
}

func parseMeetingAnchor(s string) time.Time {
	s = strings.TrimSpace(s)
	if s != "" {
		if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		}
	}
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
}

func meetingScopeNote(role string, scopeAll bool) string {
	if role == "tech" && !scopeAll {
		return "내 배정 업무 기준"
	}
	return "전체 업무 기준"
}

// meetingMineQuery mine 쿼리만 (앞에 & 없음). 날짜와 조합할 때 사용.
func meetingMineQuery(role string, mine bool) string {
	v := url.Values{}
	if role == "tech" {
		if mine {
			v.Set("mine", "1")
		} else {
			v.Set("mine", "0")
		}
	} else if mine {
		v.Set("mine", "1")
	}
	return v.Encode()
}
