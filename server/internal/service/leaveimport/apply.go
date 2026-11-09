package leaveimport

import (
	"fmt"
	"time"

	"customer-support/internal/service/leavecalc"
	"customer-support/internal/service/leaveledger"
)

type Check struct {
	UserID       string
	ExcelRemain  string
	LedgerRemain string
	OK           bool
}

type Report struct {
	Applied      int
	Skipped      int
	GrantMin     int
	UseMin       int
	ExcelRemain  int
	LedgerRemain int
	Checks       []Check
}

func Apply(led *leaveledger.Service, rows []PreviewRow, year int) (Report, error) {
	if year <= 0 {
		year = time.Now().Year()
	}
	start := fmt.Sprintf("%d-01-01", year)
	end := fmt.Sprintf("%d-12-31", year)
	tx, err := led.DB.Begin()
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var rep Report
	for _, r := range rows {
		if r.GrantEmpty {
			rep.Skipped++
			continue
		}
		if !Ready(r) {
			rep.Skipped++
			continue
		}
		org := r.OrgID
		if r.HireDate != "" {
			conflict, _, err := led.FillHireIfEmpty(tx, r.UserID, org, r.HireDate)
			if err != nil {
				return Report{}, err
			}
			_ = conflict
		}
		gmin := DaysToMinutes(r.GrantDays)
		if gmin > 0 {
			if err := led.AddOpeningOn(tx, r.UserID, start, end, gmin, "1", "기초 잔액 이관"); err != nil {
				return Report{}, err
			}
			rep.GrantMin += gmin
		}
		mmin := DaysToMinutes(r.MilitaryDays)
		if mmin > 0 {
			if err := led.AddOpeningOn(tx, r.UserID, start, end, mmin, "MIL", "예비군 보전 이관"); err != nil {
				return Report{}, err
			}
			rep.GrantMin += mmin
		} else if mmin < 0 {
			req := "IMP:" + r.UserID + ":MILADJ"
			if !led.HasConsReq(tx, req) {
				if err := led.ConsumeApproved(tx, r.UserID, req, start, -mmin); err != nil {
					return Report{}, err
				}
			}
			rep.UseMin += -mmin
		}
		for _, u := range r.Usages {
			if !u.Deduct || u.Minutes <= 0 {
				continue
			}
			suf := ""
			if u.Minutes == leavecalc.DefaultHalfDayMinutes {
				suf = ":h"
			}
			req := "IMP:" + r.UserID + ":" + u.On + suf
			if led.HasConsReq(tx, req) {
				continue
			}
			if err := led.ConsumeApproved(tx, r.UserID, req, u.On, u.Minutes); err != nil {
				return Report{}, fmt.Errorf("%s %s: %w", r.Name, u.On, err)
			}
			rep.UseMin += u.Minutes
		}
		rep.Applied++
	}
	if err := tx.Commit(); err != nil {
		return Report{}, err
	}

	asOf := fmt.Sprintf("%d-08-01", year)
	if led.Now != nil {
		asOf = led.Now().Format("2006-01-02")
	}
	for _, r := range rows {
		if !Ready(r) || r.GrantEmpty {
			continue
		}
		want := DaysToMinutes(r.RemainDays)
		rep.ExcelRemain += want
		bal, err := led.Balance(r.UserID, asOf)
		if err != nil {
			return Report{}, err
		}
		got := 0
		if bal.Measurable {
			got = bal.AvailableMinutes
		}
		rep.LedgerRemain += got
		rep.Checks = append(rep.Checks, Check{
			UserID:       r.UserID,
			ExcelRemain:  leavecalc.FormatDays(want),
			LedgerRemain: leavecalc.FormatDays(got),
			OK:           want == got,
		})
	}
	return rep, nil
}

func CountsOnly(rep Report) string {
	return fmt.Sprintf("기초잔액이관 확정 %d명 건너뜀 %d명 부여 %s 사용 %s 엑셀잔여 %s 원장잔여 %s",
		rep.Applied, rep.Skipped,
		leavecalc.FormatDays(rep.GrantMin), leavecalc.FormatDays(rep.UseMin),
		leavecalc.FormatDays(rep.ExcelRemain), leavecalc.FormatDays(rep.LedgerRemain))
}
