package leavecalc

import (
	"errors"
	"sort"
)

var (
	// ErrInsufficientBalance is returned when consume/reserve would go negative.
	ErrInsufficientBalance = errors.New("insufficient leave balance")
	// ErrUnmeasurable is returned when hire date is missing (display "—").
	ErrUnmeasurable = errors.New("leave balance unmeasurable")
)

// Consumption is an append-only ledger line. Restore uses negative Minutes.
type Consumption struct {
	GrantKey string
	Minutes  int // use/expire >0; restore <0
	Kind     string
}

// GrantRemaining is leftover minutes on one grant as of T.
type GrantRemaining struct {
	Grant     Grant
	Remaining int
	Valid     bool // granted_on ≤ T < expires_on
	Expired   bool // T ≥ expires_on
}

// remainingOnGrant is grant.Minutes minus net consumptions.
func remainingOnGrant(g Grant, cons []Consumption) int {
	left := g.Minutes
	for _, c := range cons {
		if c.GrantKey == g.GrantKey {
			left -= c.Minutes
		}
	}
	return left
}

// grantStatus: valid while granted_on ≤ asOf < expires_on (만료일 당일은 소멸).
func grantStatus(g Grant, asOf string) (valid, expired bool, err error) {
	asOfT, err := parseDate(asOf)
	if err != nil {
		return false, false, err
	}
	granted, err := parseDate(g.GrantedOn)
	if err != nil {
		return false, false, err
	}
	expires, err := parseDate(g.ExpiresOn)
	if err != nil {
		return false, false, err
	}
	if cmpDate(asOfT, granted) < 0 {
		return false, false, nil
	}
	if cmpDate(asOfT, expires) >= 0 {
		return false, true, nil
	}
	return true, false, nil
}

// InspectGrants computes leftover per grant.
func InspectGrants(grants []Grant, cons []Consumption, asOf string) ([]GrantRemaining, error) {
	out := make([]GrantRemaining, 0, len(grants))
	for _, g := range grants {
		valid, expired, err := grantStatus(g, asOf)
		if err != nil {
			return nil, err
		}
		out = append(out, GrantRemaining{
			Grant:     g,
			Remaining: remainingOnGrant(g, cons),
			Valid:     valid,
			Expired:   expired,
		})
	}
	return out, nil
}

func fifoOrder(items []GrantRemaining) []GrantRemaining {
	sorted := append([]GrantRemaining(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i].Grant, sorted[j].Grant
		if a.ExpiresOn != b.ExpiresOn {
			return a.ExpiresOn < b.ExpiresOn
		}
		if a.GrantedOn != b.GrantedOn {
			return a.GrantedOn < b.GrantedOn
		}
		return a.GrantKey < b.GrantKey
	})
	return sorted
}

// ValidRemainingMinutes is leftover on grants still usable at asOf (before pending).
func ValidRemainingMinutes(grants []Grant, cons []Consumption, asOf string) (int, error) {
	items, err := InspectGrants(grants, cons, asOf)
	if err != nil {
		return 0, err
	}
	sum := 0
	for _, it := range items {
		if it.Valid && it.Remaining > 0 {
			sum += it.Remaining
		}
	}
	return sum, nil
}

// AvailableMinutes = valid remaining − pending reservation. Never below 0.
func AvailableMinutes(grants []Grant, cons []Consumption, pendingMinutes int, asOf string) (int, error) {
	left, err := ValidRemainingMinutes(grants, cons, asOf)
	if err != nil {
		return 0, err
	}
	if pendingMinutes < 0 {
		pendingMinutes = 0
	}
	avail := left - pendingMinutes
	if avail < 0 {
		return 0, nil
	}
	return avail, nil
}

// CanReserve is true when pending+need fits in valid remaining.
func CanReserve(grants []Grant, cons []Consumption, pendingMinutes, need int, asOf string) (bool, error) {
	avail, err := AvailableMinutes(grants, cons, pendingMinutes, asOf)
	if err != nil {
		return false, err
	}
	return need <= avail, nil
}

// ConsumeFIFO allocates need minutes from earliest-expiring valid grants.
func ConsumeFIFO(grants []Grant, cons []Consumption, need int, asOf string) ([]Consumption, error) {
	if need < 0 {
		return nil, errors.New("consume minutes must be >= 0")
	}
	if need == 0 {
		return nil, nil
	}
	items, err := InspectGrants(grants, cons, asOf)
	if err != nil {
		return nil, err
	}
	var used []Consumption
	left := need
	for _, it := range fifoOrder(items) {
		if left == 0 {
			break
		}
		if !it.Valid || it.Remaining <= 0 {
			continue
		}
		take := it.Remaining
		if take > left {
			take = left
		}
		used = append(used, Consumption{GrantKey: it.Grant.GrantKey, Minutes: take, Kind: KindUse})
		left -= take
	}
	if left > 0 {
		return nil, ErrInsufficientBalance
	}
	return used, nil
}

// Restore writes negative use lines (kind=restore) for prior consume lines.
func Restore(uses []Consumption) []Consumption {
	out := make([]Consumption, 0, len(uses))
	for _, u := range uses {
		if u.Minutes == 0 {
			continue
		}
		m := u.Minutes
		if m > 0 {
			m = -m
		}
		out = append(out, Consumption{GrantKey: u.GrantKey, Minutes: m, Kind: KindRestore})
	}
	return out
}

// Expire leftover on grants whose expires_on ≤ asOf. Idempotent if expire lines already exist.
func Expire(grants []Grant, cons []Consumption, asOf string) ([]Consumption, error) {
	items, err := InspectGrants(grants, cons, asOf)
	if err != nil {
		return nil, err
	}
	var out []Consumption
	for _, it := range items {
		if !it.Expired || it.Remaining <= 0 {
			continue
		}
		out = append(out, Consumption{GrantKey: it.Grant.GrantKey, Minutes: it.Remaining, Kind: KindExpire})
	}
	return out, nil
}

// Balance is a computed snapshot. Measurable=false → UI shows "—".
type Balance struct {
	Measurable       bool
	GrantedMinutes   int
	UsedMinutes      int
	ExpiredMinutes   int
	PendingMinutes   int
	AvailableMinutes int
	Grants           []Grant
}

// Evaluate computes grants at T then available after consumptions and pending.
func Evaluate(hireDate, asOf string, policy Policy, cons []Consumption, pendingMinutes int) (Balance, error) {
	gr, err := ComputeGrants(hireDate, asOf, policy)
	if err != nil {
		return Balance{}, err
	}
	if !gr.Measurable {
		return Balance{Measurable: false}, nil
	}
	avail, err := AvailableMinutes(gr.Grants, cons, pendingMinutes, asOf)
	if err != nil {
		return Balance{}, err
	}
	var granted, used, expired int
	items, err := InspectGrants(gr.Grants, cons, asOf)
	if err != nil {
		return Balance{}, err
	}
	for _, it := range items {
		granted += it.Grant.Minutes
	}
	for _, c := range cons {
		switch c.Kind {
		case KindUse:
			used += c.Minutes
		case KindRestore:
			used += c.Minutes // restore minutes are negative
		case KindExpire:
			expired += c.Minutes
		}
	}
	if used < 0 {
		used = 0
	}
	if pendingMinutes < 0 {
		pendingMinutes = 0
	}
	return Balance{
		Measurable:       true,
		GrantedMinutes:   granted,
		UsedMinutes:      used,
		ExpiredMinutes:   expired,
		PendingMinutes:   pendingMinutes,
		AvailableMinutes: avail,
		Grants:           gr.Grants,
	}, nil
}
