package leaveapp

import (
	"fmt"

	"customer-support/internal/service/leavecalc"
	"customer-support/internal/service/leaveledger"
)

type Preview struct {
	Registered   bool   `json:"registered"`
	Measurable   bool   `json:"measurable"`
	RemainLabel  string `json:"remain_label"`
	AfterLabel   string `json:"after_label"`
	NeedLabel    string `json:"need_label"`
	AbsenceLabel string `json:"absence_label"`
	Shortage     bool   `json:"shortage"`
	Message      string `json:"message"`
	Deducts      bool   `json:"deducts"`
}

func (s *Service) Preview(userID string, in Input) (Preview, error) {
	asOf := in.StartDate
	if asOf == "" {
		asOf = s.now().Format("2006-01-02")
	}
	sn, err := s.ledger().Snapshot(userID, asOf)
	if err != nil {
		return Preview{}, err
	}
	out := Preview{
		Registered:  sn.Registered,
		Measurable:  sn.Measurable,
		RemainLabel: sn.RemainLabel,
		AfterLabel:  sn.RemainLabel,
		Deducts:     leavecalc.DeductsBalance(in.TypeKey),
	}
	if in.TypeKey == "" {
		return out, nil
	}
	holidays, err := s.holidayDates()
	if err != nil {
		return Preview{}, err
	}
	if in.PeriodMode == "" {
		in.PeriodMode = defaultPeriod(in.TypeKey)
	}
	if in.EndDate == "" {
		in.EndDate = in.StartDate
	}
	_, minutes, err := computeDays(in, holidays)
	if err != nil {
		out.Message = err.Error()
		return out, nil
	}
	out.NeedLabel = leavecalc.FormatDays(minutes)
	if !out.Deducts || !sn.Measurable {
		out.AfterLabel = sn.RemainLabel
		return out, nil
	}
	deduct, abs := leavecalc.SplitNeed(minutes, sn.Balance.AvailableMinutes)
	out.AbsenceLabel = leavecalc.FormatDays(abs)
	out.AfterLabel = leavecalc.FormatDays(sn.Balance.AvailableMinutes - deduct)
	out.Shortage = abs > 0
	if out.Shortage {
		out.Message = fmt.Sprintf("부족한 %s은 결근 처리되어 급여에서 일당이 차감됩니다", leavecalc.FormatDays(abs))
	}
	return out, nil
}

func (s *Service) MySnapshot(userID, asOf string) (leaveledger.Snapshot, error) {
	return s.ledger().Snapshot(userID, asOf)
}
