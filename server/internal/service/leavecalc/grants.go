package leavecalc

import (
	"fmt"
	"time"
)

// Grant is one auto-generated accrual. grant_key is idempotent for the same hire + occurrence.
type Grant struct {
	GrantKey  string
	Minutes   int
	GrantedOn string
	ExpiresOn string
	Source    string
}

// GrantResult is the grant schedule as of T.
// Measurable=false when hire date is missing — remaining must display "—", not 0.
type GrantResult struct {
	Measurable bool
	Grants     []Grant
}

// ComputeGrants returns auto grants on/after ledger_start_date with granted_on ≤ asOf.
// Attendance 80% is not judged. Opening balances are not generated here.
func ComputeGrants(hireDate, asOf string, policy Policy) (GrantResult, error) {
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

	include := func(granted time.Time) bool {
		if cmpDate(granted, asOfT) > 0 {
			return false
		}
		if hasLedger && cmpDate(granted, ledgerStart) < 0 {
			return false
		}
		return true
	}

	var out []Grant
	monthlyExpire := addYearsClamped(hire, policy.MonthlyExpireAfterYears)
	for i := 1; i <= policy.MonthlyMaxOccurrences; i++ {
		granted := addMonthsClamped(hire, i)
		if !include(granted) {
			continue
		}
		out = append(out, Grant{
			GrantKey:  MonthlyGrantKey(granted),
			Minutes:   MonthlyGrantDays * policy.WorkMinutesPerDay,
			GrantedOn: formatDate(granted),
			ExpiresOn: formatDate(monthlyExpire),
			Source:    SourceAutoMonthly,
		})
	}

	for y := 1; ; y++ {
		granted := addYearsClamped(hire, y)
		if cmpDate(granted, asOfT) > 0 {
			break
		}
		if !include(granted) {
			continue
		}
		days := yearlyGrantDays(y, policy)
		out = append(out, Grant{
			GrantKey:  YearlyGrantKey(granted),
			Minutes:   days * policy.WorkMinutesPerDay,
			GrantedOn: formatDate(granted),
			ExpiresOn: formatDate(addYearsClamped(granted, policy.YearlyExpireAfterYears)),
			Source:    SourceAutoYearly,
		})
	}
	return GrantResult{Measurable: true, Grants: out}, nil
}

// MonthlyGrantKey is M:YYYY-MM of the grant date.
func MonthlyGrantKey(grantedOn time.Time) string {
	return GrantKeyMonthlyPrefix + dateOnly(grantedOn).Format("2006-01")
}

// YearlyGrantKey is Y:YYYY of the anniversary date.
func YearlyGrantKey(grantedOn time.Time) string {
	return fmt.Sprintf("%s%d", GrantKeyYearlyPrefix, dateOnly(grantedOn).Year())
}

// NextMonthlyGrantDate is the next H+n month date after asOf, or "" if tenure ≥1 year
// (monthly window closed) or hire missing.
func NextMonthlyGrantDate(hireDate, asOf string, policy Policy) (string, error) {
	policy = policy.normalized()
	asOfT, err := parseDate(asOf)
	if err != nil {
		return "", err
	}
	hire, hasHire, err := parseDateOpt(hireDate)
	if err != nil {
		return "", err
	}
	if !hasHire {
		return "", nil
	}
	for i := 1; i <= policy.MonthlyMaxOccurrences; i++ {
		g := addMonthsClamped(hire, i)
		if cmpDate(g, asOfT) > 0 {
			return formatDate(g), nil
		}
	}
	return "", nil
}
