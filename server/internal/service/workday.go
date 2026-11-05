package service

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
	"customer-support/internal/repository"
)

const workdaySearchCap = 60

// SkipInfo 다음 영업일을 고르며 건너뛴 날.
type SkipInfo struct {
	Date   string
	Reason string
}

// WorkdayFinder 휴일 달력+연차로 다음 영업일을 계산한다(95-84).
type WorkdayFinder struct {
	Cal         *Calendar
	Leaves      *repository.StaffLeaveRepo
	YearMissing func(int) bool
}

var defaultLeaves *repository.StaffLeaveRepo

// SetStaffLeaves NextWorkday 패키지 함수가 쓸 연차 저장소.
func SetStaffLeaves(r *repository.StaffLeaveRepo) {
	defaultLeaves = r
}

func DefaultLeaves() *repository.StaffLeaveRepo {
	return defaultLeaves
}

// NextWorkday from 기준 다음 영업일. includeFrom=false 이면 다음날부터 찾는다.
func NextWorkday(from time.Time, who []string, orgID string, includeFrom bool) (string, []SkipInfo) {
	return WorkdayFinder{
		Cal:    DefaultCalendar(),
		Leaves: DefaultLeaves(),
	}.NextWorkday(from, who, orgID, includeFrom)
}

// PrevWorkday from 기준 이전 영업일. includeFrom=false 이면 전날부터 찾는다.
// 연차를 넣지 않으려면 who 를 비운다(95-132 직전 근무일).
func PrevWorkday(from time.Time, who []string, orgID string, includeFrom bool) (string, []SkipInfo) {
	return WorkdayFinder{
		Cal:    DefaultCalendar(),
		Leaves: DefaultLeaves(),
	}.PrevWorkday(from, who, orgID, includeFrom)
}

func (f WorkdayFinder) NextWorkday(from time.Time, who []string, orgID string, includeFrom bool) (string, []SkipInfo) {
	if from.IsZero() {
		from = time.Now()
	}
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	cur := from
	if !includeFrom {
		cur = from.AddDate(0, 0, 1)
	}
	end := cur.AddDate(0, 0, workdaySearchCap)
	leaveByDate := f.leaveIndex(cur.Format("2006-01-02"), end.Format("2006-01-02"), orgID)
	who = compactWho(who)
	var skipped []SkipInfo
	missingNoted := false
	for i := 0; i <= workdaySearchCap; i++ {
		ds := cur.Format("2006-01-02")
		if i == workdaySearchCap {
			for cur.Weekday() == time.Saturday || cur.Weekday() == time.Sunday {
				cur = cur.AddDate(0, 0, 1)
			}
			ds = cur.Format("2006-01-02")
			skipped = append(skipped, SkipInfo{Date: ds, Reason: "휴일 정보를 확인하세요"})
			return ds, skipped
		}
		if f.yearMissing(cur.Year()) && !missingNoted {
			missingNoted = true
		}
		reason, skip := f.skipReason(cur, ds, who, leaveByDate)
		if skip {
			skipped = append(skipped, SkipInfo{Date: ds, Reason: reason})
			cur = cur.AddDate(0, 0, 1)
			continue
		}
		if missingNoted {
			skipped = append(skipped, SkipInfo{Date: ds, Reason: "휴일 정보를 확인하세요"})
		}
		return ds, skipped
	}
	return cur.Format("2006-01-02"), skipped
}

func (f WorkdayFinder) PrevWorkday(from time.Time, who []string, orgID string, includeFrom bool) (string, []SkipInfo) {
	if from.IsZero() {
		from = time.Now()
	}
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	cur := from
	if !includeFrom {
		cur = from.AddDate(0, 0, -1)
	}
	start := cur.AddDate(0, 0, -workdaySearchCap)
	leaveByDate := f.leaveIndex(start.Format("2006-01-02"), cur.Format("2006-01-02"), orgID)
	who = compactWho(who)
	var skipped []SkipInfo
	missingNoted := false
	for i := 0; i <= workdaySearchCap; i++ {
		ds := cur.Format("2006-01-02")
		if i == workdaySearchCap {
			for cur.Weekday() == time.Saturday || cur.Weekday() == time.Sunday {
				cur = cur.AddDate(0, 0, -1)
			}
			ds = cur.Format("2006-01-02")
			skipped = append(skipped, SkipInfo{Date: ds, Reason: "휴일 정보를 확인하세요"})
			return ds, skipped
		}
		if f.yearMissing(cur.Year()) && !missingNoted {
			missingNoted = true
		}
		reason, skip := f.skipReason(cur, ds, who, leaveByDate)
		if skip {
			skipped = append(skipped, SkipInfo{Date: ds, Reason: reason})
			cur = cur.AddDate(0, 0, -1)
			continue
		}
		if missingNoted {
			skipped = append(skipped, SkipInfo{Date: ds, Reason: "휴일 정보를 확인하세요"})
		}
		return ds, skipped
	}
	return cur.Format("2006-01-02"), skipped
}

func (f WorkdayFinder) yearMissing(y int) bool {
	if f.YearMissing != nil {
		return f.YearMissing(y)
	}
	if f.Cal != nil && f.Cal.repo != nil {
		return f.Cal.repo.YearMissing(y)
	}
	return false
}

func (f WorkdayFinder) skipReason(t time.Time, ds string, who []string, leaveByDate map[string][]model.StaffLeave) (string, bool) {
	if t.Weekday() == time.Saturday {
		return "토", true
	}
	if t.Weekday() == time.Sunday {
		return "일", true
	}
	if f.Cal != nil {
		if ok, name := f.Cal.IsHoliday(ds); ok {
			if strings.TrimSpace(name) == "" {
				name = "휴무"
			}
			return name, true
		}
	} else if ok, name := IsHoliday(ds); ok {
		if strings.TrimSpace(name) == "" {
			name = "휴무"
		}
		return name, true
	}
	if len(who) == 0 {
		return "", false
	}
	on := annualOnDate(leaveByDate[ds], who)
	if allWhoOnAnnual(who, leaveByDate[ds]) {
		return strings.Join(on, "·") + " 연차", true
	}
	return "", false
}

// InspectWorkday 고른 날짜가 휴무·전원 연차인지, 일부만 연차인지.
func (f WorkdayFinder) InspectWorkday(date string, who []string, orgID string) (holiday bool, holidayName string, allLeave bool, someLeave []string) {
	t, _, ok := model.ParseYMD(date)
	if !ok {
		return false, "", false, nil
	}
	ds := t.Format("2006-01-02")
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		holiday, holidayName = true, weekdayKR(t.Weekday())
	}
	if !holiday {
		if f.Cal != nil {
			holiday, holidayName = f.Cal.IsHoliday(ds)
		} else {
			holiday, holidayName = IsHoliday(ds)
		}
	}
	who = compactWho(who)
	if len(who) == 0 || f.Leaves == nil {
		return holiday, holidayName, false, nil
	}
	list, _ := f.Leaves.ListInRangeForOrg(ds, ds, orgID)
	on := annualOnDate(list, who)
	if len(on) == 0 {
		return holiday, holidayName, false, nil
	}
	if allWhoOnAnnual(who, list) {
		return holiday, holidayName, true, on
	}
	return holiday, holidayName, false, on
}

func allWhoOnAnnual(who []string, list []model.StaffLeave) bool {
	who = compactWho(who)
	if len(who) == 0 {
		return false
	}
	matched := map[string]bool{}
	for _, lv := range list {
		if strings.TrimSpace(lv.Kind) != model.LeaveKindAnnual {
			continue
		}
		for _, w := range who {
			if matchLeaveWho(lv, w) {
				matched[strings.ToLower(strings.TrimSpace(w))] = true
			}
		}
	}
	for _, w := range who {
		if !matched[strings.ToLower(strings.TrimSpace(w))] {
			return false
		}
	}
	return true
}

func (f WorkdayFinder) leaveIndex(from, to, orgID string) map[string][]model.StaffLeave {
	out := map[string][]model.StaffLeave{}
	if f.Leaves == nil {
		return out
	}
	list, err := f.Leaves.ListInRangeForOrg(from, to, orgID)
	if err != nil {
		return out
	}
	for _, it := range list {
		out[it.Date] = append(out[it.Date], it)
	}
	return out
}

func annualOnDate(list []model.StaffLeave, who []string) []string {
	var names []string
	seen := map[string]bool{}
	for _, w := range who {
		hit := false
		label := w
		for _, lv := range list {
			if strings.TrimSpace(lv.Kind) != model.LeaveKindAnnual {
				continue
			}
			if matchLeaveWho(lv, w) {
				hit = true
				if strings.TrimSpace(lv.UserName) != "" {
					label = lv.UserName
				}
				break
			}
		}
		if hit && !seen[label] {
			seen[label] = true
			names = append(names, label)
		}
	}
	return names
}

func matchLeaveWho(lv model.StaffLeave, who string) bool {
	who = strings.TrimSpace(who)
	if who == "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(lv.UserID), who) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(lv.UserName), who)
}

func compactWho(who []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range who {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		k := strings.ToLower(w)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, w)
	}
	return out
}

func weekdayKR(d time.Weekday) string {
	names := []string{"일", "월", "화", "수", "목", "금", "토"}
	return names[int(d)]
}

// FormatSkipNotice 「11/07(토)·11/08(일) 휴무로 11/09(월)로 정했습니다」
func FormatSkipNotice(chosen string, skipped []SkipInfo) string {
	if len(skipped) == 0 {
		return ""
	}
	var parts []string
	infoOnly := true
	for _, s := range skipped {
		if s.Reason == "휴일 정보를 확인하세요" && s.Date == chosen {
			continue
		}
		infoOnly = false
		t, _, ok := model.ParseYMD(s.Date)
		label := s.Date
		if ok {
			label = fmt.Sprintf("%02d/%02d(%s)", int(t.Month()), t.Day(), weekdayKR(t.Weekday()))
		}
		if s.Reason != "" && s.Reason != "토" && s.Reason != "일" {
			label += " " + s.Reason
		}
		parts = append(parts, label)
	}
	ct, _, ok := model.ParseYMD(chosen)
	end := chosen
	if ok {
		end = fmt.Sprintf("%02d/%02d(%s)", int(ct.Month()), ct.Day(), weekdayKR(ct.Weekday()))
	}
	if infoOnly {
		return "휴일 정보를 확인하세요. " + end + "로 정했습니다"
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "·") + " 휴무로 " + end + "로 정했습니다"
}

func FormatPartialLeaveNotice(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.Join(names, "·") + " 연차"
}
