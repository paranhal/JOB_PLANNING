package handler

import (
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

// WeeklyMeXLSX GET /meeting/weekly/me.xlsx 본인 담당만. D-95
func (h *MeetingHandler) WeeklyMeXLSX(c echo.Context) error {
	if h == nil || h.stats == nil {
		return echo.ErrNotFound
	}
	anchor := parseMeetingAnchor(c.QueryParam("date"))
	mon, _ := meetingWeekMonFri(anchor)
	rep, err := h.stats.BuildWeeklyReportOrg(mon, currentOrg(c))
	if err != nil {
		return err
	}
	var users []model.User
	if h.userRepo != nil {
		users, _ = h.userRepo.ListAssignable()
	}
	uid, keys := meetingSelfKeys(c, users)
	name := strings.TrimSpace(ctxString(c, "user_name"))
	if strings.TrimSpace(ctxString(c, "view_as_user_id")) != "" {
		name = strings.TrimSpace(ctxString(c, "view_as_name"))
	}
	rep = filterWeeklyReportMine(rep, uid, keys, name)

	f := excelize.NewFile()
	defer f.Close()
	if err := writeWeeklyStatsSheet(f, rep); err != nil {
		return err
	}
	if err := writeWeeklyAllListSheet(f, rep); err != nil {
		return err
	}
	h.writeWeeklyPersonalExtraSheets(c, f, mon)
	_ = f.DeleteSheet("Sheet1")
	buf, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	day := strings.ReplaceAll(rep.Friday, "-", "")
	if day == "" {
		day = mon.AddDate(0, 0, 4).Format("20060102")
	}
	safe := asciiNamePart(name)
	ascii := day + "_my_weekly_" + safe + ".xlsx"
	utf8 := day + "_내주간자료_" + name + ".xlsx"
	return writeExcelFile(c, buf.Bytes(), ascii, utf8)
}

func filterWeeklyReportMine(rep model.WeeklyReport, uid string, keys []string, name string) model.WeeklyReport {
	var ev []model.WeeklyEventRow
	for _, e := range rep.Events {
		if weeklyAssigneeMatch(e.Assignee, uid, keys) {
			ev = append(ev, e)
		}
	}
	rep.Events = ev
	var pr []model.WeeklyPersonRow
	for _, r := range rep.PersonRows {
		if r.IsTeam {
			continue
		}
		if weeklyAssigneeMatch(r.Label, uid, keys) || (name != "" && r.Label == name) {
			pr = append(pr, r)
		}
	}
	if len(pr) == 0 && name != "" {
		pr = []model.WeeklyPersonRow{{Label: name}}
	}
	rep.PersonRows = pr
	return rep
}

func weeklyAssigneeMatch(assignee, uid string, keys []string) bool {
	a := strings.TrimSpace(assignee)
	if a == "" {
		return false
	}
	if uid != "" && a == uid {
		return true
	}
	for _, k := range keys {
		if strings.TrimSpace(k) != "" && a == strings.TrimSpace(k) {
			return true
		}
	}
	return false
}

func asciiNamePart(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return "user"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "user"
	}
	return b.String()
}

func (h *MeetingHandler) writeWeeklyPersonalExtraSheets(c echo.Context, f *excelize.File, mon time.Time) {
	if h == nil || f == nil {
		return
	}
	_, fri := meetingWeekMonFri(mon)
	data := map[string]interface{}{}
	h.fillWeeklyPersonalCards(c, mon, fri, data)
	if show, _ := data["ShowLeave"].(bool); show {
		rows, _ := data["LeaveRows"].([]weeklyLeaveRow)
		_ = writeSimpleSheet(f, "휴가", []string{"문서", "구분", "기간", "상태"}, func(i int) []interface{} {
			if i >= len(rows) {
				return nil
			}
			r := rows[i]
			return []interface{}{r.DocNo, r.Type, r.Period, r.Status}
		})
	}
	if show, _ := data["ShowInbox"].(bool); show {
		rows, _ := data["InboxRows"].([]weeklyInboxRow)
		_ = writeSimpleSheet(f, "결재대기", []string{"제목", "상태"}, func(i int) []interface{} {
			if i >= len(rows) {
				return nil
			}
			r := rows[i]
			return []interface{}{r.Title, r.Status}
		})
	}
}

func writeSimpleSheet(f *excelize.File, name string, headers []string, rowAt func(int) []interface{}) error {
	if _, err := f.NewSheet(name); err != nil {
		return err
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(name, cell, h)
	}
	for i := 0; ; i++ {
		vals := rowAt(i)
		if vals == nil {
			break
		}
		for j, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			_ = f.SetCellValue(name, cell, v)
		}
	}
	return nil
}
