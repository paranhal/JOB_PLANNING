package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"customer-support/internal/model"
	"customer-support/internal/repository"
	"customer-support/internal/service"
)

type HolidayHandler struct {
	repo  *repository.HolidayRepo
	leave *repository.StaffLeaveRepo
	users *repository.UserRepo
	auth  *AuthHandler
	api   service.HolidayAPI
}

type holidayCalDay struct {
	Day         int
	Date        string
	Kind        string
	Name        string
	Weekend     bool
	IsSunday    bool
	IsSaturday  bool
	InMonth     bool
	DayClass    string
	CellClass   string
	DayStyle    string
	CellStyle   string
	Title       string
}

type holidayCalMonth struct {
	Month int
	Label string
	Weeks [][]holidayCalDay
}

func (h *HolidayHandler) List(c echo.Context) error {
	nowY := time.Now().Year()
	year, _ := strconv.Atoi(strings.TrimSpace(c.QueryParam("year")))
	if year < 2000 || year > 2100 {
		year = nowY
	}
	items, err := h.repo.ListByYear(year)
	if err != nil {
		return err
	}
	byDate := map[string]model.Holiday{}
	for _, it := range items {
		byDate[it.Date] = it
	}
	years := holidayYearTabs(nowY, year, h.repo)
	lastSync, _ := h.repo.LastSyncedAt(year)
	tab := strings.TrimSpace(c.QueryParam("tab"))
	if tab != "leave" {
		tab = "holiday"
	}
	var leaves []model.StaffLeave
	var users []model.User
	if h.leave != nil {
		leaves, _ = h.leave.ListByYear(year)
	}
	if h.users != nil {
		users, _ = h.users.ListAssignable()
	}
	return c.Render(http.StatusOK, "admin/holidays.html", map[string]interface{}{
		"Title":         "휴무일 관리",
		"Active":        "holidays",
		"Year":          year,
		"Years":         years,
		"Tab":           tab,
		"Items":         items,
		"Leaves":        leaves,
		"Users":         users,
		"Months":        buildHolidayPreview(year, byDate),
		"IsAdmin":       isAdminRole(c),
		"LastSynced":    lastSync,
		"YearMissing":   len(items) == 0,
		"MissingBanner": model.HolidayMissingBanner(year),
		"FlashOK":       c.QueryParam("ok"),
		"FlashErr":      c.QueryParam("err"),
		"EditDate":      strings.TrimSpace(c.QueryParam("edit")),
	})
}

func (h *HolidayHandler) Create(c echo.Context) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	year := holidayFormYear(c)
	hld := model.Holiday{
		Date: c.FormValue("holiday_date"),
		Name: c.FormValue("name"),
		Kind: c.FormValue("kind"),
	}
	if err := h.repo.Create(hld); err != nil {
		return holidayRedirect(c, year, "", err.Error())
	}
	return holidayRedirect(c, year, "등록했습니다", "")
}

func (h *HolidayHandler) Update(c echo.Context) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	year := holidayFormYear(c)
	old := firstNonEmpty(c.FormValue("old_date"), c.FormValue("holiday_date"))
	hld := model.Holiday{
		Date: firstNonEmpty(c.FormValue("holiday_date"), old),
		Name: c.FormValue("name"),
		Kind: c.FormValue("kind"),
	}
	if err := h.repo.Update(old, hld); err != nil {
		return holidayRedirect(c, year, "", err.Error())
	}
	return holidayRedirect(c, year, "수정했습니다", "")
}

func (h *HolidayHandler) Delete(c echo.Context) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	year := holidayFormYear(c)
	date := firstNonEmpty(c.FormValue("holiday_date"), c.FormValue("old_date"))
	if err := h.repo.Delete(date); err != nil {
		return holidayRedirect(c, year, "", err.Error())
	}
	return holidayRedirect(c, year, "삭제했습니다", "")
}

func (h *HolidayHandler) SyncAPI(c echo.Context) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	year := holidayFormYear(c)
	last, _ := h.repo.LastSyncedAt(year)
	n, err := service.SyncYear(h.repo, h.api, year)
	if err != nil {
		return holidayRedirect(c, year, "", model.HolidaySyncFailBanner(year, last))
	}
	return holidayRedirect(c, year, fmt.Sprintf("공휴일 %d건을 동기화했습니다.", n), "")
}

func (h *HolidayHandler) CreateLeave(c echo.Context) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	if h.leave == nil {
		return holidayRedirectTab(c, holidayFormYear(c), "leave", "", "저장소가 없습니다")
	}
	year := holidayFormYear(c)
	from := firstNonEmpty(c.FormValue("leave_from"), c.FormValue("leave_date"))
	to := firstNonEmpty(c.FormValue("leave_to"), from)
	dates, err := service.ExpandLeaveWorkingDates(from, to)
	if err != nil {
		return holidayRedirectTab(c, year, "leave", "", err.Error())
	}
	if len(dates) == 0 {
		return holidayRedirectTab(c, year, "leave", "", "주말·공휴일은 건너뛰어 등록할 근무일이 없습니다")
	}
	uid := strings.TrimSpace(c.FormValue("user_id"))
	kind := strings.TrimSpace(c.FormValue("leave_kind"))
	note := strings.TrimSpace(c.FormValue("note"))
	uname := ""
	if h.users != nil {
		if u, _ := h.users.GetByID(uid); u != nil {
			uname = u.FullName
		}
	}
	n := 0
	for _, ds := range dates {
		err := h.leave.Create(model.StaffLeave{
			UserID:    uid,
			UserName:  uname,
			Date:      ds,
			Kind:      kind,
			Note:      note,
			CreatedBy: currentUserID(c),
		})
		if err != nil {
			if n == 0 {
				return holidayRedirectTab(c, year, "leave", "", err.Error())
			}
			return holidayRedirectTab(c, year, "leave", fmt.Sprintf("%d건 등록, 일부는 건너뛰었습니다: %s", n, err.Error()), "")
		}
		n++
	}
	return holidayRedirectTab(c, year, "leave", fmt.Sprintf("연차 %d건을 등록했습니다", n), "")
}

func (h *HolidayHandler) DeleteLeave(c echo.Context) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	if h.leave == nil {
		return holidayRedirectTab(c, holidayFormYear(c), "leave", "", "저장소가 없습니다")
	}
	year := holidayFormYear(c)
	if err := h.leave.Delete(c.FormValue("leave_id")); err != nil {
		return holidayRedirectTab(c, year, "leave", "", err.Error())
	}
	return holidayRedirectTab(c, year, "leave", "삭제했습니다", "")
}

func (h *HolidayHandler) requireAdmin(c echo.Context) error {
	if isAdminRole(c) {
		return nil
	}
	if h.auth != nil {
		return h.auth.forbidden(c)
	}
	return echo.NewHTTPError(http.StatusForbidden, "관리자만 변경할 수 있습니다")
}

func holidayRedirect(c echo.Context, year int, ok, errMsg string) error {
	return holidayRedirectTab(c, year, firstNonEmpty(c.FormValue("tab"), c.QueryParam("tab")), ok, errMsg)
}

func holidayRedirectTab(c echo.Context, year int, tab, ok, errMsg string) error {
	q := url.Values{}
	if year >= 2000 {
		q.Set("year", strconv.Itoa(year))
	}
	if tab == "leave" {
		q.Set("tab", "leave")
	}
	if ok != "" {
		q.Set("ok", ok)
	}
	if errMsg != "" {
		q.Set("err", errMsg)
	}
	loc := "/admin/holidays"
	if enc := q.Encode(); enc != "" {
		loc += "?" + enc
	}
	return c.Redirect(http.StatusSeeOther, loc)
}

func holidayFormYear(c echo.Context) int {
	year, _ := strconv.Atoi(strings.TrimSpace(firstNonEmpty(c.FormValue("year"), c.QueryParam("year"))))
	if year < 2000 || year > 2100 {
		if t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(c.FormValue("holiday_date")), time.Local); err == nil {
			return t.Year()
		}
		return time.Now().Year()
	}
	return year
}

func holidayYearTabs(nowY, selected int, repo *repository.HolidayRepo) []int {
	seen := map[int]bool{}
	var years []int
	add := func(y int) {
		if y < 2000 || y > 2100 || seen[y] {
			return
		}
		seen[y] = true
		years = append(years, y)
	}
	for y := 2025; y <= 2027; y++ {
		add(y)
	}
	for y := nowY - 1; y <= nowY+2; y++ {
		add(y)
	}
	add(selected)
	if repo != nil {
		listed, _ := repo.ListYears()
		for _, y := range listed {
			add(y)
		}
	}
	sort.Ints(years)
	return years
}

func buildHolidayPreview(year int, byDate map[string]model.Holiday) []holidayCalMonth {
	loc := time.Local
	out := make([]holidayCalMonth, 0, 12)
	for m := 1; m <= 12; m++ {
		first := time.Date(year, time.Month(m), 1, 0, 0, 0, 0, loc)
		last := first.AddDate(0, 1, -1)
		start := first.AddDate(0, 0, -int(first.Weekday()))
		end := last.AddDate(0, 0, (6-int(last.Weekday())+7)%7)
		var weeks [][]holidayCalDay
		var week []holidayCalDay
		for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
			ds := cursor.Format("2006-01-02")
			tn := toneForDate(ds, "")
			kind, name := "", ""
			if h, ok := byDate[ds]; ok {
				kind, name = h.Kind, h.Name
				tn.HolidayName = h.Name
				tn.Title = h.Name
				applyHolidayKind(&tn, h.Kind)
			}
			week = append(week, holidayCalDay{
				Day:        cursor.Day(),
				Date:       ds,
				Kind:       kind,
				Name:       name,
				Weekend:    tn.IsSunday || tn.IsSaturday,
				IsSunday:   tn.IsSunday,
				IsSaturday: tn.IsSaturday,
				InMonth:    cursor.Month() == time.Month(m),
				DayClass:   tn.DayClass,
				CellClass:  tn.CellClass,
				DayStyle:   tn.DayStyle,
				CellStyle:  tn.CellStyle,
				Title:      tn.Title,
			})
			if len(week) == 7 {
				weeks = append(weeks, week)
				week = nil
			}
		}
		out = append(out, holidayCalMonth{Month: m, Label: fmt.Sprintf("%d월", m), Weeks: weeks})
	}
	return out
}

func holidayMissingBanner(repo *repository.HolidayRepo, year int) string {
	if repo == nil || year <= 0 || !repo.YearMissing(year) {
		return ""
	}
	return model.HolidayMissingBanner(year)
}

func holidayDateSet(repo *repository.HolidayRepo, from, to string) map[string]bool {
	if repo == nil {
		return map[string]bool{}
	}
	from = expandHolidayBound(from, -7)
	to = expandHolidayBound(to, 7)
	m, err := repo.DatesInRange(from, to)
	if err != nil || m == nil {
		return map[string]bool{}
	}
	return m
}

func expandHolidayBound(date string, deltaDays int) string {
	t, err := time.ParseInLocation(dateLayout, date, time.Local)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, deltaDays).Format(dateLayout)
}

func isDisplayOffDay(date string, off map[string]bool) bool {
	if service.HasHolidayCalendar() {
		return !service.IsWorkingDay(date)
	}
	t, err := time.ParseInLocation(dateLayout, date, time.Local)
	if err != nil {
		return false
	}
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return true
	}
	return off[date]
}

func markRegisterOffDays(cols []RegisterColumn, off map[string]bool) {
	for i := range cols {
		cols[i].IsOffDay = isDisplayOffDay(cols[i].Date, off)
	}
}

func markRegisterDayOffDays(cols []RegisterDayColumn, off map[string]bool) {
	for i := range cols {
		cols[i].IsOffDay = isDisplayOffDay(cols[i].Date, off)
	}
}

func markMonthOffDays(weeks [][]RegisterMonthDay, off map[string]bool) {
	for wi := range weeks {
		for di := range weeks[wi] {
			d := weeks[wi][di].Date
			if d == "" {
				continue
			}
			if isDisplayOffDay(d, off) {
				weeks[wi][di].IsWeekend = true
			}
		}
	}
}
