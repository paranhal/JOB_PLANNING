package repository

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"customer-support/internal/model"
)

type QuoteRepo struct {
	db *sql.DB
}

func NewQuoteRepo(db *sql.DB) *QuoteRepo {
	return &QuoteRepo{db: db}
}

const salesQuoteSelect = `
	SELECT quote_id, COALESCE(sales_id,''), quote_no, COALESCE(rev,0),
		COALESCE(form_type,'A'), COALESCE(recipient_kind,'customer'),
		COALESCE(customer_id,''), COALESCE(recipient_name,''),
		COALESCE(attn_name,''), COALESCE(attn_title,''), quote_date,
		COALESCE(title,''), COALESCE(valid_until_text,''), COALESCE(due_text,''),
		COALESCE(place_text,''), COALESCE(payment_text,''),
		COALESCE(vat_mode,'excluded'), COALESCE(round_rule,'none'),
		COALESCE(subtotal,0), COALESCE(vat,0), COALESCE(total,0),
		COALESCE(owner_user_id,''), COALESCE(owner_name,''), COALESCE(owner_phone,''),
		COALESCE(remarks,''), COALESCE(purpose,'deal'), COALESCE(budget_year,0),
		COALESCE(overhead_rate,0), COALESCE(tech_fee_rate,0),
		COALESCE(status,'draft'), COALESCE(is_legacy,0),
		COALESCE(rev_reason,''), COALESCE(is_reverse_calc,0), COALESCE(target_total,0),
		COALESCE(maint_block,0),
		COALESCE(created_at,''), COALESCE(updated_at,'')
	FROM sales_quotes`

type QuoteFilter struct {
	Search  string
	Status  string
	SalesID string
	Purpose string
}

func (r *QuoteRepo) List(f QuoteFilter) ([]model.SalesQuote, error) {
	q := salesQuoteSelect + ` WHERE 1=1`
	var args []interface{}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + s + "%"
		q += ` AND (quote_no LIKE ? OR COALESCE(title,'') LIKE ? OR COALESCE(recipient_name,'') LIKE ?)`
		args = append(args, like, like, like)
	}
	if s := strings.TrimSpace(f.Status); s != "" {
		q += ` AND status=?`
		args = append(args, model.NormalizeQuoteStatus(s))
	}
	if s := strings.TrimSpace(f.SalesID); s != "" {
		q += ` AND sales_id=?`
		args = append(args, s)
	}
	if s := strings.TrimSpace(f.Purpose); s != "" {
		q += ` AND purpose=?`
		args = append(args, model.NormalizeQuotePurpose(s))
	}
	q += ` ORDER BY quote_date DESC, quote_no DESC, rev DESC`
	rows, err := r.db.Query(q, args...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	items, err := scanQuotes(rows)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Lines, _ = r.listLines(items[i].QuoteID)
	}
	return items, nil
}

func (r *QuoteRepo) Get(id string) (*model.SalesQuote, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRow(salesQuoteSelect+` WHERE quote_id=?`, id)
	q, err := scanQuote(row)
	if err != nil {
		return nil, err
	}
	q.Lines, err = r.listLines(q.QuoteID)
	if err != nil {
		return nil, err
	}
	return q, nil
}

func (r *QuoteRepo) LastRoundRule() string {
	v, _ := NewSettingsRepo(r.db).Get(SettingQuoteLastRound)
	return model.NormalizeRoundRule(v)
}

func (r *QuoteRepo) Create(q *model.SalesQuote) error {
	if q == nil {
		return fmt.Errorf("견적이 필요합니다")
	}
	if err := r.prepareSave(q, true); err != nil {
		return err
	}
	n, err := NextSeq(r.db, "sales_quotes")
	if err != nil {
		return err
	}
	q.QuoteID = fmt.Sprintf("Q-%03d", n)
	_, err = r.db.Exec(`
		INSERT INTO sales_quotes (
			quote_id, sales_id, quote_no, rev, form_type, recipient_kind,
			customer_id, recipient_name, attn_name, attn_title, quote_date, title,
			valid_until_text, due_text, place_text, payment_text, vat_mode, round_rule,
			subtotal, vat, total, owner_user_id, owner_name, owner_phone, remarks,
			purpose, budget_year, overhead_rate, tech_fee_rate, status, is_legacy,
			rev_reason, is_reverse_calc, target_total, maint_block
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		q.QuoteID, q.SalesID, q.QuoteNo, q.Rev, q.FormType, q.RecipientKind,
		q.CustomerID, q.RecipientName, q.AttnName, q.AttnTitle, q.QuoteDate, q.Title,
		q.ValidUntilText, q.DueText, q.PlaceText, q.PaymentText, q.VATMode, q.RoundRule,
		q.Subtotal, q.VAT, q.Total, q.OwnerUserID, q.OwnerName, q.OwnerPhone, q.Remarks,
		q.Purpose, q.BudgetYear, q.OverheadRate, q.TechFeeRate, q.Status, boolToInt(q.IsLegacy),
		q.RevReason, boolToInt(q.IsReverseCalc), q.TargetTotal, boolToInt(q.MaintBlock))
	if err != nil {
		if isUniqueErr(err) {
			return model.ErrQuoteNoDuplicate
		}
		return err
	}
	if err := r.replaceLines(q); err != nil {
		return err
	}
	_ = NewSettingsRepo(r.db).Set(SettingQuoteLastRound, q.RoundRule)
	logCreate(r.db, "sales_quotes", "quote_id", q.QuoteID, q.QuoteNo)
	return nil
}

func (r *QuoteRepo) Update(q *model.SalesQuote) error {
	if q == nil || strings.TrimSpace(q.QuoteID) == "" {
		return fmt.Errorf("quote_id 필요")
	}
	cur, err := r.Get(q.QuoteID)
	if err != nil {
		return err
	}
	latest, err := r.IsLatest(cur)
	if err != nil {
		return err
	}
	if !latest {
		return model.ErrQuoteReadOnly
	}
	q.QuoteNo = cur.QuoteNo
	q.QuoteDate = cur.QuoteDate
	q.Rev = cur.Rev
	q.IsLegacy = cur.IsLegacy
	q.RevReason = cur.RevReason
	if !q.IsReverseCalc {
		q.IsReverseCalc = cur.IsReverseCalc
		if q.TargetTotal == 0 {
			q.TargetTotal = cur.TargetTotal
		}
	}
	if err := r.prepareSave(q, false); err != nil {
		return err
	}
	return touchUpdate(r.db, "sales_quotes", "quote_id", q.QuoteID, q.QuoteNo, func() error {
		_, err := r.db.Exec(`
			UPDATE sales_quotes SET
				sales_id=?, form_type=?, recipient_kind=?, customer_id=?, recipient_name=?,
				attn_name=?, attn_title=?, title=?, valid_until_text=?, due_text=?, place_text=?,
				payment_text=?, vat_mode=?, round_rule=?, subtotal=?, vat=?, total=?,
				owner_user_id=?, owner_name=?, owner_phone=?, remarks=?, purpose=?, budget_year=?,
				overhead_rate=?, tech_fee_rate=?, status=?, is_reverse_calc=?, target_total=?,
				maint_block=?, updated_at=CURRENT_TIMESTAMP
			WHERE quote_id=?`,
			q.SalesID, q.FormType, q.RecipientKind, q.CustomerID, q.RecipientName,
			q.AttnName, q.AttnTitle, q.Title, q.ValidUntilText, q.DueText, q.PlaceText,
			q.PaymentText, q.VATMode, q.RoundRule, q.Subtotal, q.VAT, q.Total,
			q.OwnerUserID, q.OwnerName, q.OwnerPhone, q.Remarks, q.Purpose, q.BudgetYear,
			q.OverheadRate, q.TechFeeRate, q.Status, boolToInt(q.IsReverseCalc), q.TargetTotal,
			boolToInt(q.MaintBlock), q.QuoteID)
		if err != nil {
			return err
		}
		if err := r.replaceLines(q); err != nil {
			return err
		}
		_ = NewSettingsRepo(r.db).Set(SettingQuoteLastRound, q.RoundRule)
		return nil
	})
}

func (r *QuoteRepo) CopyAsNew(id, today string) (*model.SalesQuote, error) {
	src, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	dst := *src
	dst.QuoteID = ""
	dst.QuoteNo = ""
	dst.Rev = 0
	dst.IsLegacy = false
	dst.Status = model.QuoteStatusDraft
	dst.QuoteDate = today
	dst.Lines = append([]model.SalesQuoteLine(nil), src.Lines...)
	for i := range dst.Lines {
		dst.Lines[i].LineID = ""
		dst.Lines[i].QuoteID = ""
	}
	if err := r.Create(&dst); err != nil {
		return nil, err
	}
	return &dst, nil
}

func (r *QuoteRepo) Revise(id, reason string) (*model.SalesQuote, error) {
	src, err := r.Get(id)
	if err != nil {
		return nil, err
	}
	latest, err := r.IsLatest(src)
	if err != nil {
		return nil, err
	}
	if !latest {
		return nil, model.ErrQuoteReadOnly
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, model.ErrQuoteRevReason
	}
	dst := *src
	dst.QuoteID = ""
	dst.Rev = src.Rev + 1
	dst.RevReason = reason
	dst.Status = model.QuoteStatusDraft
	dst.Lines = append([]model.SalesQuoteLine(nil), src.Lines...)
	for i := range dst.Lines {
		dst.Lines[i].LineID = ""
		dst.Lines[i].QuoteID = ""
	}
	if err := r.prepareSave(&dst, false); err != nil {
		return nil, err
	}
	n, err := NextSeq(r.db, "sales_quotes")
	if err != nil {
		return nil, err
	}
	dst.QuoteID = fmt.Sprintf("Q-%03d", n)
	_, err = r.db.Exec(`
		INSERT INTO sales_quotes (
			quote_id, sales_id, quote_no, rev, form_type, recipient_kind,
			customer_id, recipient_name, attn_name, attn_title, quote_date, title,
			valid_until_text, due_text, place_text, payment_text, vat_mode, round_rule,
			subtotal, vat, total, owner_user_id, owner_name, owner_phone, remarks,
			purpose, budget_year, overhead_rate, tech_fee_rate, status, is_legacy,
			rev_reason, is_reverse_calc, target_total, maint_block
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		dst.QuoteID, dst.SalesID, dst.QuoteNo, dst.Rev, dst.FormType, dst.RecipientKind,
		dst.CustomerID, dst.RecipientName, dst.AttnName, dst.AttnTitle, dst.QuoteDate, dst.Title,
		dst.ValidUntilText, dst.DueText, dst.PlaceText, dst.PaymentText, dst.VATMode, dst.RoundRule,
		dst.Subtotal, dst.VAT, dst.Total, dst.OwnerUserID, dst.OwnerName, dst.OwnerPhone, dst.Remarks,
		dst.Purpose, dst.BudgetYear, dst.OverheadRate, dst.TechFeeRate, dst.Status, boolToInt(dst.IsLegacy),
		dst.RevReason, boolToInt(dst.IsReverseCalc), dst.TargetTotal, boolToInt(dst.MaintBlock))
	if err != nil {
		if isUniqueErr(err) {
			return nil, model.ErrQuoteNoDuplicate
		}
		return nil, err
	}
	if err := r.replaceLines(&dst); err != nil {
		return nil, err
	}
	logCreate(r.db, "sales_quotes", "quote_id", dst.QuoteID, dst.DisplayNo())
	return &dst, nil
}

func (r *QuoteRepo) IsLatest(q *model.SalesQuote) (bool, error) {
	if q == nil {
		return false, fmt.Errorf("견적이 필요합니다")
	}
	var max int
	err := r.db.QueryRow(`SELECT COALESCE(MAX(rev),0) FROM sales_quotes WHERE quote_no=?`, q.QuoteNo).Scan(&max)
	if err != nil {
		return false, err
	}
	return q.Rev >= max, nil
}

func (r *QuoteRepo) ListByQuoteNo(no string) ([]model.SalesQuote, error) {
	no = strings.TrimSpace(no)
	if no == "" {
		return nil, nil
	}
	rows, err := r.db.Query(salesQuoteSelect+` WHERE quote_no=? ORDER BY rev`, no)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanQuotes(rows)
}

func (r *QuoteRepo) NonDealQuoteSalesIDs() (map[string]bool, error) {
	rows, err := r.db.Query(`
		SELECT sales_id FROM sales_quotes
		WHERE TRIM(COALESCE(sales_id,'')) != ''
		GROUP BY sales_id
		HAVING SUM(CASE WHEN purpose='deal' THEN 1 ELSE 0 END)=0`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		id = strings.TrimSpace(id)
		if id != "" {
			out[id] = true
		}
	}
	return out, rows.Err()
}

func (r *QuoteRepo) ListBudgetFollowups(today time.Time) ([]model.SalesQuote, error) {
	items, err := r.List(QuoteFilter{Purpose: model.QuotePurposeBudget})
	if err != nil {
		return nil, err
	}
	var out []model.SalesQuote
	for _, q := range items {
		if model.BudgetFollowupActive(q.BudgetYear, today) {
			out = append(out, q)
		}
	}
	return out, nil
}

func (r *QuoteRepo) SetStatus(id, status string) error {
	q, err := r.Get(id)
	if err != nil {
		return err
	}
	q.Status = model.NormalizeQuoteStatus(status)
	return r.Update(q)
}

func (r *QuoteRepo) QuoteNoTaken(no string, rev int, exceptID string) (bool, error) {
	no = strings.TrimSpace(no)
	var n int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM sales_quotes WHERE quote_no=? AND rev=? AND quote_id!=?`,
		no, rev, strings.TrimSpace(exceptID)).Scan(&n)
	return n > 0, err
}

func (r *QuoteRepo) nextSeqForDay(iso string) (int, error) {
	prefix := model.FormatQuoteNo(iso, 1)
	if prefix == "" {
		return 0, fmt.Errorf("견적일이 올바르지 않습니다")
	}
	dayPart := strings.TrimSuffix(prefix, "-001")
	like := dayPart + "-%"
	rows, err := r.db.Query(`SELECT quote_no FROM sales_quotes WHERE quote_no LIKE ?`, like)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	max := 0
	for rows.Next() {
		var no string
		if err := rows.Scan(&no); err != nil {
			return 0, err
		}
		i := strings.LastIndex(no, "-")
		if i < 0 {
			continue
		}
		var seq int
		fmt.Sscanf(no[i+1:], "%d", &seq)
		if seq > max {
			max = seq
		}
	}
	return max + 1, rows.Err()
}

func (r *QuoteRepo) prepareSave(q *model.SalesQuote, allocNo bool) error {
	q.FormType = model.NormalizeQuoteForm(q.FormType)
	q.VATMode = model.NormalizeVATMode(q.VATMode)
	q.RoundRule = model.NormalizeRoundRule(q.RoundRule)
	q.Status = model.NormalizeQuoteStatus(q.Status)
	q.Purpose = model.NormalizeQuotePurpose(q.Purpose)
	q.RecipientKind = strings.TrimSpace(q.RecipientKind)
	if q.RecipientKind == "" {
		q.RecipientKind = model.QuoteRecipientCustomer
	}
	if err := q.ValidatePurpose(); err != nil {
		return err
	}
	if q.OverheadRate <= 0 || q.TechFeeRate <= 0 {
		oh, tech := r.StandardRates()
		if q.OverheadRate <= 0 {
			q.OverheadRate = oh
		}
		if q.TechFeeRate <= 0 {
			q.TechFeeRate = tech
		}
	}
	if err := q.ValidateOwner(); err != nil {
		return err
	}
	if allocNo && !q.IsLegacy {
		if strings.TrimSpace(q.QuoteDate) == "" {
			q.QuoteDate = time.Now().Format("2006-01-02")
		}
		if _, err := model.ParseQuoteDateISO(q.QuoteDate); err != nil {
			return err
		}
		seq, err := r.nextSeqForDay(q.QuoteDate)
		if err != nil {
			return err
		}
		q.QuoteNo = model.FormatQuoteNo(q.QuoteDate, seq)
		q.Rev = 0
	}
	if _, err := model.ParseQuoteDateISO(q.QuoteDate); err != nil {
		return err
	}
	if err := model.AssertQuoteNoMatchesDate(q.QuoteNo, q.QuoteDate, q.IsLegacy); err != nil {
		return err
	}
	taken, err := r.QuoteNoTaken(q.QuoteNo, q.Rev, q.QuoteID)
	if err != nil {
		return err
	}
	if taken {
		return model.ErrQuoteNoDuplicate
	}
	for i := range q.Lines {
		if strings.TrimSpace(q.Lines[i].RateID) == "" {
			continue
		}
		rate, err := r.GetLaborRate(q.Lines[i].RateID)
		if err != nil || rate == nil {
			continue
		}
		if q.Lines[i].LaborYear == 0 {
			q.Lines[i].LaborYear = rate.Year
		}
		q.Lines[i].MarkPriceOverride(rate.Monthly)
	}
	if err := model.ApplyQuoteTotals(q, q.Lines); err != nil {
		return err
	}
	return nil
}

func (r *QuoteRepo) replaceLines(q *model.SalesQuote) error {
	if _, err := r.db.Exec(`DELETE FROM sales_quote_lines WHERE quote_id=?`, q.QuoteID); err != nil {
		return err
	}
	for i := range q.Lines {
		ln := &q.Lines[i]
		n, err := NextSeq(r.db, "sales_quote_lines")
		if err != nil {
			return err
		}
		ln.LineID = fmt.Sprintf("QL-%03d", n)
		ln.QuoteID = q.QuoteID
		ln.Seq = i + 1
		ln.Amount = model.LineAmount(*ln)
		if _, err := r.db.Exec(`
			INSERT INTO sales_quote_lines (
				line_id, quote_id, seq, item_id, group_label, name, spec, qty, unit,
				unit_price, amount, gov_price, mm_rate, discount_rate, note,
				rate_id, labor_year, price_overridden
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			ln.LineID, ln.QuoteID, ln.Seq, ln.ItemID, ln.GroupLabel, ln.Name, ln.Spec,
			ln.Qty, ln.Unit, ln.UnitPrice, ln.Amount, ln.GovPrice, ln.MMRate, ln.DiscountRate, ln.Note,
			ln.RateID, ln.LaborYear, boolToInt(ln.PriceOverridden)); err != nil {
			return err
		}
	}
	var sum int
	if err := r.db.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM sales_quote_lines WHERE quote_id=?`, q.QuoteID).Scan(&sum); err != nil {
		return err
	}
	want := q.Subtotal
	if model.QuoteFormIsLabor(q.FormType) {
		want = 0
		for i := range q.Lines {
			want += model.LineAmount(q.Lines[i])
		}
	}
	if sum != want {
		return model.ErrQuoteLineSum
	}
	if model.QuoteFormIsGrouped(q.FormType) {
		if err := model.AssertGroupSumEquals(q.Lines, sum); err != nil {
			return err
		}
	}
	return nil
}

func (r *QuoteRepo) listLines(quoteID string) ([]model.SalesQuoteLine, error) {
	rows, err := r.db.Query(`
		SELECT line_id, quote_id, seq, COALESCE(item_id,''), COALESCE(group_label,''),
			name, COALESCE(spec,''), COALESCE(qty,0), COALESCE(unit,'EA'),
			COALESCE(unit_price,0), COALESCE(amount,0), COALESCE(gov_price,0),
			COALESCE(mm_rate,0), COALESCE(discount_rate,0), COALESCE(note,''),
			COALESCE(rate_id,''), COALESCE(labor_year,0), COALESCE(price_overridden,0)
		FROM sales_quote_lines WHERE quote_id=? ORDER BY seq, line_id`, quoteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []model.SalesQuoteLine
	for rows.Next() {
		var ln model.SalesQuoteLine
		var over int
		if err := rows.Scan(&ln.LineID, &ln.QuoteID, &ln.Seq, &ln.ItemID, &ln.GroupLabel,
			&ln.Name, &ln.Spec, &ln.Qty, &ln.Unit, &ln.UnitPrice, &ln.Amount, &ln.GovPrice,
			&ln.MMRate, &ln.DiscountRate, &ln.Note, &ln.RateID, &ln.LaborYear, &over); err != nil {
			return nil, err
		}
		ln.PriceOverridden = over != 0
		lines = append(lines, ln)
	}
	return lines, rows.Err()
}

type quoteScanner interface {
	Scan(dest ...interface{}) error
}

func scanQuote(row quoteScanner) (*model.SalesQuote, error) {
	var q model.SalesQuote
	var legacy, revCalc, maint int
	err := row.Scan(
		&q.QuoteID, &q.SalesID, &q.QuoteNo, &q.Rev, &q.FormType, &q.RecipientKind,
		&q.CustomerID, &q.RecipientName, &q.AttnName, &q.AttnTitle, &q.QuoteDate, &q.Title,
		&q.ValidUntilText, &q.DueText, &q.PlaceText, &q.PaymentText, &q.VATMode, &q.RoundRule,
		&q.Subtotal, &q.VAT, &q.Total, &q.OwnerUserID, &q.OwnerName, &q.OwnerPhone, &q.Remarks,
		&q.Purpose, &q.BudgetYear, &q.OverheadRate, &q.TechFeeRate, &q.Status, &legacy,
		&q.RevReason, &revCalc, &q.TargetTotal, &maint,
		&q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	q.IsLegacy = legacy != 0
	q.IsReverseCalc = revCalc != 0
	q.MaintBlock = maint != 0
	q.FormType = model.NormalizeQuoteForm(q.FormType)
	q.Purpose = model.NormalizeQuotePurpose(q.Purpose)
	return &q, nil
}

func scanQuotes(rows *sql.Rows) ([]model.SalesQuote, error) {
	var items []model.SalesQuote
	for rows.Next() {
		q, err := scanQuote(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *q)
	}
	return items, rows.Err()
}

func (r *QuoteRepo) CountPeerDocs() (quotes, orders, contracts int) {
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM sales_quotes`).Scan(&quotes)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM sales_orders`).Scan(&orders)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM work_projects`).Scan(&contracts)
	return
}

func (r *QuoteRepo) Company() model.QuoteCompany {
	return LoadQuoteCompany(r.db)
}
