package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

type ASRepo struct {
	db *sql.DB
}

func NewASRepo(db *sql.DB) *ASRepo {
	return &ASRepo{db: db}
}

// touchReceipt §25.2 AS 접수 수정 이력을 남긴다.
func (r *ASRepo) touchReceipt(asID string, fn func() error) error {
	label := asID
	var num string
	if err := r.db.QueryRow(`SELECT COALESCE(as_number,'') FROM as_receipts WHERE as_id=?`, asID).Scan(&num); err == nil && num != "" {
		label = num
	}
	return touchUpdate(r.db, "as_receipts", "as_id", asID, label, fn)
}

// List AS 목록 조회
// status: overdue / today / visit_past|visit_today|visit_upcoming / open|in_progress / done|completed / completed_today / 일반상태
// mineUserID / mineKeys: 본인 배정 필터 (user_id 우선, 이름·아이디 보조)
func (r *ASRepo) List(status, search string, page, pageSize int) ([]model.ASListItem, int, error) {
	return r.ListFiltered(status, search, "", nil, "", "", page, pageSize)
}

const visitDateToday = `date('now','localtime')`

// daysElapsedSQL 목록 경과일수.
// 완료·종료: 완료일 − 접수일. 미완료: 오늘 − 접수일. (음수는 0)
const daysElapsedSQL = `MAX(0, CAST((
		CASE WHEN ar.status IN ` + model.SQLStatusFullyClosed + `
			THEN julianday(date(COALESCE(ar.complete_datetime, ar.updated_at)))
			ELSE julianday(` + visitDateToday + `)
		END
	) - julianday(date(ar.receipt_datetime)) AS INTEGER))`

// visitAlreadyDone 예정일 당일 이후에 처리(방문) 이력이 남아 있는지.
// 일정을 미루거나 앞당겨 다녀온 뒤 예정일만 옛 날짜로 남아 있는 건을
// '예정일 경과'로 세지 않기 위해 쓴다.
func visitAlreadyDone(alias string) string {
	return `EXISTS (SELECT 1 FROM as_processes p
		WHERE p.as_id = ` + alias + `as_id
		  AND date(p.process_datetime) >= date(` + alias + `visit_scheduled_date))`
}

func (r *ASRepo) ListFiltered(status, search, mineUserID string, mineKeys []string, sort, dir string, page, pageSize int) ([]model.ASListItem, int, error) {
	offset := (page - 1) * pageSize

	baseQuery := `
		SELECT ar.as_id, ar.as_number, ar.receipt_datetime,
		       c.org_name, COALESCE(a.product_name,'') AS product_name,
		       ar.symptom, ar.urgency, ar.status, COALESCE(ar.assigned_to,''),
		       ` + daysElapsedSQL + ` AS days_elapsed,
		       COALESCE(ar.visit_scheduled_date,''),
		       CASE WHEN COALESCE(ar.visit_scheduled_date,'') = '' THEN 0
		            ELSE CAST(julianday(` + visitDateToday + `) - julianday(ar.visit_scheduled_date) AS INTEGER) END AS visit_days,
		       CASE WHEN COALESCE(ar.visit_scheduled_date,'') = '' THEN 0
		            WHEN ` + visitAlreadyDone("ar.") + ` THEN 1 ELSE 0 END AS visit_done,
		       COALESCE(ar.is_reopen,0) AS is_reopen,
		       COALESCE(ar.parent_as_id,'') AS parent_as_id,
		       CASE WHEN TRIM(COALESCE(ar.receipt_group_id,'')) = '' THEN 0
		            ELSE (SELECT COUNT(*) FROM as_receipts g WHERE g.receipt_group_id = ar.receipt_group_id)
		       END AS group_size,
		       COALESCE(ar.urgency_reason,''), COALESCE(ar.urgency_reason_note,'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE 1=1`

	countQuery := `SELECT COUNT(*) FROM as_receipts ar JOIN customers c ON c.customer_id=ar.customer_id WHERE 1=1`
	args := []interface{}{}
	countArgs := []interface{}{}
	visitBucket := false

	switch status {
	case "overdue":
		cond := ` AND ar.status IN ('received','assigned','in_progress')
		          AND julianday('now') - julianday(ar.receipt_datetime) > 3`
		baseQuery += cond
		countQuery += cond
	case "today":
		cond := ` AND date(ar.receipt_datetime) = date('now')`
		baseQuery += cond
		countQuery += cond
	case "visit_past":
		cond := ` AND ar.status IN ` + model.SQLStatusOpsInProgress + `
		          AND COALESCE(ar.visit_scheduled_date,'') != ''
		          AND date(ar.visit_scheduled_date) < ` + visitDateToday + `
		          AND NOT ` + visitAlreadyDone("ar.")
		baseQuery += cond
		countQuery += cond
		visitBucket = true
	case "visit_done_open":
		// 예정일은 지났지만 이미 다녀온 건 — 다음 일정을 다시 잡아야 하는 상태
		cond := ` AND ar.status IN ` + model.SQLStatusOpsInProgress + `
		          AND COALESCE(ar.visit_scheduled_date,'') != ''
		          AND date(ar.visit_scheduled_date) < ` + visitDateToday + `
		          AND ` + visitAlreadyDone("ar.")
		baseQuery += cond
		countQuery += cond
		visitBucket = true
	case "visit_today":
		cond := ` AND ar.status IN ` + model.SQLStatusOpsInProgress + `
		          AND COALESCE(ar.visit_scheduled_date,'') != ''
		          AND date(ar.visit_scheduled_date) = ` + visitDateToday
		baseQuery += cond
		countQuery += cond
		visitBucket = true
	case "visit_upcoming":
		cond := ` AND ar.status IN ` + model.SQLStatusOpsInProgress + `
		          AND COALESCE(ar.visit_scheduled_date,'') != ''
		          AND date(ar.visit_scheduled_date) > ` + visitDateToday
		baseQuery += cond
		countQuery += cond
		visitBucket = true
	case "transfer_overdue":
		// 이관 + (회신확인일 없음 또는 경과) — 담당자 독촉용
		cond := ` AND ar.status = 'transfer'
		          AND (COALESCE(ar.visit_scheduled_date,'') = ''
		               OR date(ar.visit_scheduled_date) < ` + visitDateToday + `)`
		baseQuery += cond
		countQuery += cond
		visitBucket = true
	case "open":
		cond := ` AND ar.status IN ` + model.SQLStatusOpenIncomplete
		baseQuery += cond
		countQuery += cond
	case "in_progress":
		cond := ` AND ar.status IN ` + model.SQLStatusOpsInProgress
		baseQuery += cond
		countQuery += cond
	case "partial_complete":
		cond := ` AND ar.status = 'partial_complete'`
		baseQuery += cond
		countQuery += cond
	case "done", "completed":
		cond := ` AND ar.status IN ` + model.SQLStatusFullyClosed
		baseQuery += cond
		countQuery += cond
	case "completed_today":
		// 통계상 오늘 완료(부분완료 포함)
		cond := ` AND ar.status IN ` + model.SQLStatusStatsCompleted + `
		          AND date(COALESCE(ar.complete_datetime, ar.updated_at)) = date('now')`
		baseQuery += cond
		countQuery += cond
	case "week_received":
		ws, we := weekRangeDates()
		cond := ` AND date(ar.receipt_datetime) BETWEEN date(?) AND date(?)`
		baseQuery += cond
		countQuery += cond
		args = append(args, ws, we)
		countArgs = append(countArgs, ws, we)
	case "week_completed":
		ws, we := weekRangeDates()
		cond := ` AND ar.status IN ` + model.SQLStatusStatsCompleted + `
		          AND date(COALESCE(ar.complete_datetime, ar.updated_at)) BETWEEN date(?) AND date(?)`
		baseQuery += cond
		countQuery += cond
		args = append(args, ws, we)
		countArgs = append(countArgs, ws, we)
	case "week_in_progress":
		ws, we := weekRangeDates()
		cond := ` AND ar.status IN ('received','assigned','in_progress','partial_complete')
		          AND date(ar.receipt_datetime) BETWEEN date(?) AND date(?)`
		baseQuery += cond
		countQuery += cond
		args = append(args, ws, we)
		countArgs = append(countArgs, ws, we)
	case "":
	default:
		baseQuery += ` AND ar.status = ?`
		countQuery += ` AND ar.status = ?`
		args = append(args, status)
		countArgs = append(countArgs, status)
	}

	if mineCond, mineArgs := mineAssigneeCond("ar.", mineUserID, mineKeys); mineCond != "" {
		baseQuery += mineCond
		countQuery += mineCond
		args = append(args, mineArgs...)
		countArgs = append(countArgs, mineArgs...)
	}

	if search != "" {
		like := "%" + search + "%"
		baseQuery += ` AND (ar.as_number LIKE ? OR c.org_name LIKE ? OR ar.symptom LIKE ?)`
		countQuery += ` AND (ar.as_number LIKE ? OR c.org_name LIKE ? OR ar.symptom LIKE ?)`
		args = append(args, like, like, like)
		countArgs = append(countArgs, like, like, like)
	}

	orderBy := buildASOrderBy(sort, dir)
	if visitBucket && (sort == "" || sort == "receipt") {
		orderBy = "ar.visit_scheduled_date ASC, c.org_name ASC"
	}
	baseQuery += " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
	args = append(args, pageSize, offset)

	var total int
	if err := r.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.Query(baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []model.ASListItem
	for rows.Next() {
		var item model.ASListItem
		var receiptStr string
		var visitDone, isReopen, groupSize int
		if err := rows.Scan(
			&item.ASID, &item.ASNumber, &receiptStr,
			&item.OrgName, &item.ProductName,
			&item.Symptom, &item.Urgency, &item.Status, &item.AssignedTo,
			&item.DaysElapsed, &item.VisitScheduledDate, &item.VisitDaysOverdue, &visitDone, &isReopen,
			&item.ParentASID, &groupSize, &item.UrgencyReason, &item.UrgencyReasonNote,
		); err != nil {
			return nil, 0, err
		}
		item.VisitDone = visitDone == 1
		item.IsReopen = isReopen == 1
		item.GroupSize = groupSize
		item.ReceiptDatetime = parseTime(receiptStr)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func buildASOrderBy(sort, dir string) string {
	d := strings.ToUpper(strings.TrimSpace(dir))
	if d != "ASC" && d != "DESC" {
		d = "DESC"
	}
	switch sort {
	case "org_name":
		return fmt.Sprintf("c.org_name %s, ar.receipt_datetime DESC", d)
	case "days":
		return fmt.Sprintf("(%s) %s, ar.receipt_datetime DESC", daysElapsedSQL, d)
	case "visit":
		return fmt.Sprintf("ar.visit_scheduled_date %s, c.org_name ASC", d)
	case "as_number":
		return fmt.Sprintf("ar.as_number %s", d)
	case "status":
		return fmt.Sprintf("ar.status %s, ar.receipt_datetime DESC", d)
	case "assigned":
		return fmt.Sprintf("ar.assigned_to %s, ar.receipt_datetime DESC", d)
	case "receipt":
		return fmt.Sprintf("ar.receipt_datetime %s", d)
	default:
		return "ar.receipt_datetime DESC"
	}
}

func cleanAssignees(assignees []string) []string {
	out := make([]string, 0, len(assignees))
	seen := map[string]bool{}
	for _, a := range assignees {
		a = strings.TrimSpace(a)
		if a == "" || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	return out
}

// mineAssigneeCond 본인 배정: assigned_user_id 또는 표시명/아이디 매칭
func mineAssigneeCond(alias, userID string, keys []string) (string, []interface{}) {
	names := cleanAssignees(keys)
	userID = strings.TrimSpace(userID)
	if userID == "" && len(names) == 0 {
		return "", nil
	}
	parts := []string{}
	args := []interface{}{}
	if userID != "" {
		parts = append(parts, fmt.Sprintf("(%sassigned_user_id!='' AND %sassigned_user_id=?)", alias, alias))
		args = append(args, userID)
	}
	if len(names) > 0 {
		ph := make([]string, len(names))
		for i, n := range names {
			ph[i] = "?"
			args = append(args, n)
		}
		parts = append(parts, fmt.Sprintf("TRIM(%sassigned_to) IN (%s)", alias, strings.Join(ph, ",")))
	}
	return ` AND (` + strings.Join(parts, " OR ") + `)`, args
}

// Stats AS 현황 통계 (완료=전체 완료+종료) — 목록/현황 화면용
func (r *ASRepo) Stats() (*model.ASStats, error) {
	return r.StatsFiltered("", nil)
}

// DashboardStats 대시보드용. Completed=오늘 완료. mine 필터 가능.
func (r *ASRepo) DashboardStats(mineUserID string, mineKeys []string) (*model.ASStats, error) {
	return r.StatsFiltered(mineUserID, mineKeys)
}

// StatsFiltered 배정 기준 통계. Completed = 오늘 완료. Week* = 월~일. Visit* = 진행중 예정일 버킷.
func (r *ASRepo) StatsFiltered(mineUserID string, mineKeys []string) (*model.ASStats, error) {
	ws, we := weekRangeDates()
	q := `
		SELECT
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN status = 'assigned' THEN 1 ELSE 0 END), 0) AS assigned,
			COALESCE(SUM(CASE WHEN status IN ` + model.SQLStatusOpsInProgress + ` THEN 1 ELSE 0 END), 0) AS in_progress,
			COALESCE(SUM(CASE WHEN status IN ` + model.SQLStatusStatsCompleted + `
			         AND date(COALESCE(complete_datetime, updated_at))=date('now') THEN 1 ELSE 0 END), 0) AS completed,
			COALESCE(SUM(CASE WHEN status IN ('received','assigned','in_progress','partial_complete')
			         AND julianday('now')-julianday(receipt_datetime) > 3 THEN 1 ELSE 0 END), 0) AS overdue,
			COALESCE(SUM(CASE WHEN date(receipt_datetime)=date('now') THEN 1 ELSE 0 END), 0) AS today,
			COALESCE(SUM(CASE WHEN date(receipt_datetime) BETWEEN date(?) AND date(?) THEN 1 ELSE 0 END), 0) AS week_recv,
			COALESCE(SUM(CASE WHEN status IN ` + model.SQLStatusStatsCompleted + `
			         AND date(COALESCE(complete_datetime, updated_at)) BETWEEN date(?) AND date(?) THEN 1 ELSE 0 END), 0) AS week_done,
			COALESCE(SUM(CASE WHEN status IN ` + model.SQLStatusOpsInProgress + `
			         AND COALESCE(visit_scheduled_date,'') != ''
			         AND date(visit_scheduled_date) < ` + visitDateToday + `
			         AND NOT ` + visitAlreadyDone("as_receipts.") + ` THEN 1 ELSE 0 END), 0) AS visit_past,
			COALESCE(SUM(CASE WHEN status IN ` + model.SQLStatusOpsInProgress + `
			         AND COALESCE(visit_scheduled_date,'') != ''
			         AND date(visit_scheduled_date) < ` + visitDateToday + `
			         AND ` + visitAlreadyDone("as_receipts.") + ` THEN 1 ELSE 0 END), 0) AS visit_done_open,
			COALESCE(SUM(CASE WHEN status IN ` + model.SQLStatusOpsInProgress + `
			         AND COALESCE(visit_scheduled_date,'') != ''
			         AND date(visit_scheduled_date) = ` + visitDateToday + ` THEN 1 ELSE 0 END), 0) AS visit_today,
			COALESCE(SUM(CASE WHEN status IN ` + model.SQLStatusOpsInProgress + `
			         AND COALESCE(visit_scheduled_date,'') != ''
			         AND date(visit_scheduled_date) > ` + visitDateToday + ` THEN 1 ELSE 0 END), 0) AS visit_upcoming,
			COALESCE(SUM(CASE WHEN status = 'transfer'
			         AND (COALESCE(visit_scheduled_date,'') = ''
			              OR date(visit_scheduled_date) < ` + visitDateToday + `) THEN 1 ELSE 0 END), 0) AS transfer_overdue
		FROM as_receipts WHERE 1=1`
	args := []interface{}{ws, we, ws, we}
	if mineCond, mineArgs := mineAssigneeCond("", mineUserID, mineKeys); mineCond != "" {
		q += mineCond
		args = append(args, mineArgs...)
	}
	var stats model.ASStats
	err := r.db.QueryRow(q, args...).Scan(
		&stats.TotalReceived, &stats.Assigned, &stats.InProgress,
		&stats.Completed, &stats.Overdue, &stats.TodayReceived,
		&stats.WeekReceived, &stats.WeekCompleted,
		&stats.VisitPast, &stats.VisitDoneOpen, &stats.VisitToday, &stats.VisitUpcoming,
		&stats.TransferOverdue,
	)
	return &stats, err
}

// weekRangeDates 현재 주(월~일)의 시작·종료 날짜 YYYY-MM-DD
func weekRangeDates() (start, end string) {
	now := time.Now()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	sinceMon := (int(today.Weekday()) + 6) % 7 // Mon=0 ... Sun=6
	mon := today.AddDate(0, 0, -sinceMon)
	sun := mon.AddDate(0, 0, 6)
	return mon.Format("2006-01-02"), sun.Format("2006-01-02")
}

// weekElapsedDays 이번 주 월요일부터 오늘까지 일수(1~7)
func weekElapsedDays() int {
	now := time.Now()
	sinceMon := (int(now.Weekday()) + 6) % 7
	return sinceMon + 1
}

// WeekRangeLabel 대시보드 표시용 주간 기간
func WeekRangeLabel() (start, end string) {
	return weekRangeDates()
}

// WeekElapsedDays 월요일~오늘 경과일수(1~7)
func WeekElapsedDays() int {
	return weekElapsedDays()
}

// AssigneeDashboardStats 담당자별 주간 진행중·일평균 완료·지연
func (r *ASRepo) AssigneeDashboardStats() ([]model.AssigneeDashStats, error) {
	ws, we := weekRangeDates()
	elapsed := weekElapsedDays()
	if elapsed < 1 {
		elapsed = 1
	}
	q := `
		SELECT
			COALESCE(NULLIF(ar.assigned_user_id,''), '') AS uid,
			COALESCE(NULLIF(MAX(u.full_name), ''), NULLIF(MAX(ar.assigned_to), ''), '미배정') AS name,
			COALESCE(MAX(u.username), '') AS username,
			COALESCE(SUM(CASE WHEN ar.status IN ('received','assigned','in_progress','partial_complete')
			         AND date(ar.receipt_datetime) BETWEEN date(?) AND date(?) THEN 1 ELSE 0 END), 0) AS week_ip,
			COALESCE(SUM(CASE WHEN ar.status IN ` + model.SQLStatusStatsCompleted + `
			         AND date(COALESCE(ar.complete_datetime, ar.updated_at)) BETWEEN date(?) AND date(?) THEN 1 ELSE 0 END), 0) AS week_done,
			COALESCE(SUM(CASE WHEN ar.status IN ` + model.SQLStatusStatsCompleted + `
			         AND date(COALESCE(ar.complete_datetime, ar.updated_at))=date('now') THEN 1 ELSE 0 END), 0) AS day_done,
			COALESCE(SUM(CASE WHEN ar.status IN ('received','assigned','in_progress','partial_complete')
			         AND julianday('now')-julianday(ar.receipt_datetime) > 3 THEN 1 ELSE 0 END), 0) AS overdue
		FROM as_receipts ar
		LEFT JOIN users u ON u.user_id = ar.assigned_user_id
		WHERE COALESCE(ar.assigned_to,'') != '' OR COALESCE(ar.assigned_user_id,'') != ''
		GROUP BY COALESCE(NULLIF(ar.assigned_user_id,''), ar.assigned_to)
		HAVING week_ip > 0 OR week_done > 0 OR day_done > 0 OR overdue > 0
		ORDER BY week_ip DESC, overdue DESC, name ASC`
	rows, err := r.db.Query(q, ws, we, ws, we)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.AssigneeDashStats
	for rows.Next() {
		var s model.AssigneeDashStats
		if err := rows.Scan(&s.UserID, &s.Name, &s.Username, &s.WeekInProgress, &s.WeekCompleted, &s.DayCompleted, &s.Overdue); err != nil {
			return nil, err
		}
		s.DayAvgCompleted = float64(s.WeekCompleted) / float64(elapsed)
		items = append(items, s)
	}
	return items, rows.Err()
}

// GetByID AS 단건 조회
func (r *ASRepo) GetByID(id string) (*model.ASReceipt, error) {
	query := `
		SELECT ar.as_id, ar.as_number, ar.receipt_datetime,
		       ar.customer_id, COALESCE(ar.asset_id,''),
		       COALESCE(ar.receipt_channel,''), COALESCE(ar.requester,''),
		       COALESCE(ar.symptom,''), COALESCE(ar.urgency,'normal'),
		       COALESCE(ar.priority,'normal'), COALESCE(ar.requester_type,''),
		       COALESCE(ar.requester_name,''), COALESCE(ar.assigned_to,''),
		       COALESCE(ar.assigned_user_id,''),
		       COALESCE(ar.received_by,''),
		       COALESCE(ar.visit_scheduled_date,''),
		       COALESCE(ar.schedule_confirmed,0),
		       ar.status, COALESCE(ar.process_type,''), COALESCE(ar.work_place,''), COALESCE(ar.cause_type,''),
		       COALESCE(ar.cause_detail,''), COALESCE(ar.conclusion,''),
		       COALESCE(ar.action_taken,''), COALESCE(ar.parts_used,''),
		       ar.is_recurrence, COALESCE(ar.is_reopen,0), COALESCE(ar.replace_review,0),
		       COALESCE(ar.result_code,''), COALESCE(ar.revisit_reason,''),
		       COALESCE(ar.transfer_detail,''), COALESCE(ar.confirm_target,''), COALESCE(ar.confirm_contact,''),
		       COALESCE(ar.hold_reason,''), COALESCE(ar.hold_next_action,''),
		       COALESCE(ar.followup_action,''),
		       COALESCE(ar.customer_confirmer,''),
		       COALESCE(ar.start_datetime,''), COALESCE(ar.complete_datetime,''),
		       COALESCE(ar.confirm_datetime,''), COALESCE(ar.cancel_datetime,''),
		       COALESCE(ar.parent_as_id,''), COALESCE(ar.reopen_reason,''), COALESCE(ar.followup_note,''),
		       COALESCE(ar.receipt_group_id,''),
		       COALESCE(ar.urgency_reason,''), COALESCE(ar.urgency_reason_note,''),
		       COALESCE(ar.cause_cat1,''), COALESCE(ar.cause_cat2,''), COALESCE(ar.cause_cat3,''),
		       c.org_name, COALESCE(a.product_name,''), COALESCE(a.install_location,'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.as_id = ?`

	var as model.ASReceipt
	var receiptStr, startStr, completeStr, confirmStr, cancelStr string
	var isRecurrence, isReopen, replaceReview, scheduleConfirmed int

	err := r.db.QueryRow(query, id).Scan(
		&as.ASID, &as.ASNumber, &receiptStr,
		&as.CustomerID, &as.AssetID,
		&as.ReceiptChannel, &as.Requester,
		&as.Symptom, &as.Urgency, &as.Priority,
		&as.RequesterType, &as.RequesterName, &as.AssignedTo,
		&as.AssignedUserID,
		&as.ReceivedBy,
		&as.VisitScheduledDate,
		&scheduleConfirmed,
		&as.Status, &as.ProcessType, &as.WorkPlace, &as.CauseType,
		&as.CauseDetail, &as.Conclusion,
		&as.ActionTaken, &as.PartsUsed,
		&isRecurrence, &isReopen, &replaceReview,
		&as.ResultCode, &as.RevisitReason,
		&as.TransferDetail, &as.ConfirmTarget, &as.ConfirmContact,
		&as.HoldReason, &as.HoldNextAction,
		&as.FollowupAction, &as.CustomerConfirmer,
		&startStr, &completeStr, &confirmStr, &cancelStr,
		&as.ParentASID, &as.ReopenReason, &as.FollowupNote, &as.ReceiptGroupID,
		&as.UrgencyReason, &as.UrgencyReasonNote,
		&as.CauseCat1, &as.CauseCat2, &as.CauseCat3,
		&as.OrgName, &as.ProductName, &as.InstallLocation,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	as.ReceiptDatetime = parseTime(receiptStr)
	as.ScheduleConfirmed = scheduleConfirmed == 1
	as.IsRecurrence = isRecurrence == 1
	as.IsReopen = isReopen == 1
	as.ReplaceReview = replaceReview == 1
	if t := parseTime(startStr); !t.IsZero() {
		as.StartDatetime = &t
	}
	if t := parseTime(completeStr); !t.IsZero() {
		as.CompleteDatetime = &t
	}
	if t := parseTime(confirmStr); !t.IsZero() {
		as.ConfirmDatetime = &t
	}
	if t := parseTime(cancelStr); !t.IsZero() {
		as.CancelDatetime = &t
	}
	return &as, nil
}

// Create AS 접수 등록
func (r *ASRepo) Create(as *model.ASReceipt) error {
	if err := model.RequireAppDateYear(as.VisitScheduledDate); err != nil {
		return err
	}
	if !as.ReceiptDatetime.IsZero() && !model.AppDateYearOK(as.ReceiptDatetime) {
		return model.ErrAppDateYear
	}
	receiptAt := as.ReceiptDatetime
	if receiptAt.IsZero() {
		receiptAt = time.Now()
	}
	as.ProjectID = resolveStoredProjectID(r.db, as.CustomerID, lookupASProductText(r.db, as.AssetID, as.Symptom), model.ScopeWorkAS)
	now := time.Now().Format("2006-01-02 15:04:05")
	receiptStr := receiptAt.Format("2006-01-02 15:04:05")
	status := model.DeriveASWorkflowStatus(as.AssignedTo, as.AssignedUserID, as.ScheduleConfirmed)

	var lastErr error
	for i := 0; i < 8; i++ {
		num, err := NextASNumber(r.db, receiptAt)
		if err != nil {
			return err
		}
		as.ASNumber = num
		as.ASID = num // 신규: PK = 표시용 접수번호
		_, err = r.db.Exec(`
		INSERT INTO as_receipts (
			as_id, as_number, receipt_datetime, customer_id, asset_id,
			receipt_channel, requester, symptom, urgency, priority,
			requester_type, requester_name, assigned_to, assigned_user_id, received_by,
			visit_scheduled_date, schedule_confirmed, status,
			is_recurrence, is_reopen, parent_as_id, reopen_reason, followup_note,
			project_id, receipt_group_id, confirm_contact,
			urgency_reason, urgency_reason_note, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			as.ASID, as.ASNumber, receiptStr, as.CustomerID, nullStr(as.AssetID),
			as.ReceiptChannel, as.Requester, as.Symptom, as.Urgency, as.Priority,
			as.RequesterType, as.RequesterName, as.AssignedTo, as.AssignedUserID, as.ReceivedBy,
			nullStr(as.VisitScheduledDate), boolToInt(as.ScheduleConfirmed),
			status,
			boolToInt(as.IsRecurrence), boolToInt(as.IsReopen),
			nullStr(as.ParentASID), nullStr(as.ReopenReason), nullStr(as.FollowupNote),
			nullStr(as.ProjectID), nullStr(as.ReceiptGroupID), nullStr(as.ConfirmContact),
			nullStr(as.UrgencyReason), nullStr(as.UrgencyReasonNote), now, now,
		)
		if err == nil {
			// §25.2 AS 접수 등록은 이력 미기록(○)
			reindexASSearch(r.db, as.ASID)
			return nil
		}
		lastErr = err
		if !isASNumberTaken(err) {
			return err
		}
	}
	return lastErr
}

func isASNumberTaken(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if !strings.Contains(s, "unique") {
		return false
	}
	return strings.Contains(s, "as_number") || strings.Contains(s, "as_id") || strings.Contains(s, "as_receipts")
}

// ListReopens 이 접수를 원본으로 다시 접수된 건들 (최신순). 이관후속은 제외. §12.7 · §34.3.5
func (r *ASRepo) ListReopens(asID string) ([]model.ASHistoryItem, error) {
	return r.listByParent(asID, true)
}

// ListTransferFollowups 이 접수를 원본으로 이관후속된 건들. 재접수와 섞지 않는다. §34.3.5
func (r *ASRepo) ListTransferFollowups(asID string) ([]model.ASHistoryItem, error) {
	return r.listByParent(asID, false)
}

func (r *ASRepo) listByParent(asID string, reopenOnly bool) ([]model.ASHistoryItem, error) {
	flag := 0
	if reopenOnly {
		flag = 1
	}
	rows, err := r.db.Query(`
		SELECT ar.as_id, ar.as_number, date(ar.receipt_datetime),
		       COALESCE(ar.visit_scheduled_date,''), COALESCE(date(ar.complete_datetime),''),
		       COALESCE(ar.assigned_to,''), COALESCE(ar.symptom,''),
		       COALESCE(ar.action_taken,''), ar.status
		FROM as_receipts ar
		WHERE ar.parent_as_id = ? AND COALESCE(ar.is_reopen,0) = ?
		ORDER BY ar.receipt_datetime DESC`, asID, flag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ASHistoryItem
	for rows.Next() {
		var it model.ASHistoryItem
		if err := rows.Scan(&it.ASID, &it.ASNumber, &it.ReceiptDate,
			&it.VisitDate, &it.CompleteDate, &it.Visitor, &it.Symptom,
			&it.ActionTaken, &it.Status); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Update AS 상태 및 처리 내용 수정 (일시 미입력 시 상태 변경에 따라 자동 기록)
func (r *ASRepo) Update(as *model.ASReceipt) error {
	if err := model.RequireAppDateYear(as.VisitScheduledDate); err != nil {
		return err
	}
	if as.CompleteDatetime != nil && !as.CompleteDatetime.IsZero() && !model.AppDateYearOK(*as.CompleteDatetime) {
		return model.ErrAppDateYear
	}
	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05")

	var oldStatus string
	var oldComplete string
	r.db.QueryRow(`SELECT status, COALESCE(complete_datetime,'') FROM as_receipts WHERE as_id=?`, as.ASID).
		Scan(&oldStatus, &oldComplete)

	startDT := ""
	if as.StartDatetime != nil && !as.StartDatetime.IsZero() {
		startDT = as.StartDatetime.Format("2006-01-02 15:04:05")
	}

	completeDT := ""
	clearComplete := false
	if as.CompleteDatetime != nil && !as.CompleteDatetime.IsZero() {
		completeDT = as.CompleteDatetime.Format("2006-01-02 15:04:05")
	} else if (oldStatus != "completed" && oldStatus != "closed") &&
		(as.Status == "completed" || as.Status == "closed") && oldComplete == "" {
		completeDT = nowStr
	} else if as.Status != "completed" && as.Status != "closed" &&
		(as.CompleteDatetime == nil || as.CompleteDatetime.IsZero()) {
		clearComplete = true
	}

	q := `UPDATE as_receipts SET
			status=?, assigned_to=?, assigned_user_id=?, process_type=?, work_place=?, cause_type=?,
			cause_detail=?, conclusion=?,
			action_taken=?, parts_used=?, result_code=?, revisit_reason=?, hold_reason=?, followup_action=?,
			customer_confirmer=?, is_recurrence=?, is_reopen=?, replace_review=?,
			visit_scheduled_date=?, schedule_confirmed=?,
			transfer_detail=?, confirm_target=?, confirm_contact=?,
			cause_cat1=?, cause_cat2=?, cause_cat3=?,
			updated_at=?`
	args := []interface{}{
		as.Status, as.AssignedTo, as.AssignedUserID, as.ProcessType, as.WorkPlace, as.CauseType,
		as.CauseDetail, as.Conclusion,
		as.ActionTaken, as.PartsUsed, as.ResultCode, as.RevisitReason, as.HoldReason, as.FollowupAction,
		as.CustomerConfirmer, boolToInt(as.IsRecurrence), boolToInt(as.IsReopen), boolToInt(as.ReplaceReview),
		nullStr(as.VisitScheduledDate), boolToInt(as.ScheduleConfirmed),
		as.TransferDetail, as.ConfirmTarget, as.ConfirmContact,
		nullStr(as.CauseCat1), nullStr(as.CauseCat2), nullStr(as.CauseCat3),
		nowStr,
	}

	if startDT != "" {
		q += `, start_datetime=?`
		args = append(args, startDT)
	}
	if completeDT != "" {
		q += `, complete_datetime=?`
		args = append(args, completeDT)
	} else if clearComplete {
		q += `, complete_datetime=NULL`
	}
	if completeDT != "" && as.CustomerConfirmer != "" {
		q += `, confirm_datetime=?`
		args = append(args, nowStr)
	}

	q += ` WHERE as_id=?`
	args = append(args, as.ASID)

	err := touchUpdate(r.db, "as_receipts", "as_id", as.ASID, as.ASNumber, func() error {
		_, err := r.db.Exec(q, args...)
		return err
	})
	if err != nil {
		return err
	}
	// 접수 완료·종료·취소 시 확인·재방문 하부업무도 함께 닫아 지연 목록에 남지 않게 한다.
	if as.Status == "completed" || as.Status == "closed" || as.Status == "cancelled" {
		_ = NewASWorkRepo(r.db).CloseOpenByAS(as.ASID)
	}
	reindexASSearch(r.db, as.ASID)
	return nil
}

// SetStartDatetimeFromFirstProcess 착수시각이 비어 있고 조치가 있으면
// 최초 process_datetime으로 채운다. 이미 값이 있으면 덮어쓰지 않는다. (§4.2 · §12.5-4)
func (r *ASRepo) SetStartDatetimeFromFirstProcess(asID string) error {
	_, err := r.db.Exec(`
		UPDATE as_receipts SET start_datetime = (
			SELECT MIN(p.process_datetime) FROM as_processes p WHERE p.as_id = as_receipts.as_id)
		 WHERE as_id = ?
		   AND TRIM(COALESCE(start_datetime,'')) = ''
		   AND EXISTS (SELECT 1 FROM as_processes p WHERE p.as_id = as_receipts.as_id)`, asID)
	return err
}

// UpdateReceipt AS 접수 항목 수정 (접수번호·PK는 유지). 워크플로 상태면 자동 재파생.
func (r *ASRepo) UpdateReceipt(as *model.ASReceipt) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	receiptStr := as.ReceiptDatetime.Format("2006-01-02 15:04:05")

	var curStatus string
	_ = r.db.QueryRow(`SELECT status FROM as_receipts WHERE as_id=?`, as.ASID).Scan(&curStatus)

	as.ProjectID = resolveStoredProjectID(r.db, as.CustomerID, lookupASProductText(r.db, as.AssetID, as.Symptom), model.ScopeWorkAS)
	q := `UPDATE as_receipts SET
			receipt_datetime=?, customer_id=?, asset_id=?,
			receipt_channel=?, requester=?, symptom=?, urgency=?, priority=?,
			requester_type=?, requester_name=?, assigned_to=?, assigned_user_id=?,
			received_by=?, visit_scheduled_date=?, schedule_confirmed=?,
			confirm_contact=?, urgency_reason=?, urgency_reason_note=?,
			project_id=?, updated_at=?`
	args := []interface{}{
		receiptStr, as.CustomerID, nullStr(as.AssetID),
		as.ReceiptChannel, as.Requester, as.Symptom, as.Urgency, as.Priority,
		as.RequesterType, as.RequesterName, as.AssignedTo, as.AssignedUserID,
		as.ReceivedBy, nullStr(as.VisitScheduledDate), boolToInt(as.ScheduleConfirmed),
		as.ConfirmContact, as.UrgencyReason, as.UrgencyReasonNote,
		nullStr(as.ProjectID), now,
	}
	if model.IsASWorkflowStatus(curStatus) {
		q += `, status=?`
		args = append(args, model.DeriveASWorkflowStatus(as.AssignedTo, as.AssignedUserID, as.ScheduleConfirmed))
	}
	q += ` WHERE as_id=?`
	args = append(args, as.ASID)
	err := touchUpdate(r.db, "as_receipts", "as_id", as.ASID, as.ASNumber, func() error {
		_, err := r.db.Exec(q, args...)
		return err
	})
	if err != nil {
		return err
	}
	reindexASSearch(r.db, as.ASID)
	return nil
}

// UpdateVisitScheduledDate 예정업무일·일정확정 수정 (+워크플로 상태 재파생)
func (r *ASRepo) UpdateVisitScheduledDate(asID, date string, scheduleConfirmed bool) error {
	if err := model.RequireAppDateYear(date); err != nil {
		return err
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	var assignedTo, assignedUID, curStatus string
	_ = r.db.QueryRow(`SELECT COALESCE(assigned_to,''), COALESCE(assigned_user_id,''), status FROM as_receipts WHERE as_id=?`, asID).
		Scan(&assignedTo, &assignedUID, &curStatus)
	q := `UPDATE as_receipts SET visit_scheduled_date=?, schedule_confirmed=?, updated_at=?`
	args := []interface{}{nullStr(date), boolToInt(scheduleConfirmed), now}
	if strings.TrimSpace(date) != "" {
		q += `, schedule_no_date_reason=NULL, schedule_no_date_at=NULL`
	}
	if model.IsASWorkflowStatus(curStatus) {
		q += `, status=?`
		args = append(args, model.DeriveASWorkflowStatus(assignedTo, assignedUID, scheduleConfirmed))
	}
	q += ` WHERE as_id=?`
	args = append(args, asID)
	return r.touchReceipt(asID, func() error {
		_, err := r.db.Exec(q, args...)
		return err
	})
}

// AssignUnplanned 미계획 업무함에서 예정일을 넣고(확정) 선택적으로 담당자를 붙인다.
func (r *ASRepo) AssignUnplanned(asID, date, assignee, assigneeUID string) error {
	date = strings.TrimSpace(date)
	if date == "" {
		return fmt.Errorf("날짜가 필요합니다")
	}
	assignee = strings.TrimSpace(assignee)
	assigneeUID = strings.TrimSpace(assigneeUID)
	if assignee != "" || assigneeUID != "" {
		now := time.Now().Format("2006-01-02 15:04:05")
		if err := r.touchReceipt(asID, func() error {
			_, err := r.db.Exec(`UPDATE as_receipts SET assigned_to=?, assigned_user_id=?, updated_at=? WHERE as_id=?`,
				assignee, assigneeUID, now, asID)
			return err
		}); err != nil {
			return err
		}
	}
	return r.UpdateVisitScheduledDate(asID, date, true)
}

// SetScheduleNoDate 「미정+사유」 등록. 예정일은 비우고 14일 후 재검토 기준일을 남긴다.
func (r *ASRepo) SetScheduleNoDate(asID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("미정 사유가 필요합니다")
	}
	now := time.Now()
	today := now.Format("2006-01-02")
	ts := now.Format("2006-01-02 15:04:05")
	var assignedTo, assignedUID, curStatus string
	_ = r.db.QueryRow(`SELECT COALESCE(assigned_to,''), COALESCE(assigned_user_id,''), status FROM as_receipts WHERE as_id=?`, asID).
		Scan(&assignedTo, &assignedUID, &curStatus)
	status := curStatus
	if model.IsASWorkflowStatus(curStatus) {
		status = model.DeriveASWorkflowStatus(assignedTo, assignedUID, false)
	}
	return r.touchReceipt(asID, func() error {
		_, err := r.db.Exec(`
		UPDATE as_receipts
		SET visit_scheduled_date=NULL, schedule_confirmed=0,
		    schedule_no_date_reason=?, schedule_no_date_at=?, status=?, updated_at=?
		WHERE as_id=?`, reason, today, status, ts, asID)
		return err
	})
}

// SyncWorkflowStatus 보류 해제 등에서 필드 기준 상태로 맞춤
func (r *ASRepo) SyncWorkflowStatus(asID string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	var assignedTo, assignedUID string
	var confirmed int
	err := r.db.QueryRow(`SELECT COALESCE(assigned_to,''), COALESCE(assigned_user_id,''), COALESCE(schedule_confirmed,0) FROM as_receipts WHERE as_id=?`, asID).
		Scan(&assignedTo, &assignedUID, &confirmed)
	if err != nil {
		return err
	}
	st := model.DeriveASWorkflowStatus(assignedTo, assignedUID, confirmed == 1)
	return r.touchReceipt(asID, func() error {
		_, err := r.db.Exec(`UPDATE as_receipts SET status=?, hold_reason='', hold_next_action='', updated_at=? WHERE as_id=?`,
			st, now, asID)
		return err
	})
}

// ListSchedulePending 배정됐으나 일정 미확정 건
func (r *ASRepo) ListSchedulePending(mineUserID string, mineKeys []string, limit int) ([]model.ASListItem, int, error) {
	if limit <= 0 {
		limit = 20
	}
	base := `
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.status IN ('received','assigned')
		  AND (COALESCE(ar.assigned_to,'') != '' OR COALESCE(ar.assigned_user_id,'') != '')
		  AND COALESCE(ar.schedule_confirmed,0) = 0`
	args := []interface{}{}
	if mineCond, mineArgs := mineAssigneeCond("ar.", mineUserID, mineKeys); mineCond != "" {
		base += mineCond
		args = append(args, mineArgs...)
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := `
		SELECT ar.as_id, ar.as_number, ar.receipt_datetime,
		       c.org_name, COALESCE(a.product_name,'') AS product_name,
		       ar.symptom, ar.urgency, ar.status, COALESCE(ar.assigned_to,''),
		       ` + daysElapsedSQL + ` AS days_elapsed
		` + base + `
		ORDER BY ar.receipt_datetime DESC
		LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []model.ASListItem
	for rows.Next() {
		var item model.ASListItem
		var receiptStr string
		if err := rows.Scan(&item.ASID, &item.ASNumber, &receiptStr,
			&item.OrgName, &item.ProductName, &item.Symptom, &item.Urgency, &item.Status, &item.AssignedTo, &item.DaysElapsed); err != nil {
			return nil, 0, err
		}
		item.ReceiptDatetime = parseTime(receiptStr)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// SetHold 보류 처리 (nextAction: action|transfer|cancel)
func (r *ASRepo) SetHold(asID, reason, nextAction string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	return r.touchReceipt(asID, func() error {
		_, err := r.db.Exec(`UPDATE as_receipts SET status='hold', hold_reason=?, hold_next_action=?, updated_at=? WHERE as_id=?`,
			reason, nextAction, now, asID)
		return err
	})
}

// ReleaseHold 보류 해제 → 필드 기준 워크플로 상태로 복귀
func (r *ASRepo) ReleaseHold(asID string) error {
	return r.SyncWorkflowStatus(asID)
}

// SetTransfer 이관(결과 대기) — 상태는 진행중, 확인예정일 없으면 오늘+7일
func (r *ASRepo) SetTransfer(asID string) error {
	now := time.Now()
	follow := model.DefaultTransferFollowupDate(now)
	ts := now.Format("2006-01-02 15:04:05")
	return r.touchReceipt(asID, func() error {
		_, err := r.db.Exec(`UPDATE as_receipts SET status='in_progress', transfer_detail='waiting',
		visit_scheduled_date = CASE WHEN COALESCE(visit_scheduled_date,'') = '' THEN ? ELSE visit_scheduled_date END,
		schedule_confirmed = 1,
		updated_at=? WHERE as_id=?`, follow, ts, asID)
		return err
	})
}

// CompleteTransfer 이관 건 결과코드로 완료 (완료일 지정)
func (r *ASRepo) CompleteTransfer(asID, resultCode, completeDate string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	completeDT := completeDate
	if completeDT == "" {
		completeDT = time.Now().Format("2006-01-02")
	}
	if len(completeDT) == 10 {
		completeDT = completeDT + " 00:00:00"
	}
	err := r.touchReceipt(asID, func() error {
		_, err := r.db.Exec(`UPDATE as_receipts SET status='completed', result_code=?, complete_datetime=?, updated_at=? WHERE as_id=?`,
			resultCode, completeDT, now, asID)
		return err
	})
	if err != nil {
		return err
	}
	_ = NewASWorkRepo(r.db).CloseOpenByAS(asID)
	return nil
}

// SetCancelled 접수취소
func (r *ASRepo) SetCancelled(asID, cancelDate string) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	cancelDT := cancelDate
	if cancelDT == "" {
		cancelDT = time.Now().Format("2006-01-02")
	}
	if len(cancelDT) == 10 {
		cancelDT = cancelDT + " 00:00:00"
	}
	err := r.touchReceipt(asID, func() error {
		_, err := r.db.Exec(`UPDATE as_receipts SET status='cancelled', cancel_datetime=?, updated_at=? WHERE as_id=?`,
			cancelDT, now, asID)
		return err
	})
	if err != nil {
		return err
	}
	_ = NewASWorkRepo(r.db).CloseOpenByAS(asID)
	return nil
}

// ListPastHistory 동일 기관의 완료·종료 AS 이력 (현재 건 제외)
func (r *ASRepo) ListPastHistory(customerID, excludeASID string, limit int) ([]model.ASHistoryItem, error) {
	if customerID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT ar.as_id, ar.as_number,
		       COALESCE(ar.receipt_datetime,''),
		       COALESCE(ar.visit_scheduled_date,''),
		       COALESCE(ar.start_datetime,''),
		       COALESCE(ar.complete_datetime,''),
		       COALESCE(ar.assigned_to,''),
		       COALESCE(ar.symptom,''),
		       COALESCE(ar.action_taken,'')
		FROM as_receipts ar
		WHERE ar.customer_id=?
		  AND ar.status IN ` + model.SQLStatusFullyClosed
	args := []interface{}{customerID}
	if excludeASID != "" {
		q += ` AND ar.as_id!=?`
		args = append(args, excludeASID)
	}
	q += ` ORDER BY ar.receipt_datetime DESC, ar.as_number DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.ASHistoryItem
	for rows.Next() {
		var it model.ASHistoryItem
		var receiptStr, visitStr, startStr, completeStr string
		if err := rows.Scan(
			&it.ASID, &it.ASNumber,
			&receiptStr, &visitStr, &startStr, &completeStr,
			&it.Visitor, &it.Symptom, &it.ActionTaken,
		); err != nil {
			return nil, err
		}
		receiptAt := parseTime(receiptStr)
		completeAt := parseTime(completeStr)
		if !receiptAt.IsZero() {
			it.ReceiptDate = receiptAt.Format("2006-01-02")
		}
		if visitStr != "" {
			it.VisitDate = visitStr
		} else if t := parseTime(startStr); !t.IsZero() {
			it.VisitDate = t.Format("2006-01-02")
		}
		if !completeAt.IsZero() {
			it.CompleteDate = completeAt.Format("2006-01-02")
		}
		it.DurationText = formatElapsed(receiptAt, completeAt)
		items = append(items, it)
	}
	return items, rows.Err()
}

// ListHistoryByAsset 동일 자산의 AS 이력 (현재 건 제외 가능)
func (r *ASRepo) ListHistoryByAsset(assetID, excludeASID string, limit int) ([]model.ASHistoryItem, error) {
	if assetID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT ar.as_id, ar.as_number,
		       COALESCE(ar.receipt_datetime,''),
		       COALESCE(ar.visit_scheduled_date,''),
		       COALESCE(ar.start_datetime,''),
		       COALESCE(ar.complete_datetime,''),
		       COALESCE(ar.assigned_to,''),
		       COALESCE(ar.symptom,''),
		       COALESCE(ar.action_taken,''),
		       COALESCE(ar.status,'')
		FROM as_receipts ar
		WHERE ar.asset_id=?`
	args := []interface{}{assetID}
	if excludeASID != "" {
		q += ` AND ar.as_id!=?`
		args = append(args, excludeASID)
	}
	q += ` ORDER BY ar.receipt_datetime DESC, ar.as_number DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.ASHistoryItem
	for rows.Next() {
		var it model.ASHistoryItem
		var receiptStr, visitStr, startStr, completeStr, status string
		if err := rows.Scan(
			&it.ASID, &it.ASNumber,
			&receiptStr, &visitStr, &startStr, &completeStr,
			&it.Visitor, &it.Symptom, &it.ActionTaken, &status,
		); err != nil {
			return nil, err
		}
		receiptAt := parseTime(receiptStr)
		completeAt := parseTime(completeStr)
		if !receiptAt.IsZero() {
			it.ReceiptDate = receiptAt.Format("2006-01-02")
		}
		if visitStr != "" {
			it.VisitDate = visitStr
		} else if t := parseTime(startStr); !t.IsZero() {
			it.VisitDate = t.Format("2006-01-02")
		}
		if !completeAt.IsZero() {
			it.CompleteDate = completeAt.Format("2006-01-02")
		}
		it.DurationText = formatElapsed(receiptAt, completeAt)
		it.Status = status
		it.StatusLabel = statsStatusLabel(status)
		items = append(items, it)
	}
	return items, rows.Err()
}

func formatElapsed(from, to time.Time) string {
	if from.IsZero() || to.IsZero() {
		return "—"
	}
	d := to.Sub(from)
	if d < 0 {
		return "—"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%d일 %d시간", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%d시간 %d분", hours, mins)
	}
	return fmt.Sprintf("%d분", int(d.Minutes()))
}

// Delete 접수 및 처리 이력 삭제
func (r *ASRepo) Delete(asID string) error {
	var num string
	_ = r.db.QueryRow(`SELECT as_number FROM as_receipts WHERE as_id=?`, asID).Scan(&num)
	return touchDelete(r.db, "as_receipts", "as_id", asID, num, func() error {
		tx, err := r.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`DELETE FROM as_processes WHERE as_id=?`, asID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM as_work_items WHERE as_id=?`, asID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM as_keyword_links WHERE as_id=?`, asID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM as_receipts WHERE as_id=?`, asID); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		deleteASSearch(r.db, asID)
		return nil
	})
}

// ListOverdue 지연 AS 목록 (3일 초과 미처리)
func (r *ASRepo) ListOverdue(page, pageSize int) ([]model.ASListItem, int, error) {
	return r.List("overdue", "", page, pageSize)
}

// ListAssignedOpen 배정자(이름 또는 아이디)의 미완료 AS
func (r *ASRepo) ListAssignedOpen(mineUserID string, mineKeys []string, page, pageSize int) ([]model.ASListItem, int, error) {
	return r.ListFiltered("open", "", mineUserID, mineKeys, "", "", page, pageSize)
}

// ListByCustomer 고객별 AS 이력 목록 (최신순, 최대 50건)
func (r *ASRepo) ListByCustomer(customerID string) ([]model.ASListItem, error) {
	query := `
		SELECT ar.as_id, ar.as_number, ar.receipt_datetime,
		       c.org_name, COALESCE(a.product_name,'') AS product_name,
		       ar.symptom, ar.urgency, ar.status, COALESCE(ar.assigned_to,''),
		       ` + daysElapsedSQL + ` AS days_elapsed,
		       COALESCE(ar.urgency_reason,''), COALESCE(ar.urgency_reason_note,'')
		FROM as_receipts ar
		JOIN customers c ON c.customer_id = ar.customer_id
		LEFT JOIN assets a ON a.asset_id = ar.asset_id
		WHERE ar.customer_id = ?
		ORDER BY ar.receipt_datetime DESC
		LIMIT 50`

	rows, err := r.db.Query(query, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []model.ASListItem
	for rows.Next() {
		var item model.ASListItem
		var receiptStr string
		if err := rows.Scan(
			&item.ASID, &item.ASNumber, &receiptStr,
			&item.OrgName, &item.ProductName,
			&item.Symptom, &item.Urgency, &item.Status, &item.AssignedTo,
			&item.DaysElapsed, &item.UrgencyReason, &item.UrgencyReasonNote,
		); err != nil {
			return nil, err
		}
		item.ReceiptDatetime = parseTime(receiptStr)
		items = append(items, item)
	}
	return items, rows.Err()
}
