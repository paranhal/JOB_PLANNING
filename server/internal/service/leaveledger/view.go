package leaveledger

import (
	"fmt"
	"strings"
	"time"

	"customer-support/internal/service/leavecalc"
)

type Snapshot struct {
	Registered   bool
	Measurable   bool
	RemainLabel  string
	GrantedLabel string
	UsedLabel    string
	PendingLabel string
	ExpireOn     string
	Balance      leavecalc.Balance
}

func UnregisteredRemain() string { return "— (연차 미등록)" }

func (s *Service) Snapshot(userID, asOf string) (Snapshot, error) {
	if asOf == "" {
		asOf = s.asOf()
	}
	out := Snapshot{ExpireOn: fiscalExpire(asOf)}
	ok, err := s.IsRegistered(s.DB, userID)
	if err != nil {
		return out, err
	}
	out.Registered = ok
	if !ok {
		out.RemainLabel = UnregisteredRemain()
		out.GrantedLabel, out.UsedLabel, out.PendingLabel = "—", "—", "—"
		return out, nil
	}
	bal, err := s.Balance(userID, asOf)
	if err != nil {
		return out, err
	}
	out.Balance = bal
	out.Measurable = bal.Measurable
	if !bal.Measurable {
		out.RemainLabel = UnregisteredRemain()
		out.GrantedLabel, out.UsedLabel, out.PendingLabel = "—", "—", "—"
		return out, nil
	}
	out.GrantedLabel = leavecalc.FormatDays(bal.GrantedMinutes)
	out.UsedLabel = leavecalc.FormatDays(bal.UsedMinutes)
	out.PendingLabel = leavecalc.FormatDays(bal.PendingMinutes)
	out.RemainLabel = leavecalc.FormatDays(bal.AvailableMinutes)
	return out, nil
}

func fiscalExpire(asOf string) string {
	asOf = strings.TrimSpace(asOf)
	if len(asOf) >= 4 {
		return asOf[:4] + "-12-31"
	}
	return fmt.Sprintf("%d-12-31", time.Now().Year())
}

type YearItem struct {
	LeaveReqID string
	DocNo      string
	TypeKey    string
	TypeLabel  string
	StartDate  string
	EndDate    string
	Status     string
	LeaveLabel string
	AbsLabel   string
	Military   bool
}

func (s *Service) YearUsage(userID, year string) ([]YearItem, []YearItem, error) {
	if len(year) < 4 {
		year = s.asOf()[:4]
	}
	year = year[:4]
	rows, err := s.DB.Query(`SELECT leave_req_id, doc_no, type_key, start_date, end_date, status,
		COALESCE(deduct_minutes,0), COALESCE(absence_minutes,0)
		FROM leave_requests
		WHERE user_id=? AND substr(start_date,1,4)=? AND status IN ('pending','approved','cancelled')
		ORDER BY start_date, created_at`, userID, year)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var used, military []YearItem
	for rows.Next() {
		var it YearItem
		var deduct, abs int
		if err := rows.Scan(&it.LeaveReqID, &it.DocNo, &it.TypeKey, &it.StartDate, &it.EndDate, &it.Status, &deduct, &abs); err != nil {
			return nil, nil, err
		}
		td, _ := leavecalc.TypeByKey(it.TypeKey)
		it.TypeLabel = td.Label
		if it.TypeLabel == "" {
			it.TypeLabel = it.TypeKey
		}
		it.LeaveLabel = leavecalc.FormatDays(deduct)
		it.AbsLabel = leavecalc.FormatDays(abs)
		it.Military = it.TypeKey == leavecalc.TypeMilitaryTraining
		if it.Military {
			military = append(military, it)
			continue
		}
		used = append(used, it)
	}
	return used, military, rows.Err()
}

type MemberRow struct {
	UserID, Name, OrgID, OrgName, HireDate string
	AutoEnabled                            bool
	Snapshot                               Snapshot
}

func (s *Service) OrgMemberRows(orgID, asOf string) ([]MemberRow, error) {
	if asOf == "" {
		asOf = s.asOf()
	}
	q := `SELECT u.user_id, COALESCE(NULLIF(TRIM(u.full_name),''), u.username, ''), COALESCE(u.org_id,''),
		COALESCE(o.org_name,''), COALESCE(p.hire_date,''), COALESCE(p.leave_auto_enabled,0)
		FROM users u
		LEFT JOIN employee_profiles p ON p.user_id=u.user_id
		LEFT JOIN orgs o ON o.org_id=u.org_id
		WHERE COALESCE(u.is_active,1)=1`
	args := []any{}
	if strings.TrimSpace(orgID) != "" {
		q += ` AND u.org_id=?`
		args = append(args, orgID)
	}
	q += ` ORDER BY o.org_name, u.full_name`
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemberRow
	for rows.Next() {
		var r MemberRow
		var auto int
		if err := rows.Scan(&r.UserID, &r.Name, &r.OrgID, &r.OrgName, &r.HireDate, &auto); err != nil {
			return nil, err
		}
		r.AutoEnabled = auto == 1
		sn, err := s.Snapshot(r.UserID, asOf)
		if err != nil {
			return nil, err
		}
		r.Snapshot = sn
		out = append(out, r)
	}
	return out, rows.Err()
}

type GapRow struct {
	UserID, Name, OrgID, OrgName, HireDate, Reason string
}

func (s *Service) GapRows(orgID string) ([]GapRow, []GapRow, error) {
	rows, err := s.OrgMemberRows(orgID, s.asOf())
	if err != nil {
		return nil, nil, err
	}
	var unreg, nohire []GapRow
	for _, r := range rows {
		if !r.Snapshot.Registered {
			unreg = append(unreg, GapRow{r.UserID, r.Name, r.OrgID, r.OrgName, r.HireDate, "연차 미등록"})
		}
		if strings.TrimSpace(r.HireDate) == "" {
			nohire = append(nohire, GapRow{r.UserID, r.Name, r.OrgID, r.OrgName, r.HireDate, "입사일 없음"})
		}
	}
	return unreg, nohire, nil
}

type AbsenceRow struct {
	AbsenceID, UserID, Name, OrgID, LeaveReqID, Kind, Note, CreatedAt string
	Minutes                                                           int
	DaysLabel                                                         string
}

func (s *Service) AbsenceRows(orgID, month string) ([]AbsenceRow, error) {
	if len(month) >= 7 {
		month = month[:7]
	}
	q := `SELECT a.absence_id, a.user_id, COALESCE(NULLIF(TRIM(u.full_name),''), u.username, ''), COALESCE(u.org_id,''),
		COALESCE(a.leave_req_id,''), a.minutes, a.kind, COALESCE(a.note,''), IFNULL(a.created_at,'')
		FROM leave_absence_records a
		JOIN users u ON u.user_id=a.user_id
		WHERE 1=1`
	args := []any{}
	if strings.TrimSpace(orgID) != "" {
		q += ` AND u.org_id=?`
		args = append(args, orgID)
	}
	if month != "" {
		q += ` AND substr(datetime(IFNULL(a.created_at,''),'+9 hours'),1,7)=?`
		args = append(args, month)
	}
	q += ` ORDER BY a.created_at DESC`
	rs, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []AbsenceRow
	for rs.Next() {
		var r AbsenceRow
		if err := rs.Scan(&r.AbsenceID, &r.UserID, &r.Name, &r.OrgID, &r.LeaveReqID, &r.Minutes, &r.Kind, &r.Note, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.DaysLabel = leavecalc.FormatDays(r.Minutes)
		out = append(out, r)
	}
	return out, rs.Err()
}

type HistoryRow struct {
	Kind, Key, Days, On, Note, Source string
}

func (s *Service) History(userID string) ([]HistoryRow, error) {
	rows, err := s.DB.Query(`SELECT grant_key, minutes, granted_on, expires_on, source, COALESCE(note,'')
		FROM leave_grants WHERE user_id=? ORDER BY granted_on, created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryRow
	for rows.Next() {
		var key, granted, expires, source, note string
		var min int
		if err := rows.Scan(&key, &min, &granted, &expires, &source, &note); err != nil {
			return nil, err
		}
		out = append(out, HistoryRow{
			Kind: "부여", Key: key, Days: leavecalc.FormatDays(min),
			On: granted, Note: strings.TrimSpace(note + " " + expires), Source: source,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	cons, err := s.loadCons(s.DB, userID)
	if err != nil {
		return nil, err
	}
	for _, c := range cons {
		out = append(out, HistoryRow{
			Kind: c.Kind, Key: c.GrantKey, Days: leavecalc.FormatDays(c.Minutes), Source: "consumption",
		})
	}
	return out, nil
}

type SettleRow struct {
	UserID, Name, HireDate string
	FiscalRemain           string
	HireRemain             string
}

func (s *Service) SettlementRows(orgID, asOf string) ([]SettleRow, error) {
	members, err := s.OrgMemberRows(orgID, asOf)
	if err != nil {
		return nil, err
	}
	pol := s.policyFor(s.DB, orgID)
	var out []SettleRow
	for _, m := range members {
		r := SettleRow{UserID: m.UserID, Name: m.Name, HireDate: m.HireDate, FiscalRemain: m.Snapshot.RemainLabel, HireRemain: "—"}
		if strings.TrimSpace(m.HireDate) != "" {
			bal, err := leavecalc.Evaluate(m.HireDate, asOf, pol, nil, 0)
			if err == nil && bal.Measurable {
				r.HireRemain = leavecalc.FormatDays(bal.AvailableMinutes)
			}
		}
		out = append(out, r)
	}
	return out, nil
}
