package repository

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"customer-support/internal/model"
)

const asImportSchemaSQL = `
CREATE TABLE IF NOT EXISTS as_import_batches (
  batch_id      TEXT PRIMARY KEY,
  filename      TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'reported',
  created_at    TEXT NOT NULL DEFAULT '',
  created_by    TEXT NOT NULL DEFAULT '',
  applied_at    TEXT NOT NULL DEFAULT '',
  cancelled_at  TEXT NOT NULL DEFAULT '',
  total_rows    INTEGER NOT NULL DEFAULT 0,
  error_rows    INTEGER NOT NULL DEFAULT 0,
  warning_rows  INTEGER NOT NULL DEFAULT 0,
  applied_rows  INTEGER NOT NULL DEFAULT 0,
  note          TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS as_import_rows (
  batch_id      TEXT NOT NULL,
  row_no        INTEGER NOT NULL,
  excluded      INTEGER NOT NULL DEFAULT 0,
  issues        TEXT NOT NULL DEFAULT '',
  customer_raw  TEXT NOT NULL DEFAULT '',
  customer_id   TEXT NOT NULL DEFAULT '',
  asset_raw     TEXT NOT NULL DEFAULT '',
  asset_id      TEXT NOT NULL DEFAULT '',
  receipt_raw   TEXT NOT NULL DEFAULT '',
  receipt_date  TEXT NOT NULL DEFAULT '',
  process_raw   TEXT NOT NULL DEFAULT '',
  process_date  TEXT NOT NULL DEFAULT '',
  visit_date    TEXT NOT NULL DEFAULT '',
  symptom       TEXT NOT NULL DEFAULT '',
  action        TEXT NOT NULL DEFAULT '',
  channel       TEXT NOT NULL DEFAULT '',
  requester     TEXT NOT NULL DEFAULT '',
  worker        TEXT NOT NULL DEFAULT '',
  worker_uid    TEXT NOT NULL DEFAULT '',
  category      TEXT NOT NULL DEFAULT '',
  model         TEXT NOT NULL DEFAULT '',
  applied_as_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (batch_id, row_no)
);
`

func applyASImport(db *sql.DB) {
	if db == nil {
		return
	}
	if _, err := db.Exec(asImportSchemaSQL); err != nil {
		log.Printf("022 as_import schema: %v", err)
		return
	}
	if _, err := db.Exec(`ALTER TABLE as_receipts ADD COLUMN import_batch_id TEXT`); err != nil &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		log.Printf("022 import_batch_id: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_as_receipts_import_batch ON as_receipts(import_batch_id)`); err != nil {
		log.Printf("022 import_batch index: %v", err)
	}
}

type ASImportRepo struct{ db *sql.DB }

func NewASImportRepo(db *sql.DB) *ASImportRepo { return &ASImportRepo{db: db} }

var (
	ErrImportNotReported = fmt.Errorf("검증 리포트를 거치지 않으면 반영할 수 없습니다")
	ErrImportHasErrors   = fmt.Errorf("오류 건을 고치거나 제외한 뒤에 반영하세요")
	ErrImportNotApplied  = fmt.Errorf("반영된 배치만 일괄 취소할 수 있습니다")
)

var importCustomerAliases = map[string]string{
	"어진동행정복지커뮤니티센터":  "어진동행정복합커뮤니티센터",
	"아산중앙도서관":        "아산시중앙도서관",
	"한국기술교육대학교다산도서관": "한국기술교육대학교다산정보관",
	"충남교육청학생문화교육원":   "충남교육청학생교육문화원",
}

var importChannelCodes = map[string]string{
	"전화": "phone", "메일": "email", "이메일": "email",
	"현장": "visit", "방문": "visit", "원콜": "onecall",
	"제조사요청": "maker", "협력사요청": "partner",
}

// ParseASImportFile 엑셀·CSV를 행으로 읽는다. DB에 넣지 않는다. §12.11.7 1단계
func ParseASImportFile(filename string, data []byte) ([]model.ASImportRow, error) {
	name := strings.ToLower(strings.TrimSpace(filename))
	var table [][]string
	var err error
	if strings.HasSuffix(name, ".csv") || looksLikeCSV(data) {
		table, err = readImportCSV(data)
	} else {
		table, err = readImportXLSX(data)
	}
	if err != nil {
		return nil, err
	}
	return parseImportTable(table), nil
}

func looksLikeCSV(data []byte) bool {
	if len(data) >= 2 && data[0] == 0x50 && data[1] == 0x4B {
		return false
	}
	s := string(data)
	if len(s) > 400 {
		s = s[:400]
	}
	return strings.Contains(s, ",") && strings.Count(s, "\n") >= 1
}

func readImportCSV(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	return r.ReadAll()
}

func readImportXLSX(data []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("엑셀을 열 수 없습니다: %w", err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("시트가 없습니다")
	}
	return f.GetRows(sheets[0])
}

func parseImportTable(rows [][]string) []model.ASImportRow {
	if len(rows) == 0 {
		return nil
	}
	start, col := detectImportLayout(rows)
	var out []model.ASImportRow
	for i := start; i < len(rows); i++ {
		row := rows[i]
		cust := importCell(row, col.customer)
		recv := importCell(row, col.receipt)
		sym := importCell(row, col.symptom)
		if cust == "" && recv == "" && sym == "" {
			continue
		}
		modelName := importCell(row, col.model)
		cat := importCell(row, col.category)
		r := model.ASImportRow{
			RowNo:       i + 1,
			CustomerRaw: cust,
			AssetRaw:    strings.TrimSpace(strings.Trim(cat+" "+modelName, " ")),
			ReceiptRaw:  recv,
			ProcessRaw:  importCell(row, col.process),
			VisitDate:   parseImportDate(importCell(row, col.visit)),
			Symptom:     sym,
			Action:      importCell(row, col.action),
			Channel:     importChannelCodes[importCell(row, col.channel)],
			Requester:   importCell(row, col.requester),
			Worker:      importCell(row, col.worker),
			Category:    cat,
			Model:       modelName,
		}
		if d, ok := parseImportDateOK(recv); ok {
			r.ReceiptDate = d
		}
		if d, ok := parseImportDateOK(r.ProcessRaw); ok {
			r.ProcessDate = d
		} else if strings.TrimSpace(r.ProcessRaw) == "" {
			r.ProcessDate = r.ReceiptDate
		}
		out = append(out, r)
	}
	return out
}

type importCols struct {
	receipt, process, category, model, channel, customer, requester, worker, symptom, action, visit int
}

func detectImportLayout(rows [][]string) (start int, col importCols) {
	col = importCols{
		receipt: 0, process: 1, category: 2, model: 3, channel: 4,
		customer: 5, requester: 6, worker: 7, symptom: 8, action: 10, visit: -1,
	}
	for i := 0; i < len(rows) && i < 6; i++ {
		idx := map[string]int{}
		for c, v := range rows[i] {
			k := normalizeImportName(v)
			if k != "" {
				idx[k] = c
			}
		}
		cust, okC := firstCol(idx, "거래처명", "거래처", "기관명", "고객명", "고객")
		recv, okR := firstCol(idx, "접수일자", "접수일", "접수일시")
		if !okC || !okR {
			continue
		}
		col.customer, col.receipt = cust, recv
		col.process, _ = firstCol(idx, "처리일자", "처리일", "완료일", "완료일자")
		col.category, _ = firstCol(idx, "제품분류", "분류")
		col.model, _ = firstCol(idx, "제품모델명", "모델명", "모델")
		col.channel, _ = firstCol(idx, "접수형태", "접수채널", "채널")
		col.requester, _ = firstCol(idx, "고객담당자", "요청자")
		col.worker, _ = firstCol(idx, "수행담당자", "담당자")
		col.symptom, _ = firstCol(idx, "내용", "증상", "증상내용")
		col.action, _ = firstCol(idx, "답변및처리", "조치", "조치내용", "처리내용")
		col.visit, _ = firstCol(idx, "예정업무일", "예정일", "방문일")
		return i + 1, col
	}
	return 3, col
}

func firstCol(idx map[string]int, names ...string) (int, bool) {
	for _, n := range names {
		if c, ok := idx[normalizeImportName(n)]; ok {
			return c, true
		}
	}
	return -1, false
}

func importCell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func (r *ASImportRepo) ListBatches() ([]model.ASImportBatch, error) {
	rows, err := r.db.Query(`
		SELECT batch_id, filename, status, created_at, created_by, applied_at, cancelled_at,
		       total_rows, error_rows, warning_rows, applied_rows, note
		FROM as_import_batches ORDER BY created_at DESC, batch_id DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ASImportBatch
	for rows.Next() {
		var b model.ASImportBatch
		if err := rows.Scan(&b.BatchID, &b.Filename, &b.Status, &b.CreatedAt, &b.CreatedBy,
			&b.AppliedAt, &b.CancelledAt, &b.TotalRows, &b.ErrorRows, &b.WarningRows, &b.AppliedRows, &b.Note); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *ASImportRepo) GetBatch(id string) (*model.ASImportBatch, error) {
	b := &model.ASImportBatch{}
	err := r.db.QueryRow(`
		SELECT batch_id, filename, status, created_at, created_by, applied_at, cancelled_at,
		       total_rows, error_rows, warning_rows, applied_rows, note
		FROM as_import_batches WHERE batch_id=?`, id).
		Scan(&b.BatchID, &b.Filename, &b.Status, &b.CreatedAt, &b.CreatedBy,
			&b.AppliedAt, &b.CancelledAt, &b.TotalRows, &b.ErrorRows, &b.WarningRows, &b.AppliedRows, &b.Note)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.ListRows(id)
	if err != nil {
		return b, err
	}
	for _, row := range rows {
		if row.Excluded {
			b.ExcludedRows++
			continue
		}
		if !row.HasBlocker() {
			b.ReadyRows++
		}
	}
	return b, nil
}

func (r *ASImportRepo) ListRows(batchID string) ([]model.ASImportRow, error) {
	rows, err := r.db.Query(`
		SELECT batch_id, row_no, excluded, issues, customer_raw, customer_id, asset_raw, asset_id,
		       receipt_raw, receipt_date, process_raw, process_date, visit_date, symptom, action,
		       channel, requester, worker, worker_uid, category, model, applied_as_id
		FROM as_import_rows WHERE batch_id=? ORDER BY row_no`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanASImportRows(rows)
}

func scanASImportRows(rows *sql.Rows) ([]model.ASImportRow, error) {
	var out []model.ASImportRow
	for rows.Next() {
		var r model.ASImportRow
		var excl int
		var issues string
		if err := rows.Scan(&r.BatchID, &r.RowNo, &excl, &issues, &r.CustomerRaw, &r.CustomerID, &r.AssetRaw, &r.AssetID,
			&r.ReceiptRaw, &r.ReceiptDate, &r.ProcessRaw, &r.ProcessDate, &r.VisitDate, &r.Symptom, &r.Action,
			&r.Channel, &r.Requester, &r.Worker, &r.WorkerUID, &r.Category, &r.Model, &r.AppliedASID); err != nil {
			return nil, err
		}
		r.Excluded = excl == 1
		r.Issues = model.SplitIssueCodes(issues)
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateBatch 파일을 스테이징에 넣고 검증 리포트를 만든다. 접수 테이블에는 넣지 않는다.
func (r *ASImportRepo) CreateBatch(filename, createdBy string, drafts []model.ASImportRow) (*model.ASImportBatch, error) {
	if len(drafts) == 0 {
		return nil, fmt.Errorf("가져올 행이 없습니다")
	}
	if len(drafts) > 8000 {
		return nil, fmt.Errorf("한 번에 8,000행까지입니다")
	}
	n, err := NextSeq(r.db, "as_import_batch")
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("IMP%03d", n)
	now := time.Now().Format("2006-01-02 15:04:05")
	idx := r.loadImportIndex()
	if _, err := r.db.Exec(`INSERT INTO as_import_batches(batch_id, filename, status, created_at, created_by)
		VALUES (?,?,?,?,?)`, id, filename, model.ImportStatusReported, now, createdBy); err != nil {
		return nil, err
	}
	for i := range drafts {
		drafts[i].BatchID = id
		r.validateImportRow(&drafts[i], idx, drafts[:i])
		if err := r.insertImportRow(drafts[i]); err != nil {
			return nil, err
		}
	}
	if err := r.refreshBatchStats(id); err != nil {
		return nil, err
	}
	return r.GetBatch(id)
}

func (r *ASImportRepo) insertImportRow(row model.ASImportRow) error {
	excl := 0
	if row.Excluded {
		excl = 1
	}
	_, err := r.db.Exec(`INSERT INTO as_import_rows(
		batch_id, row_no, excluded, issues, customer_raw, customer_id, asset_raw, asset_id,
		receipt_raw, receipt_date, process_raw, process_date, visit_date, symptom, action,
		channel, requester, worker, worker_uid, category, model, applied_as_id)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		row.BatchID, row.RowNo, excl, model.JoinIssueCodes(row.Issues), row.CustomerRaw, row.CustomerID,
		row.AssetRaw, row.AssetID, row.ReceiptRaw, row.ReceiptDate, row.ProcessRaw, row.ProcessDate,
		row.VisitDate, row.Symptom, row.Action, row.Channel, row.Requester, row.Worker, row.WorkerUID,
		row.Category, row.Model, row.AppliedASID)
	return err
}

type importIndex struct {
	customers     map[string]string
	validCustomer map[string]bool
	users         map[string]string
	assets        map[string][]string
	existing      map[string]bool
}

func (r *ASImportRepo) loadImportIndex() importIndex {
	idx := importIndex{
		customers:     map[string]string{},
		validCustomer: map[string]bool{},
		users:         map[string]string{},
		assets:        map[string][]string{},
		existing:      map[string]bool{},
	}
	if rows, err := r.db.Query(`SELECT customer_id, COALESCE(org_name,''), COALESCE(official_name,'') FROM customers`); err == nil {
		for rows.Next() {
			var id, org, official string
			if rows.Scan(&id, &org, &official) == nil {
				idx.validCustomer[id] = true
				for _, n := range []string{org, official} {
					k := normalizeImportName(n)
					if k != "" {
						if _, ok := idx.customers[k]; !ok {
							idx.customers[k] = id
						}
					}
				}
			}
		}
		rows.Close()
	}
	if rows, err := r.db.Query(`SELECT customer_id, COALESCE(short_name,'') FROM maintenance_site_config`); err == nil {
		for rows.Next() {
			var id, name string
			if rows.Scan(&id, &name) == nil {
				k := normalizeImportName(name)
				if k != "" {
					if _, ok := idx.customers[k]; !ok {
						idx.customers[k] = id
					}
				}
			}
		}
		rows.Close()
	}
	if rows, err := r.db.Query(`SELECT user_id, full_name FROM users WHERE is_active=1`); err == nil {
		for rows.Next() {
			var id, name string
			if rows.Scan(&id, &name) == nil {
				idx.users[strings.TrimSpace(name)] = id
			}
		}
		rows.Close()
	}
	if rows, err := r.db.Query(`SELECT customer_id, COALESCE(model_name,''), COALESCE(product_name,''), asset_id FROM assets`); err == nil {
		for rows.Next() {
			var cid, modelName, product, aid string
			if rows.Scan(&cid, &modelName, &product, &aid) == nil {
				for _, k := range []string{normalizeImportName(modelName), normalizeImportName(product)} {
					if k == "" {
						continue
					}
					key := cid + "|" + k
					if !containsString(idx.assets[key], aid) {
						idx.assets[key] = append(idx.assets[key], aid)
					}
				}
			}
		}
		rows.Close()
	}
	if rows, err := r.db.Query(`SELECT customer_id, DATE(receipt_datetime), COALESCE(symptom,'') FROM as_receipts`); err == nil {
		for rows.Next() {
			var cid, d, sym string
			if rows.Scan(&cid, &d, &sym) == nil {
				idx.existing[cid+"|"+d+"|"+normalizeImportName(sym)] = true
			}
		}
		rows.Close()
	}
	return idx
}

func (r *ASImportRepo) validateImportRow(row *model.ASImportRow, idx importIndex, prior []model.ASImportRow) {
	row.Issues = nil
	if strings.TrimSpace(row.CustomerRaw) == "" || strings.TrimSpace(row.Symptom) == "" || strings.TrimSpace(row.ReceiptRaw) == "" {
		row.Issues = appendIssue(row.Issues, model.ImportIssueMissing)
	}
	if strings.TrimSpace(row.ReceiptRaw) != "" {
		if d, ok := parseImportDateOK(row.ReceiptRaw); ok {
			row.ReceiptDate = d
		} else {
			row.ReceiptDate = ""
			row.Issues = appendIssue(row.Issues, model.ImportIssueDate)
		}
	}
	if strings.TrimSpace(row.ProcessRaw) != "" {
		if d, ok := parseImportDateOK(row.ProcessRaw); ok {
			row.ProcessDate = d
		} else {
			row.Issues = appendIssue(row.Issues, model.ImportIssueDate)
		}
	}
	if strings.TrimSpace(row.VisitDate) != "" && model.NormalizeAppDate(row.VisitDate) == "" {
		row.Issues = appendIssue(row.Issues, model.ImportIssueDate)
		row.VisitDate = ""
	}

	name := row.CustomerRaw
	if alias, ok := importCustomerAliases[normalizeImportName(name)]; ok {
		name = alias
	}
	if row.CustomerID == "" {
		row.CustomerID = idx.customers[normalizeImportName(name)]
	}
	if row.CustomerID == "" || !idx.validCustomer[row.CustomerID] {
		row.CustomerID = ""
		row.Issues = appendIssue(row.Issues, model.ImportIssueCustomer)
	}

	if row.Worker != "" {
		row.WorkerUID = idx.users[row.Worker]
	}
	if row.CustomerID != "" && row.Model != "" && row.AssetID == "" {
		ids := idx.assets[row.CustomerID+"|"+normalizeImportName(row.Model)]
		switch {
		case len(ids) == 1:
			row.AssetID = ids[0]
		default:
			row.Issues = appendIssue(row.Issues, model.ImportIssueAsset)
		}
	}

	dupKey := row.CustomerID + "|" + row.ReceiptDate + "|" + normalizeImportName(row.Symptom)
	if row.CustomerID != "" && row.ReceiptDate != "" && idx.existing[dupKey] {
		row.Issues = appendIssue(row.Issues, model.ImportIssueDuplicate)
	}
	for _, p := range prior {
		if p.Excluded {
			continue
		}
		pk := p.CustomerID + "|" + p.ReceiptDate + "|" + normalizeImportName(p.Symptom)
		if row.CustomerID != "" && pk == dupKey {
			row.Issues = appendIssue(row.Issues, model.ImportIssueDuplicate)
			break
		}
	}
}

func appendIssue(list []string, code string) []string {
	for _, x := range list {
		if x == code {
			return list
		}
	}
	return append(list, code)
}

func (r *ASImportRepo) refreshBatchStats(id string) error {
	rows, err := r.ListRows(id)
	if err != nil {
		return err
	}
	errN, warnN := 0, 0
	for _, row := range rows {
		if row.HasBlocker() {
			errN++
		} else if row.HasWarning() {
			warnN++
		}
	}
	_, err = r.db.Exec(`UPDATE as_import_batches SET total_rows=?, error_rows=?, warning_rows=? WHERE batch_id=?`,
		len(rows), errN, warnN, id)
	return err
}

func (r *ASImportRepo) SetExcluded(batchID string, rowNo int, excluded bool) error {
	b, err := r.GetBatch(batchID)
	if err != nil || b == nil {
		return fmt.Errorf("배치 없음")
	}
	if b.Status != model.ImportStatusReported {
		return fmt.Errorf("검증 중인 배치만 고칠 수 있습니다")
	}
	v := 0
	if excluded {
		v = 1
	}
	if _, err := r.db.Exec(`UPDATE as_import_rows SET excluded=? WHERE batch_id=? AND row_no=?`, v, batchID, rowNo); err != nil {
		return err
	}
	return r.revalidateBatch(batchID)
}

func (r *ASImportRepo) PatchRow(batchID string, rowNo int, customerID, receiptDate, visitDate, symptom string) error {
	b, err := r.GetBatch(batchID)
	if err != nil || b == nil {
		return fmt.Errorf("배치 없음")
	}
	if b.Status != model.ImportStatusReported {
		return fmt.Errorf("검증 중인 배치만 고칠 수 있습니다")
	}
	rows, err := r.ListRows(batchID)
	if err != nil {
		return err
	}
	idx := r.loadImportIndex()
	var cur *model.ASImportRow
	var prior []model.ASImportRow
	for i := range rows {
		if rows[i].RowNo == rowNo {
			cur = &rows[i]
			prior = rows[:i]
			break
		}
	}
	if cur == nil {
		return fmt.Errorf("행 없음")
	}
	if customerID != "" {
		cur.CustomerID = customerID
	}
	if receiptDate != "" {
		cur.ReceiptRaw = receiptDate
		cur.ReceiptDate = receiptDate
	}
	if visitDate != "" {
		cur.VisitDate = visitDate
	}
	if symptom != "" {
		cur.Symptom = symptom
	}
	r.validateImportRow(cur, idx, prior)
	if _, err := r.db.Exec(`UPDATE as_import_rows SET customer_id=?, receipt_raw=?, receipt_date=?, visit_date=?, symptom=?, asset_id=?, issues=?, worker_uid=?
		WHERE batch_id=? AND row_no=?`,
		cur.CustomerID, cur.ReceiptRaw, cur.ReceiptDate, cur.VisitDate, cur.Symptom, cur.AssetID,
		model.JoinIssueCodes(cur.Issues), cur.WorkerUID, batchID, rowNo); err != nil {
		return err
	}
	return r.revalidateBatch(batchID)
}

func (r *ASImportRepo) revalidateBatch(id string) error {
	rows, err := r.ListRows(id)
	if err != nil {
		return err
	}
	idx := r.loadImportIndex()
	var kept []model.ASImportRow
	for i := range rows {
		r.validateImportRow(&rows[i], idx, kept)
		if _, err := r.db.Exec(`UPDATE as_import_rows SET customer_id=?, receipt_date=?, process_date=?, visit_date=?, asset_id=?, issues=?, worker_uid=?
			WHERE batch_id=? AND row_no=?`,
			rows[i].CustomerID, rows[i].ReceiptDate, rows[i].ProcessDate, rows[i].VisitDate, rows[i].AssetID,
			model.JoinIssueCodes(rows[i].Issues), rows[i].WorkerUID, id, rows[i].RowNo); err != nil {
			return err
		}
		if !rows[i].Excluded {
			kept = append(kept, rows[i])
		}
	}
	return r.refreshBatchStats(id)
}

// ApplyBatch 검증을 통과한 행만 반영한다. data_origin=import, 키워드는 확정하지 않는다. §12.11.7
func (r *ASImportRepo) ApplyBatch(batchID string) (int, error) {
	b, err := r.GetBatch(batchID)
	if err != nil {
		return 0, err
	}
	if b == nil || b.Status != model.ImportStatusReported {
		return 0, ErrImportNotReported
	}
	rows, err := r.ListRows(batchID)
	if err != nil {
		return 0, err
	}
	if err := r.refreshBatchStats(batchID); err != nil {
		return 0, err
	}
	b, _ = r.GetBatch(batchID)
	if b == nil || b.ErrorRows > 0 {
		return 0, ErrImportHasErrors
	}
	var applied []string
	rollback := func() {
		as := NewASRepo(r.db)
		for _, id := range applied {
			_ = as.Delete(id)
		}
	}
	kw := NewASKeywordRepo(r.db)
	now := time.Now().Format("2006-01-02 15:04:05")
	n := 0
	for _, row := range rows {
		if row.Excluded {
			continue
		}
		if row.HasBlocker() {
			rollback()
			return 0, ErrImportHasErrors
		}
		asID, err := r.insertImportedReceipt(batchID, row)
		if err != nil {
			rollback()
			return 0, err
		}
		applied = append(applied, asID)
		if _, err := r.db.Exec(`UPDATE as_import_rows SET applied_as_id=? WHERE batch_id=? AND row_no=?`, asID, batchID, row.RowNo); err != nil {
			rollback()
			return 0, err
		}
		// 사전 대조 후보만 계산한다. 링크는 만들지 않는다. §12.11.7
		_, _ = kw.Suggest(row.Symptom + " " + row.Action)
		n++
	}
	if n == 0 {
		return 0, fmt.Errorf("반영할 행이 없습니다")
	}
	if _, err := r.db.Exec(`UPDATE as_import_batches SET status=?, applied_at=?, applied_rows=? WHERE batch_id=?`,
		model.ImportStatusApplied, now, n, batchID); err != nil {
		rollback()
		return 0, err
	}
	return n, nil
}

func (r *ASImportRepo) insertImportedReceipt(batchID string, row model.ASImportRow) (string, error) {
	receiptAt, err := time.ParseInLocation("2006-01-02", row.ReceiptDate, time.Local)
	if err != nil {
		return "", fmt.Errorf("%d행 접수일: %w", row.RowNo, err)
	}
	receiptAt = time.Date(receiptAt.Year(), receiptAt.Month(), receiptAt.Day(), 9, 0, 0, 0, time.Local)
	if err := model.RequireAppDateYear(row.ReceiptDate); err != nil {
		return "", err
	}
	asNumber, err := NextASNumber(r.db, receiptAt)
	if err != nil {
		return "", err
	}
	visit := model.NormalizeAppDate(row.VisitDate)
	confirmed := 0
	noDateReason := ""
	if visit == "" {
		noDateReason = model.ImportNoDateReason
	} else {
		confirmed = 1
	}
	status := "received"
	var complete interface{}
	start := receiptAt.Format("2006-01-02 15:04:05")
	action := strings.TrimSpace(row.Action)
	if action != "" || row.ProcessDate != "" {
		status = "completed"
		cd := row.ProcessDate
		if cd == "" {
			cd = row.ReceiptDate
		}
		complete = cd + " 18:00:00"
		if t, err := time.ParseInLocation("2006-01-02", cd, time.Local); err == nil {
			start = time.Date(t.Year(), t.Month(), t.Day(), 9, 0, 0, 0, time.Local).Format("2006-01-02 15:04:05")
		}
	}
	projectID := resolveStoredProjectID(r.db, row.CustomerID, row.Model+" "+row.Category+" "+row.Symptom, model.ScopeWorkAS)
	unlinked := ""
	if row.AssetID == "" && (row.Model != "" || row.Category != "") {
		unlinked = strings.TrimSpace(row.Category + " / " + row.Model)
	}
	importKey := fmt.Sprintf("%s:%d", batchID, row.RowNo)
	now := time.Now().Format("2006-01-02 15:04:05")
	_, err = r.db.Exec(`INSERT INTO as_receipts (
			as_id, as_number, receipt_datetime, customer_id, asset_id,
			receipt_channel, requester, requester_name, symptom,
			urgency, priority, assigned_to, assigned_user_id,
			visit_scheduled_date, schedule_confirmed, schedule_no_date_reason,
			status, start_datetime, complete_datetime, action_taken, result_code,
			data_origin, import_key, import_batch_id, asset_unlinked_reason, project_id,
			created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		asNumber, asNumber, receiptAt.Format("2006-01-02 15:04:05"), row.CustomerID, nullStr(row.AssetID),
		row.Channel, row.Requester, row.Requester, row.Symptom,
		"normal", "normal", row.Worker, nullStr(row.WorkerUID),
		nullStr(visit), confirmed, nullStr(noDateReason),
		status, start, complete, action, doneResult(status),
		"import", importKey, batchID, nullStr(unlinked), nullStr(projectID),
		now, now)
	if err != nil {
		return "", fmt.Errorf("%d행 접수: %w", row.RowNo, err)
	}
	if action != "" {
		procAt := start
		if s, ok := complete.(string); ok && s != "" {
			procAt = s
		}
		t, _ := time.ParseInLocation("2006-01-02 15:04:05", procAt, time.Local)
		if t.IsZero() {
			t = receiptAt
		}
		procNum, err := NextProcessNumber(r.db, asNumber, t)
		if err != nil {
			return "", err
		}
		if _, err := r.db.Exec(`INSERT INTO as_processes
			(process_id, process_number, as_id, process_datetime, worker, work_content, time_spent, notes)
			VALUES (?,?,?,?,?,?,?,?)`,
			procNum, procNum, asNumber, procAt, row.Worker, action, 30, unlinked); err != nil {
			return "", fmt.Errorf("%d행 조치: %w", row.RowNo, err)
		}
	}
	reindexASSearch(r.db, asNumber)
	return asNumber, nil
}

func doneResult(status string) interface{} {
	if status == "completed" {
		return "done"
	}
	return nil
}

// CancelBatch 배치로 반영한 접수를 한꺼번에 지운다. §12.11.7 · §12.11.9-8
func (r *ASImportRepo) CancelBatch(batchID string) (int, error) {
	b, err := r.GetBatch(batchID)
	if err != nil {
		return 0, err
	}
	if b == nil || b.Status != model.ImportStatusApplied {
		return 0, ErrImportNotApplied
	}
	ids := []string{}
	q, err := r.db.Query(`SELECT as_id FROM as_receipts WHERE import_batch_id=?`, batchID)
	if err != nil {
		return 0, err
	}
	for q.Next() {
		var id string
		if q.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	q.Close()
	as := NewASRepo(r.db)
	for _, id := range ids {
		if err := as.Delete(id); err != nil {
			return 0, err
		}
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	if _, err := r.db.Exec(`UPDATE as_import_rows SET applied_as_id='' WHERE batch_id=?`, batchID); err != nil {
		return 0, err
	}
	if _, err := r.db.Exec(`UPDATE as_import_batches SET status=?, cancelled_at=?, applied_rows=0 WHERE batch_id=?`,
		model.ImportStatusCancelled, now, batchID); err != nil {
		return 0, err
	}
	return len(ids), nil
}

func normalizeImportName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\u00a0", "")
	s = strings.ReplaceAll(s, "(", "")
	s = strings.ReplaceAll(s, ")", "")
	s = strings.ReplaceAll(s, "·", "")
	return s
}

func containsString(list []string, v string) bool {
	for _, e := range list {
		if e == v {
			return true
		}
	}
	return false
}

func parseImportDate(s string) string {
	d, ok := parseImportDateOK(s)
	if !ok {
		return ""
	}
	return d
}

func parseImportDateOK(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 20000 && n < 80000 {
		t := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).Add(time.Duration(n * float64(24*time.Hour)))
		out := t.Format("2006-01-02")
		if model.NormalizeAppDate(out) == "" {
			return "", false
		}
		return out, true
	}
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	if len(s) >= 10 && s[4] == '-' {
		if out := model.NormalizeAppDate(s[:10]); out != "" {
			return out, true
		}
		if _, err := time.Parse("2006-01-02", s[:10]); err == nil {
			return "", false
		}
	}
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return "", false
	}
	for _, p := range parts {
		if p == "" || !isDigits(p) {
			return "", false
		}
	}
	if len(parts[0]) == 4 {
		out := fmt.Sprintf("%s-%s-%s", parts[0], padImport2(parts[1]), padImport2(parts[2]))
		if model.NormalizeAppDate(out) == "" {
			return "", false
		}
		return out, true
	}
	mm, dd, yy := padImport2(parts[0]), padImport2(parts[1]), parts[2]
	if len(yy) == 2 {
		yy = "20" + yy
	}
	if mm > "12" || dd > "31" {
		return "", false
	}
	out := fmt.Sprintf("%s-%s-%s", yy, mm, dd)
	if model.NormalizeAppDate(out) == "" {
		return "", false
	}
	return out, true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func padImport2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}
