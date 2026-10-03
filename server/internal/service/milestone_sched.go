package service

import (
	"database/sql"
	"log"
	"time"

	"customer-support/internal/backup"
	"customer-support/internal/model"
	"customer-support/internal/repository"
)

// WeekDeadline 그 주 마지막 영업일 10:00. 공휴일 미등록 연도는 금요일. §63.2
func WeekDeadline(r *repository.StatsRepo, monday time.Time) (at time.Time, date string, holidayMissing bool) {
	loc := monday.Location()
	if loc == nil {
		loc = time.Local
	}
	from, toEx := repository.WeekRange(monday)
	if r == nil {
		day := monday.AddDate(0, 0, 4).Format("2006-01-02")
		t := monday.AddDate(0, 0, 4)
		at = time.Date(t.Year(), t.Month(), t.Day(), 10, 0, 0, 0, loc)
		return at, day, true
	}
	_, missing, err := r.CountWorkingDays(from, toEx)
	if err != nil {
		missing = true
	}
	holidayMissing = missing
	var day string
	if missing {
		day = monday.AddDate(0, 0, 4).Format("2006-01-02")
	} else {
		cal := DefaultCalendar()
		if cal == nil {
			cal = NewCalendar(repository.NewHolidayRepo(r.DB()))
		}
		sat := monday.AddDate(0, 0, 5).Format("2006-01-02")
		day = cal.PrevWorkingDay(sat)
		if day == "" {
			day = monday.AddDate(0, 0, 4).Format("2006-01-02")
		}
	}
	t, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil {
		t = monday.AddDate(0, 0, 4)
	}
	at = time.Date(t.Year(), t.Month(), t.Day(), 10, 0, 0, 0, loc)
	return at, day, holidayMissing
}

func CatchUpMilestones(r *repository.StatsRepo, now time.Time) {
	orgs, err := repository.NewOrgRepo(r.DB()).ListActive()
	if err != nil {
		return
	}
	base := r.MetricsPolicy().BaseDate
	start := repository.WeekMonday(now.AddDate(0, 0, -16*7))
	if base != "" {
		if t, e := time.ParseInLocation("2006-01-02", base, now.Location()); e == nil {
			if b := repository.WeekMonday(t); b.After(start) {
				start = b
			}
		}
	}
	thisMon := repository.WeekMonday(now)
	for d := start; !d.After(thisMon); d = d.AddDate(0, 0, 7) {
		deadline, _, _ := WeekDeadline(r, d)
		if now.Before(deadline) && !d.Before(thisMon) {
			continue
		}
		for _, o := range orgs {
			week := d.Format("2006-01-02")
			if !r.HasOrgMilestone(week, o.OrgID) {
				_ = r.ComputeMilestones(d, o.OrgID)
				continue
			}
			if now.Weekday() == time.Monday && r.MilestoneOrgState(week, o.OrgID) != model.MilestoneFixed {
				_ = r.ComputeMilestones(d, o.OrgID)
			}
		}
	}
}

func RunMilestoneTick(r *repository.StatsRepo, now time.Time) {
	mon := repository.WeekMonday(now)
	if now.Weekday() == time.Monday && now.Hour() < 12 {
		r.RecomputeDraftWeek(mon.AddDate(0, 0, -7))
	}
	deadline, _, _ := WeekDeadline(r, mon)
	if !now.Before(deadline) {
		r.RecomputeDraftWeek(mon)
	}
}

func StartMilestones(db *sql.DB) {
	if db == nil {
		return
	}
	go func() {
		loc := backup.SeoulLocation()
		stats := repository.NewStatsRepo(db)
		now := time.Now().In(loc)
		CatchUpMilestones(stats, now)
		for {
			deadline, _, _ := WeekDeadline(stats, repository.WeekMonday(now))
			next := repository.NextMilestoneFire(now, deadline)
			d := time.Until(next)
			if d < time.Second {
				d = time.Second
			}
			timer := time.NewTimer(d)
			<-timer.C
			now = time.Now().In(loc)
			log.Printf("마일스톤 고리: %s", now.Format("2006-01-02 15:04"))
			CatchUpMilestones(stats, now)
			RunMilestoneTick(stats, now)
		}
	}()
}
