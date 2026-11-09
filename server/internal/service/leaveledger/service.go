package leaveledger

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"customer-support/internal/service/leavecalc"
)

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

type Service struct {
	DB  *sql.DB
	Now func() time.Time
}

func New(db *sql.DB) *Service {
	return &Service{DB: db}
}

func (s *Service) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) asOf() string {
	return s.now().Format("2006-01-02")
}

type Profile struct {
	UserID      string
	OrgID       string
	HireDate    string
	AutoEnabled bool
}

func (s *Service) LoadProfile(q execer, userID string) (Profile, error) {
	if q == nil {
		q = s.DB
	}
	var p Profile
	err := q.QueryRow(`SELECT user_id, COALESCE(org_id,''), COALESCE(hire_date,''), COALESCE(leave_auto_enabled,0)
		FROM employee_profiles WHERE user_id=?`, userID).Scan(&p.UserID, &p.OrgID, &p.HireDate, &p.AutoEnabled)
	if err == sql.ErrNoRows {
		return Profile{UserID: userID}, nil
	}
	return p, err
}

func (s *Service) IsRegistered(q execer, userID string) (bool, error) {
	p, err := s.LoadProfile(q, userID)
	if err != nil {
		return false, err
	}
	if p.AutoEnabled {
		return true, nil
	}
	var n int
	_ = q.QueryRow(`SELECT COUNT(*) FROM leave_grants WHERE user_id=? AND source=?`, userID, leavecalc.SourceOpening).Scan(&n)
	return n > 0, nil
}

func (s *Service) Enable(userID, orgID, hireDate string) error {
	orgID = strings.TrimSpace(orgID)
	hireDate = strings.TrimSpace(hireDate)
	if orgID == "" {
		return fmt.Errorf("조직이 없다")
	}
	_, err := s.DB.Exec(`INSERT INTO employee_profiles(user_id, org_id, hire_date, is_org_head, leave_auto_enabled)
		VALUES (?,?,?,0,1)
		ON CONFLICT(user_id) DO UPDATE SET
			org_id=excluded.org_id,
			hire_date=CASE WHEN excluded.hire_date='' THEN employee_profiles.hire_date ELSE excluded.hire_date END,
			leave_auto_enabled=1`, userID, orgID, hireDate)
	return err
}

func (s *Service) SetHireDate(userID, orgID, hireDate string) error {
	orgID = strings.TrimSpace(orgID)
	hireDate = strings.TrimSpace(hireDate)
	if orgID == "" {
		return fmt.Errorf("조직이 없다")
	}
	_, err := s.DB.Exec(`INSERT INTO employee_profiles(user_id, org_id, hire_date, is_org_head, leave_auto_enabled)
		VALUES (?,?,?,0,0)
		ON CONFLICT(user_id) DO UPDATE SET
			org_id=excluded.org_id,
			hire_date=?`, userID, orgID, hireDate, hireDate)
	return err
}

func (s *Service) SetAutoEnabled(userID, orgID string, on bool) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("조직이 없다")
	}
	v := 0
	if on {
		v = 1
	}
	_, err := s.DB.Exec(`INSERT INTO employee_profiles(user_id, org_id, is_org_head, leave_auto_enabled)
		VALUES (?,?,0,?)
		ON CONFLICT(user_id) DO UPDATE SET org_id=excluded.org_id, leave_auto_enabled=?`, userID, orgID, v, v)
	return err
}

func (s *Service) Adjust(userID, actorID, note, carryApprover string, minutes int, expiresOn string) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return fmt.Errorf("사유를 입력하세요")
	}
	if minutes == 0 {
		return fmt.Errorf("조정 분(일수)이 없다")
	}
	asOf := s.asOf()
	year := s.now().Year()
	if expiresOn == "" {
		expiresOn = fmt.Sprintf("%d-12-31", year)
	}
	fyEnd := fmt.Sprintf("%d-12-31", year)
	if expiresOn > fyEnd {
		carryApprover = strings.TrimSpace(carryApprover)
		if carryApprover == "" {
			return fmt.Errorf("예외 이월은 승인자 이름을 입력하세요")
		}
		note = note + " 이월승인자:" + carryApprover
	}
	if minutes > 0 {
		id := "LG-" + strings.ReplaceAll(uuid.NewString(), "-", "")
		key := "ADJ:" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
		_, err := s.DB.Exec(`INSERT INTO leave_grants(grant_id, user_id, grant_key, minutes, granted_on, expires_on, source, note, created_by)
			VALUES (?,?,?,?,?,?,?,?,?)`, id, userID, key, minutes, asOf, expiresOn, leavecalc.SourceManual, note, actorID)
		return err
	}
	need := -minutes
	if err := s.EnsureFiscalGrants(s.DB, userID, asOf); err != nil {
		return err
	}
	return s.ConsumeApproved(s.DB, userID, "", asOf, need)
}

func (s *Service) AddOpening(userID, grantedOn, expiresOn string, minutes int, seq string) error {
	return s.AddOpeningOn(s.DB, userID, grantedOn, expiresOn, minutes, seq, "기초 잔액")
}

func (s *Service) AddOpeningOn(q execer, userID, grantedOn, expiresOn string, minutes int, seq, note string) error {
	if q == nil {
		q = s.DB
	}
	if minutes <= 0 {
		return fmt.Errorf("기초 잔액이 없다")
	}
	seq = strings.TrimSpace(seq)
	if seq == "" {
		seq = "1"
	}
	key := "OPEN:" + userID + ":" + seq
	id := "LG-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if strings.TrimSpace(note) == "" {
		note = "기초 잔액"
	}
	_, err := q.Exec(`INSERT OR IGNORE INTO leave_grants(grant_id, user_id, grant_key, minutes, granted_on, expires_on, source, note)
		VALUES (?,?,?,?,?,?,?,?)`, id, userID, key, minutes, grantedOn, expiresOn, leavecalc.SourceOpening, note)
	return err
}

func (s *Service) FillHireIfEmpty(q execer, userID, orgID, hireDate string) (conflict bool, existing string, err error) {
	if q == nil {
		q = s.DB
	}
	hireDate = strings.TrimSpace(hireDate)
	orgID = strings.TrimSpace(orgID)
	if hireDate == "" {
		return false, "", nil
	}
	p, err := s.LoadProfile(q, userID)
	if err != nil {
		return false, "", err
	}
	if strings.TrimSpace(p.HireDate) != "" && p.HireDate != hireDate {
		return true, p.HireDate, nil
	}
	if orgID == "" {
		orgID = p.OrgID
	}
	if orgID == "" {
		return false, p.HireDate, fmt.Errorf("조직이 없다")
	}
	if p.HireDate == hireDate {
		return false, p.HireDate, nil
	}
	_, err = q.Exec(`INSERT INTO employee_profiles(user_id, org_id, hire_date, is_org_head, leave_auto_enabled)
		VALUES (?,?,?,0,0)
		ON CONFLICT(user_id) DO UPDATE SET
			org_id=CASE WHEN excluded.org_id='' THEN employee_profiles.org_id ELSE excluded.org_id END,
			hire_date=CASE WHEN employee_profiles.hire_date IS NULL OR employee_profiles.hire_date='' THEN excluded.hire_date ELSE employee_profiles.hire_date END`,
		userID, orgID, hireDate)
	return false, "", err
}

func (s *Service) HasConsReq(q execer, reqID string) bool {
	if q == nil {
		q = s.DB
	}
	var n int
	_ = q.QueryRow(`SELECT COUNT(*) FROM leave_consumptions WHERE leave_req_id=?`, reqID).Scan(&n)
	return n > 0
}

func (s *Service) policyFor(q execer, orgID string) leavecalc.Policy {
	p := leavecalc.DefaultPolicy()
	var w int
	var start string
	_ = q.QueryRow(`SELECT COALESCE(work_minutes_per_day,0), COALESCE(ledger_start_date,'') FROM leave_policies WHERE org_id=?`, orgID).Scan(&w, &start)
	if w > 0 {
		p.WorkMinutesPerDay = w
	}
	p.LedgerStartDate = strings.TrimSpace(start)
	return p
}

func (s *Service) EnsureFiscalGrants(q execer, userID, asOf string) error {
	if q == nil {
		q = s.DB
	}
	if asOf == "" {
		asOf = s.asOf()
	}
	p, err := s.LoadProfile(q, userID)
	if err != nil {
		return err
	}
	if !p.AutoEnabled {
		return nil
	}
	if strings.TrimSpace(p.HireDate) == "" {
		return nil
	}
	pol := s.policyFor(q, p.OrgID)
	gr, err := leavecalc.ComputeFiscalGrants(p.HireDate, asOf, pol)
	if err != nil {
		return err
	}
	for _, g := range gr.Grants {
		id := "LG-" + userID + "-" + strings.ReplaceAll(g.GrantKey, ":", "")
		if _, err := q.Exec(`INSERT OR IGNORE INTO leave_grants(grant_id, user_id, grant_key, minutes, granted_on, expires_on, source)
			VALUES (?,?,?,?,?,?,?)`, id, userID, g.GrantKey, g.Minutes, g.GrantedOn, g.ExpiresOn, g.Source); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) loadGrants(q execer, userID string) ([]leavecalc.Grant, map[string]string, error) {
	rows, err := q.Query(`SELECT grant_id, grant_key, minutes, granted_on, expires_on, source FROM leave_grants WHERE user_id=?`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []leavecalc.Grant
	ids := map[string]string{}
	for rows.Next() {
		var g leavecalc.Grant
		var id string
		if err := rows.Scan(&id, &g.GrantKey, &g.Minutes, &g.GrantedOn, &g.ExpiresOn, &g.Source); err != nil {
			return nil, nil, err
		}
		out = append(out, g)
		ids[g.GrantKey] = id
	}
	return out, ids, rows.Err()
}

func (s *Service) loadCons(q execer, userID string) ([]leavecalc.Consumption, error) {
	rows, err := q.Query(`SELECT g.grant_key, c.minutes, c.kind FROM leave_consumptions c
		JOIN leave_grants g ON g.grant_id=c.grant_id WHERE c.user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []leavecalc.Consumption
	for rows.Next() {
		var c leavecalc.Consumption
		if err := rows.Scan(&c.GrantKey, &c.Minutes, &c.Kind); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Service) pendingMinutes(q execer, userID, exceptReq string) (int, error) {
	var n int
	err := q.QueryRow(`SELECT COALESCE(SUM(deduct_minutes),0) FROM leave_requests
		WHERE user_id=? AND status='pending' AND leave_req_id<>?`, userID, exceptReq).Scan(&n)
	return n, err
}

func (s *Service) Balance(userID, asOf string) (leavecalc.Balance, error) {
	return s.BalanceOn(s.DB, userID, asOf)
}

func (s *Service) BalanceOn(q execer, userID, asOf string) (leavecalc.Balance, error) {
	if asOf == "" {
		asOf = s.asOf()
	}
	ok, err := s.IsRegistered(q, userID)
	if err != nil {
		return leavecalc.Balance{}, err
	}
	if !ok {
		return leavecalc.Balance{Measurable: false}, nil
	}
	if err := s.EnsureFiscalGrants(q, userID, asOf); err != nil {
		return leavecalc.Balance{}, err
	}
	grants, _, err := s.loadGrants(q, userID)
	if err != nil {
		return leavecalc.Balance{}, err
	}
	if len(grants) == 0 {
		return leavecalc.Balance{Measurable: false}, nil
	}
	cons, err := s.loadCons(q, userID)
	if err != nil {
		return leavecalc.Balance{}, err
	}
	pending, err := s.pendingMinutes(q, userID, "")
	if err != nil {
		return leavecalc.Balance{}, err
	}
	return leavecalc.EvaluateLoaded(grants, cons, pending, asOf)
}

// PlanDeduction 신청 시 차감·결근 분. 미등록·비차감 유형은 0.
func (s *Service) PlanDeduction(q execer, userID, typeKey string, need int, asOf string, consent bool) (deduct, absence int, err error) {
	if !leavecalc.DeductsBalance(typeKey) {
		return 0, 0, nil
	}
	ok, err := s.IsRegistered(q, userID)
	if err != nil || !ok {
		return 0, 0, err
	}
	if asOf == "" {
		asOf = s.asOf()
	}
	if err := s.EnsureFiscalGrants(q, userID, asOf); err != nil {
		return 0, 0, err
	}
	bal, err := s.BalanceOn(q, userID, asOf)
	if err != nil {
		return 0, 0, err
	}
	if !bal.Measurable {
		return 0, 0, nil
	}
	deduct, absence = leavecalc.SplitNeed(need, bal.AvailableMinutes)
	if absence > 0 && !consent {
		return 0, 0, leavecalc.ErrAbsenceConsent
	}
	return deduct, absence, nil
}

func (s *Service) ConsumeApproved(q execer, userID, leaveReqID, asOf string, deductMin int) error {
	if deductMin <= 0 {
		return nil
	}
	if asOf == "" {
		asOf = s.asOf()
	}
	if err := s.EnsureFiscalGrants(q, userID, asOf); err != nil {
		return err
	}
	grants, ids, err := s.loadGrants(q, userID)
	if err != nil {
		return err
	}
	cons, err := s.loadCons(q, userID)
	if err != nil {
		return err
	}
	used, left, err := leavecalc.ConsumeFIFOPartial(grants, cons, deductMin, asOf)
	if err != nil {
		return err
	}
	if left > 0 {
		return fmt.Errorf("%w: %d분", leavecalc.ErrInsufficientBalance, left)
	}
	for _, u := range used {
		gid := ids[u.GrantKey]
		if gid == "" {
			return fmt.Errorf("부여분이 없다: %s", u.GrantKey)
		}
		cid := "LC-" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := q.Exec(`INSERT INTO leave_consumptions(cons_id, user_id, grant_id, leave_req_id, minutes, kind)
			VALUES (?,?,?,?,?,?)`, cid, userID, gid, leaveReqID, u.Minutes, u.Kind); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ConvertAbsence(q execer, userID, leaveReqID string, minutes int, note string) error {
	if minutes <= 0 {
		return nil
	}
	id := "LA-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err := q.Exec(`INSERT INTO leave_absence_records(absence_id, user_id, leave_req_id, minutes, kind, note)
		VALUES (?,?,?,?,?,?)`, id, userID, leaveReqID, minutes, leavecalc.KindUse, note)
	return err
}

func (s *Service) RestoreApproved(q execer, userID, leaveReqID string) error {
	rows, err := q.Query(`SELECT g.grant_key, c.grant_id, c.minutes FROM leave_consumptions c
		JOIN leave_grants g ON g.grant_id=c.grant_id
		WHERE c.leave_req_id=? AND c.kind=?`, leaveReqID, leavecalc.KindUse)
	if err != nil {
		return err
	}
	type row struct {
		key, gid string
		min      int
	}
	var uses []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.key, &r.gid, &r.min); err != nil {
			rows.Close()
			return err
		}
		uses = append(uses, r)
	}
	rows.Close()
	var already int
	_ = q.QueryRow(`SELECT COUNT(*) FROM leave_consumptions WHERE leave_req_id=? AND kind=?`, leaveReqID, leavecalc.KindRestore).Scan(&already)
	if already == 0 {
		for _, u := range uses {
			cid := "LC-" + strings.ReplaceAll(uuid.NewString(), "-", "")
			if _, err := q.Exec(`INSERT INTO leave_consumptions(cons_id, user_id, grant_id, leave_req_id, minutes, kind)
				VALUES (?,?,?,?,?,?)`, cid, userID, u.gid, leaveReqID, -u.min, leavecalc.KindRestore); err != nil {
				return err
			}
		}
	}
	var absUsed, absRest int
	_ = q.QueryRow(`SELECT COALESCE(SUM(CASE WHEN kind=? THEN minutes ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN kind=? THEN minutes ELSE 0 END),0)
		FROM leave_absence_records WHERE leave_req_id=?`, leavecalc.KindUse, leavecalc.KindRestore, leaveReqID).Scan(&absUsed, &absRest)
	net := absUsed + absRest
	if net > 0 {
		id := "LA-" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := q.Exec(`INSERT INTO leave_absence_records(absence_id, user_id, leave_req_id, minutes, kind, note)
			VALUES (?,?,?,?,?,?)`, id, userID, leaveReqID, -net, leavecalc.KindRestore, "취소 상쇄"); err != nil {
			return err
		}
	}
	_, err = q.Exec(`DELETE FROM staff_leaves WHERE source_request_id=?`, leaveReqID)
	return err
}

func (s *Service) ExpireAsOf(asOf string) error {
	if asOf == "" {
		asOf = s.asOf()
	}
	rows, err := s.DB.Query(`SELECT DISTINCT user_id FROM leave_grants`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var users []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		users = append(users, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, uid := range users {
		if err := s.expireUser(s.DB, uid, asOf); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) expireUser(q execer, userID, asOf string) error {
	grants, ids, err := s.loadGrants(q, userID)
	if err != nil {
		return err
	}
	cons, err := s.loadCons(q, userID)
	if err != nil {
		return err
	}
	exp, err := leavecalc.Expire(grants, cons, asOf)
	if err != nil {
		return err
	}
	for _, e := range exp {
		gid := ids[e.GrantKey]
		if gid == "" {
			continue
		}
		cid := "LC-" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := q.Exec(`INSERT INTO leave_consumptions(cons_id, user_id, grant_id, leave_req_id, minutes, kind)
			VALUES (?,?,?,?,?,?)`, cid, userID, gid, nil, e.Minutes, e.Kind); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) GrantCount(userID string) int {
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM leave_grants WHERE user_id=?`, userID).Scan(&n)
	return n
}

func (s *Service) AbsenceNet(leaveReqID string) int {
	var n int
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(minutes),0) FROM leave_absence_records WHERE leave_req_id=?`, leaveReqID).Scan(&n)
	return n
}

func (s *Service) UseNet(leaveReqID string) int {
	var n int
	_ = s.DB.QueryRow(`SELECT COALESCE(SUM(minutes),0) FROM leave_consumptions WHERE leave_req_id=?`, leaveReqID).Scan(&n)
	return n
}
