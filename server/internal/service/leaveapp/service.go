package leaveapp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"customer-support/internal/hwpx"
	"customer-support/internal/model"
	"customer-support/internal/service/approval"
	"customer-support/internal/service/leavecalc"
)

type Service struct {
	DB  *sql.DB
	Eng *approval.Engine
	Now func() time.Time
}

func New(db *sql.DB, eng *approval.Engine) *Service {
	return &Service{DB: db, Eng: eng}
}

func (s *Service) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) actor(a approval.Actor) approval.Actor {
	return a
}

func (s *Service) Submit(a approval.Actor, in Input) (string, error) {
	if s == nil || s.DB == nil || s.Eng == nil {
		return "", fmt.Errorf("휴가 서비스가 없다")
	}
	if !a.CanProcess() {
		return "", fmt.Errorf("신청할 수 없다")
	}
	in.TypeKey = strings.TrimSpace(in.TypeKey)
	in.PeriodMode = strings.TrimSpace(in.PeriodMode)
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" {
		return "", fmt.Errorf("사유를 입력하세요")
	}
	td, ok := leavecalc.TypeByKey(in.TypeKey)
	if !ok || in.TypeKey == leavecalc.TypeSubstitute {
		return "", fmt.Errorf("구분을 선택하세요")
	}
	_ = td
	if in.PeriodMode == "" {
		in.PeriodMode = defaultPeriod(in.TypeKey)
	}
	if err := s.validateInput(in); err != nil {
		return "", err
	}
	head, err := s.Eng.OrgHead(a.OrgID)
	if err != nil || head == "" {
		return "", fmt.Errorf("팀장이 지정되지 않은 조직은 상신할 수 없다")
	}
	p, err := s.Eng.GetPolicy(a.OrgID)
	if err != nil {
		return "", err
	}
	if approval.RequireLeaveDeputy(p, head == a.UserID) && strings.TrimSpace(in.ApproverDeputyUserID) == "" {
		return "", fmt.Errorf("대리 결재자를 지정하세요")
	}
	if strings.TrimSpace(in.ApproverDeputyUserID) == a.UserID {
		return "", fmt.Errorf("본인은 대리 결재자가 될 수 없다")
	}
	holidays, err := s.holidayDates()
	if err != nil {
		return "", err
	}
	days, minutes, err := computeDays(in, holidays)
	if err != nil {
		return "", err
	}
	if err := s.assertNoOverlap(a.UserID, days); err != nil {
		return "", err
	}
	prefix := s.docPrefix(a.OrgID)
	day := s.now()
	scope := prefix + ":근태"
	seqDate := day.Format("060102")
	if _, err := s.DB.Exec(`INSERT INTO doc_sequences(scope, seq_date, last_no) VALUES(?,?,1)
		ON CONFLICT(scope, seq_date) DO UPDATE SET last_no=last_no+1`, scope, seqDate); err != nil {
		return "", err
	}
	var seq int
	if err := s.DB.QueryRow(`SELECT last_no FROM doc_sequences WHERE scope=? AND seq_date=?`, scope, seqDate).Scan(&seq); err != nil {
		return "", err
	}
	docNo := leavecalc.FormatDocNo(prefix, day, seq)
	id := "LR-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	phone := strings.TrimSpace(in.Phone)
	if phone == "" {
		phone = s.profilePhone(a.UserID)
	}
	_, err = s.DB.Exec(`INSERT INTO leave_requests(
		leave_req_id, org_id, user_id, doc_no, type_key, type_note, period_mode,
		start_date, end_date, start_time, end_time, half_part, work_minutes,
		swap_date, swap_state, reason, destination, contact_phone, deputy_user_id,
		approver_deputy_user_id, status)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?, 'draft')`,
		id, a.OrgID, a.UserID, docNo, in.TypeKey, strings.TrimSpace(in.TypeNote), in.PeriodMode,
		in.StartDate, in.EndDate, nullEmpty(in.StartTime), nullEmpty(in.EndTime), nullEmpty(in.HalfPart), minutes,
		nullEmpty(in.SwapDate), nullEmpty(in.SwapState), in.Reason, strings.TrimSpace(in.Destination), phone,
		nullEmpty(in.DeputyUserID), nullEmpty(in.ApproverDeputyUserID))
	if err != nil {
		return "", err
	}
	for _, d := range days {
		min := minutes
		if len(days) > 1 && in.PeriodMode == hwpx.LeavePeriodDays {
			min = minutes / len(days)
		}
		if _, err := s.DB.Exec(`INSERT INTO leave_request_days(leave_req_id, leave_date, minutes) VALUES (?,?,?)`, id, d, min); err != nil {
			return "", err
		}
	}
	arID, err := s.Eng.CreateDraft(a.OrgID, approval.DocLeave, id, a.UserID, "근태허가원 "+docNo)
	if err != nil {
		return "", err
	}
	if _, err := s.DB.Exec(`UPDATE leave_requests SET approval_request_id=? WHERE leave_req_id=?`, arID, id); err != nil {
		return "", err
	}
	if err := s.Eng.Submit(s.actor(a), arID); err != nil {
		return "", err
	}
	st := "pending"
	req, _ := s.Eng.GetRequest(arID)
	if req.Status == approval.StatusApproved {
		st = "approved"
	} else if req.Status == approval.StatusInProgress {
		st = "pending"
	}
	if _, err := s.DB.Exec(`UPDATE leave_requests SET status=? WHERE leave_req_id=? AND status='draft'`, st, id); err != nil {
		return "", err
	}
	return id, nil
}

func defaultPeriod(typeKey string) string {
	switch typeKey {
	case leavecalc.TypeHalf, leavecalc.TypeEarlyLeave, leavecalc.TypeOuting, leavecalc.TypeOther:
		return hwpx.LeavePeriodHours
	default:
		return hwpx.LeavePeriodDays
	}
}

func (s *Service) validateInput(in Input) error {
	if in.StartDate == "" {
		return fmt.Errorf("기간을 입력하세요")
	}
	if in.EndDate == "" {
		in.EndDate = in.StartDate
	}
	switch in.PeriodMode {
	case hwpx.LeavePeriodHours:
		if in.TypeKey == leavecalc.TypeHalf && in.HalfPart != "am" && in.HalfPart != "pm" {
			return fmt.Errorf("반차는 오전·오후를 고르세요")
		}
		if in.TypeKey == leavecalc.TypeEarlyLeave || in.TypeKey == leavecalc.TypeOuting {
			if in.StartTime == "" || in.EndTime == "" {
				return fmt.Errorf("시작·종료 시각을 입력하세요")
			}
			if _, err := leavecalc.ClockRangeMinutes(in.StartTime, in.EndTime); err != nil {
				return err
			}
		}
	case hwpx.LeavePeriodDays:
		if in.StartDate > in.EndDate {
			return fmt.Errorf("기간이 올바르지 않습니다")
		}
	case hwpx.LeavePeriodSwap:
		if strings.TrimSpace(in.SwapDate) == "" {
			return fmt.Errorf("대체근무 일자를 입력하세요")
		}
	default:
		return fmt.Errorf("기간 구분이 없습니다")
	}
	return nil
}

func computeDays(in Input, holidays []string) (days []string, minutes int, err error) {
	policy := leavecalc.Policy{WorkMinutesPerDay: leavecalc.DefaultWorkMinutesPerDay}
	switch in.PeriodMode {
	case hwpx.LeavePeriodHours:
		days = []string{in.StartDate}
		clock := 0
		if in.TypeKey == leavecalc.TypeHalf {
			clock = leavecalc.DefaultHalfDayMinutes
		} else if in.StartTime != "" && in.EndTime != "" {
			clock, err = leavecalc.ClockRangeMinutes(in.StartTime, in.EndTime)
			if err != nil {
				return nil, 0, err
			}
		}
		minutes, err = leavecalc.DeductMinutes(in.TypeKey, 1, clock, policy)
		return days, minutes, err
	case hwpx.LeavePeriodSwap:
		days = []string{in.StartDate}
		return days, 0, nil
	default:
		days, err = leavecalc.BusinessDatesISO(in.StartDate, in.EndDate, holidays)
		if err != nil {
			return nil, 0, err
		}
		if len(days) == 0 {
			return nil, 0, fmt.Errorf("영업일이 없습니다")
		}
		minutes, err = leavecalc.DeductMinutes(in.TypeKey, len(days), 0, policy)
		return days, minutes, err
	}
}

func (s *Service) assertNoOverlap(userID string, days []string) error {
	for _, d := range days {
		var n int
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM leave_request_days d
			JOIN leave_requests r ON r.leave_req_id=d.leave_req_id
			WHERE r.user_id=? AND r.status IN ('pending','approved') AND d.leave_date=?`, userID, d).Scan(&n)
		if n > 0 {
			return fmt.Errorf("%s 에 이미 신청이 있습니다", d)
		}
		_ = s.DB.QueryRow(`SELECT COUNT(*) FROM staff_leaves WHERE user_id=? AND leave_date=?`, userID, d).Scan(&n)
		if n > 0 {
			return fmt.Errorf("%s 에 이미 연차가 있습니다", d)
		}
	}
	return nil
}

func (s *Service) holidayDates() ([]string, error) {
	rows, err := s.DB.Query(`SELECT holiday_date FROM holidays`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Service) docPrefix(orgID string) string {
	var p string
	_ = s.DB.QueryRow(`SELECT doc_prefix FROM leave_policies WHERE org_id=?`, orgID).Scan(&p)
	if strings.TrimSpace(p) == "" {
		return leavecalc.DefaultDocPrefix
	}
	return p
}

func (s *Service) profilePhone(userID string) string {
	var p string
	_ = s.DB.QueryRow(`SELECT COALESCE(phone,'') FROM employee_profiles WHERE user_id=?`, userID).Scan(&p)
	return strings.TrimSpace(p)
}

func (s *Service) Get(id string) (*Request, error) {
	row := s.DB.QueryRow(`SELECT r.leave_req_id, r.org_id, r.user_id, r.doc_no, r.type_key, COALESCE(r.type_note,''),
		r.period_mode, r.start_date, r.end_date, COALESCE(r.start_time,''), COALESCE(r.end_time,''), COALESCE(r.half_part,''),
		r.work_minutes, COALESCE(r.swap_date,''), COALESCE(r.swap_state,''), r.reason, COALESCE(r.destination,''),
		COALESCE(r.contact_phone,''), COALESCE(r.deputy_user_id,''), COALESCE(du.full_name,''),
		COALESCE(r.approver_deputy_user_id,''), COALESCE(r.approval_request_id,''), r.status, IFNULL(r.created_at,''),
		COALESCE(NULLIF(TRIM(u.full_name),''), u.username, '')
		FROM leave_requests r
		LEFT JOIN users u ON u.user_id=r.user_id
		LEFT JOIN users du ON du.user_id=r.deputy_user_id
		WHERE r.leave_req_id=?`, id)
	var r Request
	err := row.Scan(&r.LeaveReqID, &r.OrgID, &r.UserID, &r.DocNo, &r.TypeKey, &r.TypeNote, &r.PeriodMode,
		&r.StartDate, &r.EndDate, &r.StartTime, &r.EndTime, &r.HalfPart, &r.WorkMinutes, &r.SwapDate, &r.SwapState,
		&r.Reason, &r.Destination, &r.Phone, &r.DeputyUserID, &r.DeputyName, &r.ApproverDeputyUserID,
		&r.ApprovalRequestID, &r.Status, &r.CreatedAt, &r.AuthorName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) ListMine(orgID, userID string) ([]Request, error) {
	rows, err := s.DB.Query(`SELECT r.leave_req_id, r.org_id, r.user_id, r.doc_no, r.type_key, COALESCE(r.type_note,''),
		r.period_mode, r.start_date, r.end_date, COALESCE(r.start_time,''), COALESCE(r.end_time,''), COALESCE(r.half_part,''),
		r.work_minutes, COALESCE(r.swap_date,''), COALESCE(r.swap_state,''), r.reason, COALESCE(r.destination,''),
		COALESCE(r.contact_phone,''), COALESCE(r.deputy_user_id,''), COALESCE(du.full_name,''),
		COALESCE(r.approver_deputy_user_id,''), COALESCE(r.approval_request_id,''), r.status, IFNULL(r.created_at,''),
		COALESCE(NULLIF(TRIM(u.full_name),''), u.username, '')
		FROM leave_requests r
		LEFT JOIN users u ON u.user_id=r.user_id
		LEFT JOIN users du ON du.user_id=r.deputy_user_id
		WHERE r.org_id=? AND r.user_id=?
		ORDER BY r.created_at DESC`, orgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		var r Request
		if err := rows.Scan(&r.LeaveReqID, &r.OrgID, &r.UserID, &r.DocNo, &r.TypeKey, &r.TypeNote, &r.PeriodMode,
			&r.StartDate, &r.EndDate, &r.StartTime, &r.EndTime, &r.HalfPart, &r.WorkMinutes, &r.SwapDate, &r.SwapState,
			&r.Reason, &r.Destination, &r.Phone, &r.DeputyUserID, &r.DeputyName, &r.ApproverDeputyUserID,
			&r.ApprovalRequestID, &r.Status, &r.CreatedAt, &r.AuthorName); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) OrgMembers(orgID string) ([]model.User, error) {
	rows, err := s.DB.Query(`SELECT user_id, COALESCE(username,''), COALESCE(full_name,'') FROM users
		WHERE org_id=? AND COALESCE(is_active,1)=1 ORDER BY full_name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.UserID, &u.Username, &u.FullName); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Service) SyncStatus(leaveID, status string) error {
	_, err := s.DB.Exec(`UPDATE leave_requests SET status=? WHERE leave_req_id=?`, status, leaveID)
	return err
}

func (s *Service) SnapshotJSON(leaveID string) (title, snap string, err error) {
	req, err := s.Get(leaveID)
	if err != nil || req == nil {
		return "", "", fmt.Errorf("신청서가 없다")
	}
	sn := snapshotFrom(req, s.now())
	b, err := json.Marshal(sn)
	if err != nil {
		return "", "", err
	}
	return "근태허가원 " + req.DocNo, string(b), nil
}

func snapshotFrom(req *Request, now time.Time) Snapshot {
	written := fmt.Sprintf("%d년 %02d월 %02d일", now.Year(), int(now.Month()), now.Day())
	stamp := fmt.Sprintf("%02d/%02d", int(now.Month()), now.Day())
	sn := Snapshot{
		LeaveReqID: req.LeaveReqID, DocNo: req.DocNo, WrittenOn: written, Author: req.AuthorName,
		TypeKey: req.TypeKey, TypeNote: req.TypeNote, PeriodMode: req.PeriodMode,
		Reason: req.Reason, Destination: req.Destination, Phone: req.Phone, Deputy: req.DeputyName,
		AuthorStampDate: stamp, StartDate: req.StartDate, EndDate: req.EndDate, HalfPart: req.HalfPart,
	}
	switch req.PeriodMode {
	case hwpx.LeavePeriodHours:
		t, _ := time.Parse("2006-01-02", req.StartDate)
		sn.HourYear, sn.HourMonth, sn.HourDay = t.Format("2006"), t.Format("01"), t.Format("02")
		sn.HourStart, sn.HourEnd = req.StartTime, req.EndTime
		if req.TypeKey == leavecalc.TypeHalf {
			if req.HalfPart == "pm" {
				sn.HourStart, sn.HourEnd = "14:00", "18:00"
			} else {
				sn.HourStart, sn.HourEnd = "09:00", "13:00"
			}
			sn.HourCount = "4"
		} else if req.StartTime != "" && req.EndTime != "" {
			if m, err := leavecalc.ClockRangeMinutes(req.StartTime, req.EndTime); err == nil {
				sn.HourCount = fmt.Sprintf("%d", (m+30)/60)
				if m%60 != 0 {
					sn.HourCount = fmt.Sprintf("%.1f", float64(m)/60)
				}
			}
		}
	case hwpx.LeavePeriodDays:
		st, _ := time.Parse("2006-01-02", req.StartDate)
		en, _ := time.Parse("2006-01-02", req.EndDate)
		sn.DayStart = fmt.Sprintf("%d년 %02d월 %02d일", st.Year(), int(st.Month()), st.Day())
		sn.DayEnd = fmt.Sprintf("%02d월 %02d일", int(en.Month()), en.Day())
		n := 1
		if req.WorkMinutes > 0 {
			n = req.WorkMinutes / leavecalc.DefaultWorkMinutesPerDay
			if n < 1 {
				n = 1
			}
		}
		sn.DayCount = fmt.Sprintf("%d", n)
	case hwpx.LeavePeriodSwap:
		sn.SwapDate = req.SwapDate
		sn.SwapWorked = req.SwapState == "worked"
		sn.SwapScheduled = req.SwapState == "scheduled"
	}
	return sn
}

func PermitInput(sn Snapshot, approveStamp, approveMark string) hwpx.LeavePermitInput {
	return hwpx.LeavePermitInput{
		DocNo: sn.DocNo, WrittenOn: sn.WrittenOn, Author: sn.Author,
		TypeKey: sn.TypeKey, TypeNote: sn.TypeNote, PeriodMode: sn.PeriodMode,
		HourYear: sn.HourYear, HourMonth: sn.HourMonth, HourDay: sn.HourDay,
		HourStart: sn.HourStart, HourEnd: sn.HourEnd, HourCount: sn.HourCount,
		DayStart: sn.DayStart, DayEnd: sn.DayEnd, DayCount: sn.DayCount,
		SwapDate: sn.SwapDate, SwapWorked: sn.SwapWorked, SwapScheduled: sn.SwapScheduled,
		Reason: sn.Reason, Destination: sn.Destination, Phone: sn.Phone, Deputy: sn.Deputy,
		AuthorStampDate: sn.AuthorStampDate, ApproveStampDate: approveStamp, ApproveMark: approveMark,
	}
}

func ParseSnapshot(raw string) (Snapshot, error) {
	var sn Snapshot
	err := json.Unmarshal([]byte(raw), &sn)
	return sn, err
}

func nullEmpty(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func staffKind(typeKey, halfPart string) (string, bool) {
	switch typeKey {
	case leavecalc.TypeAnnual, leavecalc.TypeMonthly, leavecalc.TypeVacation:
		return model.LeaveKindAnnual, true
	case leavecalc.TypeHalf:
		if halfPart == "pm" {
			return model.LeaveKindHalfPM, true
		}
		return model.LeaveKindHalfAM, true
	default:
		return "", false
	}
}
