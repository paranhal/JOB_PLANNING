package leavecalc

import (
	"fmt"
	"time"
)

// FiscalGrantKey is F:YYYY of the accounting year.
func FiscalGrantKey(year int) string {
	return fmt.Sprintf("%s%d", GrantKeyFiscalPrefix, year)
}

// ComputeFiscalGrants 회계연도 부여. 1/1 일괄(입사 연도는 입사일), 12/31 소멸, 이월 없음.
// 입사 연도는 15일을 재직일/연일수로 비례(반올림). 이후 연차는 YearlyGrantDays(연차차수).
// 입사일 없으면 자동 부여 없음(Measurable=false).
func ComputeFiscalGrants(hireDate, asOf string, policy Policy) (GrantResult, error) {
	policy = policy.normalized()
	asOfT, err := parseDate(asOf)
	if err != nil {
		return GrantResult{}, err
	}
	hire, hasHire, err := parseDateOpt(hireDate)
	if err != nil {
		return GrantResult{}, err
	}
	if !hasHire {
		return GrantResult{Measurable: false}, nil
	}

	var ledgerStart time.Time
	var hasLedger bool
	if policy.LedgerStartDate != "" {
		ledgerStart, err = parseDate(policy.LedgerStartDate)
		if err != nil {
			return GrantResult{}, err
		}
		hasLedger = true
	}

	hireYear := hire.Year()
	endYear := asOfT.Year()
	if endYear < hireYear {
		return GrantResult{Measurable: true}, nil
	}

	var out []Grant
	for y := hireYear; y <= endYear; y++ {
		yearIndex := y - hireYear + 1
		granted := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC)
		if y == hireYear {
			granted = hire
		}
		if cmpDate(granted, asOfT) > 0 {
			continue
		}
		if hasLedger && cmpDate(granted, ledgerStart) < 0 {
			continue
		}
		expires := time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC)
		days := 0
		if yearIndex == 1 {
			yearLen := daysInYear(y)
			remain := remainingInclusive(granted, expires)
			days = roundHalfUp(YearlyGrantDays(1)*remain, yearLen)
		} else {
			days = yearlyGrantDays(yearIndex, policy)
		}
		if days <= 0 {
			continue
		}
		out = append(out, Grant{
			GrantKey:  FiscalGrantKey(y),
			Minutes:   days * policy.WorkMinutesPerDay,
			GrantedOn: formatDate(granted),
			ExpiresOn: formatDate(expires),
			Source:    SourceAutoFiscal,
		})
	}
	return GrantResult{Measurable: true, Grants: out}, nil
}

func daysInYear(year int) int {
	if isLeapYear(year) {
		return 366
	}
	return 365
}

func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

func remainingInclusive(from, to time.Time) int {
	from, to = dateOnly(from), dateOnly(to)
	if to.Before(from) {
		return 0
	}
	return int(to.Sub(from).Hours()/24) + 1
}

func roundHalfUp(num, den int) int {
	if den <= 0 {
		return 0
	}
	if num < 0 {
		return -roundHalfUp(-num, den)
	}
	return (num + den/2) / den
}
