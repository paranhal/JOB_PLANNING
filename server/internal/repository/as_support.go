package repository

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"customer-support/internal/model"
)

func applyASSupportDetails(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS as_support_details (
  work_id       TEXT PRIMARY KEY REFERENCES as_work_items(work_id),
  support_type  TEXT NOT NULL,
  vendor_name   TEXT,
  tracking_no   TEXT,
  action_date   TEXT,
  due_date      TEXT,
  asset_id      TEXT,
  memo          TEXT
)`); err != nil {
		log.Printf("056 as_support_details: %v", err)
		return
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_support_details_type ON as_support_details(support_type)`); err != nil {
		log.Printf("056 as_support_details type index: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_support_details_asset ON as_support_details(asset_id)`); err != nil {
		log.Printf("056 as_support_details asset index: %v", err)
	}
}

const asSupportSelect = `SELECT w.work_id, w.work_number, w.as_id, w.work_kind, COALESCE(w.scheduled_date,''),
		COALESCE(w.schedule_confirmed,0), COALESCE(w.confirm_target,''), COALESCE(w.confirm_contact,''),
		COALESCE(w.assigned_to,''), COALESCE(w.assigned_user_id,''),
		COALESCE(w.status,'open'), COALESCE(w.notes,''), COALESCE(w.created_at,''), COALESCE(w.updated_at,''),
		COALESCE(d.support_type,''), COALESCE(d.vendor_name,''), COALESCE(d.tracking_no,''),
		COALESCE(d.action_date,''), COALESCE(d.due_date,''), COALESCE(d.asset_id,''), COALESCE(d.memo,'')
		FROM as_work_items w
		LEFT JOIN as_support_details d ON d.work_id = w.work_id`

func (r *ASWorkRepo) CountOpenSupport(asID string) (int, error) {
	if r == nil || r.db == nil || strings.TrimSpace(asID) == "" {
		return 0, nil
	}
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM as_work_items
		WHERE as_id=? AND work_kind=? AND status='open'`, asID, model.WorkKindSupport).Scan(&n)
	return n, err
}

func (r *ASWorkRepo) GetDetail(workID string) (*model.ASSupportDetail, error) {
	if r == nil || strings.TrimSpace(workID) == "" {
		return nil, nil
	}
	var d model.ASSupportDetail
	err := r.db.QueryRow(`SELECT work_id, support_type, COALESCE(vendor_name,''), COALESCE(tracking_no,''),
		COALESCE(action_date,''), COALESCE(due_date,''), COALESCE(asset_id,''), COALESCE(memo,'')
		FROM as_support_details WHERE work_id=?`, workID).Scan(
		&d.WorkID, &d.SupportType, &d.VendorName, &d.TrackingNo,
		&d.ActionDate, &d.DueDate, &d.AssetID, &d.Memo)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *ASWorkRepo) ListSupportByAS(asID string) ([]model.ASSupportItem, error) {
	rows, err := r.db.Query(asSupportSelect+` WHERE w.as_id=? AND w.work_kind=? ORDER BY w.work_number`,
		asID, model.WorkKindSupport)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSupportItems(rows)
}

func (r *ASWorkRepo) CreateSupport(w *model.ASWorkItem, d model.ASSupportDetail) error {
	if w == nil {
		return fmt.Errorf("하부업무가 없습니다")
	}
	w.WorkKind = model.WorkKindSupport
	if err := model.ValidateSupportDetail(d); err != nil {
		return err
	}
	if strings.TrimSpace(w.ScheduledDate) == "" {
		w.ScheduledDate = strings.TrimSpace(d.DueDate)
	}
	if err := r.Create(w); err != nil {
		return err
	}
	d.WorkID = w.WorkID
	return r.UpsertDetail(d)
}

func (r *ASWorkRepo) UpsertDetail(d model.ASSupportDetail) error {
	if err := model.ValidateSupportDetail(d); err != nil {
		return err
	}
	return upsertSupportDetailTx(r.db, d)
}

func upsertSupportDetailTx(db *sql.DB, d model.ASSupportDetail) error {
	_, err := db.Exec(`INSERT INTO as_support_details
		(work_id, support_type, vendor_name, tracking_no, action_date, due_date, asset_id, memo)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(work_id) DO UPDATE SET
		  support_type=excluded.support_type,
		  vendor_name=excluded.vendor_name,
		  tracking_no=excluded.tracking_no,
		  action_date=excluded.action_date,
		  due_date=excluded.due_date,
		  asset_id=excluded.asset_id,
		  memo=excluded.memo`,
		d.WorkID, d.SupportType, d.VendorName, d.TrackingNo, d.ActionDate, d.DueDate, d.AssetID, d.Memo)
	return err
}

func (r *ASWorkRepo) LogOpenSupportCompleteAllow(asID, asNumber string, n int) {
	if r == nil || r.db == nil || strings.TrimSpace(asID) == "" {
		return
	}
	reason := model.OpenSupportWarnMessage(n) + " 확인 후 완료"
	logUpdateWithReason(r.db, "as_receipts", "as_id", asID, asNumber, "", reason)
}

func (r *ASWorkRepo) FactoryWaitForAS(asID, today string) model.FactoryWaitDisplay {
	items, err := r.ListSupportByAS(asID)
	if err != nil {
		return model.FactoryWaitDisplay{Mode: model.FactoryWaitNone, Label: "—"}
	}
	var inAct, retAct string
	for _, it := range items {
		switch it.Detail.SupportType {
		case model.SupportFactoryInbound:
			inAct = it.Detail.ActionDate
		case model.SupportFactoryReturn:
			retAct = it.Detail.ActionDate
		}
	}
	return model.FactoryWaitFromDates(inAct, retAct, today, func(a, b string) (int, bool) {
		return leadBusinessDaysSQL(r.db, a, b)
	})
}

func (r *ASWorkRepo) ListFactoryRepairByAsset(assetID, excludeASID string, limit int) ([]model.ASHistoryItem, error) {
	if strings.TrimSpace(assetID) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT ar.as_id, ar.as_number, COALESCE(d.action_date,''), COALESCE(w.notes,''), COALESCE(d.vendor_name,'')
		FROM as_support_details d
		JOIN as_work_items w ON w.work_id = d.work_id
		JOIN as_receipts ar ON ar.as_id = w.as_id
		WHERE d.support_type=? AND TRIM(COALESCE(NULLIF(d.asset_id,''), ar.asset_id,'')) = ?
		  AND (? = '' OR ar.as_id != ?)
		ORDER BY COALESCE(NULLIF(d.action_date,''), w.created_at) DESC
		LIMIT ?`
	rows, err := r.db.Query(q, model.SupportFactoryInbound, assetID, excludeASID, excludeASID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ASHistoryItem
	for rows.Next() {
		var it model.ASHistoryItem
		var inAct, notes, vendor string
		if err := rows.Scan(&it.ASID, &it.ASNumber, &inAct, &notes, &vendor); err != nil {
			return nil, err
		}
		retAct := r.returnActionDate(it.ASID)
		it.Kind = "factory_repair"
		it.FactoryFrom = strings.TrimSpace(inAct)
		it.FactoryTo = strings.TrimSpace(retAct)
		it.ReceiptDate = it.FactoryFrom
		it.CompleteDate = it.FactoryTo
		it.StatusLabel = "제조사 수리 중"
		if it.FactoryFrom != "" && it.FactoryTo != "" {
			it.DurationText = it.FactoryFrom + " ~ " + it.FactoryTo
		} else if it.FactoryFrom != "" {
			it.DurationText = it.FactoryFrom + " ~"
		}
		it.Symptom = "제조사 수리 중"
		if vendor != "" {
			it.Symptom += " · " + vendor
		}
		if notes != "" {
			it.ActionTaken = notes
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *ASWorkRepo) returnActionDate(asID string) string {
	var s string
	_ = r.db.QueryRow(`
		SELECT COALESCE(d.action_date,'') FROM as_support_details d
		JOIN as_work_items w ON w.work_id = d.work_id
		WHERE w.as_id=? AND d.support_type=? LIMIT 1`, asID, model.SupportFactoryReturn).Scan(&s)
	return s
}

func scanSupportItems(rows *sql.Rows) ([]model.ASSupportItem, error) {
	var items []model.ASSupportItem
	for rows.Next() {
		var it model.ASSupportItem
		var conf int
		var created, updated string
		if err := rows.Scan(
			&it.Work.WorkID, &it.Work.WorkNumber, &it.Work.ASID, &it.Work.WorkKind, &it.Work.ScheduledDate,
			&conf, &it.Work.ConfirmTarget, &it.Work.ConfirmContact, &it.Work.AssignedTo, &it.Work.AssignedUserID,
			&it.Work.Status, &it.Work.Notes, &created, &updated,
			&it.Detail.SupportType, &it.Detail.VendorName, &it.Detail.TrackingNo,
			&it.Detail.ActionDate, &it.Detail.DueDate, &it.Detail.AssetID, &it.Detail.Memo,
		); err != nil {
			return nil, err
		}
		it.Work.ScheduleConfirmed = conf == 1
		it.Work.CreatedAt = parseTime(created)
		it.Work.UpdatedAt = parseTime(updated)
		it.Detail.WorkID = it.Work.WorkID
		items = append(items, it)
	}
	return items, rows.Err()
}

func (r *StatsRepo) LoadFactoryWaitCard(from, toEx string) model.FactoryWaitCard {
	out := model.FactoryWaitCard{DisplayLabel: "—"}
	if r == nil || r.db == nil {
		return out
	}
	rows, err := r.db.Query(`
		SELECT ar.as_id,
		       MAX(CASE WHEN d.support_type=? THEN COALESCE(d.action_date,'') END),
		       MAX(CASE WHEN d.support_type=? THEN COALESCE(d.action_date,'') END)
		FROM as_support_details d
		JOIN as_work_items w ON w.work_id = d.work_id
		JOIN as_receipts ar ON ar.as_id = w.as_id
		WHERE d.support_type IN (?,?)
		GROUP BY ar.as_id`,
		model.SupportFactoryInbound, model.SupportFactoryReturn,
		model.SupportFactoryInbound, model.SupportFactoryReturn)
	if err != nil {
		return out
	}
	defer rows.Close()
	var sum float64
	var nAvg int
	for rows.Next() {
		var asID string
		var inActN, retActN sql.NullString
		if err := rows.Scan(&asID, &inActN, &retActN); err != nil {
			continue
		}
		inAct, retAct := strings.TrimSpace(inActN.String), strings.TrimSpace(retActN.String)
		if inAct == "" {
			continue
		}
		inPeriod := dateInRange(inAct, from, toEx) || dateInRange(retAct, from, toEx) ||
			(retAct == "" && inAct < toEx)
		if !inPeriod {
			continue
		}
		out.Count++
		if retAct == "" {
			out.InProgress++
			continue
		}
		d, ok := leadBusinessDaysSQL(r.db, inAct, retAct)
		if !ok {
			continue
		}
		sum += float64(d)
		nAvg++
	}
	if nAvg > 0 {
		out.HasAvg = true
		out.AvgDays = float64(int(sum/float64(nAvg)*10+0.5)) / 10
		out.DisplayLabel = fmt.Sprintf("%.1f영업일", out.AvgDays)
	} else if out.InProgress > 0 {
		out.DisplayLabel = "진행 중"
	}
	return out
}

func dateInRange(d, from, toEx string) bool {
	d = strings.TrimSpace(d)
	if d == "" {
		return false
	}
	return d >= from && d < toEx
}

func leadBusinessDaysSQL(db *sql.DB, from, to string) (int, bool) {
	from, to = trimISODate(from), trimISODate(to)
	if from == "" || to == "" {
		return 0, false
	}
	if from == to {
		return 0, true
	}
	if to < from {
		return 0, false
	}
	var n sql.NullInt64
	err := db.QueryRow(`
		SELECT cc.bd_index - cr.next_bd_index
		FROM business_days cr
		JOIN business_days cc ON 1=1
		WHERE cr.d=? AND cc.d=?`, from, to).Scan(&n)
	if err != nil || !n.Valid {
		return 0, false
	}
	v := int(n.Int64)
	if v < 0 {
		return 0, true
	}
	return v, true
}

func trimISODate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
